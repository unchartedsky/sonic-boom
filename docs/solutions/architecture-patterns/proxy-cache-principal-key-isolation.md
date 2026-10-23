---
title: proxy-cache 인증 주체 캐시 키 분리 — Kong·APISIX 동작 검증과 채택 근거
date: 2026-10-22
category: architecture-patterns
module: sonic-boom 캐시 키 (Go internal + Lua 포팅)
problem_type: architecture_pattern
component: api_layer
severity: high
applies_when:
  - "캐시 키에 인증 주체(consumer/principal)를 반영할지 설계·판정할 때"
  - "Kong·APISIX proxy-cache의 주체 분리 동작을 근거로 인용할 때"
  - "\"Go·Lua 파리티\"를 동작 유지·변경의 사유로 쓸 때"
retire_when: "Kong(kong/plugins/proxy-cache/cache_key.lua) 또는 APISIX(apisix/plugins/proxy-cache)의 기본 키 격리 동작이 바뀌면 재검토 — 상류 소스·문서 재확인"
tags: [kong, apisix, cache-key, principal, consumer-isolation, authorization, set-cookie, market-research]
---

# proxy-cache 인증 주체 캐시 키 분리 — Kong·APISIX 동작 검증과 채택 근거

## Context

`docs/kong-lua-verdict.md`의 미해결 결정 1건(codex 리뷰 제안, "인증 요청은 주체가 키에 반영될 때만 캐시")을 판정하기 위해 2026-10-22에 Kong과 APISIX의 동작을 소스·공식 문서 수준에서 검증한 조사다. 발단은 2026-10-21 codex 교차 리뷰가 이 제안을 Lua 포팅 판정 회의 안건으로 올린 것이고(session history), 같은 세션에서 APISIX 소스 1차 확인과 Go·Lua의 Set-Cookie 실측(Go Miss→Hit 재생 / Lua Bypass)이 선행했다(session history).

이 조사 전까지 프로젝트가 갖고 있던 오해 소지: 10-18 bake-off 시장조사 표는 Kong 키 공식을 문서 기준 "consumer_groups 포함"으로 기록했고, 세션 요약은 "Kong 3.9.4는 principal 반영 메커니즘이 없다(무방어)"로 정리했다(session history). 두 문구 모두 절반만 맞다 — 아래에서 소스 기준으로 교정한다.

## Guidance

**시장 동작은 세 층으로 나뉜다. 설계 논쟁은 2·3층에서만 일어난다.**

1. **주체가 확정된 요청은 무조건 키에 반영** — Kong·APISIX·sonic-boom 공통이고 옵트인이 아니다.
   - Kong OSS(3.9.4 소스 및 master 동일): 키 = `sha256(prefix|method|uri|params|headers)`, `prefix_uuid(consumer_id, route_id)`는 인증 시 `consumer_id:route_id`, 미인증 시 `route_id`, 전역은 `"default"`. `vary_headers`만 헤더 성분에 반영(미설정 시 빈 문자열), `vary_query_params` 미설정 시 query 전체. 근거: Kong 저장소 `kong/plugins/proxy-cache/cache_key.lua`, `handler.lua`(3.9.4 태그·master raw fetch). Kong 문서의 `consumer_groups` 표기는 소스에 없다(3.9.4·master 모두) — 문서-소스 드리프트, 소스 기준으로 인용할 것.
   - APISIX(공식 문서): `consumer_isolation` 기본 true — `ctx.consumer_name` 또는 `remote_user` 식별 시 그 ID를 키 선두에 부착한다. `cache_key`에 이미 identity 변수(`$consumer_name`, `$consumer_group_id`, `$remote_user`, `$http_authorization`)가 있으면 무동작(운영자 선택 존중).
   - sonic-boom: Go는 캐시 키 구성에 consumer를 넣고(`internal/cache_key.go:132`), Lua도 같다(`lua/sonic_boom/key.lua:39`). 즉 1층은 이미 동등하다.
2. **Authorization 헤더 요청의 보호는 제각각이다.**
   - Kong: `cache_control=true`일 때만 `no-store`·`no-cache`·Authorization 요청을 Bypass. 기본값 false라 기본 상태에서는 Authorization 요청도 캐시된다(3.9.4 `handler.lua`의 `cacheable_request`).
   - APISIX: `$http_authorization`이 키에 들어가므로 자격증명별로 키가 분리된다(저장 자체는 막지 않는다).
   - sonic-boom: Go·Lua 모두 vary 미지정 시 Authorization이 키에도 가드에도 없다 — bake-off G8. Go는 원본의 거부 로직이 주석으로만 남아 있다(`internal/sonic_boom.go:565`).
3. **주체 확정이 안 되는 요청(쿠키 passthrough 인증)은 두 상류 제품 모두 완전히 막지 않는다.** Kong은 consumer로 확정되지 않으면 route 단위 공유, APISIX도 미식별이면 기본 키(`$host`+`$request_uri`)로 공유하되 cache_key가 임의 NGINX 변수를 받으므로 `$http_cookie` 수동 분리가 가능하다. APISIX가 이 영역에서 갖는 진짜 우위는 응답 신호 방어다: `private`/`no-store`/`no-cache`는 플래그와 무관 항상 존중, `Set-Cookie`는 `cache_set_cookie` 기본 false로 미캐시(디스크 전략은 항상), upstream `Vary` 존중(`Vary: *` 미캐시).

