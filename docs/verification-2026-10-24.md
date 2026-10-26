# 검증 결과 (2026-10-24)

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
