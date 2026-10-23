# sonic-boom 로컬 개발 진입점. 로컬과 CI가 같은 경로를 공유한다.
#
#   make build  — 플러그인 바이너리 크로스컴파일(build.sh 위임)
#   make test   — 통합 테스트: compose 백엔드 기동 후 go test를
#                 compose 네트워크 안에서 실행(서버 없으면 실패, fail-fast)
#   make e2e    — Kong + 목업 업스트림 + 백엔드 전체를 clean state로
#                 띄워 세 캐시 전략 스모크 검증
#
# 단위 테스트만 빠르게 돌릴 땐 서버가 필요 없다:
#   go test ./internal -run 'Test_InMemory|TestConfig|Test_'

GOARCH ?= $(shell uname -m | sed -e s/x86_64/amd64/ -e s/aarch64/arm64/)
COMPOSE ?= docker compose
export COMPOSE # stress_scenarios.sh가 같은 compose 인자(-p 등)를 따라가게

.PHONY: build test e2e e2e-lua e2e-apisix stress clean

.ONESHELL:
SHELL := /bin/bash

build:
	GOOS=linux GOARCH=$(GOARCH) ./build.sh

test:
	set -euo pipefail
	trap '$(COMPOSE) down >/dev/null 2>&1 || true' EXIT
	$(COMPOSE) up -d --wait redis cluster-init
	$(COMPOSE) --profile test run --rm go-test

# clean compose 기동 — 시작 전 캐시를 비운다(첫 요청 Miss 단언의 전제).
# .ONESHELL이라 레시피 안에서 전개되므로 EXIT trap이 이후 스크립트까지 덮는다.
define compose_reset
	set -euo pipefail
	trap '$(COMPOSE) down -v --remove-orphans >/dev/null 2>&1 || true' EXIT
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) up -d --build --wait
endef

e2e: build
	$(compose_reset)
	./scripts/smoke_in_memory.sh
	./scripts/smoke_redis.sh
	./scripts/smoke_rediscluster.sh

# kong-lua(sonic-boom-lua)·APISIX 어댑터 e2e — 이미지 빌드·프로필 기동·전략 3종 스모크.
# 재실행 멱등은 각 스모크가 스스로 캐시를 비운다(shdict 재기동/FLUSH).
# 공유 레시피 — 타깃별로 GW·PROFILE·SVC·PORT 4토큰만 다르다(#25).
define e2e_gateway
	# .ONESHELL은 레시피를 셸 하나로 돌려 마지막 줄 종료 코드만 남긴다 — set -e 없으면 스모크 실패가 가려진다.
	# 실패 시 down을 건너뛰어 CI 진단 단계가 컨테이너 로그를 읽게 둔다(trap으로 내리지 않는다).
	set -euo pipefail
	# 서비스 명시 — 무지정 up은 Go kong(바이너리 필요)까지 띄운다. Lua 스택만.
	$(COMPOSE) --profile $(2) up -d --build --wait $(3) mock-upstream redis cluster-init
	curl -fsS 'http://localhost:$(4)/selfcheck/x' >/dev/null
	GW=$(1) ./scripts/smoke_lua_in_memory.sh
	GW=$(1) ./scripts/smoke_lua_redis.sh
	GW=$(1) ./scripts/smoke_lua_rediscluster.sh
	$(COMPOSE) --profile $(2) down -v --remove-orphans
endef

e2e-lua:
	$(call e2e_gateway,kong-lua,lua,kong-lua,8010)

e2e-apisix:
	$(call e2e_gateway,apisix,apisix,apisix,8020)

stress: build
	$(compose_reset)
	./scripts/stress_scenarios.sh

clean:
	$(COMPOSE) down -v --remove-orphans
