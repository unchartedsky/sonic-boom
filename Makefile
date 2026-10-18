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

.PHONY: build test e2e clean

.ONESHELL:
SHELL := /bin/bash

build:
	GOOS=linux GOARCH=$(GOARCH) ./build.sh

test:
	set -euo pipefail
	trap '$(COMPOSE) down >/dev/null 2>&1 || true' EXIT
	$(COMPOSE) up -d --wait redis cluster-init
	$(COMPOSE) --profile test run --rm go-test

e2e: build
	set -euo pipefail
	trap '$(COMPOSE) down -v --remove-orphans >/dev/null 2>&1 || true' EXIT
	# 시작 전 캐시를 비운다 — 첫 요청 Miss 단언의 전제 조건
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) up -d --build --wait
	./scripts/smoke_in_memory.sh
	./scripts/smoke_redis.sh
	./scripts/smoke_rediscluster.sh

clean:
	$(COMPOSE) down -v --remove-orphans
