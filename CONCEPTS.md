# Concepts

> Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Plugin runtime (플러그인 런타임)

### External plugin server (외부 플러그인 서버)
sonic-boom's execution model: a separate process from the Kong core that receives only phase callbacks. *Avoid:* sidecar.

An instance handles multiple requests concurrently and has no instance lifecycle hooks, so config initialization runs on every request. The runtime has no defense against handler panics; the plugin's recover is the only line of defense.

### Cache strategy (캐시 전략)
The axis for choosing a store — in-memory (node-local), redis (single), redis-cluster (sharded). One plugin config uses exactly one strategy.

### Cache signal (캐시 시그널)
A request-scoped message by which the Access phase passes only the cache key and TTL to the Response phase. If the signal is still present, Response attempts a store; if not, it does not.

## Cache semantics (캐시 시맨틱)

### Filter (필터)
A sonic-boom-specific extension that narrows cache targets with path and header regex rules and assigns a TTL per rule. If no filter matches, the request is excluded from caching.

### X-Cache-Status
A four-state marker of the cache outcome — Miss (miss, will be stored), Hit (served from cache), Bypass (not a cache target, or generation mismatch), Refresh (expiry detected).

### cache_version
Cache generation marker. Changing the value invalidates the previous generation of cache; the way to change it is a config reload (the env path is pinned to the process environment).

### Principal key isolation (주체 키 분리)
A policy that reflects the authenticated principal (consumer, principal) into the cache key to create per-user cache namespaces. Unconditional reflection of an established principal is the default, and the layer on top of that is refusing to store authenticated requests that lack a principal. *Avoid:* consumer isolation, principal isolation — upstream product option names (APISIX `consumer_isolation`, Kong `cache_by_principal`) are not sonic-boom policy terms.

### Request-side cache_control guard (cache_control 가드)
Request-side cache exclusion guard (on by default). Excludes requests with `Cache-Control: no-store/no-cache` and requests carrying an `Authorization` header from the cache. Setting `cache_control: false` explicitly on a route turns it off.

### Storage gates (저장 게이트)
The gates the Response phase evaluates to decide whether to store the response — status, content-type, Set-Cookie, `Cache-Control: private/no-store/no-cache`, body size. Failing a gate overwrites the Miss label with Bypass; only exceeding the body size limit is the exception and keeps Miss.

### fill-lock
A cache stampede defense that lets only one request fill from upstream on a concurrent miss. Consists of token-based lock acquisition and a polling wait by the losers; on stores that do not support locks (in-memory), it fails open and lets everything through.
