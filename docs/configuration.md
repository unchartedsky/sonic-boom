# sonic-boom 설정 레퍼런스

전 필드의 기본값·단위·검증 규칙, 그리고 변종(Kong-Go·Kong-Lua·APISIX-Lua) 간 차이를 다룬다. 동작 원리는 [동작 방식](./how-it-works.md), 설치는 각 변종의 사용 문서가 담는다. 표의 모든 값은 스키마 정의 파일에서 읽었고 각 절 머리에 원천을 링크한다 — 이 문서와 소스가 어긋나면 소스가 옳다(그리고 이 문서의 버그다).

## 공통 필드

세 변종이 같은 기본값을 쓴다. Lua 스키마는 구현된 동작의 Go 필드만 선언하므로 이 표의 필드는 세 변종 모두에서 유효하다.

| 필드 | 타입 | 기본값 | 검증·비고 |
|------|------|--------|-----------|
| `response_code` | int 목록 | `[200, 301, 404]` | 저장 대상 응답 상태. 필수 |
| `request_method` | 문자열 목록 | `["GET", "HEAD"]` | 대소문자 무관 매치. 필수 |
| `content_type` | 문자열 목록 | `["text/plain", "application/json", "application/json; charset=utf-8"]` | 대소문자 무관 정확 매치. 필수 |
| `vary_headers` | 문자열 목록 | `[]` | 키에 반영할 요청 헤더 |
| `cache_ttl` | 초 | `300` | **0은 거부** — redis 저장소에서 영구 키가 되기 때문이다 |
| `cache_control` | bool | `true` | 요청측 가드(요청 `Cache-Control: no-store/no-cache`·`Authorization` 제외). `false`로 옵트아웃 |
| `credential_vary` | bool | `true` | consumer 미확정 `Authorization` 요청의 principal 키 반영. `false`로 옵트아웃 |
| `cacheable_body_max_size` | 바이트 | `0` (무제한) | `>` 경계로 초과 판정. 본문 캐시 라우트는 상한 필수 |
| `cache_version` | 문자열 | 빈 값 → 자동 채움 | Go는 `CACHE_VERSION` 환경변수 우선, 아니면 `2-1.0.4`. Lua는 설정값 > 환경변수 > 제품 버전 |
| `strategy` | 문자열 | `"redis"` | `redis` · `redis-cluster` · `in-memory` 중 하나. 설정 하나가 정확히 하나의 전략 |

설정 검증은 배포가 아니라 **요청마다** 실행된다. 실패해도 프로세스를 죽이지 않고 Error 로그와 함께 `Bypass`로 통과시킨다 — 잘못된 설정 하나가 게이트웨이 전체를 죽이지 않게 하려는 설계다.

## 전략별 하위 설정

### redis — 원천: internal/redis_config.go, lua/kong/plugins/sonic-boom-lua/schema.lua

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `host` | (없음 — Go는 필수) | Kong-Lua는 `localhost` 기본. 미선언 시 Go는 전 요청 `Bypass` |
| `port` | `6379` | |
| `db_number` | `0` | `client_side_cache` 활성 시 반드시 `0` |
| `username` / `password` | 빈 값 | |
| `pool_size` | `10` | |
| `max_retries` | `3` | `-1`이면 재시도 없음 |
| `min_retry_backoff_ms` / `max_retry_backoff_ms` | `8` / `512` | 밀리초. `-1`이면 비활성 |
| `dial_timeout` | `5` | 초 |
| `read_timeout` / `write_timeout` | `3` / `3` | 초 |
| `pool_timeout` | `5` | 초 |
| `idle_timeout` | `1` | 초 |

타임아웃 계열(Go)의 값 규칙: `0`은 무제한, `-1`은 비활성, 양수는 초 단위. `config_v1.yml`의 `dial_timeout: 0` 등이 무제한을 뜻하는 이유다.

Lua 스키마는 `max_retries`·retry backoff·`pool_timeout`을 선언하지 않는다 — 재시도 없는 fail-open으로 동작한다. 선언하지 않은 필드를 쓰면 선언형 로드가 거부된다(아래 "변종 차이"). Lua의 타임아웃은 1 이상만 받는다(0·-1 불가).

