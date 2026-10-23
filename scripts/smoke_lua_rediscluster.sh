#!/usr/bin/env bash
set -euo pipefail

# Lua 포팅 redis-cluster 스모크(GW=kong-lua 기본 | apisix). 재실행 멱등: 클러스터 노드 3개 FLUSHALL
# (노드별 슬롯이 독립적이라 전 노드에 실행한다) — 서문은 common.sh의 lua_smoke가 공유한다.
source "$(dirname "$0")/common.sh"
lua_smoke rediscluster "/lua-redis-cluster"

# ---- 해제 실측: Set-Cookie 미저장 요청의 fill-lock이 log 단계 타이머로 사라지는가(#22) ----
# 락 키는 클러스터 한 노드에만 살므로 세 노드를 모두 훑는다.
B="${GW_BASE}"
sc1="$(curl -s -D- -o /dev/null -m 10 "$B/lua-redis-cluster/setcookie/api/users?lockrel=$(date +%s)" | tr -d '\r' | awk -F': ' '/^X-Cache-Status:/{print $2}')"
curl -s -o /dev/null -m 10 "$B/lua-redis-cluster/setcookie/api/users?lockrel2"
sleep 1 # 해제는 log 단계 타이머 — 한 바퀴 돌고 나서 센다
leftover="$(docker compose exec -T redis-cluster-node-1 sh -c \
  'for h in 172.30.0.11 172.30.0.12 172.30.0.13; do redis-cli -h "$h" -p 7000 --scan --pattern "sb-lock:*"; done')"
echo "  Set-Cookie 뒤 sb-lock 잔여: ${leftover:-<없음>} (X-Cache-Status=${sc1:-<none>})"
[[ -z "${leftover}" ]] || { echo "FAIL: 풀리지 않은 락 남음 — ${leftover}" >&2; exit 1; }

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
assert_burst_le "${GW_BASE}/lua-cluster-stampede/api/big?page=${CTAG}" 2 mock-upstream "${UP_ID}"
# 대조군(#9): 락 없는 같은 버스트는 업스트림을 동시성만큼 때려야 한다 — 위 상한 단언이
# 스스로 통과하는 빈 통과가 아님을 증명한다.
NTAG="nl-$(date +%s)"
assert_burst_ge "${GW_BASE}/lua-cluster-nolock/api/big?page=${NTAG}" 3 mock-upstream "${UP_ID}"

echo "OK: [${GW_SVC}-rediscluster] U7 시나리오 통과"
