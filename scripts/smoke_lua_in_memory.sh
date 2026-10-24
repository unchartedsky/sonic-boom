#!/usr/bin/env bash
set -euo pipefail

# Lua 포팅 in-memory 스모크(GW=kong-lua 기본 | apisix). 재실행 멱등: 게이트웨이 재기동으로
# shdict를 비운다(첫 요청 Miss 전제) — 서문은 common.sh의 lua_smoke가 공유한다.
source "$(dirname "$0")/common.sh"
lua_smoke in_memory "/lua-in-memory"

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
want Bypass "필터 어느 룰도 매치하지 않음(nomatch)" "$B/lua-nomatch/api/users"
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
	# strategy=redis인데 redis 블록이 없는 라우트: YAML 배치는 로드 거부가 불가능하므로(실측 —
	# if/then·oneOf 무시, check_schema 훅은 Admin API 모드 전용) 런타임 fail-open이 계약이다.
	# 접속 실패 → Miss → 업스트림 통과로 서비스가 살아있는지를 단언한다.
	mh_xcs="$(curl -s -m 10 -o /dev/null -w '%{header_json}' "$B/lua-schema-reject-redis/api/users" 2>/dev/null | grep -o '"x-cache-status":\["[^"]*"' | cut -d'"' -f4)"
	echo "  redis 미선언 라우트 런타임: X-Cache-Status=${mh_xcs} (기대 Bypass — fail-open 후 미저장)"
	[[ "${mh_xcs}" == "Bypass" ]] || { echo "FAIL: 저장소 없는 라우트가 fail-open하지 않는다" >&2; exit 1; }
	echo "  저장소 미선언 fail-open: OK"
fi
echo "OK: [${GW_SVC}-in-memory] U4 고정 시나리오 통과"
