# Kong-Go 사용 가이드

sonic-boom을 Kong Gateway 3.9.x OSS에 **외부 플러그인 서버**(Kong과 별도 프로세스로 떠서 페이즈 콜백만 수신하는 실행 모델)로 붙이는 방법. 동작 원리는 [동작 방식](./how-it-works.md), 필드는 [설정 레퍼런스](./configuration.md)가 담는다.

## 이 변종을 고르는 이유

Go로 읽고 고치고 싶을 때 이 변종이다. 플러그인이 별도 프로세스로 떠서 플러그인 장애가 Kong 프로세스 전체를 덮지 않고 OTel 메트릭·트레이스가 이 변종에만 있다. 반대로 그 프로세스 모델이 지켜야 할 규율이 있다(아래 주의점). 성능은 세 변종이 같은 급이다 — 근거는 [docs/bench-kong-lua-vs-go.md](./bench-kong-lua-vs-go.md).

Kong에 이미 Lua 번들이 있거나 별도 프로세스를 띄우고 싶지 않다면 [Kong-Lua 가이드](./usage-kong-lua.md)가 더 가볍다.

## 설치

원천은 `Dockerfile`과 `docker-compose.yaml`이다 — 이 절차가 그대로 e2e를 띄운다.

1. **빌드** — `make build`가 `bin/linux-${GOARCH}/sonic-boom`으로 크로스컴파일한다. 대상 머신 아키텍처에 맞춰 `GOARCH`를 지정한다(amd64·arm64).

   ```bash
   GOARCH=arm64 make build
   ```

2. **바이너리 배치** — Kong 호스트에 둔다. 예: `/kong/go-plugins/sonic-boom`.

3. **pluginserver 등록** — kong.conf 또는 환경변수 네 가지:

   ```bash
   KONG_PLUGINS=bundled,sonic-boom
   KONG_PLUGINSERVER_NAMES=sonic-boom
   KONG_PLUGINSERVER_SONIC_BOOM_START_CMD=/kong/go-plugins/sonic-boom
   KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD=/kong/go-plugins/sonic-boom -dump
   ```

4. **선언형 설정 연결** — `KONG_DECLARATIVE_CONFIG=/kong/config.yml`. 저장소의 [config_v1.yml](../config_v1.yml)이 e2e를 도는 완전한 예시다.

## 최소 설정

[config_v1.yml](../config_v1.yml)의 `/redis` 라우트에서 필수만 남긴 축소본이다. 나머지는 기본값이 덮는다 — 가드 on, `response_code [200, 301, 404]`, GET·HEAD, content-type 3종.

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
  - name: sonic-boom
    config:
      cache_ttl: 60
      strategy: redis
      redis:
        host: redis
```

운영 라우트에서는 `cache_control`을 선언하지 않아 기본 on(가드)을 유지할 것을 권장한다. e2e 설정의 `/redis` 라우트가 `cache_control: false`를 명시하는 건 parity 시나리오(가드를 끈 라우트에서 vary 키 분리 검증) 고정용이지 권장값이 아니다.

## 동작 확인

저장소 전체 확인은 `make e2e` — 플러그인을 빌드하고 compose로 게이트웨이·redis·redis-cluster를 띄운 뒤 세 전략 스모크를 돌린다.

게이트웨이가 이미 떠 있다면 직접 확인한다. 같은 요청을 두 번 보내 상태가 `Miss`에서 `Hit`으로 바뀌는지 본다.

```bash
curl -sI http://localhost:8000/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8000/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

두 응답의 `X-Cache-Key`가 같은 값인지도 확인하면 판정 키까지 검증된다. 상태 값의 뜻은 [troubleshooting](./troubleshooting.md)이 정리한다.

## 주의점

- **외부 플러그인 서버 규율** — 핸들러 패닉 금지, 설정 불변 취급, TTL 0 금지. 이 세 가지를 어기면 e2e가 초록이어도 운영에서 프로세스가 죽는다. 이유와 전체 규율: [docs/solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md](./solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md)
- `cacheable_body_max_size` 기본값 0은 무제한이다. 본문을 캐시하는 라우트(POST 등)는 반드시 상한을 정한다.
- 관찰성: OTel(OpenTelemetry) 메트릭(`sonicboom_cache_requests`·`sonicboom_cache_lookup_duration`·`sonicboom_fill_lock_degraded`)과 `sonic-boom.Access`/`sonic-boom.Response` 트레이스. `OTEL_SDK_DISABLED=true`로 전체 비활성. 목록은 [설정 레퍼런스](./configuration.md).
- `X-Cache-Key`가 내부 키를 그대로 노출하는 건 수용된 기본값이다 — 외부 노출이 우려되면 게이트웨이 앞단에서 제거한다.
