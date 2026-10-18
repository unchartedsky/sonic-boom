# API 게이트웨이 캐싱 시장 조사와 sonic-boom 갭 분석

작성: 2026-10-18. 대상 커밋 `ba5f09f` (sonic-boom v1.0.4, go-pdk v0.11.2, Kong 3.9.4 기반 이미지).
방법: bake-off — 독립 후보 2건(서로 다른 모델, 서로 못 보는 신규 컨텍스트) + 독립 평가자(ce-pov, 제3 컨텍스트) + 조정자 원본 소스 검증. 실행 기록은 §8.

## 1. 결론

sonic-boom은 Kong OSS proxy-cache를 Go로 포트하면서 OSS에 없는 redis·redis-cluster 전략(Kong에서는 Enterprise proxy-cache-advanced의 영역)과 필터·cache_version·OTel을 얹은 포지션이다. 이 포지션은 시장에서 유효하다. 상태 헤더·키 분리·게이팅·부정 캐시 기본값은 이미 시장 수준이다.

문제는 기능 부족보다 정확성과 안정성이다. 조사로 확인된 버그 중 5종은 소규모 수정으로 제거된다:

| 단계 | 내용 | effort |
|---|---|---|
| **P0 즉시 수리** | 필터 TTL 무시 / return 누락(stale·버전 불일치 경로) / `cache_ttl` 0 = Redis 영구 키 / `cache_control: true` = 프로세스 사망 / Response 페이즈 recover 부재 | 소 (한 릴리스 묶음) |
| **P1 안정성·보안** | Redis 클라이언트 재사용 / 설정 경합 제거 / Authorization·Set-Cookie 가드 / 죽은 코드 정리 | 소 |
| **P2 운영 필수** | 캐시 메트릭 / 능동 무효화(PURGE + 키 프리픽스) / Cache-Control 최소 구현 | 중 |
| **P3 차별화** | storage_ttl·stale-if-error / 요청 병합(thundering herd) / ETag 재검증 / 키 구성 옵션군 | 중~大 |

## 2. 시장 기능 비교

범례: ✅ 지원 / ⚠️ 부분·조건부 / ❌ 미지원 / — 해당 없음. Envoy 열은 설정 페이지 세부를 이번 검증에서 재확인하지 못해 신뢰도가 가장 낮다(§7).

