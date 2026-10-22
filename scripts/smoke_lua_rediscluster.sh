#!/usr/bin/env bash
set -euo pipefail

# kong-lua redis-cluster 스모크. 재실행 멱등: 클러스터 노드 3개 FLUSHALL
# (노드별 슬롯이 독립적이라 전 노드에 실행한다).
docker compose exec -T redis-cluster-node-1 sh -c \
  'for h in 172.30.0.11 172.30.0.12 172.30.0.13; do redis-cli -h "$h" -p 7000 FLUSHALL; done' >/dev/null
docker compose --profile lua restart kong-lua >/dev/null
sleep 6

BASE_URL=http://localhost:8010 SKIP_SCHEMA_ASSERT=1 HIT_RETRY_TIMEOUT=5 \
  "$(dirname "$0")/smoke_lib.sh" "rediscluster" "/lua-redis-cluster"

# ---- U7 시나리오 ----
# vendoring 무결성: 재작명 누락(resty.xmodem/resty.redis 직접 참조) 없음
! grep -rn 'require "resty.xmodem"\|require "resty.redis"' lua/sonic_boom/ \
  || { echo "FAIL: vendored 모듈이 번들 경로를 참조" >&2; exit 1; }
echo "  vendoring 재작명 무결성: OK"

# ACL username 조립: Kong fork가 AUTH user pass 형태를 지원하는 소스 경로 보유
grep -q 'auth(self.config.username, self.config.password)' lua/sonic_boom/vendor/rediscluster.lua \
  || { echo "FAIL: ACL username 조립 경로 없음" >&2; exit 1; }
echo "  ACL username 조립(소스 수준, 런타임 실측은 백로그): OK"

# ---- U4c: 클러스터 fill-lock — SET NX 왕복(서버) + 종단간 버스트 단언 ----
# fork 범용 디스패치가 NX를 존중하지 않으면 락이 fail-open으로 풀려 버스트 단언이 잡는다.
LK="sb-lock:nxcheck-$(date +%s)"
r1="$(docker compose exec -T redis-cluster-node-1 sh -c "redis-cli -c -h 172.30.0.11 -p 7000 SET ${LK} tok1 EX 30 NX")"
r2="$(docker compose exec -T redis-cluster-node-1 sh -c "redis-cli -c -h 172.30.0.11 -p 7000 SET ${LK} tok2 EX 30 NX")"
echo "  cluster SET NX 왕복: 1차=[${r1}] 2차=[${r2}] (기대 OK / (nil))"
[[ "${r1}" == *"OK"* && "${r2}" != *"OK"* ]] || { echo "FAIL: 클러스터 SET NX 미지원 — fill-lock이 fail-open으로 풀린다" >&2; exit 1; }

UP_ID="$(docker compose ps -q mock-upstream)"
CTAG="cx-$(date +%s)"
CBURST="http://localhost:8010/lua-cluster-stampede/api/big?page=${CTAG}"
CBEFORE=$(docker logs "${UP_ID}" 2>/dev/null | wc -l | tr -d ' ')
seq 4 | xargs -P 4 -I{} curl -s -o /dev/null -m 60 "${CBURST}"
sleep 3 # log 타이머 저장·access log flush 여유
CAFTER=$(docker logs "${UP_ID}" 2>/dev/null | wc -l | tr -d ' ')
CDELTA=$((CAFTER - CBEFORE))
echo "  cluster fill-lock 버스트: 업스트림 ${CDELTA}회 (기대 ≤2)"
[[ "${CDELTA}" -le 2 ]] || { echo "FAIL: 클러스터 fill-lock — 업스트림 ${CDELTA}회(fail-open 의심)" >&2; exit 1; }

echo "OK: [lua-rediscluster] U7 시나리오 통과"
