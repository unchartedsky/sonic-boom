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
make e2e-lua    # Kong-Lua(kong-lua, :8010) 3전략 스모크 + selfcheck
make e2e-apisix # APISIX 어댑터(apisix, :8020) — 같은 스모크 3종을 GW=apisix로
make stress # 동접 스트레스 시나리오 — 스탬피드(fill-lock on/off)·CSC on/off 실측 + 단언(기대 미달 시 exit 1)
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
- **클러스터 구성**: 고정 태그 `redis:8.10.2-alpine` 노드 3개(고정 IP `172.30.0.11-13`)와
  `cluster-init` 컨테이너가 클러스터를 생성한다(멀티아치라 양쪽 아치 네이티브).
  각 노드는 자기 고정 IP를 announce한다 — kong 컨테이너가 MOVED 리다이렉트를
  compose 네트워크 안에서 따라갈 수 있어야 하기 때문이다.
- **clean state**: `make e2e`는 시작 전 `docker compose down -v`로 캐시를 비운다.
  첫 요청 Miss 단언의 전제 조건이다.

## 스모크 검증 항목 (`scripts/smoke_lib.sh`)

각 전략(in-memory, redis, redis-cluster) 공통 14단언 + gzip suite(characterization):

1. 첫 요청 `X-Cache-Status: Miss` — 남아 있는 캐시가 회귀를 가리는지 즉시 드러난다
2. 같은 키 재요청 `Hit`
3. 캐시 불가 메서드(DELETE) `Bypass`
3-1. Set-Cookie 응답 `Bypass`(재요청 포함 2회) — 저장 게이트가 미스 시그널을 저장하지 않고 Bypass로 되돌린다
3-2. `Cache-Control: private/no-store/no-cache` 응답 `Bypass`(디렉티브별 2회) — 양 게이트웨이 공통. 픽스처는 `/ccprivate`·`/ccnostore`·`/ccnocache`로 분리돼 게이트를 격리한다
4. vary 헤더: `Authorization` 자격증명별 첫 요청 `Miss`
5. 같은 자격증명 재요청 `Hit`
6. 다른 자격증명은 다른 캐시 키로 `Miss`

gzip suite(`test/upstream/nginx.conf`가 유일한 압축 지점 — `gzip_proxied any` 포함):

- gzip 응답: `Accept-Encoding: gzip` 첫 요청 Miss → 재요청 Hit, `Content-Encoding: gzip`
  존재, `--compressed` 해제 본문이 원문과 바이트 일치, Content-Length는 raw gzip 바이트 수와 일치
- 교차 오염: Accept-Encoding 없는 재요청이 같은 키 Hit — gzip 엔트리가 그대로 재생되는
  현재 동작을 바이트 수준으로 고정(전략 기본 라우트에는 Accept-Encoding vary가 없다)
- gzip×vary 키 분리: 전략별 `-vary` 복제 라우트(`config_v1.yml`)에서 gzip 키와
  identity(헤더 없음/`identity` 명시) 키가 분리된다
- HEAD: 캐시 키에 Method가 포함돼 GET 키와 분리(Miss), nil body 캐싱 왕복(재요청 Hit)
- gzip POST 바디: 동일 압축 바디 2회는 같은 키 Hit, 비압축 동일 의미 바디는 다른 키
- deflate·br: 사전 압축 픽스처(test/upstream/response.{deflate,br})를 해당
  Content-Encoding으로 내주는 정적 location 기반. 재생 바이트 == 픽스처 바이트,
  Content-Encoding 보존, 교차 오염 재생, vary 라우트에서 gzip 키와의 분리를 단언.
  플러그인이 본문을 불투명 바이트로 취급함을 세 인코딩에서 공통으로 고정한다.

개별 실행: `./scripts/smoke_in_memory.sh`, `./scripts/smoke_redis.sh`,
`./scripts/smoke_rediscluster.sh` (Kong이 떠 있고 캐시가 비어 있어야 한다).

## 스트레스 시나리오 (`scripts/stress_scenarios.sh`, `make stress`)

지표는 업스트림(mock-upstream nginx 접근 로그) 호출 수와 p50/p95/p99·rps다.

- **스탬피드**: 900KB 응답을 `limit_rate 100k`로 흘려 약 8초의 미스 창을 만들고
  동시 버스트(`xargs -P` curl). fill-lock 끔(`/redis-nolock`)은 전원 업스트림,
  켬(`/redis-stampede`, wait 15s)은 1건만 fill하고 나머지는 채워진 값으로 서빙된다.
- **CSC**: 같은 조건(db 0)에서 CSC off(`/redis-nocsc`)·on(`/redis-csc`)의 warm 비교.
  compose redis가 근거리라 이득이 작게 나오며, redis RTT가 큰 환경일수록 벌어진다.