| 축 | Kong proxy-cache (OSS 3.x) | Kong Proxy Cache Advanced | AWS API GW (REST) | Azure APIM | Apigee ResponseCache | APISIX proxy-cache | Tyk | KrakenD | Envoy cache filter | sonic-boom |
|---|---|---|---|---|---|---|---|---|---|---|
| 캐시 키 | sha256(UUID·method·URI·query·headers·consumer_groups) 고정 | 동일 + principal(3.16+) | 헤더·경로·쿼리에서 명시적 선별 | vary-by-header/query/developer | CacheKey+KeyFragment(2KB 제한) | nginx 변수 자유 조합 + consumer_isolation(기본 on) | method+URL+cache_by_headers+body 해시+API ID+auth/IP | 백엔드 최종 URL + Vary 값 | RFC 7234 + allowed_vary_headers 통과 시 Vary | consumer+service+route+method+path+쿼리 전체+vary+TTL+body 해시 (`cache_key.go:129`) |
| TTL | `cache_ttl`(기본 300s); `cache_control=true` 시 s-maxage>max-age | 동일 | 0–3600s 기본 300s, Cache-Control 미반영 | 정책 duration(필수); max-age는 정책식으로 자가 파싱 | `UseResponseCacheHeaders` on 시 s-maxage>max-age>Expires와 정책 TTL 중 최솟값 | 디스크: X-Accel-Expires/Expires/CC 우선, 메모리: `cache_control` on 시 s-maxage/max-age | `cache_timeout` + 커스텀 헤더(`x-tyk-cache-action-set-ttl`) | Cache-Control 필수(없으면 미캐시) | RFC 7234 수명 계산 가능할 때만 | 정적 `cache_ttl`뿐; `cache_control` 필드는 존재하나 켜면 프로세스 사망 (`sonic_boom.go:382`) |
| 무효화 | Admin API lookup/delete/purge-all(클러스터 이벤트) | 동일 | flush-stage-cache + 클라이언트 max-age=0(IAM `execute-api:InvalidateCache`) | cache-remove-value | InvalidateCache 정책 + Caches API | 현행 문서에 없음(구판 disk PURGE 존재 — 버전별 확인 필요) | `DELETE /tyk/cache/{api-id}` | 없음(재시작뿐) | 없음 | 없음 — cache_version bump 후 lazy purge만 (`sonic_boom.go:368-378`) |
| 상태 표시 | X-Cache-Status 4종 + X-Cache-Key + Age(헤더별 토글) | 동일 | CloudWatch CacheHitCount/MissCount | 없음(정책 식) | flow 변수 cachehit | Apisix-Cache-Status MISS/HIT/EXPIRED/BYPASS | X-Tyk-Cached-Response: 1 | 없음(문서 직시) | LookupStatus(소스 기준) | X-Cache-Status 4종 + X-Cache-Key 무조건 노출 (`sonic_boom.go:315`) |
| 저장소 | memory만(Redis는 Advanced로 이관) | redis/redis-cluster/sentinel/memory | 관리형 캐시 클러스터(암호화 옵션) | 내장(리전 공유) / 외부 Redis | 관리형 분산 캐시(암호화) | disk(nginx) / memory LRU | Redis(전용 분리 가능) | 인메모리 LRU | HttpCache 인터페이스(simple/filesystem) | ristretto / redis / redis-cluster (`sonic_boom.go:145-228`) |
| SWR / stale-if-error | ❌(storage_ttl로 stale 보존만) | 동일 | ❌ | ❌ | ❌ | ⚠️ nginx 상속 | ❌ | ✅ max-stale/stale-if-error 해석 | ⚠️ stale 상태 처리 | ❌ (storage_ttl 개념 없음) |
| ETag/304 | ❌(revalidation 미지원 명시) | ❌ | ❌ | ❌ | ❌ | ⚠️ nginx 상속 | ❌ | ✅ ETag/Last-Modified 재검증 | ✅ 304 재검증(소스) | ❌ |
| 부정 캐시 | 기본 [200,301,404] | 동일 | 메서드 단위 TTL | 정책 나름 | ExcludeErrorResponse 기본 true | cache_http_status 지정(5xx는 502/504만) | cache_response_codes | GET/HEAD만 | 상태코드 화이트리스트 | 기본 [200,301,404], 상태코드별 TTL 없음 |
| 요청 병합 | ❌ | ❌ | — (클러스터 흡수) | ❌(rate-limit 후치 권고만) | — (lookup timeout 30s) | ⚠️ 문서 미명시 | ❌ | ❌ | 미확인(설계 문서에 서술 없음) | ❌ |
| Vary | 요청 측 vary_headers/vary_query_params만 | 동일 | 키 파라미터 선별로 대체 | vary-by-* (요청 측) | UseAcceptHeader(기본 false) | ✅ upstream Vary 존중(RFC 9111 §4.1), `Vary: *` 미캐시 | cache_by_headers(요청 측) | 백엔드 Vary 조합 | opt-in(allowed_vary_headers) | 요청 측 vary_headers만, upstream Vary 무시 (`cache_key.go:103-112`) |
| 관찰성 | 응답 헤더(+Kong-Debug 게이트) | 동일 | CloudWatch 지표 | 정책 식/Trace | flow 변수 | 상태 헤더 + 로그 | 헤더 + 분석 | 로그 지연 차이만 | 미확인 | OTel trace만, 캐시 결과 속성·메트릭 없음 |
| 보안 | cache_control on 시 Authorization 거부 | 동일 | 캐시 암호화, 무단 max-age=0 차단 | allow-private-response-caching 기본 false | 암호화 기본 | private/no-store/no-cache 항상 존중, Set-Cookie 기본 미캐시, consumer_isolation | auth/IP 키 포함 | Set-Cookie 특기 없음 | "not ready for deployment" 자체 경고 | Authorization·Set-Cookie 무방비 (`sonic_boom.go:452-457` 주석, `:728`) |

### 제품별 메커니즘 요약 (검증 근거)

