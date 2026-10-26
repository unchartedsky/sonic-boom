# Kong Lua 포팅 PoC 판정 자료 (U9)

날짜: 2026-10-21 · 브랜치 `andromedarabbit/kong-lua` · 소비자: **사람 판정 회의**
계획: `docs/plans/2026-10-21-1136-feat-kong-lua-port-plan.md` (U1–U9 전 단위 완료, 9 커밋)

## 종료 기준 대조

| 항목 | 결과 | 근거 |
|---|---|---|
| **기능** | ✅ 통과 | 전략 3종 스모크(6단언+gzip·deflate·br 인코딩+stale 재검증) 초록: `scripts/smoke_lua_{in_memory,redis,rediscluster}.sh`. 재생 비교 `scripts/parity_replay.sh` exit 0 — 시퀀스·status·body 일치(명시 차이 Set-Cookie 분기 제외). **범위 명시: 재생 비교는 redis 전략 라우트 기준** — 전략 전체 동등성은 각 전략 스모크로 담보 |
| **라이브러리 충분성** | ✅ 충족(한계 명시) | 이미지 번들 부재 3종 실측 후 vendoring으로 해소(resty.sha256→openssl.digest, lua-resty-redis, xmodem). `lua_regex_match_limit` 이미지 지원 확인(생성 nginx.conf 반영). 타임아웃·keepalive 매핑 동작(재사용 로그 실측). shdict no memory fail-open 코드 경로 구현(강제 소형 zone 실측은 미실행 — 예외 기록). **cluster ACL username은 소스 조립 수준 검증만** — 런타임 실측은 백로그(compose 클러스터 무인증) |
| **성능** | ✅ 선등록 문턱 안 | `docs/bench-kong-lua-vs-go.md`: HIT Lua 15,175 RPS·p99 24ms vs Go 1,524 RPS·p99 150ms; MISS 4,708 vs 134 RPS. 문턱(p99 회귀 >20%·RPS 저하 >10%) 미달 — 유의미 열세 없음. PoC 조건(맥 단일 머신, c50) 한정. **2026-10-22 재측정**(코어 이전·fill-lock 기본 on 이후): HIT Lua 16,030 RPS·p99 31ms vs Go 2,822·74ms, MISS Lua 742·226ms vs Go 263·289ms — 여전히 문턱 안. MISS 하락은 비저장 라우트의 fill-lock 대기 때문이다(같은 문서 재측정 절) |
| **관측성** | ✅ 최소 형태 | 모든 로그에 `sonic_boom.product=kong-lua` 접두 부착(R8). **span 속성은 백로그**(Kong tracing 연동 미구현 — 범위 경계). 자기검증 라우트(`/selfcheck`)가 회귀 감지 초동선 |
| **이식성** | ✅ 예비 신호(한계 명시) | 코어(`lua/sonic_boom/`, 455줄, vendor 제외)에서 `kong.*` 참조 **0건**(grep 실측). 어댑터 424줄(schema+handler) — 비율 ~1:1로 "얇은 어댑터"는 아니며, 게이트웨이별 스키마·단락 메커니즘 재작성이 이식 비용의 대부분이다. **APISIX 연동 자체는 미검증** — 본 신호는 "코어가 재사용 가능하게 분리됐다"까지만 말한다. **2026-10-22 갱신: APISIX 어댑터로 실증됐다**(`docs/apisix-verdict.md`). 페이즈 로직을 코어 상태기계 `flow.lua`로 옮긴 뒤 Kong 어댑터는 172줄, APISIX 어댑터는 195줄로 줄어 어댑터:코어가 ~1:5다 |

## 미해결 결정 (판정 회의 전 사용자 결정 필요)

1. **인증 주체 키 분리 강화**(codex 리뷰 제안): consumer 부재·vary 미포함 쿠키 인증 요청의
   교차 사용자 캐시 노출 가능. "인증 요청은 주체가 키에 반영될 때만 캐시"는 현재 Go 동등성과
   충돌하는 새 제품 행동 — 채택 시 R4에 4번째 명시 차이로 추가. 현재 Go·Lua 모두 같은 동작(파리티 유지).

## 명시 차이 요약 (Go dc9c909 대비, R4)

1. `Set-Cookie`·`Cache-Control: private/no-store` 응답 미저장(재생 비교 S8에서 실측 분기)
2. ~~`cache_control` 스키마 거부 + 기본값~~ 해소(2026-10-26 Lua 포팅) — 요청층 cache_control 가드가 Go와 기본 on 동일(기본값 true, 옵트아웃 계약, 판정은 코어 flow.lua 단일 구현). 남은 차이는 미선언 필드(client_side_cache·log)뿐이다
3. 정규식 엔진 RE2→PCRE(`lua_regex_match_limit 10000` 완화, ReDoS 표면 축소·선형 보장은 상실)
4. 저장 시점: Go(응답 버퍼링 후 동기) → Lua(redis/cluster는 log 단계 timer 비동기; in-memory는 body_filter 동기)
5. 버전 불일치 purge 후 상태 라벨 — Go=Bypass, Lua=Miss 폴스루(Lua 키 공간은 캐시 버전이 접두로 분리돼 purge는 방어 코드다)
6. redis.host 검증 — Go는 required라 미선언 시 전 요청 Bypass, Lua(Kong)는 localhost 기본값으로 fail-open Miss. Kong 선언형 스키마는 조건부 required가 없어(선언하지 않은 서비스까지 검증에 걸침 — 실측) 정렬 불가, 의도적 차이로 문서화한다. APISIX YAML 배치도 로드 거부가 불가능하다(실측: if/then·oneOf 무시, check_schema 훅은 Admin API 모드 전용) — 런타임 fail-open Miss로 Kong-Lua와 동일하며, 로드 강제는 etcd 모드의 check_schema 훅이 담당한다.

> 2026-10-22 갱신: 명시 차이 1은 Go에 역포팅돼 해소됐다(U1·U2 — Set-Cookie·private/no-store/no-cache 응답 양쪽 모두 미저장). Lua에 없던 `no-cache` 검사도 추가됐다. 위 목록은 PoC 시점 기록으로 보존한다.

## 신기능 이식 백로그 (판정 통과 시, APISIX 확장과 함께 순서 제안)

| 순서 | 항목 | 근거/비고 |
|---|---|---|
| 1 | ~~fill-lock(스탬피드 완화)~~ **완료(U4, 2026-10-22)** | `lua/sonic_boom/fill_lock.lua`가 internal/fill_lock.go를 미러링. in-memory는 shdict add, redis는 SET NX, 클러스터는 fork 범용 디스패치(종단간 버스트 단언이 가드 — 실패 시 fail-open). resty.lock은 미채택(노드 로컬) |
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

1. 종료 기준 대조 승인(팀 감당 항목은 2026-10-22 판정 대상에서 제외 — 에이전트로 운영하므로 별도 증거가 필요 없다)
2. 미해결 결정 1건(인증 주체 키 분리) 채택 여부
3. APISIX 확장 착수 — 코어 재사용 전제로 `adapters/apisix/` 착수 승인 여부 (2026-10-22 진행됨 — 판정은 `docs/apisix-verdict.md`의 안건으로 이어진다)
4. Kong Lua판의 운용 전환(PoC→운영 라우트 적용) 여부와 시점
