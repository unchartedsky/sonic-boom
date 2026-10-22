---
title: APISIX Lua 플러그인 — 코어 상태기계 이전 + APISIX 어댑터 - Plan
type: feat
date: 2026-10-22
artifact_contract: ce-unified-plan/v1
execution: code
---

# feat: APISIX Lua 플러그인 — 코어 상태기계 이전 + APISIX 어댑터

## Context

`docs/kong-lua-verdict.md`의 판정 안건 ③은 "APISIX 확장 착수 — 코어 재사용 전제로 어댑터 착수 승인 여부"다. Kong-Lua PoC가 코어(`lua/sonic_boom/`)를 `kong.*` 참조 0건으로 분리해 뒀다. 다만 판정 문서 이식성 행이 짚었듯 어댑터:코어 비율이 ~1:1이다. `handler.lua` 386줄의 대부분은 게이트웨이 중립 페이즈 로직이다. 시그널 수명, 저장 판정, 청크 버퍼링, fill-lock 해제, log 타이머가 여기에 든다. 그래서 그대로 복제하면 같은 로직이 두 벌이 된다. 지난 리뷰에서 나온 `release_only`·`sync` 같은 결함도 양쪽에서 따로 고쳐야 한다.

이 계획은 페이즈 로직을 코어 상태기계(`flow.lua`)로 끌어올린다. 그 뒤 APISIX 3.19 어댑터를 얇은 PDK 셰임으로 붙인다. 검증은 Kong-Lua PoC와 같은 깊이로 한다. 3전략 스모크, Kong-Lua↔APISIX 재생 비교, CI 3잡, 벤치, 판정 문서를 모두 포함한다. 결과물은 안건 ③·④의 판정 근거가 된다.

선행 계획: `docs/plans/2026-10-21-1136-feat-kong-lua-port-plan.md`(Kong-Lua PoC). 판정 근거: `docs/kong-lua-verdict.md`.

## Key Technical Decisions

- **KTD1. 코어로 끌어올리기** (session-settled: user-directed). `handler.lua:21-384`의 페이즈 로직을 `lua/sonic_boom/flow.lua`로 옮긴다. 변경은 `kong.X → env.X` 치환만 허용한다(기계적 이동). 어댑터는 env 테이블(게터·세터)과 ctx 저장소, 단락 방식만 소유한다.
- **KTD2. 검증 깊이 = Kong-Lua PoC와 동일** (session-settled: user-directed). 3전략 스모크, 재생 비교, CI 3잡, 벤치, 판정 문서를 포함한다.
- **KTD3. 제품 식별은 `lua/sonic_boom/product.lua` 세터로 둔다.** 필드는 `{label, key_prefix, version}`이고, 어댑터가 모듈 로드 시 1회 덮어쓴다. 코어의 하드코딩 4곳(`memory.lua`, `redis.lua:5`, `rediscluster_store.lua:6`, `fill_lock.lua:8`)은 로그 라벨을 **호출 시점**에 읽는다. load-time 업밸류로 캡처하지 않으므로 로드 순서에 의존하지 않는다. shdict 이름 `sonic_boom_cache`·`redis_cluster_slot_locks`는 그대로 둔다. 게이트웨이마다 자기 shdict가 있기 때문이다.
- **KTD4. 키 네임스페이스 분리.** `key.derive(k, version, prefix)`가 `(prefix or "sbl") .. ":" .. version .. ":" .. sha256`을 반환한다. 접두어는 해시 입력에 들어가지 않는다. Kong은 `sbl`, APISIX는 `sba`를 쓴다. 그래서 Kong 골든 벡터는 불변이다. 락 키(`sb-lock:..key`)도 함께 갈리므로 공유 redis에서도 교차 서빙이 없다. 재생 비교 잡 자체도 이 분리로 보호된다.
- **KTD5. APISIX 플러그인 식별.**
  - 이름은 `sonic-boom-lua`, 파일은 `lua/apisix/plugins/sonic-boom-lua.lua`다. Kong 어댑터와 같은 제품(Lua 코어)이고, 라벨과 키 접두어로 구분된다.
  - 우선순위는 99다. 권장 대역 (0,100)의 상단이라 Hit이 다른 커스텀 플러그인보다 먼저 단락한다. 인증은 rewrite 단계에서 consumer를 확정하므로 consumer_name이 이미 있다.
  - 로그 라벨은 `product=apisix`, ctx 키는 `ctx.sonic_boom_signal`이다. APISIX의 ctx는 전 플러그인이 공유하므로 고유 키가 필요하다.