- **Kong proxy-cache(OSS)** — sonic-boom의 직계 원형. `handler.lua`의 X-Cache-Status 4종·signal_cache_req·Age/X-Cache-Key가 sonic-boom 코드와 거의 1:1. 차이: (a) 원본은 stale/버전 불일치 시 `return signal_cache_req(...)`로 **업스트림으로 진행** — Go 포트는 return이 빠졌다. (b) `storage_ttl`이 freshness와 저장 수명을 분리("hours or days" 권고). (c) Admin API `/proxy-cache`로 lookup/delete/purge-all. (d) `vary_query_params` 화이트리스트(미지정 시 전체), `ignore_uri_case`, `response_headers.*` 토글, 3.16+ `cache_by_principal`. (e) `cache_control=true`일 때만 Authorization 요청을 거부하고, Set-Cookie는 처리하지 않는다(sonic-boom이 물려받은 원본 쪽 결함). (f) OSS 전략은 memory뿐 — Redis는 Advanced 전용(Kong Discussion #11999 공식 확인). 근거: developer.konghq.com/plugins/proxy-cache/, Kong/kong handler.lua raw fetch.
- **AWS API Gateway** — 스테이지 단위 관리형 캐시(0.5GB~, TTL 0–3600s 기본 300s, 1MiB 한도, 암호화 옵션). GET만 기본 캐시. 키는 파라미터 명시 선별 — 키에 없는 파라미터는 응답 공유(`?type=admin` 함정을 공식 문서가 경고). 무효화는 stage flush 또는 IAM 권한 있는 클라이언트 max-age=0. Cache-Control은 TTL에 미반영. 근거: docs.aws.amazon.com apigateway-caching.
- **Azure APIM** — cache-lookup/cache-store 정책쌍. vary-by-header/query/developer(-groups), `allow-private-response-caching` 기본 false(Authorization 포함 요청 기본 금지), 내장 캐시 또는 외부 Redis. 백엔드 max-age 반영은 정책식 자가 파싱. 무효화는 cache-remove-value. 캐시 미스 폭주 대비로 lookup 뒤 rate-limit 권고. 근거: learn.microsoft.com cache-lookup-policy.
- **Apigee ResponseCache** — CacheKey/KeyFragment(2KB), Scope 5단계 격리, ExpirySettings, `UseResponseCacheHeaders`(s-maxage>max-age>Expires와 정책 TTL 중 최솟값), ExcludeErrorResponse 기본 true(200–205 예외), SkipCacheLookup/Population 식. 무효화는 InvalidateCache + Caches API. 객체 256KB 한도, 상시 암호화. 근거: docs.cloud.google.com response-cache-policy.
- **APISIX proxy-cache** — disk(nginx proxy_cache)/memory 전략. cache_key는 nginx 변수 조합, cache_bypass/no_cache 조건식, consumer_isolation 기본 true, cache_set_cookie 기본 false. **private/no-store/no-cache는 플래그와 무관 항상 존중, upstream Vary 존중(`Vary: *` 미캐시)** — 8개 제품 중 유일한 적극적 Vary. 근거: apisix.apache.org proxy-cache.
- **Tyk** — Redis 저장소(전용 분리 `enable_separate_cache_store`). 키=method+URL+cache_by_headers+body 해시+API ID+auth(없으면 IP). 업스트림 제어는 표준 Cache-Control이 아니라 커스텀 헤더(`x-tyk-cache-action-set-ttl`). 무효화는 API 단위 flush. 근거: tyk.io response-caching.
- **KrakenD** — 백엔드별 `qos/http-cache`. **Cache-Control이 없으면 아예 캐시하지 않는다**(시장에서 가장 표준 준수적). max-stale/stale-if-error 존중, ETag/Last-Modified 재검증(304면 stale 서브). 무효화 인터페이스 없음. 히트 추적 헤더도 없음을 문서가 직시. 근거: krakend.io backends/caching.
- **Envoy cache filter** — HttpCache 인터페이스(simple 메모리, filesystem LRU). RFC 기반 수명 계산, 요청 no-store 존중, HEAD 미저장·상태코드 화이트리스트(설정 페이지 기반 — 이번 검증 미범위), 304 재검증(`handleCacheHitWithValidation`, 소스 확인). 설계 문서 스스로 "Work in Progress — not ready for deployment". 근거: envoyproxy.io cache_filter, source docs cache_filter.md.
- **참고** — nginx: `proxy_cache_lock`(요청 병합 표준 해법), `proxy_cache_use_stale`(error/timeout/updating + RFC 5861 헤더), `proxy_cache_background_update`, `proxy_cache_revalidate`, `proxy_cache_convert_head`(HEAD를 GET 캐시로), Set-Cookie·`Vary: *` 미캐시, OSS purge 없음(ngx_cache_purge 서드파티 / NGINX Plus purge). Varnish: busy object 대기열(요청 병합 내장), grace=사실상 SWR, hit-for-miss/hit-for-pass. CDN: Fastly Surrogate-Key·Cloudflare 캐시 태그 purge — 태그 역인덱스가 필요해 게이트웨이 플러그인 범위를 넘는다(참고만).

## 3. 이미 시장 수준인 것 (갭 아님)

X-Cache-Status 4종·Age·X-Cache-Status(Kong과 동일), 부정 캐시 기본값(Kong과 동일), consumer·service·route 키 분리, 상태코드·메서드·content-type 게이팅, body 최대 크기 제한, 필터(path/header 정규식 + TTL — Kong OSS에 없는 sonic-boom 고유 확장), **redis/redis-cluster 전략(OSS Kong의 공백을 메우는 차별점)**, OTel tracing 내장(KrakenD는 히트 추적조차 없음), 스토어 장애 시 fail-open(`sonic_boom.go:326-357`).

## 4. 갭 목록 (검증 상태 포함)

심각도: [치명] 설정·경로 도달 시 프로세스·데이터 사고 / [높음] 운영·성능·보안 실질 위험 / [중간] 시장 대비 기능 결여 / [낮음] 개선 여지. "검증"은 이번 실행에서 원본 소스 또는 코드 직접 대조로 확인했음을 뜻한다.

### 4.1 정확성·안정성 버그

| # | 갭 | 근거 | 검증 |
|---|---|---|---|
| G1 [치명] | `cache_control: true` + 캐시 히트 시 `logger.Fatal()` → zerolog `os.Exit(1)`로 **플러그인 서버 프로세스 전체 사망** | `sonic_boom.go:381-384`; zerolog@v1.35.1 log.go:401 | 검증(모듈 소스) |
| G2 [높음] | 필터별 `cache_ttl`이 저장·판정에 무시됨 — Response는 전역 `conf.CacheTTL`을 저장(`:771`), stale 판정도 전역값(`:389`). config_v1.yml의 필터 TTL 30은 실제로 전역 15초로 동작. 필터 TTL은 키 공간만 분리 | `sonic_boom.go:289`, `cache_key.go:20` | 검증(코드) |
| G3 [높음] | 버전 불일치·stale 경로에 `return` 누락 — purge/Bypass·Refresh 시그널 후 떨어져, 방금 purge한 낡은 값을 `X-Cache-Status: Hit` 헤더(`:435` 덮어쓰기)로 Exit(`:438`). 원본 Lua은 두 지점 모두 `return`으로 업스트림 진행. 주석(`:385` "don't serve stale data")과 코드가 정반대 | `sonic_boom.go:368-378`, `:386-395`, `:435-438`; handler.lua | 검증(코드+원본) |
| G4 [높음] | `cache_ttl` 기본 0 + `gte=0` → gocache redis store가 expiration을 go-redis에 그대로 위임(`redis.go:79`)해 **영구 키 적재**. 동시에 stale 판정 `> 0`이 즉시 참이 되어 절대 히트 못 하고 Redis 메모리만 증가 | `sonic_boom.go:56`; gocache store/redis v4.2.12 | 검증(모듈 소스) |
| G5 [높음] | 요청마다 Redis/Cluster 클라이언트 2회 생성(Access `:320`·Response `:783` → `:150`/`:171`). go-redis 풀은 클라이언트 단위라 커넥션 churn. in-memory만 sync.Map 재사용 중(`:192`) | `sonic_boom.go` 해당 라인 | 검증(코드) |
| G6 [높음] | 요청마다 `conf.Init()`이 `conf.logger` 재할당(`:114`) + `log.go:92` 전역 로그레벨 변경 — go-pdk #78에서 Kong이 "one instance handles many requests concurrently, wrap mutable shared state"라고 공식 답변. **데이터 경합** | `sonic_boom.go:243`, `:665`; go-pdk #78 | 검증(이슈 원문) |
| G7 [높음] | go-pdk v0.11.2 서버에 recover 전무(`pbserver.go` — `h(pdk)` 직접 호출). 핸들러 패닉 = 프로세스 사망. sonic-boom은 Access에만 recover 있고 **Response에는 없음** | pbserver.go:159-171; `sonic_boom.go:232-241` vs `:664~` | 검증(모듈 소스) |
| G8 [높음] | Authorization 헤더 요청 캐싱 무방비 — Kong 원본의 거부 로직이 주석으로만 존재(`:452-457`). consumer가 확정되는 라우트는 키 분리로 완화되나, passthrough 라우트에서는 자격증명 간 응답 공유 가능 | `sonic_boom.go:452-457` | 검증(코드+원본) |
| G9 [높음] | Set-Cookie 포함 응답을 통째로 캐시·재생 — hop-by-hop/삭제 목록(`:559-585`)에 없음. nginx·APISIX는 기본 미캐시. 원본 Kong도 같은 결함(물려받음) | `sonic_boom.go:728`, `:767`, `:438` | 검증(코드+원본) |

### 4.2 기능 격차 (시장 대비)

| # | 갭 | 근거 | 시장 선례 |
|---|---|---|---|
| G10 [중간] | Cache-Control 디렉티브 전무 — 응답 no-store/no-cache/private 무시(`:844-852` 주석), 요청 no-cache/max-age=0 무시(`:340-344`), max-age TTL 파생 없음 | `sonic_boom.go` 주석 3곳 | APISIX(항상 존중)·KrakenD(필수)·Envoy·Kong |
| G11 [중간] | 능동 무효화 부재 — 유일한 수단이 cache_version bump(런타임 변경 불가 env 경로 `:101-109` 포함) + lazy purge. 키가 접두사 없는 10진 해시(`cache_key.go:157`)라 SCAN 범위 purge도 불가 | `cache_key.go:157` | Kong Admin API, AWS flush, Azure cache-remove-value, Tyk flush, nginx Plus purge |
| G12 [중간] | thundering herd 방지 부재 — 핫 키 만료 직후 동시 요청 전부 업스트림으로 | `sonic_boom.go:352-357` | nginx proxy_cache_lock, Varnish busy object |
| G13 [중간] | storage_ttl·stale-if-error 부재 — freshness와 저장 수명이 같아(`:771`→`:790`) upstream 장애 시 stale 서브 불가. 단 **진짜 SWR(백그라운드 재검증)은 go-pdk 제약상 불가** — upstream 직접 호출 수단이 없다(#78 페이즈 모델) | `sonic_boom.go:771`, `:790` | Kong storage_ttl, nginx use_stale, Varnish grace |
| G14 [중간] | 관찰성: hit/miss/bypass 메트릭 없음. OTel span에도 캐시 결과 속성 없음(`:264-269`) | `sonic_boom.go:264-269` | AWS CloudWatch, APISIX 헤더 |
| G15 [중간] | upstream Vary 무시 — gzip×vary는 config_v1.yml의 vary 라우트 예시(설정 규약)로 때우는 중(`scripts/smoke_lib.sh`에 e2e 단언 존재). `Vary: *` 미캐시 처리도 없음 | `cache_key.go:103-112` | APISIX·KrakenD·Envoy |
| G16 [낮음] | `ignore_uri_case` 미지원 — README TODO | README.md:132 | Kong |
| G17 [낮음] | `vary_query_params` 화이트리스트 부재 — utm_* 등 추적 파라미터가 히트율 분산 | `cache_key.go:114` | Kong, AWS |
| G18 [낮음] | HEAD가 GET 엔트리를 재활용하지 못함(Method가 키에 포함, convert_head 없음) | `cache_key.go:15` | nginx convert_head 기본 on |
| G19 [낮음] | TTL이 키 해시에 포함 — TTL 변경이 전체 캐시 무효화로 작동 | `cache_key.go:20` | Kong은 미포함 |
| G20 [낮음] | 상태코드별 TTL 없음 | `sonic_boom.go:51` | nginx proxy_cache_valid |
| G21 [낮음] | `request_method` 검증 느슨(required뿐, 임의 메서드 허용) | `sonic_boom.go:52` | Kong 스키마 oneof |
| G22 [낮음] | 설정 검증이 debug 모드에서만(`:278-282` isDebug 게이트) — 운영에서는 요청 시점에야 드러남 | `sonic_boom.go:278-282` | — |
| G23 [낮음] | 죽은 코드·죽은 검사 — 필터 기본 TTL 루프가 range 복사본을 고쳐 no-op(`:117-121`), `validate.Struct(conf)`가 CacheValue가 아니라 conf를 검사(`:776`; cache_value.go:16 ReqBody required와 함께 지뢰), responseData 블록(`:399-414`), reqBody 읽고 버림(`:759-763`) | 각 라인 | — |
| G24 [낮음] | X-Cache-Key 무조건 클라이언트 노출(isDebug 게이트 아님) | `sonic_boom.go:315` | Kong response_headers 토글 |
| G25 [낮음] | 조건부 요청(ETag/304) 없음 — 시장에서도 Envoy·KrakenD·Varnish 급 고급 영역 | — | Envoy, KrakenD, nginx revalidate |

## 5. 개선 로드맵

공통 전제: go-pdk external plugin은 별도 프로세스로 페이즈 콜백만 받고 초기화/종료 훅이 없다(#78). 그래서 모든 제안은 (a) 설정 값 키 sync.Map(이미 in-memory가 사용, `sonic_boom.go:42-43`), (b) 페이즈 내 eager 초기화, (c) Kong 설정 리로드 경유의 세 안에서만 구현 가능성을 판정했다. effort: 소=1일 미만, 중=1–3일, 大=3일 이상(대략치).

### P0 — 즉시 수리 (소, 한 릴리스 묶음)

1. **필터 TTL 정합** — `:771`을 `TTL: int64(cacheSignal.CacheTTL)`로, `:389` 판정을 Access가 구한 cacheTTL로. 저장·판정·키의 TTL 일치. 테스트: 필터 TTL 30 설정 시 16초 뒤에도 Hit 단언.
2. **return 복원** — `:378`(버전 불일치)과 `:394`(stale) 뒤에 `return`. 원본 Lua와 동일 — 낡은 값 서빙이 사라지고 업스트림 재검증으로 바뀐다(히트율 통계 변화는 수용).
3. **TTL 기본값·검증** — 기본 300(Kong 정렬) + `gt=0`. 기존 TTL 0 배포가 있다면 릴리스 노트로 명시.
4. **Fatal 강등** — `:381-383`을 Error+Bypass로. 설정 플래그 하나가 프로세스를 죽이는 것은 fail-fast가 아니라 요청 경로 DoS다.
5. **Response recover** — Access의 defer recover(`:232-241`)를 Response에도. go-pdk 서버가 recover 없음이 확인된 이상 플러그인 쪽 방어가 유일하다.

### P1 — 안정성·보안 (소)

6. **Redis 클라이언트 재사용** — `RedisConfig`/`RedisClusterConfig` 값(가능한 구조체)을 키로 하는 sync.Map 패턴을 redis에도 적용. 훅 부재로 종료 시점은 없지만 프로세스 수명과 운명이 같으므로 무해 — in-memory와 동일 수용. 설정 변경 시 이전 클라이언트 잔존은 라우트당 설정 수가 적어 실무 무해. 벤치마크(`sonic_boom_bench_test.go`)로 전후 입증.
7. **설정 경합 제거** — logger를 인스턴스당 1회 생성(go-pdk는 인스턴스를 lazy 생성·재생성하므로 전역 map 키 = 설정 직렬화 값), `SetGlobalLevel` 호출도 최초 1회로.
8. **Authorization 가드** — Authorization 헤더가 있고 consumer가 비어 있으면 Bypass(원본 주석의 Go 이식). Kong 원본은 cache_control=true일 때만 거부하므로 이 제안은 원본보다 강한 기본값 — 옵트아웃 플래그와 함께 문서화. Azure의 `allow-private-response-caching` 기본 false와 같은 방향.
9. **Set-Cookie 가드** — Set-Cookie 응답 미캐시(cacheableResponse 조기 반환) + `headersToDelete`에 set-cookie 추가(이중 방어). 쿠키+캐시가 필요하면 `cache_set_cookie` 옵트인(APISIX 동명 옵션).
10. **죽은 코드 정리** — no-op 필터 루프는 인덱스 순회로 수정, `:776`은 cacheValue 검증으로 교체(ReqBody 태그와 함께 정리), `:399-414`·`:759-763` 제거, checkConfig를 isDebug 밖으로.

### P2 — 운영 필수 (중)

11. **캐시 메트릭** — 1단계: OTel span에 `cache.status`(hit/miss/bypass/refresh) 속성. 2단계: `main.go`에서 별도 포트로 prometheus `/metrics`(go-pdk 서버가 이미 리스너를 여는 구조라 페이즈와 무관) 또는 `otel/metric` 카운터(go.mod에 이미 간접 의존). 히트/미스/바이패스 + 전략 라벨.
12. **능동 무효화: PURGE 메서드 + 키 프리픽스** — 키를 `sonic-boom:{cache_version}:{hash}`로(`cache_key.go:150-159` 한 곳). 효과: 버전 bump가 이전 세대 전체를 즉시 도달 불가로(lazy가 아니라), Redis 키 가시성·SCAN 범위 purge 확보, DB-less 배포에서 설정 리로드만으로 무효화. 여기에 Access 초입 `method == "PURGE"` → 키 생성 → `marshal.Delete` → `Exit(204)`(nginx OSS ngx_cache_purge 패턴)를 얹으면 개별 키 능동 무효화 완성. **인증 가드 필수**. 키 포맷 변경은 기존 캐시 전면 무효 — CACHE_VERSION 버스팅으로 일괄 처리.
13. **Cache-Control 최소 구현** — (a) 응답 no-store/no-cache/private는 `cache_control` 플래그와 무관 항상 저장 거부(APISIX 방식), (b) 요청 no-cache → miss 처리, (c) 플래그 on 시 s-maxage/max-age를 저장 TTL로, (d) `only-if-cached` 미스 시 504(RFC 7234 §5.2.1.7). max-stale/min-fresh 전면 구현은 기각(§6).

### P3 — 차별화 (중~大)

14. **storage_ttl + stale-if-error** — 저장 만료는 storage_ttl, freshness 판정은 TTL로 분리. Response에서 업스트림 5xx 판명 시 stale 엔트리로 `Exit` + `X-Cache-Status: Stale`(nginx use_stale 상당). 백그라운드 SWR은 go-pdk 제약상 불가 — "stale 서브 후 다음 요청이 정상 재검증"까지가 구현 가능한 계약. "낡은 데이터 허용"은 비즈니스 판단 사항(§7).
15. **thundering herd 방지** — in-memory 전략은 `golang.org/x/sync/singleflight`. redis 전략은 `SET sonic-boom:lock:{key} NX PX <timeout>` — 획득 실패 시 짧은 재조회 후 타임아웃이면 통과(fail-open). PX로 보유자 사망 자해제. 다중 노드+공유 Redis 배포 여부에 따라 편익 결정(§7).
16. **조건부 재검증(ETag/304)** — ETag를 CacheValue에 저장 → stale 히트 시 Access에서 `kong.service.request.SetHeader("If-None-Match", etag)` 주입 후 통과 → Response에서 304면 저장된 200 본문으로 `Exit(200)` + Age 갱신(nginx proxy_cache_revalidate 패스스루 패턴). 이득은 대역폭이지 지연이 아니다. ETag 없는 업스트림에선 무효.
17. **키 구성 옵션군(각 소)** — `ignore_uri_case`(README TODO 해소), `vary_query_params` 화이트리스트, `response_headers` 토글(X-Cache-Key 기본 비노출 권장), HEAD→GET 키 정규화, 상태코드별 TTL, `request_method` oneof 검증, upstream Vary(v1은 "응답 Vary 헤더를 요청 측 키에 합집합" 단순화 + `Vary: *` 미캐시 — APISIX 계약).

## 6. 기각한 접근

- **Kong Admin API purge 엔드포인트 이식** — go-pdk external plugin에는 admin 이벤트가 없고(#78 페이즈 모델), 기본 배포가 DB-less(`KONG_DATABASE=off`)라 Kong 자신도 hybrid memory 전략처럼 제약을 받는 구조. PURGE 메서드 + 버전 프리픽스로 대체.
- **플러그인 내 별도 purge HTTP 서버** — 기술 가능하나 배포·보안·헬스체크 비용이 버전 프리픽스 대비 과대. 무효화 빈도가 낮은 워크로드에서 정당화되지 않음.
- **백그라운드 SWR(플러그인이 upstream 직접 호출)** — go-pdk에 upstream 호출 수단이 없고 Kong 프록시 체인(인증·리라이트)을 우회한다. stale-if-error로 축소.
- **Redis Pub/Sub 멀티노드 무효화** — 버전 bump가 같은 문제를 운영으로 풀고, 수요 근거 없음. 필요하면 프리픽스+SCAN으로 대부분 소화.
- **CDN surrogate key/캐시 태그** — 태그-키 역인덱스 저장소가 필요해 게이트웨이 플러그인 범위 초과. 참고로만.
- **RFC 7234 전면 구현을 최상위에** — full compliance는 시장에서 Envoy·KrakenD뿐. 최소 안전 집합(P2-13)이 위험 대비 편익 최대.
- **gocache 제거·직접 Redis 전환** — 현재 문제(클라이언트 재사용)는 sync.Map 패턴으로 해결되고 marshaler(msgpack)는 실사용 중. 교체 비용 > 편익.
- **게이트웨이 이주(APISIX 등)** — 과제 범위 밖. 참고: SWR·병합·Vary가 본질 요구라면 APISIX proxy-cache가 해당 기능을 이미 갖추고 있다.

## 7. 미검증 가정·열린 질문

**이번 실행에서 확인한 것**: zerolog Fatal=os.Exit(1), go-pdk pbserver recover 부재, gocache→go-redis expiration 위임(모듈 소스), 갭 표의 파일:라인(코드 직접 대조), Kong 원본 return·Set-Cookie 무처리·Redis=Advanced(원본 fetch), #78 경합 경고(이슈 원문), APISIX Vary·Azure 기본값·KrakenD 필수 TTL(공식 문서).

**미확인(구현 전 확인 필요)**:
1. `kong.Response.Exit` 이후 Response 페이즈의 실제 실행 여부 — G3의 부수 서술과 PURGE 재저장 방지 설계에 영향.
2. ristretto 만료 항목의 Get 동작 — stale 분기의 in-memory 도달 가능성 판정에 필요.
3. msgpack 역직렬화의 신규 필드 무시 여부 — CacheValue 필드 추가 계열(P3-14·16) 전제. 키 프리픽스 도입과 같은 릴리스로 일괄 무효화 가능.
4. `kong.service.request.SetHeader` 주입 → 304 수신 → Response 치환 경로의 실동작 — P3-16 구현 전 검증.
5. Envoy 설정 페이지 세부(HEAD 미저장, 상태코드 목록, 요청 병합 부재 단정) — 매트릭스 Envoy 열의 신뢰도가 가장 낮음.

**운영 판단 사실(저장소 밖)**:
6. 다중 노드 + 공유 Redis 배포가 기본인가 — P3-15 Redis 락 vs singleflight 선택 기준.
7. Authorization passthrough 라우트가 실존하는가 — G8/P1-8 우선순위 기준.
8. stale serve(낡은 데이터 허용) 계약의 비즈니스 승인 여부 — P3-14 채택 조건.
9. go-pdk 상위 버전에서 recover·훅 변화 여부 — 분석은 v0.11.2 기준.

## 8. Bake-off 실행 기록

- **후보 1**(Opus, 호스트 서브에이전트): 10열 시장 매트릭스·갭 22건·P0~P3 제안. 53회 도구 호출. 기여: 매트릭스 전체, P0-8/9 보안 번들 패키징, storage_ttl 프레이밍, 버전 프리픽스, Kong 원본 Set-Cookie 결함 근거, only-if-cached 504, G20·G21. 오차: 버전 불일치 경로의 최종 라벨을 "Bypass"로 서술(실제는 `:435` Hit 덮어쓰기), Envoy 요청 병합을 미검증 단정(❌)으로 표기.
- **후보 2**(Sonnet, 호스트 서브에이전트): 8제품 비교·갭 18건·P1~P10 제안, go-pdk/zerolog/gocache 모듈 소스 직접 검증. 68회 도구 호출. 기여: G6·G7(경합·recover), 죽은 코드 인벤토리, PURGE 메서드 무효화, SWR 불가능성 논증, ETag 패스스루 메커니즘, #78 근거 체계. **판정: 베이스 채택.**
- **후보 3**(GLM, ccs 헤드리스): 탈락. 1차 시도 ccs 기본 601초 타임아웃, continue 경로 문법 오류(E104), 리커버리 재출동은 "No messages returned from query"로 2.5초 만에 거부. 리커버리 출동 소진으로 탈락 확정 — 모델 패밀리 다양성(2 families 목표)은 미달, 두 후보는 Claude 계열(단, 상이 모델)로 진행.
- **독립 평가자**(ce-pov, Opus, 제3 신규 컨텍스트): 후보별 근거 상태 표본 검증, 상호 모순 7건 판정(2건에서 후보 2 승 — 라벨·Envoy), 의존성 10건, "후보 2 베이스 + 후보 1 기여 흡수" 권고. 조정자 결론과 일치.
- **조정자 검증**: 모듈 캐시 스팟체크 3종(zerolog/pbserver/gocache) + 갭 표 라인 대조. 평가자 권고와 독립적으로 동일 결론 도달 후 채택.
- **미충족**: 후보 3 탈락으로 3-후보 목표는 미달(2-후보 최소 완성 조건은 충족). GLM 라우트 기록은 세션 로그 참조.
