---
module: sonic-boom internal (Kong go-pdk external plugin)
date: 2026-10-18
problem_type: architecture_pattern
component: api_layer
severity: high
applies_when:
  - "Kong go-pdk external plugin(서버 모드)의 페이즈 로직을 작성·수정할 때"
  - "Lua 플러그인(Kong proxy-cache 등)을 Go로 포팅할 때"
  - "플러그인 프로세스 수명 자원(Redis 클라이언트·로거)을 초기화·재사용할 때"
  - "캐시 저장소에 TTL·만료 시맨틱을 위임할 때"
symptoms:
  - "cache_control: true 설정 후 첫 캐시 히트에서 플러그인 서버 프로세스가 죽는다"
  - "필터에 준 TTL과 다르게 실제 만료가 전역 cache_ttl로 동작한다"
  - "cache_ttl 미설정 시 Redis에 만료 없는 키가 쌓인다"
  - "버전 불일치·만료 캐시 히트가 X-Cache-Status: Hit으로 낡은 값을 서빙한다"
  - "설정한 content_type 목록과 무관하게 모든 content type이 캐시된다"
root_cause: wrong_api
tags: [kong, go-pdk, external-plugin, cache, redis, ttl, process-safety]
retire_when: "go-pdk 서버가 recover 또는 인스턴스 라이프사이클 훅을 제공하는 버전이 나오면 재검토(go-pdk 저장소 release notes와 #78 후속 확인)"
---

# go-pdk external plugin의 런타임 제약과 포팅 규율

상세 근거·전체 갭 목록은 `docs/bakeoffs/2026-10-18-api-gw-caching-gap-analysis.md`(2026-10-18 bake-off, 커밋 `ba5f09f` 기준)에 있다. 이 문서는 그 조사에서 나온 **규율**만 남긴다.

## Context

sonic-boom은 Kong 3.9.4의 공식 proxy-cache Lua 플러그인을 go-pdk external plugin 서버 모드로 포팅한 응답 캐시 플러그인이다. 2026-10-18 bake-off(독립 후보 2건 + 독립 평가 + 원본 소스 검증)와 당일 선행 세션들(3층 테스트 체계 구축, APISIX 이전 검토, 의존성 업그레이드)에서 런타임 제약 위반으로 인한 결함 9종이 확인됐다(전체 목록·파일:라인 근거는 bake-off 보고서 §4.1 G1–G9). 핵심 발견은 결함 자체보다 **그 결함을 만든 구조적 이유**다.

## Guidance

