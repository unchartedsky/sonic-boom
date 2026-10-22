# Changelog

## Unreleased

### Added

- **`Set-Cookie` 응답 미저장** — 개인화 응답 오염 방지(G9). Set-Cookie 응답은 더 이상 캐시에 저장·재생되지 않고 `X-Cache-Status: Bypass`로 통과한다(APISIX proxy-cache 기본값 정합, Lua 포팅과의 명시 차이 해소).

### Fixed

검증된 결함 9종 수정(전체 근거: `docs/bakeoffs/2026-10-18-api-gw-caching-gap-analysis.md`, 계획: `docs/plans/2026-10-18-2126-fix-verified-cache-bugs-plan.md`).

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
