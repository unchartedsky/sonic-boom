#!/usr/bin/env bash
# 재생 비교 — 같은 순차 요청 묶음을 두 게이트웨이에 재생해
# X-Cache-Status 시퀀스·HTTP status·body(sha256 접두) 일치를 증명한다.
# 쌍(PARITY_PAIR): go-lua(기본, Go :8000 ↔ Kong-Lua :8010) | lua-apisix(Kong-Lua :8010 ↔ APISIX :8020).
# lua-apisix는 같은 코어·같은 /lua-* 경로를 쓰므로 A측도 lua 경로로 재생한다.
# 구 명시 차이(R4 Set-Cookie)는 Go 역포팅으로 해소됐다 — S8은 이제 공통 동작 검증이다.
# 범위: redis 전략 라우트 기준(R7). X-Cache-Key·Age 값은 제품별 파생이라 비교 제외하되
# Age는 Hit에 존재하고 증가한다(갱신 조건 정합, R5).
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/common.sh

PARITY_PAIR="${PARITY_PAIR:-go-lua}"
case "${PARITY_PAIR}" in
	go-lua) A=http://localhost:8000 B=http://localhost:8010 A_TAG=go B_TAG=lua ;;
	lua-apisix) A=http://localhost:8010 B=http://localhost:8020 A_TAG=kong-lua B_TAG=apisix ;;
	*) echo "FAIL: 알 수 없는 PARITY_PAIR='${PARITY_PAIR}' (go-lua|lua-apisix)" >&2; exit 1 ;;
esac
# A측 재생 경로: go-lua는 Go 경로, lua-apisix는 B측과 같은 lua 경로.
a_path() { [[ "${PARITY_PAIR}" == lua-apisix ]] && echo "$2" || echo "$1"; }

# 초기화: 양쪽 캐시 비움(redis 전 db + 게이트웨이 재기동으로 shdict)
docker compose exec -T redis redis-cli FLUSHALL >/dev/null
docker compose --profile lua restart kong-lua >/dev/null
if [[ "${PARITY_PAIR}" == lua-apisix ]]; then
	docker compose --profile apisix restart apisix >/dev/null
else
	docker compose restart kong >/dev/null
fi
sleep 8
wait_for_kong "$A"; wait_for_kong "$B"
# 재시작 직후 워커 교체 경합 방지: 게이트웨이별 실제 캐시 라우트 왕복이 안정될 때까지 대기
for _ in $(seq 1 20); do
	g="$(curl -s -o /dev/null -m 2 -w '%{http_code}' "$A$(a_path /redis/api/users /lua-redis/api/users)?warm" || true)"
	l="$(curl -s -o /dev/null -m 2 -w '%{http_code}' "$B/lua-redis/api/users?warm" || true)"
	[[ "$g" == 200 && "$l" == 200 ]] && break
	sleep 1
done
sleep 2

TMPD="$(mktemp -d)"; trap 'rm -rf "${TMPD}"' EXIT

hit() { # <tag> <base> <method> <path> [curl opts...]
	local tag="$1" base="$2" method="$3" path="$4"; shift 4
	curl -s -m 10 -D "${TMPD}/${tag}.hdr" -o "${TMPD}/${tag}.body" "$@" -X "$method" "${base}${path}" || true
	local st xcs sig
	st="$(awk 'NR==1{print $2}' "${TMPD}/${tag}.hdr" | tr -d '\r')"
	xcs="$(awk -F': ' 'tolower($1)=="x-cache-status"{sub(/\r$/,"");print $2}' "${TMPD}/${tag}.hdr" | head -1)"
	sig="$(shasum -a 256 "${TMPD}/${tag}.body" | cut -c1-16)"
	echo "${st:-000}|${xcs:-none}|${sig:-empty}" > "${TMPD}/${tag}.res"
}

