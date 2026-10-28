# Kong-Lua 사용 가이드

sonic-boom-lua를 Kong Gateway 3.9.x OSS에 Lua 플러그인으로 붙이는 방법. 별도 프로세스 없이 Kong 워커 안에서 돈다. 동작 원리는 [동작 방식](./how-it-works.md), 필드는 [설정 레퍼런스](./configuration.md).

## 이 변종을 고르는 이유

플러그인 서버 프로세스를 띄우고 싶지 않을 때 이 변종이다. Kong 워커 안에서 바로 돌아 Go 툴체인도 필요 없고 성능은 Go판과 같은 급이다([docs/bench-kong-lua-vs-go.md](./bench-kong-lua-vs-go.md)) — 선택 기준은 성능이 아니라 운영 형태다.

이 변종에 없는 것: OTel 메트릭·트레이스, redis 재시도·타임아웃 튜닝. 이런 차이가 선택에 영향을 준다면 두 문서를 비교해 본다.

## 설치

원천은 `Dockerfile.kong-lua`와 `docker-compose.yaml`(profile `lua`)이다.

1. **Lua 트리 배치** — `lua/` 디렉터리 전체를 Kong 기본 Lua 트리 **밖**에 둔다(예: `/kong-lua/`). 트리 안에 넣으면 번들 모듈을 덮어쓰는 사고가 난다. 디렉터리 구조가 곧 `require` 경로이므로 통째로 복사한다.

2. **플러그인 등록** — kong.conf 또는 환경변수:

   ```bash
   KONG_PLUGINS=bundled,sonic-boom-lua
   KONG_LUA_PACKAGE_PATH=/kong-lua/?.lua;;
   KONG_NGINX_HTTP_LUA_REGEX_MATCH_LIMIT=10000
   ```

   `;;`는 기본 `package.path` 폴스루다 — 빠뜨리면 이 경로에 있지 않은 다른 Lua 플러그인이 깨진다.

3. **캐시 존 주입** — Lua판 in-memory 저장소는 nginx shared dict를 쓴다. nginx.conf 지시문이 필요해서 kong.conf 항목으로는 줄 수 없다(같은 지시문의 반복 선언을 kong.conf가 허용하지 않는다). include 파일로 준다:

   ```nginx
   lua_shared_dict sonic_boom_cache 32m;
   lua_shared_dict redis_cluster_slot_locks 1m;
   ```

   이 내용의 [conf/kong-lua-extra.conf](../conf/kong-lua-extra.conf)를 만들어 `KONG_NGINX_HTTP_INCLUDE=/kong/extra-nginx.conf`로 건다. 저장소의 compose가 정확히 이 구성으로 띄운다.

4. **선언형 설정 연결** — `KONG_DECLARATIVE_CONFIG`에 [config_lua.yml](../config_lua.yml)을 마운트한다. 로컬 확인은 포트 8010(compose profile `lua`)으로 뜬다.

## 최소 설정

[config_lua.yml](../config_lua.yml)의 라우트에서 필수만 남긴 축소본이다.

```yaml
_format_version: "1.1"
services:
- host: my-upstream
  name: my-cacheable-service
  port: 80
  protocol: http
  routes:
  - name: my-route
    paths:
    - /my-path
    strip_path: true
  plugins:
  - name: sonic-boom-lua
    config:
      cache_ttl: 60
      strategy: in-memory
```

redis를 쓰려면 `strategy: redis`에 `redis.host`를 준다 — 이 변종은 `host` 기본값이 `localhost`다(Go판은 미선언 시 전 요청 `Bypass`).

## 동작 확인

저장소 전체 확인은 `make e2e-lua` — compose profile `lua`로 게이트웨이를 띄운 뒤 스모크를 돌린다.

게이트웨이가 떠 있다면 직접 확인한다:

```bash
curl -sI http://localhost:8010/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8010/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

## 주의점

- **스키마에 없는 필드는 로드가 거부된다.** Go판 설정을 그대로 가져오면 선언형 로드가 깨진다 — `log`·`in_memory` 튜닝·`client_side_cache`·`max_retries`·retry backoff·`pool_timeout`은 이 변종에 없다. 전체 목록은 [설정 레퍼런스](./configuration.md)의 변종 차이 표.
- in-memory 캐시 존 크기(기본 32m)는 nginx.conf에서 관리한다 — 플러그인 설정이 아니다.
- redis·redis-cluster 재시도가 없다(fail-open). 연결이 불안한 환경이라면 Go판의 재시도 튜닝이 필요한지 검토한다.
- redis·redis-cluster 저장은 응답 반환 뒤 비동기로 일어난다(log 단계 타이머). 응답 직후 아주 빠른 재요청은 아직 `Miss`일 수 있다.
- 변종 간 전체 차이는 [docs/kong-lua-verdict.md](./kong-lua-verdict.md)의 명시 차이 목록이 원천이다.
