#!/usr/bin/env bash
set -euo pipefail

# kong-lua redis 스모크. 재실행 멱등: redis FLUSHDB(make e2e의 down -v 관행 대응).
docker compose exec -T redis redis-cli FLUSHDB >/dev/null
docker compose --profile lua restart kong-lua >/dev/null
sleep 6

BASE_URL=http://localhost:8010 SKIP_SCHEMA_ASSERT=1 HIT_RETRY_TIMEOUT=5 \
  "$(dirname "$0")/smoke_lib.sh" "redis" "/lua-redis"

# ---- U5 시나리오: redis 중단 시 fail-open ----
B=http://localhost:8010
docker compose stop redis >/dev/null
sleep 1
got="$(curl -s -D- -o /dev/null -m 10 "$B/lua-redis/api/users?down1" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}')"
docker compose start redis >/dev/null
echo "  redis 중단 시 요청: ${got:-<none>} (기대 Miss)"
[[ "${got}" == "Miss" ]] || { echo "FAIL: redis 중단 fail-open" >&2; exit 1; }
docker compose logs kong-lua --since 30s 2>&1 | grep -q "redis connect failed" \
  && echo "  redis 중단 오류 로그: OK" || { echo "FAIL: 오류 로그 없음" >&2; exit 1; }
echo "OK: [lua-redis] U5 시나리오 통과"
