#!/usr/bin/env bash
# U8 게이트웨이 벤치 — Go(:8000) vs Lua(:8010), hit·miss 경로 교차 3회.
# 규율: 같은 저장소 전략 짝(hit=공유 dict/ristretto 노드 내, miss=상한 초과 라우트),
# GO→LUA 교차 3회 반복(대표값+편차), 클라이언트는 호스트 hey(Docker VM과 분리),
# 양 게이트웨이 cpus 2.0 고정(compose). 선등록 문턱: p99 회귀 >20%, RPS 저하 >10%.
set -euo pipefail
cd "$(dirname "$0")/.."

DUR="${BENCH_DURATION:-10s}"
CONC="${BENCH_CONCURRENCY:-50}"
ROUNDS=3

run_bench() { # <url> → "rps p50 p99"
	local out
	out="$(hey -c "$CONC" -z "$DUR" "$1" 2>&1)"  # hey은 요약을 stderr로 찍는다
	local rps p50 p99
	rps="$(awk '/Requests\/sec:/{print $2}' <<<"$out")"
	p50="$(awk '/ 50%% in /{print $3, $4}' <<<"$out" | tr -d ' ')"  # %% 리터럴
	p99="$(awk '/ 99%% in /{print $3, $4}' <<<"$out" | tr -d ' ')"
	echo "${rps:-0} ${p50:-na} ${p99:-na}"
}

median() { printf '%s\n' "$@" | sort -g | awk '{a[NR]=$1} END{print (NR%2)?a[(NR+1)/2]:(a[NR/2]+a[NR/2+1])/2}'; }
max()    { printf '%s\n' "$@" | sort -g | awk '{a[NR]=$1} END{print a[NR]}'; }

suite() { # <이름> <go_url> <lua_url> <warmup_url...>
	local name="$1" go_url="$2" lua_url="$3"
	echo "== ${name} (c=${CONC}, z=${DUR}, 교차 ${ROUNDS}회) =="
	local gor=() g5=() g9=() lur=() lu5=() lu9=()
	for _ in $(seq 1 "$ROUNDS"); do
		read -r r p5 p9 <<<"$(run_bench "$go_url")";  gor+=("$r"); g5+=("$p5"); g9+=("$p9")
		read -r r p5 p9 <<<"$(run_bench "$lua_url")"; lur+=("$r"); lu5+=("$p5"); lu9+=("$p9")
	done
	echo "GO : RPS[$(printf '%.0f ' "${gor[@]}")] 중앙=$(median "${gor[@]}") 최대편차=$(max "${gor[@]}") p50=$(median "${g5[@]}") p99=$(median "${g9[@]}")"
	echo "LUA: RPS[$(printf '%.0f ' "${lur[@]}")] 중앙=$(median "${lur[@]}") 최대편차=$(max "${lur[@]}") p50=$(median "${lu5[@]}") p99=$(median "${lu9[@]}")"
}

# 준비: 스택 기동 + hit 캐시 데우기 + 캐시 비움(miss 라우트는 자체 미저장)
docker compose --profile lua up -d --wait >/dev/null 2>&1
for _ in $(seq 1 30); do
	if curl -sf -o /dev/null http://localhost:8000/in-memory/api/users?warm && \
	   curl -sf -o /dev/null http://localhost:8010/lua-in-memory/api/users?warm; then
		break
	fi
	sleep 1
done
curl -sf -o /dev/null http://localhost:8000/in-memory/api/users?bench=hit
curl -sf -o /dev/null http://localhost:8010/lua-in-memory/api/users?bench=hit

suite "HIT (in-memory)"  "http://localhost:8000/in-memory/api/users?bench=hit"  "http://localhost:8010/lua-in-memory/api/users?bench=hit"
suite "MISS (maxsize)"   "http://localhost:8000/go-maxsize/api/users?bench=miss" "http://localhost:8010/lua-maxsize/api/users?bench=miss"
