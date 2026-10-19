#!/usr/bin/env bash
# 스모크·스트레스 스크립트 공통 헬퍼.
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
