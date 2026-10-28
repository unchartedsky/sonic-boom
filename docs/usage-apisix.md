# APISIX-Lua 사용 가이드

sonic-boom-lua를 Apache APISIX(standalone 배치)에 붙이는 방법. Kong-Lua와 같은 Lua 코어를 쓰므로 판정 규칙·설정 필드가 같다 — 다른 것은 붙이는 방식과 APISIX 고유의 주의점이다. 동작 원리는 [동작 방식](./how-it-works.md), 필드는 [설정 레퍼런스](./configuration.md).

## 이 변종을 고르는 이유

게이트웨이가 APISIX라면 이 변종 하나뿐이다. 성능은 Kong-Lua와 같은 급이다([docs/bench-apisix-vs-kong-lua.md](./bench-apisix-vs-kong-lua.md)). Kong을 쓴다면 [Kong-Go](./usage-kong-go.md)·[Kong-Lua](./usage-kong-lua.md) 가이드를 본다.

## 설치

원천은 `Dockerfile.apisix`와 [conf/apisix-config.yaml](../conf/apisix-config.yaml)이다.

1. **Lua 트리 배치** — `lua/` 디렉터리 전체를 `/opt/sonic-boom/`으로 복사한다. 디렉터리 구조가 `require` 경로다.

2. **APISIX config.yaml 굽기** — `conf/apisix-config.yaml`을 `/usr/local/apisix/conf/config.yaml`로 복사한다. 핵심 항목:

   ```yaml
   extra_lua_path: /opt/sonic-boom/?.lua
   custom_lua_shared_dict:
     sonic_boom_cache: 32m
     redis_cluster_slot_locks: 1m
   plugins:
     - proxy-rewrite
     - sonic-boom-lua
     - proxy-cache
   ```

   주의: standalone 배치의 `plugins` 목록은 기본 목록을 **대체**한다. 쓰는 내장 플러그인을 직접 나열해야 한다.

3. **standalone 모드** — `APISIX_STAND_ALONE=true`로 띄운다.

4. **라우트 파일 마운트** — [config_apisix.yaml](../config_apisix.yaml)을 `/usr/local/apisix/conf/apisix.yaml`로 마운트한다. **파일은 반드시 `#END`로 끝나야 한다** — 없으면 APISIX가 라우트를 조용히 로드하지 않는다. 증상이 에러가 아니라 '라우트가 없음'이라 찾기 어렵다.

## 최소 라우트

[config_apisix.yaml](../config_apisix.yaml)의 라우트에서 핵심만 남긴 축소본이다.

```yaml
upstreams:
- id: my-upstream
  type: roundrobin
  nodes:
    "my-upstream-host:80": 1

routes:
- id: my-route
  uri: /my-path/*
  upstream_id: my-upstream
  plugins:
    proxy-rewrite:
      regex_uri: ["^/my-path(/.*)", "$1"]   # Kong의 strip_path를 흉내 낸다
    sonic-boom-lua:
      cache_ttl: 60
      strategy: in-memory
```

APISIX에는 Kong의 `strip_path`가 없어서 `proxy-rewrite`의 `regex_uri`로 경로 접두를 뗀다. redis 비밀번호는 `encrypt_fields`로 마스킹된다.

## 동작 확인

저장소 전체 확인은 `make e2e-apisix` — compose profile `apisix`로 띄운 뒤 스모크를 돌린다(포트 8020).

게이트웨이가 떠 있다면:

```bash
curl -sI http://localhost:8020/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8020/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

## 주의점

- **standalone YAML 배치에서는 설정 검증이 강제되지 않는다.** `check_schema` 훅이 etcd(Admin API) 배치에서만 돈다 — standalone YAML은 설정 오류가 로드 거부가 아니라 런타임 fail-open(전 요청 `Miss`)으로 드러난다. 그래서 이 변종에서는 배포 후 동작 확인이 선택이 아니라 검증의 일부다.
- **priority가 99다.** `Hit`일 때 플러그인이 응답을 바로 끝내기 때문에 99보다 낮은 priority를 가진 커스텀 플러그인(인증 등)과 실행 순서가 겹칠 수 있다. 커스텀 플러그인을 쓴다면 우선순위를 점검한다.
- **fill-lock 꼬리 지연** — fill-lock의 대기 동작이 꼬리 지연(latency tail)에 영향을 준다는 관찰이 있다. 꼬리 지연에 민감한 라우트는 `fill_lock.enabled: false`를 검토한다. 근거: [docs/apisix-verdict.md](./apisix-verdict.md) 운영 관찰 절.
- 변종 간 전체 차이는 [docs/apisix-verdict.md](./apisix-verdict.md)의 명시 차이 목록이 원천이다.