### redis-cluster — 원천: internal/redis_config.go

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `addrs` | (없음 — 필수) | 클러스터 노드 주소 목록, 예: `["redis-cluster:7000"]` |
| `username` / `password` | 빈 값 | |
| `pool_size` | `10` | |
| `max_retries` | `3` | `-1`이면 재시도 없음 |
| `min_retry_backoff_ms` / `max_retry_backoff_ms` | `8` / `512` | |
| `dial/read/write/pool/idle_timeout` | `5/3/3/5/1` | 초. 0/-1 규칙은 redis와 같다 |

DB 선택 필드는 없다 — 클러스터가 키 배치를 정한다.

### in-memory — 원천: internal/in_memory_config.go

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `max_cost` | `1000000` | ristretto 총 비용 상한 |
| `num_counters` | `1000000` | ristretto 카운터 수(근사 LFU용) |
| `buffer_items` | `64` | ristretto 쓰기 버퍼 크기 |

알려진 특성 두 가지([docs/testing.md](./testing.md) "알려진 특성"): 설정 값 키로 클라이언트·저장소가 프로세스 수명 동안 공유되므로 **최초 생성 시의 TTL이 이후 요청에 고정**된다. ristretto는 쓰기를 비동기 버퍼링해 저장 직후 조회가 빗나갈 수 있다.

### client_side_cache (redis 하위) — 원천: internal/redis_config.go

go-redis의 클라이언트 사이드 캐싱(RESP3 `CLIENT TRACKING` 기반 로컬 L1)이다. 실험 기능이고 **standalone redis, DB 0 전용** — 위반 시 go-redis가 경고와 함께 비활성화한다.

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `enabled` | `false` | |
| `max_entries` | `0` | 0이면 라이브러리 기본 상한 |
| `max_memory_bytes` | `0` | 0이면 무제한(항목 수 상한이 우선) |
| `max_staleness_ms` | `0` | 0이면 비활성. 캐시 TTL 이하 권장 — 신선도 계약 보호 |

Lua 스키마에는 이 필드가 없다(Go 전용).

### fill_lock — 원천: internal/sonic_boom.go, lua/sonic_boom/flow.lua

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `enabled` | `true` | `false`면 동시 미스 전원이 업스트림으로 간다 |
| `ttl_ms` | `3000` | 락의 안전망 TTL. 승자 장애 시 자동 해제 상한. 0 이하면 런타임이 3000으로 보정 |
| `wait_ms` | `200` | 패자가 승자의 저장을 기다리는 상한. 0이면 대기 없이 통과 |

in-memory 전략은 저장소가 락 원자적 설정을 지원하지 않아 fill-lock이 항상 fail-open이다 — 오류가 아니라 구조적 제약이며 Go는 `sonicboom_fill_lock_degraded` 메트릭으로 이 사실을 관측한다.

### log — 원천: internal/log.go

Go판 전용 필드다(Lua 스키마 미선언).

| 필드 | 기본값 | 비고 |
|------|--------|------|
| `log_level` | `"info"` | zerolog 레벨. 잘못된 값은 검증 실패(전 요청 `Bypass`) |
| `console_logging_enabled` | `true` | stderr 콘솔 출력 |
| `diode_enabled` | `true` | 비차단 버퍼(1000건/10ms) — 로그 폭주 시 일부 유실을 감수한다 |
| `file_log.enabled` | `false` | 파일 로깅 on/off |
| `file_log.filename` | `"sonic-boom.log"` | |
| `file_log.folder` | `"/tmp/logs"` | |
| `file_log.maxsize` | `0` | MB. 0이면 lumberjack 기본값 100MB |
| `file_log.maxage` | `0` | 일. 0이면 오래된 로그를 날짜로 지우지 않음 |
| `file_log.maxbackups` | `0` | 0이면 백업 파일을 개수로 지우지 않음 |
| `file_log.localtime` / `file_log.compress` | `false` | 롤링 파일 시각 기준·압축 |

## 필터와 규칙

`filters`는 경로·헤더 정규식으로 캐시 대상을 좁히는 sonic-boom 고유 확장이다. **어느 필터에도 맞지 않으면 캐시에서 제외된다** — 필터가 있으면 기본 동작이 '제외'다.

```yaml
filters:
- name: 정적-에셋만
  rules:
  - regexp: "^/assets/"
  cache_ttl: 60        # 생략 또는 0이면 전역 cache_ttl 상속
- name: 특정 헤더가 있을 때만
  rules:
  - header: X-Client
    regexp: "^mobile$"
```

