# Concepts

> Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## 플러그인 런타임

### 외부 플러그인 서버
Kong 본체와 별도 프로세스로 떠 페이즈 콜백만 수신하는 sonic-boom의 실행 모델. *Avoid:* 사이드카.

인스턴스는 여러 요청을 동시에 처리하고 인스턴스 수명주기 훅이 없어, 설정 초기화는 요청마다 실행된다. 핸들러 패닉에 대한 런타임 쪽 방어가 없어 플러그인 쪽 recover가 유일한 방어선이다.

### 캐시 전략
저장소를 고르는 축 — in-memory(노드 로컬), redis(단일), redis-cluster(샤딩). 하나의 플러그인 설정은 정확히 하나의 전략을 쓴다.

### 캐시 시그널
Access 페이즈가 Response 페이즈로 캐시 키와 TTL만을 전달하는 요청 범위 메시지. 시그널이 남아 있으면 Response가 저장을 시도하고, 없으면 저장하지 않는다.

## 캐시 시맨틱

### 필터
경로·헤더 정규식 규칙으로 캐시 대상을 좁히고 규칙별 TTL을 주는 sonic-boom 고유 확장. 어느 필터도 맞지 않으면 캐시에서 제외된다.

### X-Cache-Status
캐시 결과 4상태 표기 — Miss(미스로 저장 예정), Hit(캐시로 응답), Bypass(캐시 대상 아님 또는 세대 불일치), Refresh(만료 감지).

### cache_version
캐시 세대 마커. 값을 바꾸면 이전 세대 캐시가 무효가 되고, 변경 경로는 설정 리로드다(env 경로는 프로세스 환경에 고정).

### 주체 키 분리
인증 주체(consumer·principal)를 캐시 키에 반영해 사용자별 캐시 네임스페이스를 만드는 정책. 확정된 주체의 무조건 반영이 기본이고, 그 위에 얹는 선택은 주체 없는 인증 요청의 저장 거부다. *Avoid:* consumer isolation, principal isolation — 상류 제품 옵션명(APISIX `consumer_isolation`, Kong `cache_by_principal`)을 sonic-boom 정책어로 쓰지 않는다.
