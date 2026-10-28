# Changelog

## Unreleased

### Added

- **Variant-specific usage and configuration documentation set** — Six new documents: [How it works](docs/how-it-works.md), [Configuration reference](docs/configuration.md) (all fields, defaults, per-variant differences), three per-variant usage guides ([Kong-Go](docs/usage-kong-go.md), [Kong-Lua](docs/usage-kong-lua.md), [APISIX](docs/usage-apisix.md)), and a [Troubleshooting guide](docs/troubleshooting.md). The README was restructured as a documentation map (cache strategy, behavior rules, and config examples moved to sub-documents), and the config examples are excerpted from the declarative config that runs in e2e. Added four mermaid diagrams (request lifecycle, storage gates, core/adapter structure, status determination evidence). Added new terms to CONCEPTS.md: fill-lock, storage gates, and the cache_control guard. Plan: `docs/plans/2026-10-28-1400-docs-variant-usage-guides-plan.md`.
- **Personalized-response no-store gate** — `Set-Cookie` responses and `Cache-Control: private/no-store/no-cache` responses are not stored (G9). Those responses pass through as `X-Cache-Status: Bypass` (consistent with the APISIX proxy-cache default; resolves a documented difference with the Lua port). Also adds a `no-cache` check that the Lua variant lacked.

### Changed

- **Principal reflection for requests with an unidentified consumer (credential vary)** — When a request without an identified consumer carries `Authorization`, it is no longer cached in the anonymous shared slot; the Authorization value is reflected into the key component (principal), splitting it into per-credential slots. If Authorization is already declared in `vary_headers`, the existing vary component is sufficient and it is not added again. Setting `credential_vary: false` reverts to the old behavior (anonymous shared slot) (opt-out contract: undeclared is treated as on). The Lua key format marker changes sbk2→sbk3 (old keys remain until TTL expiry); Go keys renew naturally through the serialization change. Background: the APISIX priority-99 gap was judged a configuration bug that occurs with authentication following the recommended custom band (0,100) (2026-10-27) — the verdict record is in `docs/apisix-verdict.md`.
- **Request-level `cache_control` guard on by default** — Changes the previous behavior (guard off, Authorization requests also cached). `Cache-Control: no-store/no-cache` requests and `Authorization` requests are excluded from the cache by default (consistent with RFC 9111 and the original Kong plugin; resolves G8 having no default protection). Routes that need the previous behavior set `cache_control: false` explicitly. Also ported to the Lua variants (Kong, APISIX) (2026-10-26, default true).
- **Cache key serialization v2** — Replaces the hash xxhash64 (non-cryptographic, 2^32 collisions) with sha256 (64-hex), and serializes multi-valued query parameters with a per-value length prefix instead of a NUL join, removing the key collision between `?x=a%00b` (single value) and `?x=a&x=b` (multi value). Old-format keys never collide with the new keys, so they are never fetched again; they remain until TTL expiry and disappear on their own (no version marker needed).
- **`cache_control` request guard behavior finalized** — Previously only the first request was stored and later ones Bypassed; now it behaves as the request guard intended by the original Kong (see the entry below for the default). Requests with `Cache-Control: no-store/no-cache` and requests with the `Authorization` header are excluded from the cache, and all other requests are cached normally (G8 mitigation). **Operator caution**: Existing deployments that enabled this flag start actually caching and replaying responses.

### Fixed

9 verified defects fixed (full evidence: `docs/bakeoffs/2026-10-18-api-gw-caching-gap-analysis.md`, plan: `docs/plans/2026-10-18-2126-fix-verified-cache-bugs-plan.md`).

- **Fixed a defect where the Response storage path always aborted early with OTel enabled** — Storing `span_context` as-is in `kong.ctx.shared` made the type assertion always fail after a JSON marshal round-trip, so with `OTEL_SDK_DISABLED != "true"` (the deployment default) nothing was ever stored and the fill-lock winner's lock was left abandoned until TTL. Now passed through a serializable DTO instead; on restore failure, storage continues without a parent rather than aborting.
- **Filter `cache_ttl` now applies to actual expiry** — Storage and stale determination used to use the global value; now they use the per-filter TTL. An unset (0) filter TTL inherits the global `cache_ttl` (an explicit 0 also means inherit).
- **Blocked serving stale values from expired or generation-mismatched cache** — After a version-mismatch purge or a stale determination, pass through to upstream (restoring original Kong Lua semantics) and open the re-store path.
- **`cache_ttl` default 0→300, explicit 0 rejected** — TTL 0 left keys in Redis without expiry, and such keys could never be hit.
- **Config errors and panics no longer terminate the process** — Removed the Fatal(os.Exit) on the `cache_control: true` hit path, made config validation always-on (Error + Bypass on failure), and added a recover in the Response phase.
- **The `content_type` list governs gating** — Content types that do not match are not cached (previously: everything fell through and was cached).
- Dead code and dead check removal (reqBody pipeline, responseData block, `validate.Struct(conf)` wrong target, no-op filter TTL loop).

### Operator notes (release notes)

- (a) **Full cold cache**: The TTL is part of the cache key hash, so the entire key space of routes whose effective TTL changes is renewed.
- (b) **Existing TTL-0 permanent keys remain**: Expiry-less keys created by previous deployments stay in Redis unreferenced. To clean up, run `SCAN`, identify keys with `PTTL == -1`, and `DEL` them (or `FLUSHDB` when replacing the db).
- (c) **`cache_ttl` default change**: Deployments that never set it now expire entries after 300 seconds.
- (d) **content_type exact match**: Content types outside the list (including parameter variants, e.g. `application/json;charset=utf-8` without a space) are now Bypassed. Add the variants you need to the list.
- (e) **Upstream switch for expired or generation-mismatched hits**: Where stale values were previously served labeled `Hit`, they are now revalidated against upstream (Refresh/Bypass) and re-stored. The hit-rate drop at TTL boundaries and the increased upstream load are intended realignment.
