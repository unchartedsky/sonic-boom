---
title: "feat: 단위 우선·통합 보완 테스트 체계와 docker compose e2e"
date: 2026-10-18
plan_type: feat
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
---

# feat: 단위 우선·통합 보완 테스트 체계와 docker compose e2e

> 작성 위치 참고: 플랜 모드 제약으로 이 파일(`~/.claude/plans/docker-compose-eager-kurzweil.md`)에 작성했다. 승인 후 실행 단계에서 `docs/plans/2026-10-18-feat-layered-testing-e2e-plan.md`으로 복사해 저장한다(sonic-boom의 CE artifact root는 `docs/`).

## Goal Capsule

**Objective:** 세 캐시 전략(in-memory, redis, redis-cluster) 모두 로컬 docker compose 환경에서 Kong 게이트웨이까지 실제로 띄워 검증되는 상태로 만든다. 단위 테스트가 순수 로직을 먼저 커버하고, 단위로 검증할 수 없는 영역(Access/Response 등 PDK 의존 로직)을 통합·e2e가 메우는 상호보완 구조를 갖춘다.

**Means:** 유령 스켈레톤 테스트 정리 + 순수 함수 단위 테스트 보강 → compose 기반 hermetic e2e(목업 업스트림, 전략별 smoke 강화) → Makefile 단일 진입점으로 로컬·CI 동일화.

**Stop conditions:** e2e가 외부 인터넷(reqres.in) 없이 결정론적으로 통과하고, CI가 세 전략 smoke를 모두 실행하며, 빈 테이블 테스트가 더 이상 통과로 위장하지 않으면 완료.

## Product Contract

### Summary

테스트를 단위 → 통합 → e2e 3층으로 재편한다. 각 층은 검증 가능한 것을 나눠 갖는다(상호보완). 모든 통합·e2e 실행 경로는 docker compose 단일 경로이며, 서버가 없으면 실패한다(fail-fast).

### Problem Frame

2026-10-18 커버리지 실측(이 세션) 결과 전체 32.1%(statements). 핵심 흐름인 `Access`(sonic_boom.go:231)과 `Response`(:665)는 0%로, e2e 회귀 감지가 in-memory smoke 하나에만 의존한다. `sonic_boom_test.go`의 gotests 스켈레톤 13개 테스트 중 다수가 빈 케이스 테이블로 0회 실행 후 통과한다(예: `Test_overwritableHeader`는 통과하는데 함수 커버리지 0%). smoke 스크립트 3종은 동일 로직 복제이고 첫 요청 Miss 단언이 없어 남아있는 캐시가 회귀를 가린다. CI smoke 잡은 amd64에서 부러진다(`go build -o sonic-boom ./cmd`인데 `cmd/`가 비었고, kong 볼륨이 `./bin/linux-arm64`로 하드코딩). e2e 업스트림이 외부 reqres.in을 향해 있어 네트워크·레이트리밋에 좌우된다.

### Requirements

- **R1**: 세 캐시 전략(in-memory, redis, redis-cluster) 모두 로컬 docker compose로 Kong을 실기동해 e2e 검증된다.
- **R2**: 단위 테스트가 순수 로직(필터·헤더 판정·설정 검증)을 우선 커버하고, 단위로 못 덮는 PDK 의존 영역(Access/Response/시그널)은 통합·e2e가 대신 검증한다 — 두 층이 겹침 없이 상호보완된다.
- **R3**: 빈 테이블 스켈레톤 테스트가 사라진다(채워지거나 삭제되어 통과 착시가 없어진다).
- **R4**: 로컬과 CI가 같은 진입점(Makefile 타깃)을 공유한다.
- **R5**: e2e가 외부 인터넷 의존 없이 결정론적으로 돈다(hermetic).
- **R6**: 통합·e2e 테스트는 필요한 서버가 없으면 스킵이 아니라 실패한다.

### Key Decisions

- **KD1**: e2e 실행 환경은 docker compose 단일 경로다. brew 혼합 경로는 만들지 않는다. (Governs R1, R5)
- **KD2**: 테스트 레이어링은 "단위 우선, 통합·e2e가 빈틈 보완" 원칙을 따른다. (Governs R2, R3)
- **KD3**: 서버 미기동 시 통합 테스트는 실패 처리한다(skip 가드를 만들지 않는다). (Governs R6)

### Success Criteria

- `make e2e`가 맥 로컬(Apple Silicon)에서 네트워크 차단 상태에서도 통과한다.
- CI가 세 전략 smoke(in-memory, redis, redis-cluster)를 모두 실행하고 통과한다.
- `grep -c "TODO: Add test cases." internal/*_test.go` 결과가 0이다.
- 순수 함수 단위 테스트 커버리지: `overwritableHeader`, `headerToDelete`, `cacheableResponse*`, `checkConfig`가 각 80% 이상이 된다(커버리지 게이트를 다는 게 아니라 목표치).

