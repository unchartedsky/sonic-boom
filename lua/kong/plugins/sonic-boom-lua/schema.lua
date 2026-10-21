-- KTD7: 구현된 동작의 Go Config 필드만 선언한다.
-- 미포함 동작 필드(fill_lock, client_side_cache, log, cache_control)는 의도적 미선언 —
-- 설정에 나오면 알 수 없는 필드로 선언형 로드가 거부된다(침묵의 차이 방지).
return {
  name = "sonic-boom-lua",
  fields = {
    {
      config = {
        type = "record",
        fields = {
          { selfcheck = { type = "boolean", default = false } },
          { response_code = {
              type = "array",
              required = true,
              default = { 200, 301, 404 },
              elements = { type = "integer", between = { 0, 599 } },
          } },
          { request_method = {
              type = "array",
              required = true,
              default = { "GET", "HEAD" },
              elements = { type = "string" },
          } },
          { content_type = {
              type = "array",
              required = true,
              default = { "text/plain", "application/json", "application/json; charset=utf-8" },
              elements = { type = "string" },
          } },
          { vary_headers = {
              type = "array",
              required = true,
              default = {},
              elements = { type = "string" },
          } },
          { filters = {
              type = "array",
              default = {},
              elements = {
                type = "record",
                fields = {
                  { name = { type = "string" } },
                  { cache_ttl = { type = "integer", between = { 0, 2147483647 }, default = 0 } },
                  { rules = {
                      type = "array",
                      default = {},
                      elements = {
                        type = "record",
                        fields = {
                          { header = { type = "string" } },
                          { regexp = { type = "string", required = true } },
                        },
                      },
                  } },
                },
              },
          } },
          { cache_ttl = { type = "integer", between = { 1, 2147483647 }, default = 300 } },
          { cacheable_body_max_size = { type = "integer", between = { 0, 9223372036854775807 }, default = 0 } },
          { cache_version = { type = "string" } }, -- 미설정=env CACHE_VERSION>플러그인 버전(핸들러 cache_version)
          { strategy = { type = "string", one_of = { "redis", "redis-cluster", "in-memory" }, default = "redis" } },
          { redis = {
              type = "record",
              fields = {
                { host = { type = "string", default = "localhost" } }, -- Go도 required 검증 없음(연결 시점 실패)
                { port = { type = "integer", between = { 1, 65535 }, default = 6379 } },
                { db_number = { type = "integer", between = { 0, 2147483647 }, default = 0 } },
                { username = { type = "string" } },
                { password = { type = "string", referenceable = true } },
                { pool_size = { type = "integer", between = { 1, 2147483647 }, default = 10 } },
                -- Go의 max_retries·retry backoff는 미구현 동작이라 의도적 미선언(KTD4: 재시도 없는 fail-open)
                { dial_timeout = { type = "integer", between = { 1, 2147483647 }, default = 5 } },
                { read_timeout = { type = "integer", between = { 1, 2147483647 }, default = 3 } },
                { write_timeout = { type = "integer", between = { 1, 2147483647 }, default = 3 } },
                { idle_timeout = { type = "integer", between = { 1, 2147483647 }, default = 1 } },
              },
          } },
          { redis_cluster = {
              type = "record",
              fields = {
                { addrs = {
                    type = "array",
                    elements = { type = "string" },
                } },
                { username = { type = "string" } },
                { password = { type = "string", referenceable = true } },
                { pool_size = { type = "integer", between = { 1, 2147483647 }, default = 10 } },
                { dial_timeout = { type = "integer", between = { 1, 2147483647 }, default = 5 } },
                { read_timeout = { type = "integer", between = { 1, 2147483647 }, default = 3 } },
                { write_timeout = { type = "integer", between = { 1, 2147483647 }, default = 3 } },
                { idle_timeout = { type = "integer", between = { 1, 2147483647 }, default = 1 } },
              },
          } },
        },
      },
    },
  },
}
