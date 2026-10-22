---
title: 게이트웨이 내장 proxy-cache vs sonic-boom — 성능은 같은 급, 선택 근거는 기능
date: 2026-10-22
category: tooling-decisions
module: sonic-boom 캐시 플러그인(Go·Kong-Lua·APISIX-Lua) vs Kong·APISIX 내장 proxy-cache
problem_type: tooling_decision
component: api_layer
severity: medium
applies_when:
  - "라우트에 캐시를 붙일 때 sonic-boom과 게이트웨이 내장 proxy-cache 중 무엇을 쓸지 고를 때"
  - "APISIX·Kong 내장 캐시를 바탕으로 다시 만들자는 제안을 평가할 때"
  - "캐시 플러그인 성능을 서로 비교하는 벤치를 설계하거나 해석할 때"
retire_when: "Kong 또는 APISIX 내장 proxy-cache가 redis 같은 공유 저장소나 스탬피드 락을 지원하게 되면 재검토 — 상류 플러그인 스키마(apisix/plugins/proxy-cache/init.lua, kong/plugins/proxy-cache/schema.lua) 확인"
tags: [apisix, kong, proxy-cache, benchmark, buy-vs-build, fill-lock, go-pdk]
---

# 게이트웨이 내장 proxy-cache vs sonic-boom — 성능은 같은 급, 선택 근거는 기능

## Context

APISIX 어댑터를 붙인 뒤 "APISIX 기본 proxy-cache를 바탕으로 작업하는 게 낫지 않았나"라는 질문이 나왔다. 답하려고
2026-10-22에 우리 구현 3종(Go·Kong-Lua·APISIX-Lua)과 내장 캐시 2종(Kong OSS `proxy-cache`, APISIX `proxy-cache`)을
같은 조건으로 쟀다. 수치와 조건은 `docs/bench-five-caches.md`에 있다. 이 문서는 그 결과에서 다시 조사하지 않으면 잃기
쉬운 판단 근거와 벤치 함정만 남긴다.

## Guidance

**노드 로컬 메모리 캐시만 필요하면 성능은 내장 캐시를 피할 이유가 되지 않는다. sonic-boom Lua판을 고르는 근거는 기능이다.**

- 성능: HIT에서 Lua판과 같은 게이트웨이의 내장 캐시는 RPS 범위가 크게 겹친다. 기준선 대비 Kong-Lua 2.99, Kong
  proxy-cache 3.07, APISIX-Lua 2.71, APISIX proxy-cache 3.27이다. MISS에서는 넷 다 기준선 자리(0.90~1.21)다.
- 내장 캐시에 없는 것(APISIX 3.19 이미지 소스 기준):
  - **공유 저장소가 없다.** 스키마의 `cache_strategy` enum이 `disk`·`memory`뿐이다
    (`/usr/local/apisix/apisix/plugins/proxy-cache/init.lua`). 노드마다 따로 채우고 재시작하면 비워진다.
  - **스탬피드 락이 없다.** nginx 템플릿에는 `proxy_cache_lock_timeout 5s`만 있고 `proxy_cache_lock on`이 없다
    (이미지 내 `/usr/local/apisix/apisix/cli/ngx_tpl.lua`). memory 전략에도 락 코드가 없다.
  - 정규식 필터별 TTL이 없다(`cache_ttl` 하나, `cache_bypass`·`no_cache` 변수).
  - Kong과 APISIX에서 같은 동작을 보장하는 공통 코어가 없다.
- 내장 캐시가 이미 하는 것: APISIX는 `consumer_isolation` 기본 true, `cache_set_cookie` 기본 false, `cache_control`
  옵션이 있고 priority 1085로 인증(2500대) 뒤에 돈다. 즉 "주체 분리·Set-Cookie 미저장"은 차별점이 아니다
  (`docs/solutions/architecture-patterns/proxy-cache-principal-key-isolation.md`).
- **Go판(go-pdk)은 다른 급이다.** HIT은 기준선의 0.53, MISS는 0.06이다. 캐시를 붙였는데 기준선보다 느리다. 성능이
  중요한 라우트에서 Go판은 선택지가 아니다.

**권장: 라우트별로 고른다.** 노드 로컬로 충분하면 내장 proxy-cache를 쓴다(유지할 코드 0줄). 노드 간 redis 공유나 노드 간
스탬피드 방지가 필요하면 sonic-boom Lua판을 쓴다.

## Why This Matters

- "내장 캐시를 바탕으로 다시 만들자"는 제안은 공유 저장소와 락을 붙이는 순간 사실상 재작성이 된다. 결과는 Kong 쪽과 코드를
  공유하지 못하는 세 번째 구현이다. 이 판단 근거가 사라지면 같은 조사를 다시 하게 된다.
- 반대로 "우리 것이 빠르다"를 선택 근거로 쓰면 틀린다. 노드 로컬 조건에서는 같은 급이다.

## When to Apply

- 새 라우트의 캐시 플러그인을 고를 때
- 캐시 플러그인 벤치를 다시 설계할 때(아래 함정)

## Examples

벤치를 설계할 때 다음 세 가지를 지켰고, 어기면 결론이 뒤집힌다.

- **fill-lock을 끈 MISS로 비교한다.** 기본값(락 켬)으로 저장되지 않는 라우트를 재면 패자들이 `wait_ms`(200ms)를 다
  기다려 p99가 약 230ms가 된다. 락이 없는 내장 캐시와 비교하면 플러그인 비용이 아니라 락 대기를 재게 된다. 이 함정에
  실제로 빠졌었다. Kong-Lua MISS가 PoC 4,708 RPS에서 742 RPS로 떨어진 것을 보고 원인을 찾은 것이다
  (`docs/bench-kong-lua-vs-go.md` 재측정 절).
- **게이트웨이마다 자기 `/noplugins` 기준선을 같은 회차에 잰다.** 게이트웨이 기본 처리량이 서로 달라 원시 RPS로는
  플러그인 비용을 가를 수 없다.
- **회차 편차를 먼저 본다.** 맥 단일 머신에서 같은 URL의 3회 RPS가 최대 2배 차이 났다(APISIX proxy-cache HIT
  9,771–19,856). 10~20% 차이는 잡음이다.

재현: `BENCH_PAIR=all5 ./scripts/bench_gw.sh`. 비교 라우트는 `config_v1.yml`(`/go-maxsize-nolock`), `config_lua.yml`
(`/lua-maxsize-nolock`, `/kong-pc`, `/kong-pc-miss`), `config_apisix.yaml`(`/lua-maxsize-nolock`, `/apisix-pc`,
`/apisix-pc-miss`)에 있다.
