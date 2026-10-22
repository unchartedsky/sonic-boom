#!/usr/bin/env bash
set -euo pipefail

# Lua 포팅 redis 스모크(GW=kong-lua 기본 | apisix). 재실행 멱등: redis FLUSHDB(make e2e의 down -v 관행 대응).
source "$(dirname "$0")/common.sh"
gw_env
docker compose exec -T redis redis-cli FLUSHDB >/dev/null
docker compose --profile "${GW_PROFILE}" restart "${GW_SVC}" >/dev/null
sleep 6

BASE_URL="${GW_BASE}" SKIP_SCHEMA_ASSERT=1 HIT_RETRY_TIMEOUT=5 \
  "$(dirname "$0")/smoke_lib.sh" "redis" "/lua-redis"

# ---- U5 시나리오: redis 중단 시 fail-open ----
# fill-lock이 켜져 있으면 조회(setnx 포함)가 다이얼 타임아웃 2회(~10s)를 거친다 —
# fail-open 동작은 동일하나 요청이 느려지므로 커트를 20s로 둔다.
B="${GW_BASE}"
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
BURST_URL="${B}/lua-redis-stampede/api/big?page=${TAG}"
BEFORE=$(docker logs "${UP_ID}" 2>/dev/null | wc -l | tr -d ' ')
seq 4 | xargs -P 4 -I{} curl -s -o /dev/null -m 60 "${BURST_URL}"
sleep 3 # log 타이머 저장·access log flush 여유
AFTER=$(docker logs "${UP_ID}" 2>/dev/null | wc -l | tr -d ' ')
DELTA=$((AFTER - BEFORE))
echo "  fill-lock 버스트: 업스트림 ${DELTA}회 (기대 ≤2)"
[[ "${DELTA}" -le 2 ]] || { echo "FAIL: lua fill-lock 스탬피드 — 업스트림 ${DELTA}회" >&2; exit 1; }

echo "OK: [${GW_SVC}-redis] U5 시나리오 통과"
