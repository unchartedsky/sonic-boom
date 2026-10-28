---
title: 공식 문서 영어 재작성 - Plan
type: docs
date: 2026-10-28
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: knowledge-work
---

# 공식 문서 영어 재작성

## Goal Capsule

- **Objective**: 공개 저장소의 모든 사용자 대면 문서가 영어로 읽힌다 — 영어 독자가 README에서 시작해 자기 변종의 사용 가이드까지 문서만으로 갈 수 있다. 확인은 문서 밖에서 가능하다: 문서의 예제 명령을 따라 했을 때 `X-Cache-Status`가 Miss→Hit로 나오면 됐다.
- **Means**: 이번 세션의 한국어 확정본(HEAD `142a4f7`)을 축자 대응으로 영어 번역하고, ce-noslop로 영어 AI 티를 제거한 뒤 검증·커밋한다 (KTD1·KTD2).
- **Authority**: 사용자 확정 결정 — 완전 대체(한국어판은 git 이력 `10a711a`~`142a4f7`에서 복원), 범위 9개 파일(README + docs/ 6종 + CHANGELOG + CONCEPTS).
- **Stop conditions**: 번역 중 원문의 사실·구조와 어긋나는 요구가 생기면(링크 대상 변경 필요 등) 그 지점을 멈추고 보고한다 — 번역이 재설계로 드리프트하는 것을 막는다.
- **Execution profile**: 번역은 문서별 서브에이전트 병렬(동시 ≤4), 검증·noslop·커밋은 호스트가 수행. 커밋·push는 사용자 최종 지시 후(저장소 세션 패턴).

---

## Product Contract

### Requirements

- R1. 9개 파일 전부 영어로 재작성한다. 무변경 동결 요소: 코드 펜스 내부의 **명령·설정값·경로·환경변수·mermaid 문법 구조**, 표의 행·열 구조와 값 셀(기본값·필드명), 링크의 **경로 부분**. 동결 예외(번역 대상): mermaid 노드·엣지 라벨(R2), 코드 펜스 안의 한국어 산문 주석, 표의 서술 셀, 번역 대상 파일 안을 가리키는 프래그먼트 앵커(번역된 헤딩의 GitHub 슬러그로 재생성).
- R2. mermaid 다이어그램 4종의 노드·엣지 라벨과 캡션도 영어로 한다. 캡션 형식은 `*Figure N. <claim>*`.
- R3. 번역은 축자 대응이다 — 새 주장·새 예제·새 함의 추가 금지, 정보 손실 금지. FAQ 질문의 구어체 톤은 영어 구어체로 이관.
- R4. 영어 AI 티를 제거한다(ce-noslop 기준): 기계적 3중 병렬, 과잉 수식어, 상투구, em-dash 남발. 한국어판의 '—' 하우스 스타일은 영어판에서 대시·콜론·괄호로 자연스럽게 풀되 과잉 밀도는 피한다.
- R5. 용어는 용어집(Planning Contract)으로 통일한다.
- R6. 커밋 전 검증을 전부 통과한다(Verification Contract).

### Success Criteria

- 영어 독자가 README → 사용 가이드만으로 설치·동작 확인을 마칠 수 있다.
- 코드 블록·링크 대상·표 구조의 한국어판 대비 차이가 0건이다.
- ce-noslop·mermaid 문법·링크 순회가 모두 통과한다.

### Scope Boundaries

- CLAUDE.md(에이전트 지침 — 한국어 팀 규약), docs/plans/·solutions/·testing·verification·bench·verdict·bakeoff(기록·판정 문서), 코드·설정 파일은 제외.
- 링크 대상 중 기록·판정 문서(verdict·bench·testing·verification·solutions)는 한국어로 남으며, 번역된 문서에서 이들로의 링크는 유지된다(허용 — 범위 밖 문서는 번역하지 않는다).
- 한국어판의 별도 보존 파일을 만들지 않는다 — git 이력이 보존 역할을 한다(사용자 확정).

---

## Planning Contract

### Key Technical Decisions

- KTD1. **번역은 축자 대응, 체인은 경량화** — 원문은 de-review·humanize를 통과한 확정본이므로 재집필 게이트(de-work)를 다시 돌리지 않고, 번역 + ce-noslop(영어 AI 티) + 기계 검증만 거친다. 한국어 전용 도구(de-review·humanize-korean·check_figures의 캡션 정규식)는 영어판에 적용 불가 또는 불필요하다.
- KTD2. **용어집 고정** — 변종=variant, 저장 게이트=storage gates, 세대=generation, 주체 키 분리=principal key isolation(기존 solutions 문서 영어명), 재생 비교=parity replay, 스탬피드=cache stampede, 단락=short-circuit, 판정=요청 문맥 request evaluation / 상태 문맥 decision. 모든 번역 콜이 같은 용어집을 받는다.
- KTD3. **그림 검증 분리** — verify_figures.py(mermaid 문법, 언어 중립)는 그대로 쓰고, check_figures.py(캡션 짝짓기)는 캡션 정규식이 한국어 `그림 N.` 고정이라 영어 캡션을 못 본다 — 캡션 짝짓기·번호 연속·본문 참조는 수동 점검으로 대체한다. 두 스크립트 모두 저장소 밖 플러그인 캐시에 있다: `/Users/keaton/.claude/plugins/cache/oh-my-skills/doc-experts/08a6f19d7f9d/de-visualize/scripts/`.
- KTD4. **커밋 메시지는 한국어 유지** — 영어 문서에 대해서도 Conventional Commits 한국어(팀 규약). 문서 하나가 하나의 논리 커밋.

