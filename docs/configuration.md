# sonic-boom configuration reference

This document covers the defaults, units, and validation rules of every field, plus the differences across the variants (Kong-Go, Kong-Lua, APISIX-Lua). Mechanics live in [how it works](./how-it-works.md); installation lives in each variant's usage guide. Every value in the tables was read from the schema definition files, and each section links its source at the top: when this document and the source disagree, the source is right (and this document has the bug).

## Common fields

All three variants share the same defaults. The Lua schemas declare only the Go fields whose behavior is implemented, so every field in this table is valid in all three variants.

| Field | Type | Default | Validation & notes |
|------|------|--------|-----------|
| `response_code` | list of int | `[200, 301, 404]` | response statuses eligible for storage. required |
| `request_method` | list of strings | `["GET", "HEAD"]` | case-insensitive match. required |
| `content_type` | list of strings | `["text/plain", "application/json", "application/json; charset=utf-8"]` | case-insensitive exact match. required |
| `vary_headers` | list of strings | `[]` | request headers reflected in the cache key |
| `cache_ttl` | seconds | `300` | **0 is rejected** because it becomes a permanent key in the redis store |
| `cache_control` | bool | `true` | request-side guard (excludes requests carrying `Cache-Control: no-store/no-cache` or `Authorization`). opt out with `false` |
| `credential_vary` | bool | `true` | reflects the principal key for `Authorization` requests without a resolved consumer. opt out with `false` |
| `cacheable_body_max_size` | bytes | `0` (unlimited) | over-limit is judged on the `>` boundary. body-caching routes must set a limit |
| `cache_version` | string | empty → auto-filled | Go prefers the `CACHE_VERSION` env var, else `2-1.0.4`. Lua: config value > env var > product version |
| `strategy` | string | `"redis"` | one of `redis` · `redis-cluster` · `in-memory`. exactly one strategy per config |

Configuration validation runs **on every request**, not at deploy time. On failure the plugin does not kill the process; it logs an Error and lets the request through as `Bypass`, so that one bad config cannot take down the whole gateway.

## Per-strategy sub-settings

### redis — source: internal/redis_config.go, lua/kong/plugins/sonic-boom-lua/schema.lua

| Field | Default | Notes |
|------|--------|------|
| `host` | (none; required in Go) | Kong-Lua defaults to `localhost`. If undeclared, Go returns `Bypass` on every request |
| `port` | `6379` | |
| `db_number` | `0` | must be `0` when `client_side_cache` is enabled |
| `username` / `password` | empty | |
| `pool_size` | `10` | |
| `max_retries` | `3` | `-1` disables retries |
| `min_retry_backoff_ms` / `max_retry_backoff_ms` | `8` / `512` | milliseconds. `-1` disables |
| `dial_timeout` | `5` | seconds |
| `read_timeout` / `write_timeout` | `3` / `3` | seconds |
| `pool_timeout` | `5` | seconds |
| `idle_timeout` | `1` | seconds |

Value rules for the timeout family (Go): `0` means unlimited, `-1` disables, positive values are seconds. That is why `dial_timeout: 0` in `config_v1.yml` means unlimited.

The Lua schema does not declare `max_retries`, retry backoff, or `pool_timeout`; it behaves as fail-open with no retries. Using an undeclared field makes declarative loading fail (see "Variant differences" below). Lua timeouts accept only values of 1 or more (0 and -1 are not allowed).

### redis-cluster — source: internal/redis_config.go

| Field | Default | Notes |
|------|--------|------|
| `addrs` | (none; required) | list of cluster node addresses, e.g. `["redis-cluster:7000"]` |
| `username` / `password` | empty | |
| `pool_size` | `10` | |
| `max_retries` | `3` | `-1` disables retries |
| `min_retry_backoff_ms` / `max_retry_backoff_ms` | `8` / `512` | |
| `dial/read/write/pool/idle_timeout` | `5/3/3/5/1` | seconds. the 0/-1 rules match redis |

There is no DB selection field; the cluster decides key placement.

### in-memory — source: internal/in_memory_config.go

| Field | Default | Notes |
|------|--------|------|
| `max_cost` | `1000000` | ristretto total cost cap |
| `num_counters` | `1000000` | ristretto counter count (for approximate LFU) |
| `buffer_items` | `64` | ristretto write buffer size |

Two known behaviors are documented in [docs/testing.md](./testing.md): because the client and store are shared for the process lifetime under a config-value key, **the TTL from the first creation is fixed for later requests**. ristretto buffers writes asynchronously, so a lookup right after a save can miss.

### client_side_cache (under redis) — source: internal/redis_config.go

This is go-redis client-side caching (a local L1 built on RESP3 `CLIENT TRACKING`). It is experimental and **for standalone redis on DB 0 only**; on violation go-redis disables it with a warning.

| Field | Default | Notes |
|------|--------|------|
| `enabled` | `false` | |
| `max_entries` | `0` | `0` uses the library's default limit |
| `max_memory_bytes` | `0` | `0` means unlimited (the entry-count limit takes precedence) |
| `max_staleness_ms` | `0` | `0` disables. recommended at or below the cache TTL, to protect the freshness contract |

The Lua schema has none of these fields (Go only).

### fill_lock — source: internal/sonic_boom.go, lua/sonic_boom/flow.lua

