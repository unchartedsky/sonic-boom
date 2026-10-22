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

# kong-lua(Lua 포팅) 기동 확인 — profile lua로 떠운다. 스모크는 U4부터 smoke_lua_*가 붙는다.
# kong-lua(sonic-boom-lua) e2e — 이미지 빌드·profile lua 기동·전략 3종 스모크.
# 재실행 멱등은 각 스모크가 스스로 캐시를 비운다(shdict 재기동/FLUSH).
e2e-lua:
	# .ONESHELL은 레시피를 셸 하나로 돌려 마지막 줄 종료 코드만 남긴다 — set -e 없으면 스모크 실패가 가려진다.
	# 실패 시 down을 건너뛰어 CI 진단 단계가 컨테이너 로그를 읽게 둔다(trap으로 내리지 않는다).
	set -euo pipefail
	# 서비스 명시 — 무지정 up은 Go kong(바이너리 필요)까지 띄운다. Lua 스택만.
	$(COMPOSE) --profile lua up -d --build --wait kong-lua mock-upstream redis cluster-init
	curl -fsS 'http://localhost:8010/selfcheck/x' >/dev/null
	./scripts/smoke_lua_in_memory.sh
	./scripts/smoke_lua_redis.sh
	./scripts/smoke_lua_rediscluster.sh
	$(COMPOSE) --profile lua down -v --remove-orphans

# APISIX 어댑터 e2e — kong-lua와 같은 스모크 3종을 GW=apisix로 돌린다(같은 코어·같은 /lua-* 경로).
# Kong-Lua와 포트(8010/8020)·프로필이 달라 같은 러너에서 독립 실행된다.
e2e-apisix:
	set -euo pipefail  # e2e-lua와 같은 이유
	$(COMPOSE) --profile apisix up -d --build --wait apisix mock-upstream redis cluster-init
	curl -fsS 'http://localhost:8020/selfcheck/x' >/dev/null
	GW=apisix ./scripts/smoke_lua_in_memory.sh
	GW=apisix ./scripts/smoke_lua_redis.sh
	GW=apisix ./scripts/smoke_lua_rediscluster.sh
	$(COMPOSE) --profile apisix down -v --remove-orphans

stress: build
	$(compose_reset)
	./scripts/stress_scenarios.sh

clean:
	$(COMPOSE) down -v --remove-orphans
