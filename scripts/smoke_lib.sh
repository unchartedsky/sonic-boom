#!/usr/bin/env bash
# sonic-boom e2e 스모크 공통 검증 라이브러리.
# 사용법: smoke_lib.sh <전략명> <라우트 prefix> [vary 라우트 prefix]
# 전제: 호출 시점에 캐시가 비어 있어야 한다(make e2e는 down -v 후 시작).
# Kong이 안 떠 있으면 즉시 실패한다(fail-fast).
#
# gzip 단언(characterization): 현재 동작을 고정하는 게 목적이다.
# 압축 지점은 목업 업스트림 nginx가 유일하다(compose의 KONG_NGINX_HTTP_GZIP_TYPES 주석 유지).
set -euo pipefail

STRATEGY="${1:?전략명 필수}"
ROUTE="${2:?라우트 prefix 필수}"
VARY_ROUTE="${3:-${2}-vary}"
BASE="${BASE_URL:-http://localhost:8000}"
URL="${BASE}${ROUTE}/api/users?page=2"
FIXDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../test/upstream" && pwd)"
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

TMPD="$(mktemp -d)"
trap 'rm -rf "${TMPD}"' EXIT

curl_h() {
	local a
	for a in "$@"; do
		[[ "${a}" == http* ]] && LAST_URL="${a}"
	done
	curl -s -i -m 10 "$@" | tr -d '\r'
}

status_of() {
	awk -F': ' '/^X-Cache-Status:/ {print $2}' <<<"$1" | tr -d '[:space:]'
}

# -D 헤더 덤프 파일에서 값 읽기(개행 \r 제거 포함)
hdr_value() {
	awk -F': ' -v h="$2" 'tolower($1)==tolower(h){sub(/\r$/,""); print $2}' "$1" | head -1
}

# HIT_RETRY_TIMEOUT(초): Hit 기대가 실패하면 마지막 curl URL을 기한 내 재시도한다.
# redis/cluster의 log 단계 timer 저장은 응답 직후 비동기라 즉시 Hit가 경합하기 때문.
# 미설정이면 기존 동작 그대로(Go 경로 무변경).
assert_status() {
	local want="$1" got="$2" name="$3"
	if [[ "${want}" == "Hit" && -n "${HIT_RETRY_TIMEOUT:-}" && "${got}" != "Hit" && -n "${LAST_URL:-}" ]]; then
		local deadline=$(( $(date +%s) + HIT_RETRY_TIMEOUT ))
		while [[ "${got}" != "Hit" && $(date +%s) -lt ${deadline} ]]; do
			sleep 0.3
			got="$(status_of "$(curl_h "${LAST_URL}")")"
		done
	fi
	echo "  ${name}: ${got:-<none>} (기대 ${want})"
	if [[ "${got}" != "${want}" ]]; then
		echo "FAIL: [${STRATEGY}] ${name} — got '${got:-<none>}', want '${want}'" >&2
		exit 1
	fi
}

assert_eq() {
	local want="$1" got="$2" name="$3"
	if [[ "${want}" != "${got}" ]]; then
		echo "FAIL: [${STRATEGY}] ${name} — got '${got:-(empty)}', want '${want:-(empty)}'" >&2
		exit 1
	fi
	echo "  ${name}: OK"
}