**codex 제안의 위치와 채택 설계.** "주체 없는 인증 요청 미캐시"는 두 상류의 기본 동작보다 **강한** 동작이다 — 방향은 Azure APIM의 `allow-private-response-caching` 기본 false와 같은 축이다(bake-off §2). 채택을 가로막는 "Go·Lua 파리티" 논거는 부적절하다: 두 구현이 같은 동작이라는 사실은 그 동작이 시장 표준임을 뜻하지 않고 공통 결함일 수 있다(G8·G9가 그 증례). 권고: 채택 시 (a) 옵트아웃 플래그를 붙여 hit율 영향을 통제·측정 가능하게 하고, (b) APISIX식 응답 신호 방어(Set-Cookie 기본 미캐시, `private` 존중 — Lua판에는 이미 있음, `lua/kong/plugins/sonic-boom-lua/handler.lua:209`)를 Go에 역포팅해 파리티를 "강한 쪽"으로 회복한다. 세션 리서치 권고도 "APISIX 방식의 주체 강제 접두를 공유 코어에 이식해 Go·Lua가 동시 획득"이었다(session history) — sonic-boom은 1층이 이미 구현돼 있으므로 남는 과제는 (a)의 부(-) 가드와 (b)의 Go 역포팅이다.

## Why This Matters

이 결정은 hit율과 개인화 응답 누출 사이의 제품 판단이고, 판정 회의가 "Kong도 이렇게 한다"는 식의 부정확한 시장 근거에 끌리기 쉽다. 실제 시장 동작을 소스 기준으로 고정해 두면: (1) 1층은 이미 동등하므로 논쟁 대상이 아니고, (2) codex 제안은 "이례"가 아니라 Azure APIM이 가는 방향의 앞선 선택이며, (3) Kong OSS는 무방어 게이트웨이가 아니라 응답 신호 방어가 기본값이 아닌 게이트웨이라는 정확한 위치가 나온다. 세션 요약의 "Kong 3.9.4 무방어" 문구를 근거로 쓰면 판정이 반대 방향으로 왜곡될 수 있었다 — 시장 근거는 반드시 소스·공식 문서까지 확인하고 인용 연도·버전을 명시한다.

## When to Apply

- 판정 회의에서 인증 주체 키 분리(codex 제안)를 결정할 때
- 캐시 키 구성에 새 성분(쿠키, 자격증명, principal)을 넣거나 뺄 때
- Lua판 백로그(운영 전환 게이트)의 우선순위를 정할 때 — 이 문서의 (a)·(b)가 그 대상이다
- Kong·APISIX 동작을 근거로 인용하는 문서·리뷰를 쓸 때 — 버전과 소스/문서 구분을 명시한다

## Examples

| 주체 상황 | Kong OSS(3.9.4 소스) | APISIX(공식 문서) | sonic-boom Go | sonic-boom Lua |
|---|---|---|---|---|
| consumer 확정 | 키에 `consumer_id:route_id` 무조건 | `consumer_isolation` 기본 on, 키 선두 부착 | 키에 consumer 포함 (`internal/cache_key.go:132`) | 키에 consumer 포함 (`lua/sonic_boom/key.lua:39`) |
| Authorization 헤더 | `cache_control=true`면 거부, 아니면 캐시 | 키가 자격증명별 분리 | 무방비(G8, `sonic_boom.go:565` 주석만) | 무방비 |
| 쿠키 passthrough(주체 미확정) | route 단위 공유 | 기본 키로 공유, `$http_cookie` 수동 분리 차선 | 공유 — codex 지적 지점 | 공유 |
| Set-Cookie 응답 | 처리 없음(원본 결함을 물려받음) | 기본 미캐시(disk는 항상) | 저장·재생(G9) | 미저장(`handler.lua:209`) |
| `private`/`no-store`/`no-cache` | `cache_control=true`일 때만 | 항상 존중 | 미흡(bake-off G9·cache_control 항목) | 미저장(R4 명시 차이 1) |

> 2026-10-22 갱신: 아래 Examples 표의 sonic-boom 열은 조사 시점 기록이다. U1·U2로 Set-Cookie·private/no-store/no-cache 응답은 Go·Lua 양쪽 모두 무조건 미저장이 됐고, Authorization 가드는 U3에서 `cache_control: true` 옵트인으로 구현됐다. G8의 기본 상태(cache_control=false 무방비)는 시장 준거 경로에서 의도된 미결이다. Authorization 가드는 Go판에 한해 — Lua(Kong·APISIX)은 cache_control 미선언으로 요청측 가드 없음(백로그). 표는 PoC 판정 시점의 스냅샷으로 보존한다.

## Related

- `docs/bakeoffs/2026-10-18-api-gw-caching-gap-analysis.md` — G8·G9 결함, 제안 8(Authorization 가드)·9(Set-Cookie 가드), 시장 비교 표(문서 기준 키 공식)
- `docs/kong-lua-verdict.md` — 미해결 결정 1(이 문서가 뒷받침하는 판정 안건), R4 명시 차이 4건
- `docs/solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md` — 같은 조사 시기의 런타임 규율 학습
- README "캐시 가능성 규칙" — 필터 TTL 상속·content_type 매칭 등 현재 설정 시맨틱
