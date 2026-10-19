#!/usr/bin/env bash
# 동접 스트레스 시나리오 — 스탬피드(fill-lock on/off)와 CSC on/off 실측.
# 사용법: ./scripts/stress_scenarios.sh [동시성=200] [요청수=2000]
# 전제: make stress(또는 make e2e 후 compose 유지)로 Kong이 떠 있어야 한다.
# 지표: 업스트림(mock-upstream nginx) 호출 수, p50/p95/p99, rps.
# 종료 코드: 표 아래 단언 중 하나라도 실패하면 1.
set -euo pipefail

C="${1:-200}"
N="${2:-2000}"
BASE="${BASE_URL:-http://localhost:8000}"
UPSTREAM_ID=""
LAST_DELTA=0

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# nginx access log는 stdout으로 흐른다 — docker logs 줄 수로 업스트림 호출 수.
# 컨테이너명 하드코딩은 compose 프로젝트명이 달라지면 delta=0 거짓 표를 내므로 쓰지 않는다.
upstream_count() {
	docker logs "${UPSTREAM_ID}" 2>/dev/null | wc -l | tr -d ' '
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
		# xargs는 curl 실패분에 123을 반환 — 부분 결과로 계속하고 기대 미달은 단언이 잡는다.
		times=$(run_burst "${url}" "${n}" "${C}") || true
	fi
	sleep "${settle}" # access log flush 여유(느린 응답은 전송 종료 뒤 기록)
	after=$(upstream_count)

	local delta p50 p95 p99 rps
	delta=$((after - before))
	LAST_DELTA=${delta}
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
	# make stress가 저장소 루트에서 이 스크립트를 실행하므로 docker compose ps가 신뢰 가능하다.
	UPSTREAM_ID=$(docker compose ps -q mock-upstream 2>/dev/null || true)
	if [[ -z "${UPSTREAM_ID}" ]]; then
		echo "FAIL: mock-upstream 컨테이너를 찾을 수 없음 — make stress로 compose를 띄웠는지 확인" >&2
		exit 1
	fi
	echo "== 동접 스트레스 시나리오 (동시성=${C}, keepalive) =="
	echo
	echo "| 시나리오 | 동시성 | 업스트림 호출 | p50(ms) | p95(ms) | p99(ms) | rps |"
	echo "|---|---|---|---|---|---|---|"

	run_scenario "스탬피드, fill-lock 끔" /redis-nolock /api/big stampede cold "${C}" 12
	delta_nolock=${LAST_DELTA}
	run_scenario "스탬피드, fill-lock 켬" /redis-stampede /api/big stampede2 cold "${C}" 12
	delta_lock=${LAST_DELTA}

	run_scenario "warm, CSC off (db 0)" /redis-nocsc /api/users csc warm "${N}" 1
	delta_nocsc=${LAST_DELTA}
	run_scenario "warm, CSC on  (db 0)" /redis-csc /api/users csc2 warm "${N}" 1
	delta_csc=${LAST_DELTA}

	echo
	echo "해석: 스탬피드 = 업스트림 호출 수 비교(락 켬 ≪ 끔)."
	echo "CSC = warm p50/p95/p99와 rps 비교(같은 키 반복 조회의 로컬 L1 흡수)."

	# 단언 — 경계값은 1회 실측 기반(스탬피드 켬 1, 끔 200/200, warm 0/0)이므로
	# 환경·타이밍에 따라 재조정이 필요할 수 있다. 90%는 동시성 비율로 계산해 -C와 무관하게 유지.
	local lock_max=2 nolock_min=$((C * 90 / 100)) fail=0
	echo
	echo "== 단언 =="
	if (( delta_lock <= lock_max )); then
		echo "PASS: 스탬피드 fill-lock 켬 — 업스트림 ${delta_lock} ≤ ${lock_max}"
	else
		echo "FAIL: 스탬피드 fill-lock 켬 — 업스트림 ${delta_lock} > ${lock_max}"
		fail=1
	fi
	if (( delta_nolock >= nolock_min )); then
		echo "PASS: 스탬피드 fill-lock 끔 — 업스트림 ${delta_nolock} ≥ ${nolock_min} (동시성의 90%)"
	else
		echo "FAIL: 스탬피드 fill-lock 끔 — 업스트림 ${delta_nolock} < ${nolock_min} (동시성의 90%)"
		fail=1
	fi
	if (( delta_nocsc == 0 && delta_csc == 0 )); then
		echo "PASS: warm CSC off/on (db 0) — 업스트림 ${delta_nocsc}/${delta_csc} == 0/0"
	else
		echo "FAIL: warm CSC off/on (db 0) — 업스트림 ${delta_nocsc}/${delta_csc}, 기대 0/0"
		fail=1
	fi
	return "${fail}"
}

main