- **KTD6. APISIX 단락과 헤더 규칙.**
  - Hit은 access에서 `return status, body`로 단락한다. `core.response.exit`는 쓰지 않는다. 헤더는 반환 전에 `core.response.set_header`로 세팅한다.
  - **Hit에는 `Content-Length = #body`를 직접 넣는다.** `clean_for_replay`가 이 헤더를 떼어 내고, Kong은 exit가 재계산하지만 APISIX는 그렇지 않다. `smoke_lib.sh:90`의 Content-Length 단언이 이를 잡는다.
  - **응답 헤더는 `ngx.resp.get_headers()`를 기본 인자로 읽는다.** raw 모드는 대소문자를 보존해 `hdrs["set-cookie"]` 게이트가 깨진다.
- **KTD7. 스키마는 JSON schema로 1:1 번역한다.**
  - 모든 객체에 `additionalProperties=false`를 둔다. 그래서 `cache_control`·`log` 같은 미구현 필드는 Kong-Lua와 같이 거부된다.
  - `fill_lock`·`redis`는 기본 객체를 완전히 명시한다. 중첩 기본값이 비어 있는 객체 안에서 적용되지 않을 수 있기 때문이다.
  - `strategy=redis-cluster`일 때는 `if/then`으로 `redis_cluster.addrs`를 필수로 만든다. Kong-Lua보다 엄격하므로 명시 차이로 기록한다.
- **KTD8. APISIX 배포 형태.**
  - 이미지는 `apache/apisix:3.19.0-debian`이고 standalone(`data_plane` + yaml provider)로 띄운다. `config_apisix.yaml`은 `#END`로 끝낸다.
  - `extra_lua_path: /opt/sonic-boom/?.lua` 하나로 코어와 플러그인 트리를 모두 덮는다.
  - `plugins:` 목록은 `[proxy-rewrite, sonic-boom-lua]`만 둔다. 의도적으로 최소화해 드리프트가 없게 했다.
  - 라우트 경로는 `config_lua.yml`과 같은 `/lua-*`를 쓰고, `proxy-rewrite regex_uri`로 strip_path를 흉내 낸다.
  - 호스트 포트는 8020→9080이다.

## Implementation Units

### U1. 코어 제품 식별 파라미터화
- **Files:** `lua/sonic_boom/product.lua`(신규), `lua/sonic_boom/key.lua`, `lua/sonic_boom/fill_lock.lua`, `lua/sonic_boom/store/{memory,redis,rediscluster_store}.lua`, `lua/kong/plugins/sonic-boom-lua/handler.lua`(product 세팅), `lua/sonic_boom/selfcheck.lua`
- **Approach:** KTD3·KTD4를 적용한다. Kong 어댑터가 `product.label = "sonic-boom-lua product=kong-lua: "`로 설정해 출력은 그대로다.
- **Test scenarios:** 기존 selfcheck 골든 벡터는 불변이다. `key.derive(input, "0.1.0", "sba") == "sba:0.1.0:"..hash` 벡터 1건을 추가한다.
- **Verification:** `/selfcheck` OK, `make e2e-lua`와 parity가 초록이어야 한다. `grep -rn 'product=kong-lua' lua/sonic_boom`은 0건이어야 한다.

### U2. 페이즈 로직을 코어 `flow` 상태기계로 이전 — 최대 회귀 위험
- **Files:** `lua/sonic_boom/flow.lua`(신규), `lua/kong/plugins/sonic-boom-lua/handler.lua`(약 50줄의 셰임으로 축소)
- **API:**
  - `flow.new_store(conf)`
  - `flow.cache_version(conf)`: 폴백은 `product.version`
  - `flow.access(conf, env, ctx) -> exit|nil, sb|nil`: exit는 `{status, body, headers}` 형태로 selfcheck 결과나 Hit 재생을 담는다
  - `flow.header_filter(conf, sb, env) -> sb'`
  - `flow.body_filter(conf, sb, env) -> sb'`: `ngx.arg`를 직접 읽는다
  - `flow.log(conf, sb)`
