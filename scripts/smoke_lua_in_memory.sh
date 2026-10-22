#!/usr/bin/env bash
set -euo pipefail

# Lua 포팅 in-memory 스모크(GW=kong-lua 기본 | apisix). 재실행 멱등: make e2e의 down -v
# 관행 대신 게이트웨이 재기동으로 shdict를 비운다(첫 요청 Miss 전제).
source "$(dirname "$0")/common.sh"
gw_env
docker compose --profile "${GW_PROFILE}" restart "${GW_SVC}" >/dev/null
sleep 6

BASE_URL="${GW_BASE}" SKIP_SCHEMA_ASSERT=1 \
  "$(dirname "$0")/smoke_lib.sh" "in_memory" "/lua-in-memory"

# ---- U4 고정 시나리오(재사용 라이브러리 밖) ----
B="${GW_BASE}"
hdr() { curl -s -D- -o /dev/null -m 10 "$@" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}'; }
want() { # <기대 xcs> <이름> <url...> — 기대·이름을 hdr에 넘기지 않는다
	local expected="$1" name="$2"; shift 2
	local got; got="$(hdr "$@")"
	echo "  ${name}: ${got:-<none>} (기대 ${expected})"
	[[ "${got}" == "${expected}" ]] || { echo "FAIL: ${name}" >&2; exit 1; }
}

want Bypass "content_type 목록 밖(ctgate)" "$B/lua-ctgate/api/users"
want Bypass "Set-Cookie 응답 미저장(setcookie)" "$B/lua-setcookie/setcookie/api/users"
want Miss "body_max_size 초과 1회차(maxsize)" "$B/lua-maxsize/api/users?u4=1"
want Miss "body_max_size 초과 2회차(미저장 확인)" "$B/lua-maxsize/api/users?u4=1"

# 제품별 키 접두: 같은 redis를 공유해도 Kong-Lua(sbl)와 APISIX(sba)의 키·락 공간이 갈린다.
key="$(curl -s -D- -o /dev/null -m 10 "$B/lua-in-memory/api/users?kp=1" | tr -d '\r' | awk -F': ' 'tolower($1)=="x-cache-key"{print $2}')"
echo "  X-Cache-Key 접두: ${key%%:*} (기대 ${GW_KEY_PREFIX})"
[[ "${key}" == "${GW_KEY_PREFIX}:"* ]] || { echo "FAIL: 키 접두 ${GW_KEY_PREFIX} 아님 — '${key}'" >&2; exit 1; }

# APISIX: 미구현 필드(cache_control) 라우트는 스키마에서 거부돼 그 라우트만 로드되지 않는다.
# (Kong-Lua는 선언형 로드 전체가 거부되므로 별도 경로로 검증한다.)
if [[ "${GW_SVC}" == "apisix" ]]; then
	# 업스트림 목업도 404를 줄 수 있어 상태 코드만으론 거부와 수락이 갈리지 않는다 —
	# 라우트 미존재일 때만 나오는 APISIX 본문("404 Route Not Found")으로 단언한다.
	body="$(curl -s -m 10 "$B/lua-schema-reject/api/users")"
	echo "  cache_control 라우트 거부: ${body}"
	[[ "${body}" == *"404 Route Not Found"* ]] || { echo "FAIL: 스키마 거부 라우트가 살아 있다" >&2; exit 1; }
	# 로그를 먼저 담는다 — pipefail 아래 grep -q가 일찍 끝나면 logs가 SIGPIPE로 실패한다.
	logs="$(docker compose logs "${GW_SVC}" 2>&1)"
	[[ "${logs}" == *"found cache_control, key: /routes/lua-schema-reject"* ]] \
		|| { echo "FAIL: 스키마 거부 로그 없음" >&2; exit 1; }
	echo "  스키마 거부 로그: OK"
fi
echo "OK: [${GW_SVC}-in-memory] U4 고정 시나리오 통과"
