# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Sonic Boom

Kong Gateway(3.9.x OSS)용 go-pdk external plugin 응답 캐시 플러그인. Kong 공식 proxy-cache Lua 플러그인을 Go로 포팅했다. in-memory(ristretto)·redis·redis-cluster 세 캐시 전략을 지원한다.

## Reporting

Write every report, summary, or handoff to the user through the `ce-noslop` skill. This applies when you are the top-level agent writing to the user, not when you are a subagent reporting to its caller. Do not apply it to code, config, verbatim quotes, or text the user asked to post as written.

## Commands

```bash
make build   # bin/linux-${GOARCH}/sonic-boom 크로스컴파일(build.sh 위임)
make test    # 통합 테스트 — compose로 redis·redis-cluster 기동 후 golang 컨테이너에서 go test -race
make e2e     # 플러그인 빌드 + compose clean 기동(down -v) + 세 전략 스모크(공통 6단언 + gzip suite)
make e2e-lua # Lua 포팅(kong-lua) e2e — profile lua 기동(포트 8010) 후 smoke_lua_*.sh 3종, 스모크가 스스로 캐시를 비운다
make e2e-apisix # APISIX 어댑터 e2e — profile apisix 기동(포트 8020) 후 같은 스모크 3종을 GW=apisix로
make stress  # 스탬피드·CSC 실측 시나리오 + 자동 단언(기대 미달 시 exit 1)
```

- 서버 없이 단위 테스트만: `go test ./internal -run 'Test_InMemory|TestConfig|Test_'`
- 단일 테스트: `go test ./internal -run TestName`(서버가 필요한 통합 테스트는 `make test` 경유)
- 린트: `golangci-lint run ./...`(pre-commit 훅 등록, `golangci-lint run --fix`)
- e2e는 외부 인터넷 없이 hermetic하게 돈다(목업 업스트림 `test/upstream/`). 첫 요청 Miss 단언의 전제 때문에 `make e2e`는 시작 전 `down -v`로 캐시를 비운다.
- 커밋은 Conventional Commits + 한국어(`feat:`, `fix:` 등).

## Architecture

- **두 구현**: 같은 플러그인의 Go판(`main.go`+`internal/`, go-pdk external plugin 서버)과 Lua판(`lua/`)이 있다. Lua판은 게이트웨이 중립 코어(`lua/sonic_boom/`, 요청 상태기계는 `flow.lua`) + 게이트웨이 어댑터 둘(Kong `lua/kong/plugins/sonic-boom-lua/`, APISIX `lua/apisix/plugins/sonic-boom-lua.lua`)로 나뉜다. 어댑터는 PDK 게터·세터(env)·ctx 저장·단락 방식만 담당하고, 제품 식별(로그 라벨·키 접두 sbl/sba)은 `product.lua`로 주입한다. APISIX 판정 자료는 `docs/apisix-verdict.md`. PoC 시점의 Go판 대비 명시 차이는 `docs/kong-lua-verdict.md`에 기록돼 있고, 그중 Set-Cookie·Cache-Control 디렉티브 미저장 차이는 U1·U2로(양쪽 모두 무조건 미저장), fill-lock 부재는 U4로 해소됐다. 남은 차이: cache_control 스키마 거부(Lua), RE2→PCRE, 저장 시점 비동기(redis/cluster).
- **요청 흐름**: Access 페이즈(`Config.Access`)가 캐시 키를 만들고 캐시를 조회해 Hit면 즉시 응답하고, 아니면 캐시 시그널(`CacheSignal` — 캐시 키+TTL)을 ctx에 남긴다. Response 페이즈(`Config.Response`)는 시그널이 남아 있을 때만 저장을 시도한다. `X-Cache-Status`는 Miss/Hit/Bypass/Refresh 4상태다.
- **캐시 키**(`cache_key.go`): Go는 service/route/consumer/method/vary 헤더/본문 등을 값 단위 길이 접두 직렬화한 뒤 sha256 64-hex를 키로 쓴다(접두 없음). `cache_version`은 키에 없고 Hit 시점 비교로, 불일치면 purge한다. 두 Lua 변종은 `{sbl|sba}:{cache_version}:{sha256}` 접두로 세대 분리한다.
- **전략**(`newCacheManager`): gocache 저장소 3종을 구성한다. in-memory는 ristretto(비동기 쓰기), redis·redis-cluster는 설정 값 키로 프로세스 수명 공유되는 클라이언트를 쓴다. 설정 하나는 정확히 하나의 전략을 쓴다.
- **필터**: 경로·헤더 정규식 규칙(`Filter`/`Rule`, 정규식은 설정 시점에 컴파일)으로 캐시 대상과 TTL을 좁힌다. 어느 필터도 맞지 않으면 캐시에서 제외된다. 필터 TTL을 생략하면 전역 `cache_ttl`을 상속한다(즉시 만료 필터는 없다).
- **fill-lock**(`fill_lock.go`): 동시 미스 시 스탬피드 방지. 토큰 기반 락 획득 + `waitForFill` 대기.
- **설정 검증은 운영에서도 상시**다. 검증 실패 시 프로세스를 죽이지 않고 `X-Cache-Status: Bypass`로 통과한다(fail-fast는 개발 단계 기준).

## Runtime Constraints

go-pdk external plugin 서버는 핸들러 패닉에 recover가 없다. 근거와 전체 규율은 `docs/solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md` 참조. 위반은 e2e가 초록이어도 운영에서 터진다:

- 페이즈 콜백 안에서 패닉·`zerolog.Fatal()` 금지 — 패닉 = 프로세스 전체 사망. recover defer + Error 강등 + Bypass 폴백이 플러그인 쪽 유일한 방어선이다.
- 설정은 불변 취급 — 인스턴스 하나가 여러 요청을 동시에 처리하고 수명주기 훅이 없어 설정 초기화는 요청마다 실행된다. 상태 재할당·전역 변이 금지, 공유는 설정 값 키 sync.Map이나 sync.Once로.
- TTL 0 금지 — gocache redis store에서 TTL 0은 영구 키다. `cache_ttl` 기본 300, 0은 검증에서 거절.
- Lua 포팅 시 `return` 시맨틱 보존 — 원본 Lua의 `return`은 "상태 기록 후 계속 진행"이다. 버전 불일치·만료 판정 후 이걸 빼먹으면 방금 purge한 낡은 값을 `Hit`로 덮어써 서빙한다.

## Testing

3층 구조(단위 → 통합 → e2e). **`docs/testing.md`가 테스트 SSOT** — 서버가 필요한 테스트는 스킵이 아니라 실패(fail-fast). ristretto 비동기 쓰기, 설정 값 키 저장소 공유로 TTL이 고정되는 문제 같은 알려진 특성과 새 캐시 전략 추가 절차는 testing.md에 정리돼 있다. Lua판 e2e는 `make e2e-lua`(Kong)·`make e2e-apisix`(APISIX), 재생 비교는 `scripts/parity_replay.sh`(redis 전략 기준, `PARITY_PAIR=go-lua`|`lua-apisix`). CI는 게이트웨이 × 아치 단위로 잡을 분리하는 규약이 있다(docs/testing.md "CI 잡 경계" — Go·Kong-Lua·APISIX가 각자 smoke·container 잡을 갖고, 재생 비교는 쌍별 잡). 팀 용어는 `CONCEPTS.md`, 누적된 학습은 `docs/solutions/`에 있다.
