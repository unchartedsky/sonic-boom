# 검증 결과 (2026-10-24)

## 추가 검증 (2026-10-27)

**요청층 자격증명 vary(⑥) 창구** — consumer 미확정 + Authorization 요청을 principal 키 성분으로
자격증명별 슬롯에 분리(Lua sbk3·Go Principal 성분, 코어 단일 구현). TDD: selfcheck 벡터 6종 + Go
5시나리오 RED→GREEN(37/37), e2e 3게이트웨이 + principal 고정 라우트 단언, parity S17 양 쌍 초록.
문서 리뷰 5인(비대화형) 통과 후 착지. principal 고정 라우트의 실측 교훈 2건: 필드 생략(스키마 기본
true) 경로는 e2e가 직접 검증해야 하고(초안이 Bypass로 실패 — 실관찰), Kong-Lua는 DB-less 재시작 시
설정을 다시 읽어 라우트 반영에 재시작이 필요하다.

**벤치 회귀 실측 보류** — 측정 시점 기기 로드 52-81(다중 스택 잔존·Docker VM 포화)로 수치가 노이즈로
덮여 선등록 문턱 판정이 불가했다. principal 성분의 기계적 비용은 키 직렬화 문자열 연결 1회 수준이지만,
측정은 조용한 기기 창에서 재수행할 것.

## 추가 검증 (2026-10-26)

10-24 이후 두 창구를 더 통과했다: (1) Go cache_control 가드 **기본 on 전환**(TDD — 기본값 단언 RED→GREEN,
vary 스위트 이주 패턴, 핫패스 비용 실측 문턱 안, 코드 리뷰 lite 접수) (2) **Lua판 가드 포팅과 백로그 정리**
(flow.lua 코어 가드 + 스키마 2종 + selfcheck 5벡터 TDD RED→GREEN, e2e 라우트 미러링, parity S13 재작성·
S15 신설 양 쌍 초록, dependabot PR #92 머지·구 브랜치 정리, 벤치 bigbody·redis·redis-cluster HIT 신설,
스키마 거부 픽스처 client_side_cache 교체). 문서 리뷰 5인(비대화형)과 코드 리뷰 2회(lite 접수) 통과.
상세: docs/plans/2026-10-26-2110-feat-lua-cache-guard-and-backlog-plan.md, docs/bench-* 문서 10-26 절.


**결론: 전 층위 통과.** 창구 8a2f086..56f5c6b(13커밋) — 코드 리뷰 적용 28건 전부 + 창구 밖 잔여(PE5·PE2/PE3, 회귀 게이트, 스키마 실측 정정)까지. 커밋 푸시 완료, CI 초록(4m53s).

## 스코프

| 변종 | 파일 | 커밋 |
|------|------|------|
| kong-golang | `internal/` | PE5(OTel 저장 차단)·키 직렬화 v2(sha256)·fill-lock Go 미러·저장 게이트 리팩터·gofmt |
| kong-lua·apisix-lua | `lua/` + 설정 2종 | 키 충돌 제거(sbk2)·헤더 잘림 fail-closed·fill-lock 대기 관측·스키마 정렬·e2e 게이트 보강 |
| 인프라·문서 | Makefile·compose·CI·문서 | e2e 레시피 통합·compose 앵커·CI 덤프·벤치 재측정 2건·판정 문서 정정 |

## 층위별 결과

| 층위 | 대상 | 결과 | 비고 |
|------|------|------|------|
| 단위 | Go internal 유닛 | 37 PASS | 신규: span DTO 라운드트립·복원 실패 계속, NUL vs 다중값 키 분리 |
| 단위 | Lua selfcheck | 25 벡터 PASS | 신규: ratelimit 헤더 제거, 패자→Hit, sbk2 충돌 회귀 |
| 통합 | make test (-race) | PASS ×2 | Lua 배치 후, PE2·PE5 후 각 1회 |
| e2e | make e2e (Go 3전략) | PASS | manyhdr·cache_control 전략 태그 포함 |
| e2e | make e2e-lua | PASS | in-memory·redis·rediscluster + U4/U5/U7 + sb-lock 스캔·nolock 대조군·nomatch |
| e2e | make e2e-apisix | PASS | 동일 + APISIX fail-open 계약 단언 |
| e2e | OTel 활성 스모크 | PASS ×2 | OTEL_SDK_DISABLED 해제 상태에서 in-memory·redis — PE5 실측 |
| 재생 비교 | parity lua-apisix | PASS | S14 cache_version 롤오버 포함; go-lua 쌍은 CI 경유 |
| 벤치 | go-lua·lua-apisix | 문턱 안 | 상세는 bench 문서 2종의 2026-10-24 재측정 절 |
| CI | 56f5c6b | success | CI 4m53s, build 6m15s — arm64 매트릭스 포함 첫 녹색 |

## 이번에 새로 생긴 회귀 게이트

- 헤더 110개+후행 Set-Cookie 미저장 단언(전 스위트) — #3 잘림 fail-open 회귀 방지
- nolock 대조군 버스트 단언(단언기 자신의 눈멈을 탐지) — redis·redis-cluster
- Set-Cookie 뒤 `sb-lock:*` 잔여 스캔 — release-only 해제 회귀 방지
- nomatch Bypass 단언 — return 시맨틱 위험 지점
- APISIX 저장소 미선언 라우트 fail-open(Bypass) 계약 단언
- S14 cache_version 롤오버 재생 시나리오(lua-apisix)
- Go: NUL vs 다중값 키 분리, span DTO 라운드트립·복원 실패 계속 단언

## 남은 측정 과제 (문서 기록분)

- 큰 본문 HIT의 shm→Lua 본문 복사 비용
- redis·redis-cluster 전략 핫패스 벤치(현행 벤치는 노드 로컬 메모리만 측정)
- parity 다중값 인자 순서 시나리오 — ✅ 2026-10-26 S15로 착지(kong-lua-verdict 명시 차이 7)
