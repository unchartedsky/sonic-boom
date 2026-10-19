#!/usr/bin/env bash
# 동접 스트레스 시나리오 — 스탬피드(fill-lock on/off)와 CSC on/off 실측.
# 사용법: ./scripts/stress_scenarios.sh [동시성=200] [요청수=2000]
# 전제: make stress(또는 make e2e 후 compose 유지)로 Kong이 떠 있어야 한다.
# 지표: 업스트림(mock-upstream nginx) 호출 수, p50/p95/p99, rps.
set -euo pipefail

C="${1:-200}"
N="${2:-2000}"
BASE="${BASE_URL:-http://localhost:8000}"

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# nginx access log는 stdout으로 흐른다 — docker logs 줄 수로 업스트림 호출 수.
upstream_count() {
	docker logs sonic-boom-mock-upstream-1 2>/dev/null | wc -l | tr -d ' '
}

# ab 출력에서 백분위·rps 추출
pct_of() { awk -v p="$1%" '$1==p {print $2}' <<<"$2"; }

# curl 동시 버스트: ab는 같은 키 동시 버스트를 순차적으로 풀어놓아
# 스탬피드 측정에 부적합하다(실측 3@3에도 업스트림 1). 진짜 동시성은
# xargs -P로, 백분위는 수집한 time_total에서 직접 계산한다.
run_burst() {
	local url="$1" n="$2" c="$3"
	seq "${n}" | xargs -P "${c}" -I{} curl -s -o /dev/null -m 60 -w '%{time_total}\n' "${url}"
}

pct_from_times() { # 정렬된 초 단위 목록에서 백분위(ms)
	local sorted="$1" p="$2" n idx
	n=$(wc -l <<<"${sorted}")
	idx=$(( (n * p + 99) / 100 ))
	[[ ${idx} -lt 1 ]] && idx=1
	awk -v i="${idx}" 'NR==i {printf "%.0f", $1*1000}' <<<"${sorted}"
}

run_scenario() {
	local label="$1" route="$2" path="$3" tag="$4" mode="$5" n="$6" settle="$7"
	local url="${BASE}${route}${path}?page=${tag}-$(date +%s)"
	local out before after times sorted

	if [[ "${mode}" == "warm" ]]; then
		ab -q -k -n 100 -c 10 "${url}" >/dev/null 2>&1
	fi

	before=$(upstream_count)
	if [[ "${mode}" == "warm" ]]; then
		out=$(ab -q -k -n "${n}" -c "${C}" "${url}" 2>&1)
	else
		times=$(run_burst "${url}" "${n}" "${C}")
	fi
	sleep "${settle}" # access log flush 여유(느린 응답은 전송 종료 뒤 기록)
	after=$(upstream_count)

	local delta p50 p95 p99 rps
	delta=$((after - before))
	if [[ "${mode}" == "warm" ]]; then
		p50=$(pct_of 50 "${out}")
		p95=$(pct_of 95 "${out}")
		p99=$(pct_of 99 "${out}")
		rps=$(awk '/^Requests per second:/ {print $4}' <<<"${out}" | head -1)
	else
		sorted=$(sort -n <<<"${times}")
		p50=$(pct_from_times "${sorted}" 50)
		p95=$(pct_from_times "${sorted}" 95)
		p99=$(pct_from_times "${sorted}" 99)
		rps=$(awk -v n="${n}" -v mx="$(tail -1 <<<"${sorted}")" 'BEGIN {if (mx>0) printf "%.1f", n/mx}')
	fi

	printf "| %-28s | %6d | %7d | %6s | %7s | %7s | %8s |\n" \
		"${label}" "${C}" "${delta}" "${p50:-?}" "${p95:-?}" "${p99:-?}" "${rps:-?}"
}

main() {
	wait_for_kong "${BASE}"
	echo "== 동접 스트레스 시나리오 (동시성=${C}, keepalive) =="
	echo
	echo "| 시나리오 | 동시성 | 업스트림 호출 | p50(ms) | p95(ms) | p99(ms) | rps |"
	echo "|---|---|---|---|---|---|---|"

	run_scenario "스탬피드, fill-lock 끔" /redis-nolock /api/big stampede cold "${C}" 12
	run_scenario "스탬피드, fill-lock 켬" /redis-stampede /api/big stampede2 cold "${C}" 12

	run_scenario "warm, CSC off (db 0)" /redis-nocsc /api/users csc warm "${N}" 1
	run_scenario "warm, CSC on  (db 0)" /redis-csc /api/users csc2 warm "${N}" 1

	echo
	echo "해석: 스탬피드 = 업스트림 호출 수 비교(락 켬 ≪ 끔)."
	echo "CSC = warm p50/p95/p99와 rps 비교(같은 키 반복 조회의 로컬 L1 흡수)."
}

main
