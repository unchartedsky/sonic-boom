#!/usr/bin/env bash
# sonic-boom e2e 스모크 공통 검증 라이브러리.
# 사용법: smoke_lib.sh <전략명> <라우트 prefix>
# 전제: 호출 시점에 캐시가 비어 있어야 한다(make e2e는 down -v 후 시작).
# Kong이 안 떠 있으면 즉시 실패한다(fail-fast).
set -euo pipefail

STRATEGY="${1:?전략명 필수}"
ROUTE="${2:?라우트 prefix 필수}"
BASE="${BASE_URL:-http://localhost:8000}"
URL="${BASE}${ROUTE}/api/users?page=2"

curl_h() {
	curl -s -i -m 10 "$@" | tr -d '\r'
}

status_of() {
	awk -F': ' '/^X-Cache-Status:/ {print $2}' <<<"$1" | tr -d '[:space:]'
}

assert_status() {
	local want="$1" got="$2" name="$3"
	echo "  ${name}: ${got:-<none>} (기대 ${want})"
	if [[ "${got}" != "${want}" ]]; then
		echo "FAIL: [${STRATEGY}] ${name} — got '${got:-<none>}', want '${want}'" >&2
		exit 1
	fi
}

wait_for_kong() {
	# 프로브는 플러그인이 없는 /noplugins 라우트를 쓴다.
	# 검증 대상 URL을 프로브하면 캐시이미 데워져 '첫 요청 Miss' 단언이 깨진다.
	local probe="${BASE}/noplugins/api/users?page=probe"
	local i
	for i in $(seq 1 60); do
		if curl -s -o /dev/null -m 2 "${probe}"; then
			return 0
		fi
		sleep 1
	done
	echo "FAIL: [${STRATEGY}] ${BASE}에 60초 내 접근 불가 — docker compose 기동 상태를 확인하세요" >&2
	exit 1
}

main() {
	echo "== ${STRATEGY} e2e =="
	wait_for_kong

	# 1) 첫 요청은 Miss — 남아 있는 캐시가 회귀를 가리는지 즉시 드러난다
	assert_status Miss "$(status_of "$(curl_h "${URL}")")" "첫 요청"

	# 2) 같은 키 재요청은 Hit
	assert_status Hit "$(status_of "$(curl_h "${URL}")")" "재요청"

	# 3) 캐시 불가 메서드(DELETE)는 Bypass
	assert_status Bypass "$(status_of "$(curl_h -X DELETE "${URL}")")" "DELETE"

	# 4) vary_headers: Authorization 값이 다르면 캐시 키가 분리된다
	local auth1="dXNlcm5hbWU6cGFzc3dvcmQ="
	local auth2="dXNlcm5hbWU6cGFzc3dvcmQy"
	assert_status Miss "$(status_of "$(curl_h -H "Authorization: Basic ${auth1}" "${URL}")")" "vary 첫 요청(auth1)"
	assert_status Hit "$(status_of "$(curl_h -H "Authorization: Basic ${auth1}" "${URL}")")" "vary 재요청(auth1)"
	assert_status Miss "$(status_of "$(curl_h -H "Authorization: Basic ${auth2}" "${URL}")")" "vary 다른 자격증명(auth2)"

	echo "OK: [${STRATEGY}] Miss→Hit, Bypass, vary 통과"
}

main