| Field | Default | Notes |
|------|--------|------|
| `enabled` | `true` | with `false`, all concurrent misses go to the upstream |
| `ttl_ms` | `3000` | safety-net TTL for the lock; upper bound on auto-release if the winner dies. values of 0 or less are corrected to 3000 at runtime |
| `wait_ms` | `200` | upper bound on how long losers wait for the winner's save. `0` passes through without waiting |

With the in-memory strategy, fill-lock is always fail-open because the store cannot set locks atomically. This is a structural constraint, not an error; Go exposes it through the `sonicboom_fill_lock_degraded` metric.

### log — source: internal/log.go

Go-only fields (not declared in the Lua schema).

| Field | Default | Notes |
|------|--------|------|
| `log_level` | `"info"` | zerolog level. an invalid value fails validation (`Bypass` on every request) |
| `console_logging_enabled` | `true` | stderr console output |
| `diode_enabled` | `true` | non-blocking buffer (1000 entries / 10 ms); under log storms some entries may be dropped |
| `file_log.enabled` | `false` | file logging on/off |
| `file_log.filename` | `"sonic-boom.log"` | |
| `file_log.folder` | `"/tmp/logs"` | |
| `file_log.maxsize` | `0` | MB. `0` uses the lumberjack default of 100 MB |
| `file_log.maxage` | `0` | days. `0` means old logs are never deleted by date |
| `file_log.maxbackups` | `0` | `0` means old backups are never deleted by count |
| `file_log.localtime` / `file_log.compress` | `false` | timestamp basis and compression for rolled files |

## Filters and rules

`filters` is a sonic-boom-specific extension that narrows what gets cached through path and header regular expressions. **A request that matches no filter is excluded from caching**; once filters exist, the default behavior is exclusion.

```yaml
filters:
- name: static-assets-only
  rules:
  - regexp: "^/assets/"
  cache_ttl: 60        # omitted or 0 inherits the global cache_ttl
- name: only-with-specific-header
  rules:
  - header: X-Client
    regexp: "^mobile$"
```

- `name` and `rules` are both required. Within a rule, an empty `header` string means a path rule; a non-empty value makes it a rule on that header's value.
- Rules within a filter are all evaluated as AND; across filters, the first match wins.
- A filter `cache_ttl` of 0 or omitted inherits the global `cache_ttl`. **A filter that expires immediately cannot be expressed**, because 0 means "inherit", not "no expiry".
- Regular expressions are compiled once at config load. With an invalid pattern, Go passes the request upstream without the header (the panic defense line recovers), and Lua fails to load.
- The regex engines differ: Go uses RE2, Lua uses PCRE. Patterns that require backtracking are rejected by RE2.

## Variant differences

This section collects only the differences an operator runs into while configuring. For the full list at the level of decision rules, the explicit-differences lists in [docs/kong-lua-verdict.md](./kong-lua-verdict.md) and [docs/apisix-verdict.md](./apisix-verdict.md) are the source.

| Item | Kong-Go | Kong-Lua | APISIX-Lua |
|------|---------|----------|------------|
| `redis.host` undeclared | `Bypass` on every request (required-field validation) | attempts to connect to `localhost` | etcd (Admin API) deployment rejects at validation; standalone YAML fail-opens at runtime |
| `cache_ttl: 0` | rejected at request time → `Bypass` on every request | declarative load rejected | etcd deployment rejects; standalone YAML does not enforce it (runtime fail-open) |
| field undeclared in the schema | accepts all | **declarative load rejected** | **declarative load rejected** (`additionalProperties: false`) |
| fields not declared in Lua | — | `max_retries`, retry backoff, `pool_timeout`, `client_side_cache`, `log`, `in_memory` tuning (fail-open with no retries) | same |
| redis password | plaintext | plaintext | masked via `encrypt_fields` |
| save timing | synchronous in the Response phase | in-memory synchronous; redis and cluster asynchronous after the response (log-phase timer) | same Lua core |
| plugin priority | 1 | 100 | 99 |
| observability | OTel metrics and traces | logs prefixed with `product.label` | same Lua core |
| `selfcheck` field (self-validation route) | none | present (default `false`) | present |

That `check_schema` is not enforced in an APISIX standalone YAML deployment matters operationally: config errors surface as runtime fail-open (`Miss` on every request), not as a load failure. After deploying the config file, verify it with a behavioral check such as `make e2e-apisix`.

## Cache behavior rules

Rules that do not fit neatly into a table. [how it works](./how-it-works.md) explains the decision point behind each rule.

- `content_type` is a case-insensitive **exact match**. If only `application/json` is declared, an `application/json;charset=utf-8` response is not cached; parameter variants must also be listed.
- Responses with `Set-Cookie`, or whose `Cache-Control` carries `private`, `no-store`, or `no-cache`, are not saved (aligned with the APISIX built-in proxy-cache defaults).
- The request-side `cache_control` guard is on by default: requests with `Cache-Control: no-store/no-cache` and requests with `Authorization` are excluded from caching (aligned with RFC 9111 and the original Kong plugin). Set `cache_control: false` if you need the old behavior.
- The default `cacheable_body_max_size` of 0 means "no limit". Routes that cache bodies (POST and the like) must set a limit; when unset, a body of any size gets buffered into memory.
- Skipping the save because of body size keeps `X-Cache-Status` at `Miss`; it does not turn into `Bypass`.
- The `X-Cache-Key` header exposes the internal cache key as-is. This is accepted for debugging convenience; if external exposure is a concern, strip the header in front of the gateway.