pair() { # <이름> <기대 xcs> <method> <go_path> <lua_path> [opts...] — 양쪽 xcs=기대, status·body 일치
	local name="$1" want="$2" method="$3" gpath="$4" lpath="$5"; shift 5
	hit go "$A" "$method" "$(a_path "$gpath" "$lpath")" "$@"
	hit lua "$B" "$method" "$lpath" "$@"
	local gr lr; gr="$(cat "${TMPD}/go.res")"; lr="$(cat "${TMPD}/lua.res")"
	local gxs lxs; gxs="$(cut -d'|' -f2 <<<"$gr")"; lxs="$(cut -d'|' -f2 <<<"$lr")"
	echo "  ${name}: ${A_TAG}=[${gr}] ${B_TAG}=[${lr}] (기대 ${want})"
	[[ "$gxs" == "$want" && "$lxs" == "$want" ]] || { echo "FAIL: ${name} xcs ${A_TAG}=${gxs} ${B_TAG}=${lxs} want=${want}" >&2; exit 1; }
	[[ "$(cut -d'|' -f1,3 <<<"$gr")" == "$(cut -d'|' -f1,3 <<<"$lr")" ]] || { echo "FAIL: ${name} status/body 불일치" >&2; exit 1; }
}

pairdiv() { # <이름> <go기대> <lua기대> <method> <go_path> <lua_path> [opts...]
	local name="$1" gw="$2" lw="$3" method="$4" gpath="$5" lpath="$6"; shift 6
	hit go "$A" "$method" "$(a_path "$gpath" "$lpath")" "$@"
	hit lua "$B" "$method" "$lpath" "$@"
	local gr lr; gr="$(cat "${TMPD}/go.res")"; lr="$(cat "${TMPD}/lua.res")"
	echo "  ${name}: ${A_TAG}=[${gr}] ${B_TAG}=[${lr}] (기대 ${A_TAG}=${gw} ${B_TAG}=${lw})"
	[[ "$(cut -d'|' -f2 <<<"$gr")" == "$gw" && "$(cut -d'|' -f2 <<<"$lr")" == "$lw" ]] || { echo "FAIL: ${name} 분기 기대 위반" >&2; exit 1; }
}

age_of() { awk -F': ' 'tolower($1)=="age"{sub(/\r$/,"");print $2}' "${TMPD}/$1.hdr" | head -1; }

echo "== 재생 비교 ${PARITY_PAIR} (redis 전략 범위, 기본 설정·fill-lock 켬) =="

# S1 fresh Miss → Hit
pair "S1-1 첫 요청"  Miss   GET "/redis/api/users?pr=s1" "/lua-redis/api/users?pr=s1"
pair "S1-2 재요청"   Hit    GET "/redis/api/users?pr=s1" "/lua-redis/api/users?pr=s1"

# S2 불가 메서드
pair "S2 DELETE" Bypass DELETE "/redis/api/users?pr=s2" "/lua-redis/api/users?pr=s2"

# S3 vary 자격증명 분리
A1="Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQ="
A2="Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQy"
pair "S3-1 auth1 첫" Miss GET "/redis/api/users?pr=s3" "/lua-redis/api/users?pr=s3" -H "$A1"
pair "S3-2 auth1 재"  Hit  GET "/redis/api/users?pr=s3" "/lua-redis/api/users?pr=s3" -H "$A1"
pair "S3-3 auth2"     Miss GET "/redis/api/users?pr=s3" "/lua-redis/api/users?pr=s3" -H "$A2"

# S4 POST body 키 분리
pair "S4-1 동일 바디 첫" Miss POST "/redis/api/users?pr=s4" "/lua-redis/api/users?pr=s4" -H 'Content-Type: application/json' --data-binary '{"pr":"same"}'
pair "S4-2 동일 바디 재" Hit  POST "/redis/api/users?pr=s4" "/lua-redis/api/users?pr=s4" -H 'Content-Type: application/json' --data-binary '{"pr":"same"}'
pair "S4-3 다른 바디"   Miss POST "/redis/api/users?pr=s4" "/lua-redis/api/users?pr=s4" -H 'Content-Type: application/json' --data-binary '{"pr":"other"}'

# S5 필터(Go /ex ↔ Lua /lua-filter 미러): 매치는 캐시, 불매치는 Bypass
pair "S5-1 필터 매치 첫" Miss GET "/ex/api/users?page=(x)&pr=s5" "/lua-filter/api/users?page=(x)&pr=s5"
pair "S5-2 필터 매치 재" Hit  GET "/ex/api/users?page=(x)&pr=s5" "/lua-filter/api/users?page=(x)&pr=s5"
pair "S5-3 필터 불매치"  Bypass GET "/ex/api/users?page=2&pr=s5" "/lua-filter/api/users?page=2&pr=s5"

