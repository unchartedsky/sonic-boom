# APISIX-Lua Usage Guide

How to attach sonic-boom-lua to Apache APISIX (standalone deployment). It shares the Lua core with Kong-Lua, so the judgment rules and config fields are the same. What differs is the attachment method and the APISIX-specific caveats. [How it works](./how-it-works.md) covers the principles; the [configuration reference](./configuration.md) covers the fields.

## Why this variant

If your gateway is APISIX, this is the only variant. Performance is in the same class as Kong-Lua ([docs/bench-apisix-vs-kong-lua.md](./bench-apisix-vs-kong-lua.md)). On Kong, see the [Kong-Go](./usage-kong-go.md) and [Kong-Lua](./usage-kong-lua.md) guides.

## Install

The sources of truth are `Dockerfile.apisix` and [conf/apisix-config.yaml](../conf/apisix-config.yaml).

1. **Place the Lua tree** — copy the entire `lua/` directory to `/opt/sonic-boom/`. The directory structure is the `require` path.

2. **Bake the APISIX config.yaml** — copy `conf/apisix-config.yaml` to `/usr/local/apisix/conf/config.yaml`. The key entries:

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

   Caution: in a standalone deployment, the `plugins` list **replaces** the default list. You have to list every built-in plugin you use yourself.

3. **Standalone mode** — bring it up with `APISIX_STAND_ALONE=true`.

4. **Mount the route file** — mount [config_apisix.yaml](../config_apisix.yaml) as `/usr/local/apisix/conf/apisix.yaml`. **The file must end with `#END`**. Without it, APISIX silently fails to load the routes. The symptom is not an error but missing routes, which makes it hard to notice.

## Minimal route

A reduced copy of the route from [config_apisix.yaml](../config_apisix.yaml) that keeps only the core.

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
      regex_uri: ["^/my-path(/.*)", "$1"]   # mimics Kong's strip_path
    sonic-boom-lua:
      cache_ttl: 60
      strategy: in-memory
```

APISIX has no Kong-style `strip_path`, so `proxy-rewrite`'s `regex_uri` strips the path prefix. redis passwords are masked through `encrypt_fields`.

## Verify

For a repository-wide check, run `make e2e-apisix`: bring it up with compose profile `apisix` and run the smoke tests (port 8020).

If the gateway is up:

```bash
curl -sI http://localhost:8020/my-path | grep -i x-cache
# X-Cache-Status: Miss
curl -sI http://localhost:8020/my-path | grep -i x-cache
# X-Cache-Status: Hit
```

## Caveats

- **Config validation is not enforced in a standalone YAML deployment.** The `check_schema` hook only runs in etcd (Admin API) deployments. In standalone YAML, config errors surface as runtime fail-open (`Miss` on every request) rather than load rejection. That is why, in this variant, post-deployment verification is part of validation, not an optional step.
- **priority is 99.** On a `Hit`, the plugin ends the response immediately, so execution order can overlap with custom plugins (auth and the like) that run at a priority below 99. If you use custom plugins, review the priorities.
- **fill-lock tail latency** — there is an observation that fill-lock's waiting behavior affects tail latency. For routes sensitive to tail latency, review `fill_lock.enabled: false`. Source: the operations observation section of [docs/apisix-verdict.md](./apisix-verdict.md).
- For the full set of differences between variants, the explicit difference list in [docs/apisix-verdict.md](./apisix-verdict.md) is the source.