- **env 테이블:** 정적 테이블이고 per-request ctx는 인자로 받는다. 필드는 다음과 같다.
  - 요청: `method`, `req_header(ctx,h)`, `path(ctx)`, `path_with_query(ctx)`, `raw_body(ctx)`
  - 식별자: `ids(ctx) -> consumer, service, route`
  - 응답: `set_header`, `status`, `resp_header`, `resp_headers`
  - 로그: `log`

  Kong 쪽 엔트리는 클로저로 둔다. 로드 시점에 PDK 서브테이블이 아직 없을 수 있기 때문이다.
- **Approach:** fill-lock 시그널 모양을 바이트 단위로 보존한다. 대상은 승자 `{key,ttl,lock_token}`, 패자 `{key,ttl}`, filled의 Hit 진입, release_only, 타이머의 저장→해제다.
- **Guards:**
  - `git diff --color-moved=zebra`로 리뷰한다. 이동된 코드에서 허용되는 재작성은 아래 목록뿐이고, 목록 밖의 변경이 섞인 hunk는 리뷰에서 반려한다.
    1. `kong.X(...)` → `env.X(ctx, ...)`(per-request ctx 인자 추가 포함)
    2. `kong.ctx.plugin.sb_signal = V` → 같은 지점에서 V를 반환 시그널로 돌려줌
    3. `kong.response.exit(s, b, h)` → `return {status=s, body=b, headers=h}`
    4. consumer·service·route 조회와 `.id` 읽기 → `env.ids(ctx)`
    5. `PREFIX`와 핸들러의 `product=kong-lua` 리터럴(타이머 생성 실패 로그 포함) → 호출 시점에 읽는 `product.label`
    6. `plugin.new_store`·`plugin.VERSION` → `flow.new_store`·`product.version`
  - 2번 재작성 지점(release_only 전환, HEAD 동기 저장, 본문 상한 초과)마다 아래 Test scenarios의 어느 항목이 그 경로를 덮는지 대응시켜 둔다.
  - `make e2e-lua`(amd64/arm64)와 parity가 초록이어야 한다.
  - `grep -c 'kong\.' lua/sonic_boom/*.lua`는 0이어야 한다.
  - `grep -rn 'product=kong-lua' lua/sonic_boom`은 U2 이후에도 0건이어야 한다. 이동한 코드가 라벨 리터럴을 코어로 옮겨 오면 APISIX 로그가 kong-lua로 찍히기 때문이다.
- **Test scenarios:** 기존 e2e가 덮는 경로가 전부 초록이어야 한다. 승자·패자 타임아웃·filled, 비저장 응답과 본문 상한 초과 시의 release_only, HEAD(sync/async), stale Refresh, 버전 불일치 삭제가 해당한다.

### U3. APISIX 어댑터 + in-memory 전략 장착
- **Files:**
  - `lua/apisix/plugins/sonic-boom-lua.lua`(어댑터 + 스키마)
  - `Dockerfile.apisix`: `COPY lua/ /opt/sonic-boom/` 한 줄로 트리를 통째로 복사해 `sonic_boom/`·`apisix/plugins/` 하위 경로를 보존하고(Dockerfile.kong-lua와 같은 방식), config를 굽는다. 디렉터리 내용물만 복사하면 경로가 평탄화돼 `require "sonic_boom.key"`와 플러그인 로드가 깨진다
  - `conf/apisix-config.yaml`: data_plane 설정, `custom_lua_shared_dict` 2종, status API 7085, `plugins` 최소 목록, `nginx_config.envs: [CACHE_VERSION]`
  - `config_apisix.yaml`: `/noplugins`, `/lua-*` in-memory 라우트, `/selfcheck`, `#END`
  - `docker-compose.yaml`: `apisix` 서비스, profile apisix, 8020:9080, cpus 2.0, healthcheck(`/status/ready`, start_period 60s), depends_on은 kong-lua와 같다
  - `scripts/common.sh`: `GW=kong-lua|apisix`를 받아 `GW_SVC`·`GW_PROFILE`·`GW_BASE`로 바꾸는 헬퍼
  - `scripts/smoke_lua_in_memory.sh`: GW 변수로 파라미터화
