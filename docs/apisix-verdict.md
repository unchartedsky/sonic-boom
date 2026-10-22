# APISIX 어댑터 판정 자료 (U8)

날짜: 2026-10-22 · 브랜치 `feat/apisix-lua-adapter` · 소비자: **사람 판정 회의**
계획: `docs/plans/2026-10-22-1939-feat-apisix-lua-adapter-plan.md` (U1–U8)
선행 판정: `docs/kong-lua-verdict.md` (Kong-Lua PoC)

## 무엇을 만들었나

- Kong-Lua 핸들러의 페이즈 로직을 코어 상태기계 `lua/sonic_boom/flow.lua`로 옮겼다(U2). 대상은 시그널 수명,
  저장 판정, 청크 버퍼링, fill-lock 해제, log 타이머다. 이로써 Kong 어댑터는 391줄에서 64줄로 줄었다.
- 같은 상태기계에 APISIX 3.19 어댑터 `lua/apisix/plugins/sonic-boom-lua.lua`(195줄, 스키마 포함)를 붙였다(U3·U4).
  어댑터가 담당하는 것은 APISIX PDK 게터·세터(env), ctx 저장, 단락(`return status, body`)뿐이다.
- 키 접두를 제품별로 분리했다. Kong-Lua는 `sbl`, APISIX는 `sba`다. 접두는 해시 입력에 들지 않으므로 같은
  redis를 공유해도 키·락 공간이 섞이지 않는다(U1).

## 종료 기준 6항목 대조

| 항목 | 결과 | 근거 |
|---|---|---|
| **기능** | ✅ 통과 | `make e2e-apisix`에서 3전략 스모크(공통 단언·gzip·deflate·br·vary·stale)가 Kong-Lua와 같은 스크립트로 통과했다. redis 중단 fail-open, fill-lock 버스트(업스트림 1회), 클러스터 SET NX 왕복도 통과했다. 재생 비교 `PARITY_PAIR=lua-apisix ./scripts/parity_replay.sh`는 S1–S12에서 시퀀스·status·body가 일치했다(Go 전용 S13 제외). **범위 명시: 재생 비교는 redis 전략 라우트 기준**이다 |
| **라이브러리 충분성** | ✅ 충족 | 이미지 번들이 런타임에서 실측됐다. `resty.openssl.digest`·`cjson.safe`는 `/selfcheck` OK로, `resty.lock`은 클러스터 스모크로 확인했다. vendoring한 redis·rediscluster는 수정 없이 재사용했다. 공유 존 2종은 `custom_lua_shared_dict`로 선언했고 APISIX 기본 존 이름과 겹치지 않는다. `lua_regex_match_limit`은 APISIX가 100000으로 하드코딩한다(아래 명시 차이 1) |
| **성능** | ✅ 선등록 문턱 안 | `docs/bench-apisix-vs-kong-lua.md` 참조. 각 게이트웨이의 `/noplugins` 대비 오버헤드 비율로 판정했다. HIT은 APISIX의 RPS 유지율이 +16.1%, p99 배율이 -25.9%로 Kong-Lua보다 낫다. MISS는 사실상 같다(+5.0%, -0.1%). PoC 조건(맥 단일 머신, c50)에 한정된다 |
| **관측성** | ✅ 최소 형태 | 코어 로그에 `sonic-boom-lua product=apisix:` 접두가 붙는다(product 모듈, 호출 시점 조회). span 속성은 Kong-Lua와 같이 백로그다 |
| **이식성** | ✅ 실증 | 코어(`lua/sonic_boom/`, vendor 제외 1,044줄)에서 `kong.*` 참조는 재귀 grep **0건**이다. Kong 어댑터는 172줄(handler 64 + schema 108), APISIX 어댑터는 195줄이다. Kong-Lua 판정 때의 어댑터:코어 ~1:1이 ~1:5로 바뀌었다. 게이트웨이마다 새로 쓴 것은 스키마 번역과 env 게터·세터뿐이다 |
| **팀 감당** | ⚠️ 미판정(판정 회의 과제) | Kong-Lua 판정에서 확정한 에이전트 중심 방식(사람이 에이전트 산출물을 읽고 판정, 약 30분)을 그대로 적용한다. 대상 산출물은 판정 회의에서 정한다 |

## 명시 차이 (APISIX vs Kong-Lua)

1. **정규식 match limit**이 100000 대 10000이다. APISIX가 nginx 템플릿에 하드코딩해 설정으로 바꿀 수 없다.
   snippet으로 중복 선언하면 nginx가 기동하지 않는다. 필터 정규식의 ReDoS 예산이 10배 넓다.
2. **클러스터 주소 필수 검증이 더 엄격하다.** `strategy=redis-cluster`인데 `redis_cluster.addrs`가 없으면 스키마
   단계에서 거부한다. Kong-Lua는 요청 시점에야 실패한다.
3. **스키마 거부 단위가 다르다.** APISIX standalone은 잘못된 라우트 하나만 건너뛰고
   `failed to check item data` 오류 로그를 남긴다. Kong-Lua는 선언형 로드 전체가 거부된다. 미구현 필드
   (`cache_control` 등)를 거부하는 결과는 같다.
4. **큰 요청 본문을 읽는다.** `core.request.get_body`는 파일로 버퍼링된 본문도 읽는다. 그래서 Kong-Lua에서는
   본문 읽기 실패로 통과하던 큰 POST가 APISIX에서는 캐시 대상이 된다.
5. **경로 정규화가 다르다.** APISIX는 `ctx.var.uri`(nginx 정규화)를 쓰고 Kong은 `kong.request.get_path`를 쓴다.
   이 차이는 키에만 영향을 주고 재생 비교 결과와는 무관하다.
6. **ctx 공유 제약이 있다.** 시그널을 `ctx.sonic_boom_signal`에 두는데, APISIX ctx는 전 플러그인이 공유한다.
   그래서 같은 라우트에 global_rule과 라우트 플러그인을 동시에 거는 구성은 미지원이다.

## 운영 관찰

- **저장되지 않는 응답이 잦은 라우트에서 fill-lock이 꼬리 지연을 만든다.** 벤치의 MISS 라우트(`/lua-maxsize`)는
  응답이 절대 저장되지 않는다. 그래서 같은 키의 동시 미스에서 패자들이 `wait_ms`(기본 200ms)를 다 기다린 뒤
  통과해 p99가 약 230ms가 됐다. 두 Lua 게이트웨이와 Go판(`internal/fill_lock.go`)이 공통으로 가진 시맨틱이다.
  이런 라우트에는 `fill_lock.enabled: false`를 권장하는 운영 가이드가 필요하다.

## 판정 회의 안건

1. 6항목 대조 승인. 팀 감당 항목에 쓸 판독 산출물을 지정한다.
2. APISIX 어댑터의 운영 적용(PoC→운영 라우트) 여부와 시점.