### Scope Boundaries

**범위 밖(후속 작업):**
- `Access`/`Response` 함수의 단위 테스트화(pdk.PDK 목 레이어 도입) — e2e가 대신 검증하므로 별도 후속
- 벤치마크 체계 확장(`sonic_boom_bench_test.go`는 존재 유지만)
- 커버리지 퍼센트 CI 게이트 도입
- 운영 배포/릴리스 파이프라인 변경

## Planning Contract

### Key Technical Decisions

- **KTD1**: e2e 실행 환경은 docker compose 단일 경로 (session-settled: user-directed — chosen over brew Kong 혼합 경로: redis-cluster가 이미 compose에 정의돼 있어 경로가 하나일 때 유지비가 가장 낮다)
- **KTD2**: 3층 테스트 레이어링 — 단위(순수 함수, `go test` 어디서나) → 통합(go test + compose 백엔드) → e2e(compose 전체 + Kong + HTTP). 층별 검증 대상: 단위=필터·헤더·설정 판정, 통합=캐시 매니저·백엔드 TTL/격리, e2e=X-Cache-Status 흐름·시그널·Kong 연동. (session-settled: user-directed — chosen over e2e-only 집중: 사용자가 "단위테스트를 먼저 보강하고 모자란 걸 통합테스트로" 지시)
- **KTD3**: fail-fast — 통합·e2e는 서버 없으면 실패. skip 가드(t.Skip), 빌드 태그 분리 없이 `go test ./...`가 전체를 실행한다. 빠른 단위 반복은 기존 관례(`go test ./internal -run Test_InMemory`)를 따른다. (session-settled: user-directed — chosen over skip 가드: 진짜 실패를 스킵이 가리는 것을 방지)
- **KTD4**: 플러그인 바이너리는 기존 `build.sh`(GOOS/GOARCH 크로스컴파일 → `bin/linux-${GOARCH}/sonic-boom`)를 그대로 쓰고, docker-compose kong 볼륨의 하드코딩 `./bin/linux-arm64`를 compose 변수 치환(`./bin/linux-${GOARCH:-arm64}`)으로 바꿔 amd64 CI에서도 동작하게 한다. 새 메커니즘을 만들지 않는다.
- **KTD5**: e2e 업스트림을 compose에 정적 목업 서비스(nginx:alpine + 고정 JSON 파일)로 두고 `config_v1.yml`의 service host를 reqres.in에서 목업으로 교체한다. config_v1.yml은 compose 전용 declarative config라 운영 영향이 없다.
- **KTD6**: smoke 스크립트 3중 복제를 공통 라이브러리(`scripts/smoke_lib.sh`)로 통합하고 다음을 단언에 추가한다: ① 첫 요청 `Miss` 단언(기존은 두 번째 요청 Hit만 검사), ② 실행마다 캐시 초기화(compose down -v → up으로 clean state), ③ 부정 케이스(캐시 불가 조건에서 Hit되지 않음), ④ 캐시 시그널(무효화) 케이스 — 시그널 헤더명은 구현 시 `internal/cache_signal.go`·`sonic_boom.go`의 signalCacheReq 처리부에서 확인해 사용한다(플랜 확정 사항 아님). 기존 3개 스크립트 파일명은 thin wrapper로 유지해 README·호환을 지킨다.
- **KTD7**: Makefile을 새로 만들어 진입점을 통일한다: `make test`(compose 백엔드 기동 → `go test ./... -race -count=1` → 정리), `make e2e`(빌드 → compose up → smoke 3종 → down -v), `make build`(build.sh 위임). CI 잡도 같은 타깃을 호출한다.

### Assumptions

(해당 없음 — 인터랙티브 실행이며 스코핑 콜아웃 3건 모두 사용자가 확정했다)

## High-Level Technical Design

```text
[단위]  go test (서버 불필요)         필터·헤더 판정·설정 검증·캐시 키
          │ 남는 것: PDK 의존 흐름
          ▼
[통합]  go test + compose(redis,     캐시 매니저 3종·TTL·격리·연결 실패
        redis-cluster)                ← 이미 존재, fail-fast 유지
          │ 남는 것: Kong 플러그인 라이프사이클
          ▼
[e2e]   compose 전체 + Kong + 목업     Miss→Hit·부정 케이스·시그널 무효화
        업스트림 (make e2e, CI smoke)
```

## Implementation Units

### U1. 유령 스켈레톤 정리 + 순수 함수 단위 테스트 보강

**Goal:** R2, R3 — 빈 테이블 테스트를 제거하고 순수 로직을 단위 테스트로 채운다.

**Dependencies:** 없음 (첫 작업)

