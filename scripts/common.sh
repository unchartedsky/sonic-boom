#!/usr/bin/env bash
# 스모크·스트레스 스크립트 공통 헬퍼.

# Lua 포팅 게이트웨이 선택: GW=kong-lua(기본)|apisix → GW_SVC·GW_PROFILE·GW_BASE·GW_KEY_PREFIX.
# 두 게이트웨이는 같은 코어·같은 /lua-* 라우트 경로를 쓰므로 스모크 본문은 공유한다.
gw_env() {
	case "${GW:-kong-lua}" in
		kong-lua) GW_SVC=kong-lua GW_PROFILE=lua GW_BASE=http://localhost:8010 GW_KEY_PREFIX=sbl ;;
		apisix) GW_SVC=apisix GW_PROFILE=apisix GW_BASE=http://localhost:8020 GW_KEY_PREFIX=sba ;;
		*) echo "FAIL: 알 수 없는 GW='${GW}' (kong-lua|apisix)" >&2; return 1 ;;
	esac
}

wait_for_kong() {
	local base="${1:?BASE URL 필수}"
	local probe="${base}/noplugins/api/users?page=probe"
	local i
	for i in $(seq 1 60); do
		curl -s -o /dev/null -m 2 "${probe}" && return 0
		sleep 1
	done
	echo "FAIL: ${base}에 60초 내 접근 불가 — docker compose 기동 상태를 확인하세요" >&2
	return 1
}

# Lua 스모크 공유 서문: 캐시 비우기 → 게이트웨이 재기동(shdict·상태 초기화) → 공유 검증 라이브러리.
# 사용법: lua_smoke <전략명: in_memory|redis|rediscluster> <라우트 prefix>
lua_smoke() {
	local strategy="$1" route="$2"
	gw_env
	case "${strategy}" in
		redis)
			docker compose exec -T redis redis-cli FLUSHDB >/dev/null
			# log 타이머 저장은 응답 직후 비동기라 Hit 재시도 기한을 둔다
			export HIT_RETRY_TIMEOUT=5 ;;
		rediscluster)
			# 노드별 슬롯이 독립적이라 전 노드에 실행한다
			docker compose exec -T redis-cluster-node-1 sh -c \
				'for h in 172.30.0.11 172.30.0.12 172.30.0.13; do redis-cli -h "$h" -p 7000 FLUSHALL; done' >/dev/null
			export HIT_RETRY_TIMEOUT=5 ;;
	esac
	docker compose --profile "${GW_PROFILE}" restart "${GW_SVC}" >/dev/null
	sleep 6
	BASE_URL="${GW_BASE}" SKIP_SCHEMA_ASSERT=1 \
		"$(dirname "${BASH_SOURCE[0]}")/smoke_lib.sh" "${strategy}" "${route}"
}

# 4동시 버스트 뒤 업스트림 호출 증분 — nginx access log가 stdout으로 흐르는 점을 이용해
# docker logs 줄 수로 센다(stress_scenarios.sh와 같은 실측 방식).
burst_delta() { # <url> <upstream id> → 증분을 stdout으로
	local url="$1" up_id="$2" before after
	before=$(docker logs "${up_id}" 2>/dev/null | wc -l | tr -d ' ')
	seq 4 | xargs -P 4 -I{} curl -s -o /dev/null -m 60 "${url}"
	sleep 3 # log 타이머 저장·access log flush 여유
	after=$(docker logs "${up_id}" 2>/dev/null | wc -l | tr -d ' ')
	echo $((after - before))
}

# fill-lock 버스트 상한: 잠긴 키 4동시 버스트의 업스트림 증분이 max 이하여야 한다.
assert_burst_le() { # <url> <max> <이름> <upstream id>
	local delta
	delta="$(burst_delta "$1" "$4")"
	echo "  fill-lock 버스트($3): 업스트림 ${delta}회 (기대 ≤$2)"
	(( delta <= $2 )) || { echo "FAIL: 버스트 업스트림 ${delta}회($3) — fill-lock 실패 의심" >&2; return 1; }
}

# fill-lock 끔 대조군: 같은 버스트가 min 회 이상 때려야 한다 — 잠긴쪽 상한 단언이
# 버스트 자체가 동시성을 못 내 스스로 통과하는 빈 통과(vacuous pass)를 막는 증거다.
assert_burst_ge() { # <url> <min> <이름> <upstream id>
	local delta
	delta="$(burst_delta "$1" "$4")"
	echo "  버스트 대조($3): 업스트림 ${delta}회 (기대 ≥$2)"
	(( delta >= $2 )) || { echo "FAIL: 대조 버스트 업스트림 ${delta}회($3) — 동시 버스트가 성립하지 않는다" >&2; return 1; }
}