- `name`은 필수, `rules`는 필수. 규칙 안의 `header`가 빈 문자열이면 경로 규칙, 값이 있으면 그 헤더 값에 대한 규칙이다.
- 필터 내 규칙은 전부 AND로 평가하고 필터 간에는 첫 매치가 승자다.
- 필터 `cache_ttl`을 0으로 두거나 생략하면 전역 `cache_ttl`을 상속한다. **즉시 만료 필터는 만들 수 없다** — 0이 "만료 없음"이 아니라 "상속"이기 때문이다.
- 정규식은 설정 로드 시점에 한 번 컴파일된다. 잘못된 패턴이면 Go는 요청을 헤더 없이 업스트림으로 통과시키고(패닉 방어선이 회복), Lua는 로드가 실패한다.
- 정규식 엔진이 다르다: Go는 RE2, Lua는 PCRE. 되돌림(백트래킹)이 필요한 패턴은 RE2에서 거부된다.

## 변종 차이

운영자가 설정을 쓸 때 마주치는 차이만 모았다. 판정 규칙 수준의 전체 목록은 [docs/kong-lua-verdict.md](./kong-lua-verdict.md)·[docs/apisix-verdict.md](./apisix-verdict.md)의 명시 차이 목록이 원천이다.

| 항목 | Kong-Go | Kong-Lua | APISIX-Lua |
|------|---------|----------|------------|
| `redis.host` 미선언 | 전 요청 `Bypass`(필수 검증) | `localhost`로 연결 시도 | etcd(Admin API) 배치는 검증 거부, standalone YAML은 런타임 fail-open |
| `cache_ttl: 0` | 요청 시점 거절 → 전 요청 `Bypass` | 선언형 로드 거부 | etcd 배치는 거부, standalone YAML은 미강제(런타임 fail-open) |
| 스키마에 미선언 필드 | 전부 수용 | **선언형 로드 거부** | **선언형 로드 거부**(`additionalProperties: false`) |
| Lua에서 미선언인 필드 | — | `max_retries`·retry backoff·`pool_timeout`·`client_side_cache`·`log`·`in_memory` 튜닝(재시도 없는 fail-open) | 같음 |
| redis 비밀번호 | 평문 | 평문 | `encrypt_fields`로 마스킹 |
| 저장 시점 | Response 단계 동기 | in-memory 동기, redis·cluster는 응답 후 비동기(log 단계 타이머) | 같은 Lua 코어 |
| 플러그인 priority | 1 | 100 | 99 |
| 관찰성 | OTel 메트릭·트레이스 | `product.label` 접두 로그 | 같은 Lua 코어 |
| `selfcheck` 필드(자가검증 라우트) | 없음 | 있음(기본 `false`) | 있음 |

APISIX standalone YAML 배치에서 `check_schema`가 강제되지 않는다는 점은 운영상 중요하다 — 설정 오류가 로드 거부가 아니라 런타임 fail-open(전 요청 `Miss`)으로 드러난다. 설정 파일을 배포한 뒤 `make e2e-apisix` 같은 동작 확인으로 검증해야 한다.

## 캐시 동작 규칙

표에 담기 어려운 규칙들이다. 각 규칙의 판정 위치는 [동작 방식](./how-it-works.md)이 설명한다.

- `content_type`은 대소문자를 무시하는 **정확 매치**다. `application/json`만 선언하면 `application/json;charset=utf-8` 응답은 캐시되지 않는다 — 파라미터 변형도 목록에 넣어야 한다.
- `Set-Cookie`가 있거나 `Cache-Control`에 `private`·`no-store`·`no-cache`가 있는 응답은 저장하지 않는다(APISIX 내장 proxy-cache 기본값과 정합).
- 요청측 `cache_control` 가드는 기본 on이다 — `Cache-Control: no-store/no-cache` 요청과 `Authorization` 요청은 캐시에서 제외된다(RFC 9111·Kong 원본 정합). 구 동작이 필요하면 `cache_control: false`.
- `cacheable_body_max_size` 기본 0은 "상한 없음"이다. 본문을 캐시하는 라우트(POST 등)는 반드시 상한을 정한다 — 미설정 시 어떤 크기의 본문도 메모리에 버퍼링된다.
- 본문 크기 초과로 저장을 건너뛰어도 `X-Cache-Status`는 `Miss`를 유지한다 — `Bypass`로 바뀌지 않는다.
- `X-Cache-Key` 헤더는 내부 캐시 키를 그대로 노출한다. 디버깅 편의로 수용된 동작이며 외부 노출이 우려되면 게이트웨이 앞단에서 제거한다.