**Files:**
- `internal/sonic_boom_test.go` — 대량 수정
- `internal/kong_ctx_ex_test.go`, `internal/log_test.go` — 소규모 보강

**Approach:**
1. 채울 수 있는 것은 케이스를 채운다: `Test_overwritableHeader`(Set-Cookie/WWW-Authenticate 등 덮어쓰기 헤더 목록), `TestConfig_cacheableResponse`/`ResponseStatus`/`ResponseContentType`(status·content-type 조합), `headerToDelete`(현재 테스트 없음 — 신규), `checkConfig`(전략별 필수값 누락 → 에러), `GetPluginFloat`, `newRollingFileWriter`
2. pdk.PDK 인스턴스가 필요해 단위로 못 채우는 것은 삭제하고 e2e 이관을 코멘트로 남긴다: `TestConfig_Access`, `TestConfig_Response`, `Test_serviceResponseRawBody`
3. `TODO: Add test cases.` 마커가 테스트 파일에 남지 않게 한다

**Test scenarios (단위):**
- `overwritableHeader`: 덮어쓰기 대상 헤더 → true, 아닌 헤더 → false 경계값
- `cacheableResponseStatus`: 설정된 코드(200/301/404) → true, 미설정(500) → false
- `checkConfig`: strategy 누락·redis host 누락 등 → 에러 리턴 (fail-fast 준수)
- `convertRedisTimeout`: 0 → 기본값, 양수 → 초 변환

**Execution note:** 단위 우선 원칙(KTD2)에 따라 이 유닛을 먼저 끝내고 다음 유닛으로 간다.

### U2. Hermetic e2e 환경 (목업 업스트림 + compose arch 일반화)

**Goal:** R1, R5 — e2e가 외부 인터넷 없이 결정론적으로 돌게 한다. amd64에서도 compose가 동작하게 한다.

**Dependencies:** 없음 (U1과 병렬 가능)

**Files:**
- `docker-compose.yaml` — 목업 서비스 추가, kong 볼륨 `${GOARCH:-arm64}` 치환
- `config_v1.yml` — 3개 service host를 reqres.in → 목업 서비스명으로 교체
- `test/upstream/` (또는 유사 경로) — 고정 JSON 응답 파일

**Approach:**
1. compose에 `mock-upstream`(nginx:alpine, 포트는 내부망만) 서비스 추가. `/api/users`가 고정 JSON을 반환하게 정적 파일 마운트
2. `config_v1.yml`의 services 3개(in-memory, redis, redis-cluster 라우트용) host를 `mock-upstream`으로 교체. reqres.in 문자열이 config에서 사라지는 것으로 hermetic 확인
3. kong 볼륨 마운트를 `./bin/linux-${GOARCH:-arm64}:/kong/go-plugins`로 바꾼다(KTD4)

**Test scenarios:** `docker compose config`로 렌더링 검증; `docker compose down -v && up -d` 후 `curl :8000/<route>/api/users`가 목업 JSON을 반환.

### U3. Smoke 스크립트 통합·강화

**Goal:** R1, R2(e2e 몫) — Miss→Hit 단언, 부정 케이스, 시그널 무효화를 검증한다.

**Dependencies:** U2

**Files:**
- `scripts/smoke_lib.sh` — 신규 공통 라이브러리
- `scripts/smoke_in_memory.sh`, `scripts/smoke_redis.sh`, `scripts/smoke_rediscluster.sh` — thin wrapper로 교체

**Approach:**
1. 공통 검증 순서(순서 보장): 서버 준비 대기 → ①첫 요청 `Miss` 단언 → ②재요청 `Hit` 단언 → ③캐시 불가 조건(예: 미허용 메서드) 요청이 Hit로 바뀌지 않는지 → ④시그널 헤더 전송 후 재요청이 `Miss`로 돌아오는지(무효화)
2. 각 전략 wrapper는 URL prefix와 시그널 헤더명만 넘긴다
3. 대기 로직은 기존 CI의 port/cluster_state 확인 패턴을 재사용한다

**Execution note:** ④의 시그널 헤더명은 `internal/cache_signal.go`와 `signalCacheReq`(`sonic_boom.go:616`) 호출부에서 실제 이름을 확인한 뒤 확정한다. e2e가 시그널 경로(현재 0% 커버리지)를 대신 검증하는 상호보완(KTD2)의 핵심 케이스다.

**Test scenarios:** 각 전략에서 ①~④ 전부 통과; Kong 미기동 시 스크립트가 즉시 실패(타임아웃 대기 후 비명료 종료가 아니라 명확한 에러 메시지).

### U4. Makefile 진입점

**Goal:** R4 — 로컬·CI 동일 진입점.

**Dependencies:** U2, U3

**Files:** `Makefile` — 신규