# gzip 단언-suite. 인수: 기본 라우트(교차 오염·HEAD·POST), vary 라우트(키 분리).
# 시나리오별 고유 쿼리스트링(page=gz*)을 쓴다 — 기존 단언의 page=2 키를
# 재사용하면 gzip 첫 요청이 Miss가 아니라 Hit이 된다.
gzip_suite() {
	local route="$1" vroute="$2"
	local gz_url="${BASE}${route}/api/users?page=gz1"
	local vary_url="${BASE}${vroute}/api/users?page=gzv1"
	local ref="${TMPD}/ref.json"

	# 참조 본문: 플러그인 없는 라우트에서 --compressed로 받은 원문 JSON
	curl -s --compressed -m 10 -o "${ref}" "${BASE}/noplugins/api/users?page=gzref"

	# A) gzip 응답: 첫 요청 Miss, 재요청 Hit. Content-Encoding: gzip 존재,
	#    해제 본문은 원문과 바이트 일치, Content-Length는 raw gzip 바이트 수와 일치
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o "${TMPD}/gz1.bin" -D "${TMPD}/gz1.hdr" "${gz_url}"
	assert_status Miss "$(hdr_value "${TMPD}/gz1.hdr" X-Cache-Status)" "gzip 첫 요청"
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o "${TMPD}/gz2.bin" -D "${TMPD}/gz2.hdr" "${gz_url}"
	assert_status Hit "$(hdr_value "${TMPD}/gz2.hdr" X-Cache-Status)" "gzip 재요청"
	assert_eq gzip "$(hdr_value "${TMPD}/gz2.hdr" Content-Encoding)" "gzip Hit Content-Encoding"
	gunzip -c "${TMPD}/gz2.bin" | cmp -s - "${ref}" || {
		echo "FAIL: [${STRATEGY}] gzip 해제 본문이 원문과 불일치" >&2; exit 1; }
	echo "  gzip 해제 본문 일치: OK"
	assert_eq "$(wc -c <"${TMPD}/gz2.bin" | tr -d ' ')" "$(hdr_value "${TMPD}/gz2.hdr" Content-Length)" "gzip Hit Content-Length(관측값 고정)"

	# B) 교차 오염: Accept-Encoding 없는 재요청이 같은 키 Hit — 재생 본문·헤더 고정
	curl -s -m 10 -o "${TMPD}/plain.bin" -D "${TMPD}/plain.hdr" "${gz_url}"
	assert_status Hit "$(hdr_value "${TMPD}/plain.hdr" X-Cache-Status)" "교차 오염(Accept-Encoding 없음)"
	cmp -s "${TMPD}/plain.bin" "${TMPD}/gz1.bin" || {
		echo "FAIL: [${STRATEGY}] 교차 오염 재생 본문이 gzip 엔트리와 불일치" >&2; exit 1; }
	assert_eq gzip "$(hdr_value "${TMPD}/plain.hdr" Content-Encoding)" "교차 오염 Content-Encoding"

	# C) gzip×vary(복제 라우트): 키 분리. gzip Miss→Hit, 헤더 없음·identity는 각각 다른 키
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o /dev/null -D "${TMPD}/v1.hdr" "${vary_url}"
	assert_status Miss "$(hdr_value "${TMPD}/v1.hdr" X-Cache-Status)" "vary gzip 첫 요청"
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o /dev/null -D "${TMPD}/v2.hdr" "${vary_url}"
	assert_status Hit "$(hdr_value "${TMPD}/v2.hdr" X-Cache-Status)" "vary gzip 재요청"
	curl -s -m 10 -o /dev/null -D "${TMPD}/v3.hdr" "${vary_url}"
	assert_status Miss "$(hdr_value "${TMPD}/v3.hdr" X-Cache-Status)" "vary 헤더 없음 → 다른 키"
	curl -s -m 10 -H 'Accept-Encoding: identity' -o /dev/null -D "${TMPD}/v4.hdr" "${vary_url}"
	assert_status Miss "$(hdr_value "${TMPD}/v4.hdr" X-Cache-Status)" "vary identity → 다른 키"
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o /dev/null -D "${TMPD}/v5.hdr" "${vary_url}"
	assert_status Hit "$(hdr_value "${TMPD}/v5.hdr" X-Cache-Status)" "vary gzip 3회째(기존 키 유지)"

	# D) HEAD: 캐시 키에 Method가 포함돼 GET으로 데운 키와는 다른 키다(첫 HEAD는 Miss).
	#    본문 없는 응답(nil rawBody 경로)의 캐싱 왕복을 HEAD 2회 Miss→Hit으로 고정
	curl -s -m 10 -I -H 'Accept-Encoding: gzip' -D "${TMPD}/h1.hdr" -o /dev/null "${gz_url}"
	assert_status Miss "$(hdr_value "${TMPD}/h1.hdr" X-Cache-Status)" "HEAD 첫 요청(GET 키와 분리)"
	curl -s -m 10 -I -H 'Accept-Encoding: gzip' -D "${TMPD}/h2.hdr" -o /dev/null "${gz_url}"
	assert_status Hit "$(hdr_value "${TMPD}/h2.hdr" X-Cache-Status)" "HEAD 재요청(nil body 캐싱 왕복)"

	# E) gzip POST 바디: 동일 압축 바디 2회는 같은 키(Miss→Hit),
	#    비압축 동일 의미 바디는 다른 raw body → 다른 키(Miss)로 고정
	printf '{"gzip":"post"}' | gzip -c >"${TMPD}/post.bin"
	local post_url="${BASE}${route}/api/users?page=gzp1"
	curl -s -m 10 -X POST -H 'Content-Encoding: gzip' --data-binary @"${TMPD}/post.bin" -o /dev/null -D "${TMPD}/p1.hdr" "${post_url}"
	assert_status Miss "$(hdr_value "${TMPD}/p1.hdr" X-Cache-Status)" "gzip POST 첫 요청"
	curl -s -m 10 -X POST -H 'Content-Encoding: gzip' --data-binary @"${TMPD}/post.bin" -o /dev/null -D "${TMPD}/p2.hdr" "${post_url}"
	assert_status Hit "$(hdr_value "${TMPD}/p2.hdr" X-Cache-Status)" "gzip POST 재요청"
	curl -s -m 10 -X POST --data-binary '{"gzip":"post"}' -o /dev/null -D "${TMPD}/p3.hdr" "${post_url}"
	assert_status Miss "$(hdr_value "${TMPD}/p3.hdr" X-Cache-Status)" "비압축 동일 의미 POST → 다른 키"

	echo "  gzip suite: OK"
}