- **Approach:** KTD5·KTD6·KTD7·KTD8을 적용한다. 착수 전 선결 확인이 세 가지 있다.
  - `docker manifest inspect apache/apisix:3.19.0-debian`으로 arm64 존재를 확인한다.
  - 이미지 내 `curl` 유무를 확인한다. 없으면 `/dev/tcp` 헬스체크로 바꾼다.
  - `core.response.set_header`의 map 형식 지원 여부를 확인한다. 미지원이면 키별로 루프를 돈다.
- **Test scenarios:**
  - `:8020/selfcheck/x`가 200이어야 한다. 이것이 이미지 내 `resty.openssl.digest`·`cjson.safe`의 런타임 증거다.
  - `GW=apisix ./scripts/smoke_lua_in_memory.sh`의 공통 단언, gzip(Content-Length 포함), 인코딩 스위트, ctgate·setcookie·maxsize가 통과해야 한다.
  - X-Cache-Key 접두어가 `sba:`여야 한다.
  - 스키마 거부: `cache_control: true` 라우트가 로드를 거부해야 한다.
- **Verification:** 위 스모크가 초록이어야 하고 `make e2e-lua`는 무회귀여야 한다.

### U4. APISIX redis·redis-cluster 전략 + `make e2e-apisix`
- **Files:** `config_apisix.yaml`(redis·cluster·nolock·stampede·cluster-stampede·filter 라우트), `scripts/smoke_lua_redis.sh`·`scripts/smoke_lua_rediscluster.sh`(GW 변수, `docker compose logs ${GW_SVC}`), `Makefile`(`e2e-apisix` 타깃, 서비스 명시: `apisix mock-upstream redis cluster-init`, `.PHONY`)
- **Test scenarios:**
  - `HIT_RETRY_TIMEOUT`을 둔 Miss→Hit
  - redis 중단 시 fail-open과 "redis connect failed" 로그
  - fill-lock 4병렬 버스트에서 업스트림 2회 이하
  - 클러스터 SET NX 왕복과 클러스터 버스트(이미지 내 `resty.lock` 런타임 증거)
- **Verification:** `make e2e-apisix`와 `make e2e-lua`가 모두 초록이어야 한다.

### U5. Kong-Lua↔APISIX 재생 비교
- **Files:** `scripts/parity_replay.sh`
- **Approach:** `PARITY_PAIR=go-lua|lua-apisix`를 추가하고 기본값은 현행을 유지한다. lua-apisix 모드의 동작은 다음과 같다.
  - A는 :8010, B는 :8020이고 양쪽 모두 같은 `/lua-*` 경로를 쓴다.
  - 리셋 블록은 FLUSHALL 후 kong-lua와 apisix를 재시작한다.
  - 워밍업과 S9 경로는 `/lua-redis`로 바꾼다.
  - Go 전용인 S13은 스킵한다.

  변경은 약 10줄이라 별도 lib으로 뺄 이득이 없다.
- **Test scenarios:** S1~S12가 S13 제외하고 양쪽 일치해야 한다. 시퀀스·status·body·Age 증가를 비교한다.
- **Verification:** 두 모드 모두 exit 0이어야 한다.

### U6. CI 3잡
- **Files:** `.github/workflows/ci.yml`, `.github/workflows/build.yml`
- **Approach:** testing.md 규약의 잡 이름을 그대로 쓴다.
  - `smoke-test-apisix`: amd64/arm64 매트릭스로 `make e2e-apisix`를 돌린다. arm64 이미지가 없으면 매트릭스에서 뺀다.
  - `parity-lua-apisix`: Go 빌드 없이 `--profile lua --profile apisix up`으로 서비스를 명시해 띄우고 `PARITY_PAIR=lua-apisix`로 돌린다. 실패 시 진단 단계를 둔다.
  - `container-apisix`: container-lua를 복제해 `ghcr.io/unchartedsky/sonic-boom-apisix`와 `Dockerfile.apisix`로 바꾼다. PR에서는 스킵한다.
- **Verification:** push 후 신규 3잡(smoke-test-apisix, parity-lua-apisix, container-apisix)과 기존 ci.yml·build.yml 잡이 전부 초록이어야 한다.

