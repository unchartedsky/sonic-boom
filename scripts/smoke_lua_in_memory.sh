#!/usr/bin/env bash
set -euo pipefail

# kong-lua in-memory 스모크. 재실행 멱등: make e2e의 down -v 관행 대신
# kong-lua 재기동으로 shdict를 비운다(첫 요청 Miss 전제).
docker compose --profile lua restart kong-lua >/dev/null
sleep 6

BASE_URL=http://localhost:8010 SKIP_SCHEMA_ASSERT=1 \
  "$(dirname "$0")/smoke_lib.sh" "in_memory" "/lua-in-memory"

# ---- U4 고정 시나리오(재사용 라이브러리 밖) ----
B=http://localhost:8010
hdr() { curl -s -D- -o /dev/null -m 10 "$@" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}'; }
want() { local got; got="$(hdr "$@")"; echo "  $2: ${got:-<none>} (기대 $1)"; [[ "$got" == "$1" ]] || { echo "FAIL: $2" >&2; exit 1; }; }

want Bypass "content_type 목록 밖(ctgate)"
want Bypass "Set-Cookie 응답 미저장(setcookie)"
want Miss "body_max_size 초과 1회차(maxsize)"
want Miss "body_max_size 초과 2회차(미저장 확인)"
echo "OK: [lua-in-memory] U4 고정 시나리오 통과"