# deflate/br 단언-suite. 플러그인은 본문을 불투명 바이트로 취급하므로
# 핵심 단언은 '재생 바이트 == 사전 압축 픽스처 바이트'다(디코딩 검증은
# 플러그인이 아니라 픽스처를 검사하는 것이라 하지 않는다).
# 사용법: enc_suite <기본 라우트> <vary 라우트> <deflate|br>
enc_suite() {
	local route="$1" vroute="$2" enc="$3"
	local fixture="${FIXDIR}/response.${enc}"
	local url="${BASE}${route}/enc/${enc}/api/users?enc1"
	local vary_url="${BASE}${vroute}/enc/${enc}/api/users?encv1"

	curl -s -m 10 -H "Accept-Encoding: ${enc}" -o "${TMPD}/e1.bin" -D "${TMPD}/e1.hdr" "${url}"
	assert_status Miss "$(hdr_value "${TMPD}/e1.hdr" X-Cache-Status)" "${enc} 첫 요청"
	curl -s -m 10 -H "Accept-Encoding: ${enc}" -o "${TMPD}/e2.bin" -D "${TMPD}/e2.hdr" "${url}"
	assert_status Hit "$(hdr_value "${TMPD}/e2.hdr" X-Cache-Status)" "${enc} 재요청"
	assert_eq "${enc}" "$(hdr_value "${TMPD}/e2.hdr" Content-Encoding)" "${enc} Hit Content-Encoding"
	cmp -s "${TMPD}/e2.bin" "${fixture}" || {
		echo "FAIL: [${STRATEGY}] ${enc} 재생 본문이 픽스처와 바이트 불일치" >&2; exit 1; }
	echo "  ${enc} 재생 바이트 == 픽스처: OK"

	# 교차 오염: Accept-Encoding 없는 재요청도 같은 키 Hit — gzip과 동일하게 재생 고정
	curl -s -m 10 -o "${TMPD}/e3.bin" -D "${TMPD}/e3.hdr" "${url}"
	assert_status Hit "$(hdr_value "${TMPD}/e3.hdr" X-Cache-Status)" "${enc} 교차 오염(Accept-Encoding 없음)"
	cmp -s "${TMPD}/e3.bin" "${fixture}" || {
		echo "FAIL: [${STRATEGY}] ${enc} 교차 오염 재생 본문 불일치" >&2; exit 1; }

	# 인코딩×vary(복제 라우트): gzip으로 데운 키와 ${enc} 키는 분리된다
	curl -s -m 10 -H 'Accept-Encoding: gzip' -o /dev/null -D "${TMPD}/ev0.hdr" "${vary_url}"
	assert_status Miss "$(hdr_value "${TMPD}/ev0.hdr" X-Cache-Status)" "${enc} vary gzip 데우기"
	curl -s -m 10 -H "Accept-Encoding: ${enc}" -o /dev/null -D "${TMPD}/ev1.hdr" "${vary_url}"
	assert_status Miss "$(hdr_value "${TMPD}/ev1.hdr" X-Cache-Status)" "${enc} vary → gzip 키와 분리"
	curl -s -m 10 -H "Accept-Encoding: ${enc}" -o /dev/null -D "${TMPD}/ev2.hdr" "${vary_url}"
	assert_status Hit "$(hdr_value "${TMPD}/ev2.hdr" X-Cache-Status)" "${enc} vary 재요청"

	echo "  ${enc} suite: OK"
}