# S6 stale: TTL 15 경과 후 Miss → 재저장 Hit
pair "S6-1 첫 요청" Miss GET "/redis/api/users?pr=s6" "/lua-redis/api/users?pr=s6"
pair "S6-2 사전 Hit" Hit GET "/redis/api/users?pr=s6" "/lua-redis/api/users?pr=s6"
echo "  S6: TTL 경과 대기(16s)..."; sleep 16
pair "S6-3 만료 후" Miss GET "/redis/api/users?pr=s6" "/lua-redis/api/users?pr=s6"
pair "S6-4 재저장"  Hit  GET "/redis/api/users?pr=s6" "/lua-redis/api/users?pr=s6"

# S7 content_type 게이트(양쪽 목록 밖 text/xml 아님 → JSON 응답이지만 목록이 text/xml만)
pair "S7 ctgate" Bypass GET "/go-ctgate/api/users?pr=s7" "/lua-ctgate/api/users?pr=s7"

# S8 Set-Cookie 응답 미저장 — 구 명시 차이(R4)가 해소돼 양쪽 모두 Bypass다.
pair "S8-1 미저장 첫" Bypass GET "/go-setcookie/setcookie/api/users?pr=s8" "/lua-setcookie/setcookie/api/users?pr=s8"
pair "S8-2 미저장 재" Bypass GET "/go-setcookie/setcookie/api/users?pr=s8" "/lua-setcookie/setcookie/api/users?pr=s8"

# S10-S12 Cache-Control 응답 디렉티브 — 양쪽 모두 무조건 준수한다.
pair "S10 private"  Bypass GET "/go-setcookie/ccprivate/api/users?pr=s10"  "/lua-setcookie/ccprivate/api/users?pr=s10"
pair "S11 no-store" Bypass GET "/go-setcookie/ccnostore/api/users?pr=s11" "/lua-setcookie/ccnostore/api/users?pr=s11"
pair "S12 no-cache" Bypass GET "/go-setcookie/ccnocache/api/users?pr=s12" "/lua-setcookie/ccnocache/api/users?pr=s12"

# S13 cache_control 요청 가드 — Go 전용 옵트인(Lua는 cache_control을 스키마에서
# 거부한다, R4 명시 차이 2). Authorization 요청은 매번 Bypass다.
if [[ "${PARITY_PAIR}" == go-lua ]]; then
	hit go "$A" GET "/cache-control/api/users?pr=s13" -H "$A1"
	hit go "$A" GET "/cache-control/api/users?pr=s13" -H "$A1"
	echo "  S13 가드: go=[$(cat "${TMPD}/go.res")] (기대 Bypass×2)"
	[[ "$(cut -d'|' -f2 <<<"$(cat "${TMPD}/go.res")")" == "Bypass" ]] || { echo "FAIL: S13 cache_control 가드" >&2; exit 1; }
else
	echo "  S13 가드: 건너뜀(Go 전용 — 두 Lua 게이트웨이 모두 cache_control을 스키마에서 거부)"
fi

# S9 Age 존재·증가(갱신 조건 정합) — 신선한 키를 데운 뒤 두 Hit로 관측
S9A="$(a_path /redis/api/users /lua-redis/api/users)?pr=s9"
hit go "$A" GET "$S9A"; hit go "$A" GET "$S9A"; ga1="$(age_of go)"
hit lua "$B" GET "/lua-redis/api/users?pr=s9"; hit lua "$B" GET "/lua-redis/api/users?pr=s9"; la1="$(age_of lua)"
[[ -n "${ga1:-}" && -n "${la1:-}" ]] || { echo "FAIL: Hit에 Age 없음 (${A_TAG}=${ga1:-none} ${B_TAG}=${la1:-none})" >&2; exit 1; }
sleep 2
hit go "$A" GET "$S9A"; ga2="$(age_of go)"
hit lua "$B" GET "/lua-redis/api/users?pr=s9"; la2="$(age_of lua)"
echo "  S9 Age: ${A_TAG} ${ga1}→${ga2}, ${B_TAG} ${la1}→${la2}"
[[ -n "${ga2:-}" && "${ga2}" -gt "${ga1}" ]] || { echo "FAIL: ${A_TAG} Age 증가 없음" >&2; exit 1; }
[[ -n "${la2:-}" && "${la2}" -gt "${la1}" ]] || { echo "FAIL: ${B_TAG} Age 증가 없음" >&2; exit 1; }
echo "OK: [parity ${PARITY_PAIR}] redis 전략 범위 — 시퀀스·status·body 일치"