- **도구 주의(실측)**: ab는 같은 키 동시 버스트를 순차적으로 풀어 스탬피드를
  만들지 못한다(3@3에도 업스트림 1). 버스트는 xargs -P curl, warm 처리량은 ab.
- **자동 단언**: 락 켬 ≤ 2, 락 끔 ≥ 동시성 90%, warm 업스트림 0 — 하나라도
  미달이면 exit 1. 경계값은 200 동시 실측(2026-10-19) 기반이라 타이밍 여유가 좁다;
  반복 FAIL이면 실측을 다시 보고 경계를 조정한다. 업스트림 집계는
  `docker compose ps -q mock-upstream`으로 컨테이너를 찾는다(프로젝트명 무관).

## 알려진 특성

- **ristretto 비동기 쓰기**: `Set()` 직후 `Get()`하면 miss일 수 있다. 테스트는
  짧은 대기 또는 기한 있는 리트라이로 대응한다(예: `Test_InMemory_TTL_Expires`).
- **InMemoryConfig별 store 공유**: `newCacheManager`는 설정값을 키로 store를
  `LoadOrStore` 공유하므로, 먼저 생성된 store의 TTL이 이후 호출에 고정된다.
  TTL을 검증하는 테스트는 고유 설정으로 자기 store를 만든다.
- **redis store의 Get 타입**: redis/redis-cluster store는 go-redis `Result()`의
  `string`을 반환한다([]byte 왕복이 아님 — in-memory만 []byte를 보존).

## 새 캐시 전략 추가 시

세 게이트웨이 공통(누락 시 해당 게이트웨이 e2e에서 간헐 실패):

1. `internal/sonic_boom_{strategy}_test.go` — put/get, TTL, 동시성, marshal, 연결 실패 패턴
2. `config_v1.yml`·`config_lua.yml`·`config_apisix.yaml` — 전략용 service·route 정의(3종 모두, 라우트 prefix 통일)
3. `scripts/smoke_{strategy}.sh` — `smoke_lib.sh`를 라우트 prefix로 호출(Go는 4줄 wrapper, Lua는 `common.sh`의 `lua_smoke` 헬퍼 경유)
4. `Makefile` e2e 타깃(`e2e_gateway` define)에 스크립트 추가 — `scripts/common.sh`의 `gw_env`가 GW·PROFILE·SVC·PORT를 계산한다
5. 대조군 라우트(fill_lock off)를 스탬피드 게이트 검증용으로 함께 추가한다면 위 4곳 + `assert_burst_le/ge` 배선까지

## CI 잡 경계 — 게이트웨이별 분리(APISIX 확장 규약)

CI 병렬화의 단위는 **게이트웨이 × 아치**다. 한 러너에서 게이트웨이 e2e를
같이 돌리면 포트(8000/8010)와 서브넷(172.30.0.0/24)이 충돌하므로, 게이트웨이별로
잡을 나눠 러너를 분리한다(`smoke-test` / `smoke-test-lua`). 재생 비교(`parity`)는
양쪽 게이트웨이가 필요하므로 자체 잡에서 둘 다 띄운다.

APISIX도 같은 규약을 따른다: `container-apisix`(이미지)·`smoke-test-apisix`
(e2e)·`parity-lua-apisix`(코어 공유라 값싼 쌍) 잡이 같은 모양으로 붙어 있다.
게이트웨이별 compose profile(`lua`, `apisix`)과 `make e2e-<gateway>` 타깃이 이 경계의
로컬 대응물이다. 호스트 포트는 Go kong 8000, kong-lua 8010, apisix 8020이다.

두 Lua 게이트웨이는 같은 코어와 같은 `/lua-*` 라우트 경로를 쓴다. 그래서 스크립트는 게이트웨이를
변수로만 고른다.

- **스모크:** `scripts/smoke_lua_*.sh`는 `GW=kong-lua`(기본)|`apisix`로 대상 서비스·프로필·베이스 URL을
  바꾼다(`scripts/common.sh`의 `gw_env`). 키 접두는 `sbl`/`sba`로 단언한다.
- **재생 비교:** `scripts/parity_replay.sh`는 `PARITY_PAIR=go-lua`(기본)|`lua-apisix`로 쌍을 고른다.
  lua-apisix에서는 Go 전용 시나리오(S13)를 건너뛴다.
- **벤치:** `scripts/bench_gw.sh`는 `BENCH_PAIR=go-lua`(기본)|`lua-apisix`|`all5`다. lua-apisix는 각자의
  `/noplugins` 대비 오버헤드 비율로 판정한다(`docs/bench-apisix-vs-kong-lua.md`). all5는 우리 구현 3종과
  Kong·APISIX 내장 `proxy-cache`를 같은 회차에 잰다(`docs/bench-five-caches.md`).
