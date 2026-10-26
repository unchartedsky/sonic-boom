# Changelog

## Unreleased

### Added

- **개인화 응답 미저장 게이트** — `Set-Cookie` 응답과 `Cache-Control: private/no-store/no-cache` 응답을 저장하지 않는다(G9). 해당 응답은 `X-Cache-Status: Bypass`로 통과한다(APISIX proxy-cache 기본값 정합, Lua 포팅과의 명시 차이 해소). Lua판에 없던 `no-cache` 검사도 추가됐다.

### Changed

- **요청층 `cache_control` 가드 기본 on** — 이전 동작(가드 끔, Authorization 요청도 캐시)에서 바뀐다. `Cache-Control: no-store/no-cache` 요청과 `Authorization` 요청이 기본으로 캐시에서 제외된다(RFC 9111·Kong 원본 정합, G8 기본 무방비 해소). 이전 동작이 필요한 라우트는 `cache_control: false`를 명시한다. Lua판(Kong·APISIX)은 이 필드가 미선언이라 가드 자체가 없다 — 포팅 전까지 명시 차이.
- **캐시 키 직렬화 v2** — 해시를 xxhash64(비암호, 2^32 충돌)에서 sha256(64-hex)으로 바꾸고, 쿼리 다중 값을 NUL join 대신 값 단위 길이 접두로 직렬화해 `?x=a%00b`(단일 값)와 `?x=a&x=b`(다중 값)의 키 충돌을 제거한다. 구 형식 키는 새 키와 절대 겹치지 않아 재조회되지 않으므로, TTL 만료까지 잔존 후 자연 소멸한다(버전 마커 불필요).
- **`cache_control` 요청 가드 동작 확정** — 기존에는 첫 요청만 저장하고 이후 Bypass했으나, 이제 Kong 원본 의도의 요청 가드로 동작한다(기본값은 아래 항목 참고). 요청 `Cache-Control: no-store/no-cache`와 `Authorization` 헤더는 캐시에서 제외되고, 나머지 요청은 정상 캐시된다(G8 완화). **운영 주의**: 이 플래그를 켠 기존 배포는 응답이 실제로 캐시·재생되기 시작한다.

### Fixed

검증된 결함 9종 수정(전체 근거: `docs/bakeoffs/2026-10-18-api-gw-caching-gap-analysis.md`, 계획: `docs/plans/2026-10-18-2126-fix-verified-cache-bugs-plan.md`).

- **OTel 활성 시 Response 저장 경로가 항상 조기 중단되던 결함 수정** — `span_context`를 `kong.ctx.shared`로 그대로 저장하면 JSON 마샬 왕복 뒤 타입 단언이 항상 실패해, `OTEL_SDK_DISABLED != "true"`(실배포 기본값)에서 아무것도 저장되지 않고 fill-lock 승자의 락이 TTL까지 방치됐다. 직렬화 가능한 DTO로 전달하도록 바꾸고, 복원 실패 시 저장을 중단하지 않고 부모 없이 계속한다.
- **필터 `cache_ttl`이 실제 만료에 적용** — 저장·stale 판정이 전역 값을 쓰던 것을 필터별 TTL로. 미설정(0) 필터 TTL은 전역 `cache_ttl`을 상속(명시적 0도 상속을 의미).
- **만료·세대 불일치 캐시의 낡은 값 서빙 차단** — 버전 불일치 purge·stale 판정 뒤 upstream으로 통과(원본 Kong Lua 시맨틱 복원)하고, 재저장 경로를 개통.
- **`cache_ttl` 기본값 0→300, 명시적 0 거절** — TTL 0은 Redis에 만료 없는 키를 남기고 절대 히트하지 못했다.
- **설정 오류·패닉이 프로세스를 종료하지 않음** — `cache_control: true` 히트 경로의 Fatal(os.Exit) 제거, 설정 검증 상시화(실패 시 Error+Bypass), Response 페이즈 recover 추가.
- **`content_type` 목록이 게이팅을 지배** — 미일치 content type은 캐시하지 않는다(기존: 폴스루로 전부 캐시).
- 죽은 코드·죽은 검사 제거(reqBody 파이프라인, responseData 블록, `validate.Struct(conf)` 오대상, no-op 필터 TTL 루프).

### 운영자 주의사항 (릴리스 노트)

- (a) **전면 콜드 캐시**: TTL이 캐시 키 해시에 포함되어, 실효 TTL이 바뀌는 라우트의 키 공간이 전부 갱신된다.
- (b) **기존 TTL 0 영구 키 잔존**: 이전 배포가 만든 만료 없는 키는 참조되지 않은 채 Redis에 남는다. 정리는 `SCAN` 후 `PTTL == -1`인 키를 판별해 `DEL`(또는 db 교체 시 `FLUSHDB`).
- (c) **`cache_ttl` 기본값 변화**: 미설정 배포의 동작이 300초 만료로 바뀐다.
- (d) **content_type 정확 매치**: 목록 밖 content type(파라미터 변형 포함, 예: `application/json;charset=utf-8` 공백 없음)은 이제 Bypass된다. 필요한 변형을 목록에 추가하라.
- (e) **만료·세대 불일치 히트의 upstream 전환**: 이전에는 낡은 값을 `Hit` 라벨로 서빙했다면, 이제 upstream 재검증(Refresh/Bypass) 후 재저장된다. TTL 경계의 히트율 하락·upstream 부하 증가는 의도된 정합화다.
