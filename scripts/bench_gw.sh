#!/usr/bin/env bash
# 게이트웨이 벤치 — hit·miss 경로 교차 3회. BENCH_PAIR로 쌍을 고른다:
#   go-lua(기본): Go(:8000) vs Kong-Lua(:8010) — 같은 게이트웨이(Kong), 플러그인 구현 비교.
#   lua-apisix: Kong-Lua(:8010) vs APISIX(:8020) — 같은 코어, 게이트웨이 비교. 두 게이트웨이는
#     기본 프록시 처리량부터 달라 원시 수치로는 플러그인 비용을 가를 수 없다. 그래서 각자의
#     /noplugins를 같은 인터리브 회차에 함께 재고, "플러그인 라우트 대 자기 /noplugins" 오버헤드
#     비율을 두 게이트웨이 사이에서 비교해 판정한다.
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

sec() { echo "${1%secs}"; }  # hey의 "0.0240secs" → 0.0240

# lua-apisix: 회차마다 A기본·A플러그인·B기본·B플러그인을 번갈아 잰다(같은 인터리브 회차).
suite_overhead() { # <이름> <a_base> <a_plugin> <b_base> <b_plugin>
	local name="$1"
	echo "== ${name} (c=${CONC}, z=${DUR}, 교차 ${ROUNDS}회, 기준=자기 /noplugins) =="
	local abr=() ab9=() apr=() ap9=() bbr=() bb9=() bpr=() bp9=() r p5 p9
	for _ in $(seq 1 "$ROUNDS"); do
		read -r r p5 p9 <<<"$(run_bench "$2")"; abr+=("$r"); ab9+=("$(sec "$p9")")
		read -r r p5 p9 <<<"$(run_bench "$3")"; apr+=("$r"); ap9+=("$(sec "$p9")")
		read -r r p5 p9 <<<"$(run_bench "$4")"; bbr+=("$r"); bb9+=("$(sec "$p9")")
		read -r r p5 p9 <<<"$(run_bench "$5")"; bpr+=("$r"); bp9+=("$(sec "$p9")")
	done
	local m_abr m_apr m_bbr m_bpr m_ab9 m_ap9 m_bb9 m_bp9
	m_abr=$(median "${abr[@]}"); m_apr=$(median "${apr[@]}"); m_bbr=$(median "${bbr[@]}"); m_bpr=$(median "${bpr[@]}")
	m_ab9=$(median "${ab9[@]}"); m_ap9=$(median "${ap9[@]}"); m_bb9=$(median "${bb9[@]}"); m_bp9=$(median "${bp9[@]}")
	echo "KONG-LUA: 기본 RPS=${m_abr} p99=${m_ab9}s | 플러그인 RPS=${m_apr} p99=${m_ap9}s"
	echo "APISIX  : 기본 RPS=${m_bbr} p99=${m_bb9}s | 플러그인 RPS=${m_bpr} p99=${m_bp9}s"
	# 오버헤드 비율: RPS 유지율(플러그인/기본), p99 배율(플러그인/기본). 판정은 APISIX가 Kong-Lua 대비
	# RPS 유지율이 10% 넘게 낮거나 p99 배율이 20% 넘게 높으면 유의미 열세(선등록 문턱).
	awk -v abr="$m_abr" -v apr="$m_apr" -v bbr="$m_bbr" -v bpr="$m_bpr" \
		-v ab9="$m_ab9" -v ap9="$m_ap9" -v bb9="$m_bb9" -v bp9="$m_bp9" 'BEGIN {
		ka = apr / abr; aa = bpr / bbr; k9 = ap9 / ab9; a9 = bp9 / bb9
		printf "오버헤드: RPS 유지율 kong-lua=%.3f apisix=%.3f (상대 %.1f%%) | p99 배율 kong-lua=%.2f apisix=%.2f (상대 %+.1f%%)\n", ka, aa, (aa/ka - 1) * 100, k9, a9, (a9/k9 - 1) * 100
		verdict = ((aa/ka - 1) * 100 < -10 || (a9/k9 - 1) * 100 > 20) ? "유의미 열세(문턱 초과)" : "문턱 안"
		print "판정: " verdict
	}'
}

if [[ "${BENCH_PAIR:-go-lua}" == "lua-apisix" ]]; then
	docker compose --profile lua --profile apisix up -d --build --wait kong-lua apisix mock-upstream redis cluster-init >/dev/null 2>&1
	for _ in $(seq 1 30); do
		if curl -sf -o /dev/null http://localhost:8010/lua-in-memory/api/users?warm && \
		   curl -sf -o /dev/null http://localhost:8020/lua-in-memory/api/users?warm; then
			break
		fi
		sleep 1
	done
	curl -sf -o /dev/null http://localhost:8010/lua-in-memory/api/users?bench=hit
	curl -sf -o /dev/null http://localhost:8020/lua-in-memory/api/users?bench=hit
	suite_overhead "HIT (in-memory)" \
		"http://localhost:8010/noplugins/api/users?bench=base" "http://localhost:8010/lua-in-memory/api/users?bench=hit" \
		"http://localhost:8020/noplugins/api/users?bench=base" "http://localhost:8020/lua-in-memory/api/users?bench=hit"
	suite_overhead "MISS (maxsize)" \
		"http://localhost:8010/noplugins/api/users?bench=base" "http://localhost:8010/lua-maxsize/api/users?bench=miss" \
		"http://localhost:8020/noplugins/api/users?bench=base" "http://localhost:8020/lua-maxsize/api/users?bench=miss"
	exit 0
fi

# 준비: 스택 기동 + hit 캐시 데우기 + 캐시 비움(miss 라우트는 자체 미저장)
docker compose --profile lua up -d --build --wait >/dev/null 2>&1
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
