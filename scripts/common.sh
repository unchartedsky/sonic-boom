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
