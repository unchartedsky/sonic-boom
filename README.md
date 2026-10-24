# Sonic Boom

![CI](https://github.com/unchartedsky/sonic-boom/workflows/CI/badge.svg)

Kong Gateway 3.9.x OSS용 응답 캐시 플러그인. in-memory·redis·redis-cluster 세 캐시 전략을 지원하고, 두 구현이 같은 코어 계약을 공유한다.

| 변종 | 구현 | 위치 | 판정 자료 |
|------|------|------|-----------|
| Go (Kong) | go-pdk external plugin 서버 | `main.go` + `internal/` | — |
| Kong-Lua | 게이트웨이 중립 코어 + Kong 어댑터 | `lua/sonic_boom/` + `lua/kong/` | [docs/kong-lua-verdict.md](./docs/kong-lua-verdict.md) |
| APISIX-Lua | 같은 코어 + APISIX 어댑터 | `lua/sonic_boom/` + `lua/apisix/` | [docs/apisix-verdict.md](./docs/apisix-verdict.md) |

필터·저장 게이트·fill-lock 같은 제품 판정은 전부 코어(`lua/sonic_boom/flow.lua`, Go는 `internal/`)에 있고, 어댑터는 PDK 게터·세터와 단락 방식만 담당한다.

## 빠른 시작

```bash
make test        # 통합 테스트 (compose로 redis·redis-cluster 기동 후 go test -race)
make e2e         # Go판 e2e — 세 캐시 전략 스모크
make e2e-lua     # Kong-Lua e2e (포트 8010)
make e2e-apisix  # APISIX e2e (포트 8020)
make stress      # 스탬피드·CSC 실측 시나리오 + 자동 단언
```

e2e는 외부 인터넷 없이 도는다(목업 업스트림 `test/upstream/`). 재생 비교는 `PARITY_PAIR=go-lua|lua-apisix ./scripts/parity_replay.sh`.

`./run.sh`(air 핫리로드 개발 서버)와 `./test.sh`(레거시 curl 스모크)는 이전 세대 도구다.

## 빌드 및 설치

```bash
make build   # bin/linux-${GOARCH}/sonic-boom 크로스컴파일
```

Kong 설치는 선언형 설정 기준 [config_v1.yml](./config_v1.yml)을 참고한다. `config_v2.yml`은 reqres.in 시절 미사용 레거시다.

## 캐시 전략

### In-Memory
- **용도**: 단일 인스턴스, 고성능 캐시
- **특징**: Ristretto 기반, 비동기 쓰기, TTL 지원
- **설정**: `strategy: in-memory`

### Redis
- **용도**: 분산 환경, 지속성 필요
- **특징**: 네트워크 기반, 클러스터 지원
- **설정**: `strategy: redis`

### Redis Cluster
- **용도**: 대규모 분산 환경
- **특징**: 자동 샤딩, 고가용성
- **설정**: `strategy: redis-cluster`

## 캐시 동작 규칙

- `filters`의 필터별 `cache_ttl`이 실제 만료에 적용된다. 필터 TTL을 0으로 두거나 생략하면 전역 `cache_ttl`을 **상속**한다(즉시 만료 필터는 만들 수 없다).
- `content_type`은 대소문자 무시 정확 매치다. 파라미터 변형(`application/json;charset=utf-8` 등)도 목록에 명시해야 캐시된다.
- `Set-Cookie` 응답이나 `Cache-Control: private/no-store/no-cache` 응답은 저장하지 않는다(개인화 응답 오염 방지 — APISIX proxy-cache 기본값 정합).
- `fill_lock`(기본 on)은 동시 미스 시 한 요청만 업스트림을 채우게 한다. 패자는 승자의 저장을 폴링 대기하고 승자가 락을 반납하면 즉시 통과한다. TTL(기본 3000ms)은 락의 안전망이다.
- 설정 검증은 운영에서도 상시 실행되며, 실패 시 프로세스를 죽이지 않고 `X-Cache-Status: Bypass`로 통과한다.

**캐시 키와 세대**: Go판은 요청 성분을 값 단위 길이 접두 직렬화한 뒤 sha256 64-hex를 키로 쓰고(접두 없음), `cache_version`은 키에 없고 Hit 시점 비교로 불일치면 purge한다. 두 Lua 변종은 `{sbl|sba}:{cache_version}:{sha256}` 접두로 세대를 분리한다 — `cache_version`을 바꾸면 새 세대 키 공간으로 갈아탄다.

**관찰성**: `X-Cache-Status`는 `Miss`(저장 안 된 미스) / `Hit`(캐시 서빙) / `Bypass`(캐시 제외 — 필터 미매치·미저장 응답·검증 실패) / `Refresh`(유효기간 지난 항목 재수급) 4상태다. `X-Cache-Key`로 판정에 쓰인 키를 볼 수 있다.

## 설정 예시

```yaml
...
config:
    response_code:                  # 캐시 고려할 HTTP 응답 코드
        - 200
        - 301
        - 404
    request_method:                 # 캐시 고려할 HTTP 요청 메소드
        - GET
        - HEAD
        - POST
        - PUT
    content_type:                   # 캐시 고려할 Content-type 응답 헤더
        - "text/plain"
        - "text/html; charset=utf-8"
        - "application/json"
        - "application/json; charset=utf-8"
    vary_headers:
        - Authorization
    cache_ttl: 15                   # 캐시할 엔티티의 TTL 값. 기본 300. 0은 거절된다(Redis 영구 키 방지)
    cacheable_body_max_size: 100000 # 캐시 고려할 최대 허용 Body 길이
    strategy: redis                 # 캐시 방식
    fill_lock:                      # 캐시 스탬피드 완화(동시 미스 시 1건만 업스트림). 기본 켜짐
        enabled: true               # 끄면 동시 미스 전원이 업스트림로 간다
        ttl_ms: 3000                # fill 주도권 락 TTL. 승자 장애 시 자동 해제 상한
        wait_ms: 200                # 패자가 채워지기를 기다리는 상한. 0이면 대기 없이 통과
    redis:
        host: redis                 # Go판은 required(미선언 시 전 요청 Bypass). Kong-Lua는 기본값 localhost
        port: 6379                  # 기본값 6379
        pool_size: 10               # 기본값 10
        max_retries: 3              # 재시도 횟수. 기본값 3. 끄려면 -1
        db_number: 0                # 기본값 0. client_side_cache 활성 시 0이어야 한다
        client_side_cache:          # 로컬 L1(go-redis 실험 기능). 기본 끔. standalone 전용
            enabled: false
            max_entries: 0          # 0이면 라이브러리 기본 상한
            max_memory_bytes: 0     # 0이면 무제한(항목 수 상한 우선)
            max_staleness_ms: 0     # 캐시 TTL 이하 권장 — 신선도 계약 보호
        min_retry_backoff_ms: 8     # 단위는 ms. 기본값 8
        max_retry_backoff_ms: 512   # 단위는 ms. 기본값 512
        dial_timeout: 5             # 단위는 초(s)
        read_timeout: 3             # 단위는 초(s)
        write_timeout: 3            # 단위는 초(s)
        pool_timeout: 5             # 단위는 초(s)
        idle_timeout: 1             # 단위는 초(s)
...
```

Lua판(Kong·APISIX)의 설정 스키마는 Go판과 대부분 같지만 차이가 있다: `cache_control` 옵트인은 Go판 전용이고, `redis.host`는 Kong-Lua에서 기본값 localhost(APISIX YAML 배치는 런타임 fail-open)다. 전체 명시 차이는 [docs/kong-lua-verdict.md](./docs/kong-lua-verdict.md)의 명시 차이 목록을 본다.

## 테스트

단위 → 통합 → e2e 3층 구조. 서버가 필요한 테스트는 compose로 띄우며,
서버가 없으면 실패한다(fail-fast).

```bash
# 통합 테스트 (compose가 redis·redis-cluster를 띄우고 go test -race 실행)
make test

# e2e — 게이트웨이별로 잡이 분리된다(CI 규약: docs/testing.md "CI 잡 경계")
make e2e          # Go판
make e2e-lua      # Kong-Lua
make e2e-apisix   # APISIX

# 재생 비교 — 두 게이트웨이에 같은 요청 묶음을 재생해 상태·status·body 일치를 증명
PARITY_PAIR=go-lua ./scripts/parity_replay.sh

# 서버 없이 돌아가는 단위 테스트만 빠르게
go test ./internal -run 'Test_InMemory|TestConfig|Test_'
```

테스트 가이드는 [docs/testing.md](./docs/testing.md), 최근 검증 결과는 [docs/verification-2026-10-24.md](./docs/verification-2026-10-24.md), 벤치 기록은 [docs/bench-kong-lua-vs-go.md](./docs/bench-kong-lua-vs-go.md)·[docs/bench-apisix-vs-kong-lua.md](./docs/bench-apisix-vs-kong-lua.md)를 본다.

## 개발

### 필수 요구사항
- Go 1.27+
- Docker & Docker Compose

### 개발 워크플로우
1. 코드 변경
2. 통합 테스트: `make test`
3. e2e: `make e2e`
4. 커밋 및 푸시

## TODO

- [x] `linux/arm64` 컨테이너 이미지 지원 ✅ 2025-02-17
- [x] 바이너리 릴리즈 ✅ 2025-02-17
- [x] in-memory 스토어 지원 ✅ 2025-02-17
- [x] Redis cluster 스토어 지원 ✅ 2025-02-19
- [x] OpenTelemetry 통합 ✅ 2025-02-19
- [x] 포괄적인 테스트 커버리지 ✅ 2025-01-21
- [x] CI/CD 파이프라인 ✅ 2025-01-21
- [x] Lua 포팅(Kong)·APISIX 어댑터 — 같은 코어, 재생 비교 게이트 ✅ 2026-10-22
- [x] fill-lock 스탬피드 방어와 락 감시 대기 ✅ 2026-10-24
- [ ] Kubernetes 예제 추가
- [ ] Kong proxycache 의 [`ignore_uri_case`](https://github.com/Kong/kong/blob/a4c0b461345d431067a2bfb7645434212eed7e5b/kong/plugins/proxy-cache/handler.lua#L247) 지원

## 참고

- [Feature request plugin instance lifecycle hooks · Issue 78 · Konggo-pdk](https://github.com/Kong/go-pdk/issues/78)
