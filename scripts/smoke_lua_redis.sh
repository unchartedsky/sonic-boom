#!/usr/bin/env bash
set -euo pipefail

# Lua 포팅 redis 스모크(GW=kong-lua 기본 | apisix). 재실행 멱등: redis FLUSHDB(make e2e의 down -v 관행 대응).
source "$(dirname "$0")/common.sh"
lua_smoke redis "/lua-redis"

B="${GW_BASE}"

# ---- 해제 실측: Set-Cookie 미저장 요청의 fill-lock이 log 단계 타이머로 사라지는가(#22) ----
sc1="$(curl -s -D- -o /dev/null -m 10 "$B/lua-redis/setcookie/api/users?lockrel=$(date +%s)" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}')"
curl -s -o /dev/null -m 10 "$B/lua-redis/setcookie/api/users?lockrel2"
sleep 1 # 해제는 log 단계 타이머 — 한 바퀴 돌고 나서 센다
leftover="$(docker compose exec -T redis redis-cli --scan --pattern 'sb-lock:*')"
echo "  Set-Cookie 뒤 sb-lock 잔여: ${leftover:-<없음>} (X-Cache-Status=${sc1:-<none>})"
[[ -z "${leftover}" ]] || { echo "FAIL: 풀리지 않은 락 남음 — ${leftover}" >&2; exit 1; }

# ---- U5 시나리오: redis 중단 시 fail-open ----
# fill-lock이 켜져 있으면 조회(setnx 포함)가 다이얼 타임아웃 2회(~10s)를 거친다 —
# fail-open 동작은 동일하나 요청이 느려지므로 커트를 20s로 둔다.
docker compose stop redis >/dev/null
sleep 1
got="$(curl -s -D- -o /dev/null -m 20 "$B/lua-redis/api/users?down1" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}')"
docker compose start redis >/dev/null
echo "  redis 중단 시 요청: ${got:-<none>} (기대 Miss)"
[[ "${got}" == "Miss" ]] || { echo "FAIL: redis 중단 fail-open" >&2; exit 1; }
# 로그를 먼저 담는다 — pipefail 아래 grep -q가 일찍 끝나면 logs가 SIGPIPE로 실패한다.
logs="$(docker compose logs "${GW_SVC}" --since 30s 2>&1)"
[[ "${logs}" == *"redis connect failed"* ]] \
  && echo "  redis 중단 오류 로그: OK" || { echo "FAIL: 오류 로그 없음" >&2; exit 1; }

# ---- U4b: fill-lock 스탬피드 — 동시 버스트에서 업스트림 1회 fill ----
UP_ID="$(docker compose ps -q mock-upstream)"
TAG="st-$(date +%s)"
assert_burst_le "${B}/lua-redis-stampede/api/big?page=${TAG}" 2 mock-upstream "${UP_ID}"
# 대조군(#9): 락 없는 같은 버스트는 업스트림을 동시성만큼 때려야 한다 — 위 상한 단언이
# 버스트 자체가 동시성을 못 내 스스로 통과하는 빈 통과가 아니라는 증거다.
NTAG="nl-$(date +%s)"
assert_burst_ge "${B}/lua-redis-nolock/api/big?page=${NTAG}" 3 mock-upstream "${UP_ID}"

echo "OK: [${GW_SVC}-redis] U5 시나리오 통과"