# stale 재검증(AE2): cache_ttl(라우트 기본 15초) 경과 후에는 캐시에서 응답하지 않는다
# (upstream 재검증 통과), 그 응답이 재저장돼 다음 요청이 Hit가 된다.
# 저장 만료와 freshness가 같은 TTL이라 만료 후엔 저장소 소멸이 먼저 — Miss로 관측된다.
# Refresh 라벨은 저장 만료 직전 근사 창에서만 경쟁적으로 도달해 안정 단언이 될 수 없다.
# 수정 전 회귀: 만료 값이 Hit 라벨로 그대로 서빙됐다.
stale_suite() {
	local route="$1"
	local stale_url="${BASE}${route}/api/users?page=st1"
	local ttl="${STALE_TTL_SECS:-16}"

	assert_status Miss "$(status_of "$(curl_h "${stale_url}")")" "stale 첫 요청"
	assert_status Hit "$(status_of "$(curl_h "${stale_url}")")" "stale 데우기"

	echo "  stale: TTL 경과 대기(${ttl}s)..."
	sleep "${ttl}"

	assert_status Miss "$(status_of "$(curl_h "${stale_url}")")" "stale 만료 후 재검증(upstream 통과)"
	assert_status Hit "$(status_of "$(curl_h "${stale_url}")")" "stale 재저장 확인"
	echo "  stale suite: OK"
}