1. **페이즈 안에서 프로세스를 죽이는 호출 금지.** go-pdk 서버(pbserver)는 핸들러 패닉에 대해 recover가 전혀 없다. 패닉 = 프로세스 전체 사망. zerolog `Fatal()`은 `os.Exit(1)`을 호출한다. 플러그인 쪽이 유일한 방어선이므로 모든 페이즈 콜백(Access·Response 등)에 recover defer를 두고, Fatal 대신 Error+안전한 기본 동작(Bypass)으로 강등한다. 현재 Access에는 recover가 있고 Response에는 없다(`internal/sonic_boom.go:232-241` vs `:664` 이후 전무).
2. **설정은 변경 불변 취급.** go-pdk #78에서 Kong이 공식 확인한 모델: 하나의 플러그인 인스턴스가 여러 요청을 동시에 처리한다. 요청마다 `conf.Init()`이 `conf.logger`를 재할당하는 것은 데이터 경합이다(`sonic_boom.go:243`, `:665` → `:114`). 상태는 설정 값 키 sync.Map(go-pdk #78이 권하는 방식, in-memory 전략이 이미 사용, `sonic_boom.go:42-43`)이나 sync.Once로 한 번만 초기화하고, 전역 함수 변수(`zerolog.CallerMarshalFunc` — f2e9ef6에서 이미 sync.Once로 수정됨)는 최초 1회만 쓴다.
3. **Lua 포팅 시 `return` 시맨틱을 그대로 옮긴다.** 원본 Lua의 `return signal_cache_req(...)`는 "상태를 기록하고 계속 진행"이다. Go에서 이 함수가 `return`하면 요청은 계속 프록시된다. 이 차이를 몰라 `return`을 빼먹으면, 버전 불일치·만료 판정 후 떨어져서 방금 purge한 낡은 값을 `X-Cache-Status: Hit` 헤더로 덮어써 서빙한다(`sonic_boom.go:368-378`, `:386-395`, 덮어쓰기는 `:435`). 원본 Lua에는 두 지점 모두 `return`이 있다.
4. **TTL 0은 저장소별 의미가 다르다.** gocache redis store는 만료를 go-redis `client.Set`에 그대로 위임하므로(store/redis v4.2.12 `redis.go:79`) TTL 0 = **영구 키**다. 설정 기본값을 0으로 두면 최소 구성에서 만료 없는 키 적재 + stale 판정 즉시 참(절대 히트 불가)이 된다. TTL은 `gt=0` 검증이나 의도된 기본값으로 막는다.
5. **저장소는 생성 시점에 TTL이 고정되고 설정 값 키로 공유된다.** gocache store는 `WithExpiration(ttl)`로 생성되고 in-memory 스토어는 설정 구조체를 키로 공유한다. 서로 다른 TTL이 같은 스토어를 재사용하면 만료가 밀린다 — 당일 통합 테스트에서 TTL=5 스토어를 TTL=1 테스트가 재사용해 만료가 5초로 밀린 flake로 실측됐다(세션 히스토리). TTL을 스토어 공유 키에 포함하거나 TTL별로 분리해야 한다. 필터 TTL이 실제 저장에 무시되는 것도 같은 뿌리다(`sonic_boom.go:771`, `:389`는 전역값 사용).
6. **캐시 테스트는 저장소의 비동기성을 존중한다.** ristretto 쓰기는 비동기라 TTL 단언은 기한 있는 eventual 단언으로(예: 3초 창 내 만료 확인), `MaxCost`를 넘기면 전거 eviction로 키가 사라진다. 같은 URL을 치는 헬스프로브가 "첫 요청"을 Hit로 만들어 Miss 단언을 깬다 — 프로브는 플러그인 없는 라우트로 보낸다(세션 히스토리의 반복 flake 2종).
7. **페이즈 모델 경계를 설계에 반영한다.** go-pdk에는 upstream 직접 호출 수단이 없다. "stale을 서브하면서 백그라운드 재검증(SWR)" 같은 설계는 불가능하다 — 가능한 계약은 stale-if-error(stale 서브 후 다음 요청이 정상 재검증)까지다. Admin API 확장도 Lua 플러그인 전용 기능이라 external plugin에는 없다. 능동 무효화가 필요하면 PURGE 메서드 + 키 프리픽스(`sonic-boom:{version}:{hash}`) 패턴으로 풀어야 한다.
8. **설정 게이팅 폴스루를 조심한다.** `cacheableResponseContentType`은 목록 매치 실패 후에도 `return true`로 끝나 `content_type` 설정이 사실상 무시된다(`sonic_boom.go:854` — 루프 뒤 폴스루). 비슷하게 rawBody가 nil이면 본문 크기 상한 검사가 우회된다(`sonic_boom.go:744-752`). 게이팅 함수의 마지막 반환은 "매치 실패"의 의미를 가져야 한다. 과거 세션에서 이 폴스루를 발견해 테스트(`internal/sonic_boom_test.go:193`)로 문서화만 해두고 미수정이었다 — **발견만 하고 넘어간 결함은 후속 목록이 아니라 즉시 수리 대상으로 트래킹한다.**

## Why This Matters

이 규율 위반은 단순 기능 누락이 아니라 프로세스 사망·데이터 누출·무한 메모리 성장으로 이어진다. recover 부재(Fatal·패닉 = 전 라우트 다운), 설정 경합(간헐적 오작동), TTL 0(무한 성장), 폴스루(설정 무시)는 모두 e2e가 초록이어도 운영에서 터지는 부류다 — "CI 초록 = 검증" 착시가 당일 세션들에서 반복 관찰된 함정이다(세션 히스토리).

## When to Apply

`applies_when` 참조. 요약: external plugin의 페이즈 로직 작성·수정, Lua 플러그인 포팅, 프로세스 수명 자원 관리, 저장소 TTL 위임.

## Examples

**나쁨 — Fatal로 설정 오류 처리:**

```go
if conf.CacheControl {
    logger.Fatal().Msg("Cache control is enabled but not implemented yet") // os.Exit(1): 전 라우트 다운
}
```

**좋음 — Error 강등 + 안전 기본값:**

```go
if conf.CacheControl {
    logger.Error().Msg("cache_control not implemented; bypassing")
    return bypass(kong) // 요청은 계속 프록시
}
```

**나쁨 — Lua `return` 시맨틱 누락:**

```go
if cacheValue.Version != conf.CacheVersion {
    _ = marshal.Delete(ctx, cacheKeyID)
    _ = conf.signalCacheReqWithStatus(kong, cacheSignal, "Bypass")
    // return 없음 → 낡은 값을 Hit 라벨로 서빙
}
```

**좋음:**

```go
if cacheValue.Version != conf.CacheVersion {
    _ = marshal.Delete(ctx, cacheKeyID)
    return conf.signalCacheReqWithStatus(kong, cacheSignal, "Bypass") // 프록시 계속 진행
}
```
