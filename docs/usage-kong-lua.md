# Kong-Lua Usage Guide

How to attach sonic-boom-lua to Kong Gateway 3.9.x OSS as a Lua plugin. It runs inside the Kong worker with no separate process. [How it works](./how-it-works.md) covers the principles; the [configuration reference](./configuration.md) covers the fields.

## Why this variant

Choose this variant when you do not want to run a plugin server process. It runs directly in the Kong worker, needs no Go toolchain, and performance is in the same class as the Go variant ([docs/bench-kong-lua-vs-go.md](./bench-kong-lua-vs-go.md)). The selection criterion is the operating shape, not performance.

What this variant lacks: OTel metrics and traces, redis retry and timeout tuning. If any of these differences affects the choice, compare the two documents.

## Install

The sources of truth are `Dockerfile.kong-lua` and `docker-compose.yaml` (profile `lua`).

1. **Place the Lua tree** — put the entire `lua/` directory **outside** the default Kong Lua tree (example: `/kong-lua/`). Placing it inside overwrites bundled modules. The directory structure is the `require` path, so copy it wholesale.

2. **Register the plugin** — in kong.conf or as environment variables:

   ```bash
   KONG_PLUGINS=bundled,sonic-boom-lua
   KONG_LUA_PACKAGE_PATH=/kong-lua/?.lua;;
   KONG_NGINX_HTTP_LUA_REGEX_MATCH_LIMIT=10000
   ```

   The `;;` is the fall-through to the default `package.path`. Drop it and other Lua plugins that do not live on this path break.

3. **Inject the cache zones** — the Lua variant's in-memory store uses nginx shared dicts. The directives belong in nginx.conf, so kong.conf entries cannot carry them (kong.conf does not allow a repeated declaration of the same directive). Provide them through an include file:

   ```nginx
   lua_shared_dict sonic_boom_cache 32m;
   lua_shared_dict redis_cluster_slot_locks 1m;
   ```

   Create [conf/kong-lua-extra.conf](../conf/kong-lua-extra.conf) with this content and pass it via `KONG_NGINX_HTTP_INCLUDE=/kong/extra-nginx.conf`. The repository's compose brings the gateway up in exactly this configuration.

4. **Point at the declarative config** — mount [config_lua.yml](../config_lua.yml) through `KONG_DECLARATIVE_CONFIG`. For local checks, the gateway comes up on port 8010 (compose profile `lua`).

## Minimal configuration

A reduced copy of the route from [config_lua.yml](../config_lua.yml) that keeps only the required fields.

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

To use redis, set `strategy: redis` and provide `redis.host`. In this variant, `host` defaults to `localhost` (the Go variant returns `Bypass` on every request when it is undeclared).

## Verify

For a repository-wide check, run `make e2e-lua`: it brings up the gateway with compose profile `lua` and runs the smoke tests.

If the gateway is up, check it directly:

```bash
curl -sI http://localhost:8010/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8010/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

## Caveats

- **Fields absent from the schema are rejected at load.** Porting the Go variant's config over as-is breaks declarative loading: `log`, `in_memory` tuning, `client_side_cache`, `max_retries`, retry backoff, and `pool_timeout` do not exist in this variant. The full list is in the variant difference table of the [configuration reference](./configuration.md).
- The in-memory cache zone size (default 32m) is managed in nginx.conf, not in plugin config.
- No retries for redis and redis-cluster (fail-open). On an unstable connection, review whether you need the Go variant's retry tuning.
- redis and redis-cluster writes happen asynchronously after the response returns (log-phase timer). A re-request fired very soon after the response can still see a `Miss`.
- For the full set of differences between variants, the explicit difference list in [docs/kong-lua-verdict.md](./kong-lua-verdict.md) is the source.