main() {
	echo "== ${STRATEGY} e2e =="
	wait_for_kong "${BASE}"

	# 0)·0-1) 스키마 거부 시나리오 — Kong 선언형 로드만 거부하는 것(cache_ttl 0)은
	# Lua에서 라우트 자체가 없다. SKIP_SCHEMA_ASSERT=1이면 건너뛴다.
	# cache_control 가드 블록은 이제 Lua도 라우트가 있어(스키마 기본 true) 무조건 돈다 —
	# 가드 무효화(필드 생략 라우트가 Bypass를 못 돌려주면)를 이 블록이 잡는다.
	if [[ -z "${SKIP_SCHEMA_ASSERT:-}" ]]; then
		assert_status Bypass "$(status_of "$(curl_h "${BASE}/ttl-zero/api/users?page=ae5")")" "cache_ttl 0 거절"
	fi

	# 요청층 cache_control 가드(기본 on): 무관 요청은 캐시되고, Authorization·
		# no-store·no-cache 요청은 매번 Bypass다(조회 이전 차단).
		# 전략 태그 — 키가 스모크 3종에서 공유되면 스모크 간 TTL 타이밍 의존이 생긴다
		local cc_base="${BASE}/cache-control"
		[[ "${ROUTE}" == /lua* ]] && cc_base="${BASE}/lua-cache-control"
		local cc_url="${cc_base}/api/users?page=${STRATEGY}-ae4"
		assert_status Miss "$(status_of "$(curl_h "${cc_url}")")" "cache_control 평문 첫 요청(Miss)"
		assert_status Hit "$(status_of "$(curl_h "${cc_url}")")" "cache_control 평문 재요청(Hit)"
		local cc_auth="Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ="
		assert_status Bypass "$(status_of "$(curl_h -H "${cc_auth}" "${cc_base}/api/users?page=ae5")")" "cache_control Authorization 거부"
		assert_status Bypass "$(status_of "$(curl_h -H "${cc_auth}" "${cc_base}/api/users?page=ae5")")" "cache_control Authorization 재요청"
		assert_status Bypass "$(status_of "$(curl_h -H 'Cache-Control: no-store' "${cc_base}/api/users?page=ae6")")" "cache_control no-store 거부"
		assert_status Bypass "$(status_of "$(curl_h -H 'Cache-Control: no-store' "${cc_base}/api/users?page=ae6")")" "cache_control no-store 재요청"
		assert_status Bypass "$(status_of "$(curl_h -H 'Cache-Control: no-cache' "${cc_base}/api/users?page=ae7")")" "cache_control no-cache 거부"
		assert_status Bypass "$(status_of "$(curl_h -H 'Cache-Control: no-cache' "${cc_base}/api/users?page=ae7")")" "cache_control no-cache 재요청"

	# 1) 첫 요청은 Miss — 남아 있는 캐시가 회귀를 가리는지 즉시 드러난다
	assert_status Miss "$(status_of "$(curl_h "${URL}")")" "첫 요청"

	# 2) 같은 키 재요청은 Hit
	assert_status Hit "$(status_of "$(curl_h "${URL}")")" "재요청"

	# 3) 캐시 불가 메서드(DELETE)는 Bypass
	assert_status Bypass "$(status_of "$(curl_h -X DELETE "${URL}")")" "DELETE"

	# 3-1) Set-Cookie 응답은 저장하지 않는다(개인화 오염 방지 — APISIX 기본값 정합).
	# 첫 요청이 Miss 시그널로 출발해도 저장 게이트에서 Bypass로 바뀐다.
	local sc_url="${BASE}${ROUTE}/setcookie/api/users?page=sc1"
	assert_status Bypass "$(status_of "$(curl_h "${sc_url}")")" "Set-Cookie 응답 미저장"
	assert_status Bypass "$(status_of "$(curl_h "${sc_url}")")" "Set-Cookie 재요청"

	# 3-3) 헤더 110개+후행 Set-Cookie — 기본 상한(100)으로 읽으면 잘린 채 저장된다(#3 회귀 게이트).
	local mh_url="${BASE}${ROUTE}/manyhdr/api/users?page=mh1"
	assert_status Bypass "$(status_of "$(curl_h "${mh_url}")")" "다중 헤더 잘림 방어(첫 요청)"
	assert_status Bypass "$(status_of "$(curl_h "${mh_url}")")" "다중 헤더 재요청(미저장 확인)"

	# 3-2) Cache-Control private/no-store/no-cache 응답도 저장하지 않는다.
	local cc_dir
	for cc_dir in ccprivate ccnostore ccnocache; do
		assert_status Bypass "$(status_of "$(curl_h "${BASE}${ROUTE}/${cc_dir}/api/users?page=sc1")")" "${cc_dir} 첫 요청 미저장"
		assert_status Bypass "$(status_of "$(curl_h "${BASE}${ROUTE}/${cc_dir}/api/users?page=sc1")")" "${cc_dir} 재요청"
	done

	# 3-2) principal 고정(vary 없는 cache_control: false 라우트) — consumer 미확정
	# Authorization 요청이 principal 성분으로 자격증명별 슬롯에 분리됨을 검증한다.
	# 같은 자격증명 재요청은 Hit(자격증명별 캐시), 다른 자격증명은 Miss(분리), 헤더 없음도 Miss(익명 슬롯 유지).
	local pr_base="${BASE}/go-principal"
	[[ "${ROUTE}" == /lua* ]] && pr_base="${BASE}/lua-principal"
	local pa="Authorization: Basic cHJpbmNpcGFsLTE="
	local pb="Authorization: Basic cHJpbmNpcGFsLTI="
	# 전략 태그 — Go 스모크 3종은 재시작 없이 ristretto를 공유해 스위트 간 슬롯이 이월된다
	assert_status Miss "$(status_of "$(curl_h -H "${pa}" "${pr_base}/api/users?page=pr1-${STRATEGY}")")" "principal auth1 첫 요청(Miss)"
	assert_status Hit "$(status_of "$(curl_h -H "${pa}" "${pr_base}/api/users?page=pr1-${STRATEGY}")")" "principal auth1 재요청(Hit)"
	assert_status Miss "$(status_of "$(curl_h -H "${pb}" "${pr_base}/api/users?page=pr1-${STRATEGY}")")" "principal auth2 다른 슬롯(Miss)"
	# 익명 요청은 auth1과 같은 URL로 — URL이 다르면 쿼리 차이만으로 Miss가 나와 슬롯 분리를 증명하지 못한다
	assert_status Miss "$(status_of "$(curl_h "${pr_base}/api/users?page=pr1-${STRATEGY}")")" "principal 익명 요청(Miss — 익명 슬롯 유지)"

	# 4) vary_headers: Authorization 값이 다르면 캐시 키가 분리된다
	local auth1="dXNlcm5hbWU6cGFzc3dvcmQ="
	local auth2="dXNlcm5hbWU6cGFzc3dvcmQy"
	assert_status Miss "$(status_of "$(curl_h -H "Authorization: Basic ${auth1}" "${URL}")")" "vary 첫 요청(auth1)"
	assert_status Hit "$(status_of "$(curl_h -H "Authorization: Basic ${auth1}" "${URL}")")" "vary 재요청(auth1)"
	assert_status Miss "$(status_of "$(curl_h -H "Authorization: Basic ${auth2}" "${URL}")")" "vary 다른 자격증명(auth2)"

	# 5) gzip 압축 트래픽(characterization)
	gzip_suite "${ROUTE}" "${VARY_ROUTE}"

	# 6) deflate·brotli 압축 트래픽(사전 압축 픽스처 기반)
	enc_suite "${ROUTE}" "${VARY_ROUTE}" deflate
	enc_suite "${ROUTE}" "${VARY_ROUTE}" br

	# 7) stale 재검증(만료 값 재서빙 회귀 방지)
	stale_suite "${ROUTE}"

	echo "OK: [${STRATEGY}] Miss→Hit, Bypass, vary, gzip, deflate, br, stale 통과"
}

main
