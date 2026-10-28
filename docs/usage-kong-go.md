# Kong-Go Usage Guide

How to attach sonic-boom to Kong Gateway 3.9.x OSS as an **external plugin server** (an execution model in which the plugin runs as a separate process from Kong and receives only phase callbacks). [How it works](./how-it-works.md) covers the principles; the [configuration reference](./configuration.md) covers the fields.

## Why this variant

Choose this variant when you want to read and modify Go. The plugin runs as a separate process, so a plugin failure does not take down the Kong process itself, and OTel metrics and traces exist only in this variant. In exchange, that process model carries rules you must keep (see Caveats). Performance is in the same class across all three variants; the data is in [docs/bench-kong-lua-vs-go.md](./bench-kong-lua-vs-go.md).

If Kong already ships a Lua bundle or you do not want another process, the [Kong-Lua guide](./usage-kong-lua.md) is lighter.

## Install

The sources of truth are `Dockerfile` and `docker-compose.yaml`. This procedure is exactly what brings up e2e.

1. **Build** — `make build` cross-compiles to `bin/linux-${GOARCH}/sonic-boom`. Set `GOARCH` to match the target machine architecture (amd64, arm64).

   ```bash
   GOARCH=arm64 make build
   ```

2. **Place the binary** — on the Kong host. Example: `/kong/go-plugins/sonic-boom`.

3. **Register the pluginserver** — four entries in kong.conf or as environment variables:

   ```bash
   KONG_PLUGINS=bundled,sonic-boom
   KONG_PLUGINSERVER_NAMES=sonic-boom
   KONG_PLUGINSERVER_SONIC_BOOM_START_CMD=/kong/go-plugins/sonic-boom
   KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD=/kong/go-plugins/sonic-boom -dump
   ```

4. **Point at the declarative config** — `KONG_DECLARATIVE_CONFIG=/kong/config.yml`. [config_v1.yml](../config_v1.yml) in this repository is the complete example that runs e2e.

## Minimal configuration

A reduced copy of the `/redis` route from [config_v1.yml](../config_v1.yml) that keeps only the required fields. Defaults cover the rest: guards on, `response_code [200, 301, 404]`, GET/HEAD, and the three content types.

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

On production routes, keep `cache_control` undeclared so the default (guards on) stays in force. The `/redis` route in the e2e config declares `cache_control: false` because the parity scenario pins it (it verifies vary key isolation on a route with the guard off); it is not a recommended value.

## Verify

For a repository-wide check, run `make e2e`: it builds the plugin, brings up the gateway, redis, and redis-cluster through compose, then runs the smoke tests for all three strategies.

If the gateway is already up, check it directly. Send the same request twice and watch the status move from `Miss` to `Hit`.

```bash
curl -sI http://localhost:8000/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8000/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

Also check that `X-Cache-Key` holds the same value in both responses; that verifies the judgment key as well. [Troubleshooting](./troubleshooting.md) explains what the status values mean.

## Caveats

- **External plugin server discipline** — no handler panics, config treated as immutable, no TTL 0. Break any of the three and the process dies in production even with e2e green. Rationale and full rules: [docs/solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md](./solutions/architecture-patterns/go-pdk-external-plugin-runtime-constraints.md)
- The default `cacheable_body_max_size` of 0 means unlimited. Routes that cache bodies (POST and the like) must set a cap.
- Observability: OTel (OpenTelemetry) metrics (`sonicboom_cache_requests`, `sonicboom_cache_lookup_duration`, `sonicboom_fill_lock_degraded`) and `sonic-boom.Access`/`sonic-boom.Response` traces. `OTEL_SDK_DISABLED=true` turns it all off. The list is in the [configuration reference](./configuration.md).
- `X-Cache-Key` exposing the internal key as-is is an accepted default. If external exposure concerns you, strip the header in front of the gateway.
