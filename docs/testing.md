# Testing Guide

sonic-boom의 테스트는 단위 → 통합 → e2e 3층 구조다. 각 층은 서로를 보완한다:
단위 테스트가 순수 로직을 먼저 검증하고, 단위로 검증할 수 없는 영역(PDK 의존 흐름)을
통합·e2e가 대신 검증한다. 서버가 필요한 테스트는 서버가 없으면 **스킵이 아니라 실패**한다(fail-fast).

## 3층 구조

| 층 | 무엇을 검증 | 실행 방법 | 서버 필요 |
|---|---|---|---|
| 단위 | 필터·헤더 판정(`cacheable*`), 설정 검증(`checkConfig`), 캐시 키, 타임아웃 변환, in-memory 캐시 | `go test ./internal -run 'Test_InMemory\|TestConfig\|Test_'` | 없음 |
| 통합 | 캐시 매니저 3종의 실제 백엔드 동작 — put/get, TTL, 동시성, 격리, 연결 실패 | `make test` | redis, redis-cluster (compose) |
| e2e | Kong 게이트웨이까지 포함한 캐시 흐름 — Miss→Hit, Bypass, vary 헤더 키 분리 | `make e2e` | compose 전체(mock-upstream 포함) |

`Access`/`Response`처럼 `pdk.PDK` 오케스트레이션이 필요한 흐름은 단위 테스트로 다루지
않고 e2e가 검증한다(상호보완). 캐시 시그널(`CacheSignal`)은 Access→Response 간 내부
ctx 핸드오프라 클라이언트가 직접 호출하는 경로가 아니며, e2e의 Miss→Hit 단언으로
함께 검증된다.

## make 진입점 (로컬·CI 동일)

```bash
make test   # compose 백엔드(redis, redis-cluster) 기동 → go test ./... -race 실행 → 정리
make e2e    # 플러그인 빌드 → compose 전체 clean 기동(down -v) → 세 전략 스모크 → 정리
make build  # build.sh 위임, bin/linux-${GOARCH}/sonic-boom 크로스컴파일
```

- 통합 테스트의 `go test`는 compose 네트워크 안의 golang 컨테이너(`go-test` 서비스,
  `test` 프로파일)에서 실행된다. 맥/리눅스/CI가 같은 방식으로 redis·cluster에 접근한다.
- redis/redis-cluster는 호스트에 포트를 노출하지 않는다. 맥의 AirPlay(7000 포트) 같은
  충돌도 없다.
- CI(`.github/workflows/ci.yml`)의 test 잡은 `make test`, smoke-test 잡은
  `make e2e GOARCH=amd64`를 호출한다.

## e2e 구성 (hermetic)

e2e는 외부 인터넷 없이 결정론적으로 돈다.

- **목업 업스트림**: `test/upstream/`(nginx)이 모든 경로에 고정 JSON을 반환한다.
  `config_v1.yml`의 모든 service host는 `mock-upstream`을 가리킨다(reqres.in 의존 제거).
- **클러스터 announce**: redis-cluster는 고정 IP `172.30.0.10`을 announce한다.
  kong 컨테이너가 MOVED 리다이렉트를 따라가려면 announce 주소가 compose 네트워크에서
  라우팅 가능해야 하기 때문이다(`IP=0.0.0.0`이면 kong에서 자기 자신을 가리켜 실패).
- **clean state**: `make e2e`는 시작 전 `docker compose down -v`로 캐시를 비운다.
  첫 요청 Miss 단언의 전제 조건이다.

## 스모크 검증 항목 (`scripts/smoke_lib.sh`)

각 전략(in-memory, redis, redis-cluster) 공통 6단언:

1. 첫 요청 `X-Cache-Status: Miss` — 남아 있는 캐시가 회귀를 가리는지 즉시 드러난다
2. 같은 키 재요청 `Hit`
3. 캐시 불가 메서드(DELETE) `Bypass`
4. vary 헤더: `Authorization` 자격증명별 첫 요청 `Miss`
5. 같은 자격증명 재요청 `Hit`
6. 다른 자격증명은 다른 캐시 키로 `Miss`

개별 실행: `./scripts/smoke_in_memory.sh`, `./scripts/smoke_redis.sh`,
`./scripts/smoke_rediscluster.sh` (Kong이 떠 있고 캐시가 비어 있어야 한다).

## 알려진 특성

- **ristretto 비동기 쓰기**: `Set()` 직후 `Get()`하면 miss일 수 있다. 테스트는
  짧은 대기 또는 기한 있는 리트라이로 대응한다(예: `Test_InMemory_TTL_Expires`).
- **InMemoryConfig별 store 공유**: `newCacheManager`는 설정값을 키로 store를
  `LoadOrStore` 공유하므로, 먼저 생성된 store의 TTL이 이후 호출에 고정된다.
  TTL을 검증하는 테스트는 고유 설정으로 자기 store를 만든다.
- **redis store의 Get 타입**: redis/redis-cluster store는 go-redis `Result()`의
  `string`을 반환한다([]byte 왕복이 아님 — in-memory만 []byte를 보존).

## 새 캐시 전략 추가 시

1. `internal/sonic_boom_{strategy}_test.go` — put/get, TTL, 동시성, marshal, 연결 실패 패턴
2. `config_v1.yml` — 전략용 service·route 정의
3. `scripts/smoke_{strategy}.sh` — `smoke_lib.sh`를 라우트 prefix로 호출하는 thin wrapper
4. `Makefile` e2e 타깃에 스크립트 추가