### Assumptions

- HEAD `142a4f7` 이후 문서 변경이 없다(번역 시작 전 재확인).
- 번역 서브에이전트는 원문을 파일에서 직접 읽는다(브리핑에 용어집·문체 규칙만 실어 보낸다).

---

## Implementation Units

### U1. 번역 브리핑 고정

**Goal:** 모든 번역 콜이 같은 지시를 받는다.
**Requirements:** R3, R4, R5
**Files:** (브리핑 텍스트 — 실행 세션에서 유지)
**Approach:** 용어집(KTD2) + 문체 규칙(plain technical English, 선언형, 축자 대응, AI 티 금지 목록) + 무변경·예외 목록(R1의 동결 요소와 동결 예외)을 표준 브리핑으로 만든다. 헤딩 `Principal key isolation — credential_vary`와 그 슬러그 앵커를 브리핑에 고정 값으로 싣는다.

### U2. README.md + docs/how-it-works.md

**Goal:** 저장소 첫 화면과 동작 문서가 영어가 된다.
**Requirements:** R1, R2, R5
**Files:** `README.md`, `docs/how-it-works.md`
**Approach:** 그림 3종(README 코어/어댑터, how-it-works 수명주기·저장 게이트)의 라벨 번역 포함. README의 H1 CI 배지 링크는 무변경, 표 2종은 구조 무변경·서술 셀만 번역(U3의 표 셀 규칙과 동일). how-it-works의 `### 주체 키 분리 — credential_vary` 헤딩은 `### Principal key isolation — credential_vary`로 고정 번역한다(troubleshooting의 앵커가 이 슬러그에 의존).

### U3. docs/configuration.md + docs/troubleshooting.md

**Goal:** 레퍼런스와 문제 해결 가이드가 영어가 된다.
**Requirements:** R1, R2, R3, R5
**Files:** `docs/configuration.md`, `docs/troubleshooting.md`
**Approach:** 표 셀의 값(기본값·필드명)은 무변경, 서술 셀만 번역. troubleshooting 그림 1종 라벨 번역. FAQ 질문행은 영어 구어체로.

### U4. 사용 가이드 3종

**Goal:** 변종별 사용 가이드가 영어가 된다.
**Requirements:** R1, R5
**Files:** `docs/usage-kong-go.md`, `docs/usage-kong-lua.md`, `docs/usage-apisix.md`
**Approach:** 병렬 처리. 설치 절차의 env·경로·명령은 무변경, 코드 주석은 영어로.

### U5. CHANGELOG.md + CONCEPTS.md

**Goal:** 릴리스 노트와 용어집이 영어가 된다.
**Requirements:** R1, R5
**Files:** `CHANGELOG.md`, `CONCEPTS.md`
**Approach:** CHANGELOG은 Unreleased 전체(기존 한국어 항목 포함)를 번역. CONCEPTS는 영어 표제어 + 한국어 원어 괄호 병기 허용, `*Avoid:*` 주석은 영어로 번역하되 금지 지시 유지.

### U6. 검증·마감

**Goal:** 영어판이 모든 검증을 통과하고 저장소에 안착한다.
**Requirements:** R6
**Files:** U2-U5 산출 9개 파일
**Approach:** 순서 — (1) 한국어판(`git show 142a4f7:<path>`)과 diff로 동결 요소(R1 목록) 무변경 확인 (2) ce-noslop 점검 (3) verify_figures.py mermaid 문법 (4) 캡션 짝짓기·번호·본문 참조 수동 점검(KTD3) (5) 링크 순회 — 경로 존재 + 프래그먼트 앵커를 헤딩 슬러그(GitHub 규칙)와 대조, 0 broken (6) 유닛별 논리 커밋(KTD4), push는 사용자 지시 후.

---

## Verification Contract

- **무변경 대조**: 각 문서의 R1 동결 요소(명령·설정값·경로·환경변수·mermaid 문법 구조·표 값 셀·링크 경로)가 `git show 142a4f7:<path>` 원문과 바이트 동일.
- **mermaid**: verify_figures.py로 4종 문법 통과(라벨 영어화 이후).
- **링크**: 순회 스크립트 0 broken.
- **AI 티**: ce-noslop 통과.
- **게이트 영향 없음**: 산출물이 *.md뿐(build.yml `!*.md` 제외).

## Definition of Done

- 9개 파일의 영어 재작성 완료 — 한국어 잔존 없음(제외 파일, R1 동결 요소 중 번역 불가한 것 없음, U5가 허용한 CONCEPTS 한국어 원어 괄호 병기는 제외)
- 무변경 대조·mermaid·링크·noslop 검증 전부 통과
- 유닛별 논리 커밋(Conventional Commits 한국어), push는 사용자 최종 지시 후
- 코드·설정·CLAUDE.md·기록 문서 변경 없음