**Approach:**
1. `make build` → `./build.sh` 위임(플랫폼 감지는 build.sh 기존 로직)
2. `make test` → `docker compose up -d redis redis-cluster` → 준비 대기 → `go test ./... -race -count=1` → `docker compose down`(redis 서비스만)
3. `make e2e` → `make build` → `docker compose down -v` → `up -d --build` → smoke 3종 순차 실행 → `down -v`
4. 타깃은 실패 시 중간 상태를 남기지 않게 `down`을 always-정리로 건다

**Test scenarios:** 맥에서 `make e2e` 1회 통과; 백엔드 없이 `make test` 실행 시 redis 테스트가 실패(fail-fast, KTD3).

### U5. CI 수정

**Goal:** R1, R4 — CI가 세 전략 e2e를 실행한다. 현재 부러진 smoke 잡을 고친다.

**Dependencies:** U3, U4

**Files:**
- `.github/workflows/ci.yml` — smoke-test 잡 재작성, test 잡 정리

**Approach:**
1. smoke-test 잡: `go build -o sonic-boom ./cmd`(빈 `cmd/`로 실패) 제거 → `make e2e` 호출로 교체. amd64 러너에서 `GOARCH=amd64` 전달
2. test 잡: 현재 인라인 bash(redis-cluster 기동·대기 로직)을 `make test` 호출로 축약 — 논리는 Makefile로 이동해 중복 제거
3. CI의 대기·헬스체크 로직은 Makefile과 smoke_lib.sh로 흡수되므로 워크플로우에는 타깃 호출만 남긴다

**Test scenarios:** PR에서 test·smoke-test 잡 모두 초록; smoke-test 잡이 세 전략 smoke를 실행한 로그 확인.

### U6. 문서 갱신

**Goal:** R4 — 문서가 실제 구조와 일치한다(docs/testing.md는 현재 smoke 잡을 "In-memory만"이라 설명하는 등 드리프트 존재).

**Dependencies:** U1–U5

**Files:** `docs/testing.md`, `README.md`

**Approach:** 3층 레이어링(단위/통합/e2e) 설명, make 타깃 안내, 시그널 케이스·hermetic 구성(목업 업스트림) 반영. 코드블록에 언어 지정(````bash`).

**Test scenarios:** 문서의 모든 명령이 실제 동작(복붙 가능).

## Verification Contract

- **단위:** `go test ./internal -run 'TestConfig|Test_overwritable|Test_generate' -v` — 채운 케이스 통과
- **전체(통합 포함):** `make test` — redis·redis-cluster compose 기동 후 `go test ./... -race -count=1` 통과
- **e2e:** `make e2e` — 세 전략 ① Miss ② Hit ③ 부정 ④ 시그널 케이스 통과
- **hermetic 증명:** 맥에서 인터넷을 끈 상태(또는 reqres.in 차단)로 `make e2e` 통과; `grep reqres.in config_v1.yml` 결과 0건
- **CI:** PR 오픈 시 test·smoke-test 잡 초록
- **품질 게이트:** `grep -c "TODO: Add test cases." internal/*_test.go` 0건; `go tool cover -func`에서 U1 대상 함수 각 80%+

## Definition of Done

- 전 유닛 커밋 완료(Conventional Commits, 한국어) 후 main에서 CI 초록
- 유령 테스트 잔존 없음(grep 검증)
- 이 플랜을 `docs/plans/`로 복사·보관
- 후속 작업(Access/Response 단위화 = pdk 목 레이어)을 `#### Deferred to Follow-Up Work`에 남김 — 본 플랜 Scope Boundaries에 이미 명시됨

## Appendix

### 세션에서 실측한 근거 (2026-10-18)

- 커버리지: total 32.1%, `Access` 0%, `Response` 0%, `signalCacheReq/WithStatus` 0%, `overwritableHeader` 0%, `headerToDelete` 0%, `checkConfig` 0%, `cacheableRequest` 57.1%, `newCacheManager` 96.3%
- `sonic_boom_test.go`: Test 함수 13개 중 빈 케이스 테이블(TODO 마커) 다수 — 통과 착시
- smoke 3종: 동일 로직 복제, `status1`(첫 요청) 검사 없음
- CI smoke 잡: `go build -o sonic-boom ./cmd` — `cmd/` 비어 실패; kong 볼륨 `./bin/linux-arm64` 하드코딩(amd64 불일치)
- `build.sh`: GOOS/GOARCH 크로스컴파일 → `bin/${GOOS}-${GOARCH}/sonic-boom` (기존 자산, 재사용)
- `config_v1.yml`: 업스트림 = reqres.in (외부 의존)
- Dockerfile: `COPY bin/${TARGETOS}-${TARGETARCH}/sonic-boom` — 다중 아키텍처 이미지는 build.yml이 이미 생산 중
