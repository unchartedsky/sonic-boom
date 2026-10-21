# Kong Lua 포팅 PoC 판정 자료 (U9)

날짜: 2026-10-21 · 브랜치 `andromedarabbit/kong-lua` · 소비자: **사람 판정 회의**
계획: `docs/plans/2026-10-21-1136-feat-kong-lua-port-plan.md` (U1–U9 전 단위 완료, 9 커밋)

## 종료 기준 6항목 대조

| 항목 | 결과 | 근거 |
|---|---|---|
| **기능** | ✅ 통과 | 전략 3종 스모크(6단언+gzip·deflate·br 인코딩+stale 재검증) 초록: `scripts/smoke_lua_{in_memory,redis,rediscluster}.sh`. 재생 비교 `scripts/parity_replay.sh` exit 0 — 시퀀스·status·body 일치(명시 차이 Set-Cookie 분기 제외). **범위 명시: 재생 비교는 redis 전략 라우트 기준** — 전략 전체 동등성은 각 전략 스모크로 담보 |
| **라이브러리 충분성** | ✅ 충족(한계 명시) | 이미지 번들 부재 3종 실측 후 vendoring으로 해소(resty.sha256→openssl.digest, lua-resty-redis, xmodem). `lua_regex_match_limit` 이미지 지원 확인(생성 nginx.conf 반영). 타임아웃·keepalive 매핑 동작(재사용 로그 실측). shdict no memory fail-open 코드 경로 구현(강제 소형 zone 실측은 미실행 — 예외 기록). **cluster ACL username은 소스 조립 수준 검증만** — 런타임 실측은 백로그(compose 클러스터 무인증) |
| **성능** | ✅ 선등록 문턱 안 | `docs/bench-kong-lua-vs-go.md`: HIT Lua 15,175 RPS·p99 24ms vs Go 1,524 RPS·p99 150ms; MISS 4,708 vs 134 RPS. 문턱(p99 회귀 >20%·RPS 저하 >10%) 미달 — 유의미 열세 없음. PoC 조건(맥 단일 머신, c50) 한정 |
| **관측성** | ✅ 최소 형태 | 모든 로그에 `sonic_boom.product=kong-lua` 접두 부착(R8). **span 속성은 백로그**(Kong tracing 연동 미구현 — 범위 경계). 자기검증 라우트(`/selfcheck`)가 회귀 감지 초동선 |
| **이식성** | ✅ 예비 신호(한계 명시) | 코어(`lua/sonic_boom/`, 455줄, vendor 제외)에서 `kong.*` 참조 **0건**(grep 실측). 어댑터 424줄(schema+handler) — 비율 ~1:1로 "얇은 어댑터"는 아니며, 게이트웨이별 스키마·단락 메커니즘 재작성이 이식 비용의 대부분이다. **APISIX 연동 자체는 미검증** — 본 신호는 "코어가 재사용 가능하게 분리됐다"까지만 말한다 |
| **팀 감당** | ⚠️ 근거 불충분(판정 회의 과제) | 루프 9틱이 전 단위를 무정체로 완료했다는 사실은 자동화 마찰이 낮다는 뜻이지 팀의 Lua 유지보수 능력 증거가 아니다. 교정 로그(아래)가 실제 디버깅 난도의 표본이다 — LuaJIT goto 미지원, cosocket 단계 제약, Kong conf 반복 키 등. **권고: 판정 회의에서 팀 소유 Lua 과제(리뷰·디버깅 1건)를 증거로 요구할지 결정** |

## 미해결 결정 (판정 회의 전 사용자 결정 필요)

1. **인증 주체 키 분리 강화**(codex 리뷰 제안): consumer 부재·vary 미포함 쿠키 인증 요청의
   교차 사용자 캐시 노출 가능. "인증 요청은 주체가 키에 반영될 때만 캐시"는 현재 Go 동등성과
   충돌하는 새 제품 행동 — 채택 시 R4에 4번째 명시 차이로 추가. 현재 Go·Lua 모두 같은 동작(파리티 유지).

## 명시 차이 요약 (Go dc9c909 대비, R4)

1. `Set-Cookie`·`Cache-Control: private/no-store` 응답 미저장(재생 비교 S8에서 실측 분기)
2. `cache_control: true` 스키마 거부(선언형 로드 거부 — Go는 요청 경로 Fatal을 U4 수정으로 강등한 상태였으나 Lua는 로드 시점 거부)
3. 정규식 엔진 RE2→PCRE(`lua_regex_match_limit 10000` 완화, ReDoS 표면 축소·선형 보장은 상실)
4. 저장 시점: Go(응답 버퍼링 후 동기) → Lua(redis/cluster는 log 단계 timer 비동기; in-memory는 body_filter 동기)

## 신기능 이식 백로그 (판정 통과 시, APISIX 확장과 함께 순서 제안)

| 순서 | 항목 | 근거/비고 |
|---|---|---|
| 1 | fill-lock(스탬피드 완화) | Go는 기본 on — 운영 동등성에 가장 먼저 필요. Lua 구현은 resty.lock 단일 비행 |
| 2 | 캐시 판정 메트릭 파이프라인 | Go 로드맵 U2 상당. Lua는 prometheus 플러그인 레지스트리 확장 |
| 3 | span 속성 관측성 | R8 확장 — Kong tracing / APISIX apisix.tracer |
| 4 | 클러스터 ACL 런타임 실측 | 무인증 클러스터에 ACL 노드 1개 켜고 AUTH 왕복 |
| 5 | CSC(클라이언트 사이드 캐싱) | Go plumbing 존재(기본 off) — 필요성 재검 후 |
| 6 | P2: PURGE 무효화 + 키 프리픽스 | gap 분석 P2. Lua는 Admin API 확장 가능(go-pdk은 불가였음) |
| 7 | P3: storage_ttl·stale-if-error·ETag | 차별화 — Lua에선 SWR도 기술적으로 가능(제약 해소) |

## 루프 실행 기록

- 9틱, 단위당 1틱, 3틱 연속 실패 정지 조건 발동 0회, compose 정지 0회.
- 틱 내 교정(스모크·스크립트 수준): goto 미지원 재작성(U4), 값 없는 쿼리 true 처리(U4),
  cosocket 단계 위반 HEAD 저장 분기(U5), kong.conf 중복 키 침묵 결함→include 주입(U8),
  재생 비교 경로 짝 오류·시나리오 기대 오류(U6), checkConfig redis.host 최소 config(U6).
- 전 틱 공통 게이트: `make test`·`make e2e`(Go 무회귀) 매 단위 초록.

## 판정 회의 안건

1. 6항목 대조 승인(특히 팀 감당 항목의 증거 요구 여부)
2. 미해결 결정 1건(인증 주체 키 분리) 채택 여부
3. APISIX 확장 착수 — 코어 재사용 전제로 `adapters/apisix/` 착수 승인 여부
4. Kong Lua판의 운용 전환(PoC→운영 라우트 적용) 여부와 시점