### U7. 게이트웨이 벤치
- **Files:** `scripts/bench_gw.sh`(`BENCH_PAIR`, A/B URL 오버라이드, 기본값은 현행), `docs/bench-apisix-vs-kong-lua.md`(신규)
- **Approach:** 코어가 같으므로 Kong-Lua를 기준선으로 삼는다. HIT은 `/lua-in-memory`, MISS는 `/lua-maxsize`로 잰다. 두 게이트웨이는 기본 프록시 처리량부터 다르므로 원시 수치 비교만으로는 플러그인 비용을 가를 수 없다. 그래서 각 게이트웨이의 `/noplugins` 라우트를 같은 인터리브 회차에 함께 잰다. 판정은 "플러그인 라우트 대 자기 게이트웨이 `/noplugins`" 오버헤드 비율을 APISIX와 Kong-Lua 사이에서 비교해 내린다. 문턱은 측정 전에 선등록하며, Kong-Lua 판정과 같은 p99 회귀 20% 초과, RPS 저하 10% 초과다. APISIX와 Kong의 원시 수치는 참고 맥락으로만 따로 보고한다.

### U8. 판정 자료 + 문서 갱신
- **Files:** `docs/apisix-verdict.md`(신규, 6항목 대조와 명시 차이), `docs/testing.md`(GW·PARITY_PAIR·e2e-apisix·포트 8020), `docs/kong-lua-verdict.md`(이식성 행의 "APISIX 미검증" 갱신), `CLAUDE.md`(Commands·Architecture), `README.md`
- **명시 차이:** APISIX를 Kong-Lua와 비교한 차이다.
  - `lua_regex_match_limit`이 100000 대 10000이다. APISIX는 이 값을 하드코딩해 변경할 수 없다.
  - redis-cluster addrs를 스키마 단계에서 필수로 강제한다(더 엄격).
  - 파일로 버퍼링된 큰 요청 본문을 읽는다. 그래서 Kong-Lua에선 통과하던 큰 POST가 캐시 대상이 된다.
  - 경로 정규화가 다르다. 이 차이는 키에만 영향을 주고 parity에는 무관하다.

## 의존 순서

U1 → U2 → U3 → U4 → (U5 ∥ U6) → U7 → U8. 각 단위는 Conventional Commits 한국어 커밋 하나로 낸다.

## Verification Contract

```bash
make test                                            # Go 무회귀
make e2e                                             # Go 무회귀
make e2e-lua                                         # Kong-Lua 무회귀(U1·U2 가드)
make e2e-apisix                                      # APISIX 3전략 + fill-lock 버스트 + SET NX
docker compose --profile lua up -d --build --wait && ./scripts/parity_replay.sh               # Go↔Kong-Lua
docker compose --profile lua --profile apisix up -d --build --wait kong-lua apisix mock-upstream redis cluster-init \
  && PARITY_PAIR=lua-apisix ./scripts/parity_replay.sh                                       # Kong-Lua↔APISIX
```

## Risks

1. **U2 코어 이전이 Kong-Lua 동작을 바꿀 수 있는 유일한 지점이다.** 기계적 이동 원칙, moved-diff 리뷰, e2e-lua·parity로 가드한다.
2. **APISIX `plugins:`는 기본 목록을 교체한다.** 최소 목록을 쓰므로 다른 플러그인을 쓰는 라우트는 로드 실패로 즉시 드러난다.
3. **standalone 설정이 `#END`로 끝나지 않으면 라우트가 조용히 로드되지 않는다.** `/noplugins` 프로브가 이를 즉시 드러낸다.
4. **이미지는 user `apisix`로 돈다.** COPY한 파일은 644로 읽기 가능하다. APISIX가 쓰는 경로(conf/nginx.conf, logs)는 마운트하지 않는다.
5. **shdict는 `custom_lua_shared_dict`로만 선언한다.** snippet으로 중복 선언하면 nginx가 기동하지 않는다.
6. **arm64 이미지 존재와 이미지 내 curl은 미확인이다.** U3 선결 확인 항목이다.
7. **`ctx.sonic_boom_signal`은 전 플러그인이 공유한다.** global_rule과 라우트 플러그인 동시 설정은 미지원으로 문서화한다.
8. **미검증 API 세부가 있다.** `core.response.set_header` map 형식, 중첩 `encrypt_fields`, 중첩 기본값 적용이 해당한다. U3에서 실측하고, 안 되면 대안을 쓴다(루프 세팅, 해당 줄 제거, 기본값 명시).
