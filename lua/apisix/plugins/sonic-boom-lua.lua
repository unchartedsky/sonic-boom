-- sonic-boom-lua: APISIX 어댑터. Kong 어댑터와 같은 코어 상태기계(sonic_boom.flow)를 쓰고,
-- 이 파일은 APISIX PDK 게터·세터 env, ctx 저장, 단락 방식(return status, body)만 담당한다.
local core = require("apisix.core")
local flow = require("sonic_boom.flow")
local product = require("sonic_boom.product")

local plugin_name = "sonic-boom-lua"

-- 제품 식별: 키 접두 sba로 Kong-Lua(sbl)와 키·락 공간을 분리한다(같은 redis를 공유해도 교차 서빙 없음).
product.label = "sonic-boom-lua product=apisix: "
product.key_prefix = "sba"
product.version = "0.1.0"

local pool_size = { type = "integer", minimum = 1, maximum = 2147483647, default = 10 }

-- Kong 어댑터 schema.lua의 1:1 번역(KTD7). 미구현 동작 필드(cache_control, client_side_cache, log)는
-- 의도적으로 선언하지 않는다 — 모든 객체가 additionalProperties=false라 설정 로드에서 거부된다.
local schema = {
  type = "object",
  additionalProperties = false,
  properties = {
    selfcheck = { type = "boolean", default = false },
    response_code = {
      type = "array",
      items = { type = "integer", minimum = 0, maximum = 599 },
      default = { 200, 301, 404 },
    },
    request_method = { type = "array", items = { type = "string" }, default = { "GET", "HEAD" } },
    content_type = {
      type = "array",
      items = { type = "string" },
      default = { "text/plain", "application/json", "application/json; charset=utf-8" },
    },
    vary_headers = { type = "array", items = { type = "string" }, default = {} },
    filters = {
      type = "array",
      default = {},
      items = {
        type = "object",
        additionalProperties = false,
        properties = {
          name = { type = "string" },
          cache_ttl = { type = "integer", minimum = 0, maximum = 2147483647, default = 0 },
          rules = {
            type = "array",
            default = {},
            items = {
              type = "object",
              additionalProperties = false,
              properties = {
                header = { type = "string" },
                regexp = { type = "string" },
              },
              required = { "regexp" },
            },
          },
        },
      },
    },
    cache_ttl = { type = "integer", minimum = 1, maximum = 2147483647, default = 300 },
    cacheable_body_max_size = { type = "integer", minimum = 0, default = 0 },
    cache_version = { type = "string" },
    strategy = { type = "string", enum = { "redis", "redis-cluster", "in-memory" }, default = "redis" },
    redis = {
      type = "object",
      additionalProperties = false,
      properties = {
        host = { type = "string", default = "localhost" },
        port = { type = "integer", minimum = 1, maximum = 65535, default = 6379 },
        db_number = { type = "integer", minimum = 0, maximum = 2147483647, default = 0 },
        username = { type = "string" },
        password = { type = "string" },
        pool_size = pool_size,
        dial_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 5 },
        read_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 3 },
        write_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 3 },
        idle_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 1 },
      },
      -- 중첩 기본값은 객체가 없으면 적용되지 않는다 — strategy 기본이 redis라 객체째 명시한다.
      default = {
        host = "localhost", port = 6379, db_number = 0, pool_size = 10,
        dial_timeout = 5, read_timeout = 3, write_timeout = 3, idle_timeout = 1,
      },
    },
    redis_cluster = {
      type = "object",
      additionalProperties = false,
      properties = {
        addrs = { type = "array", items = { type = "string" }, minItems = 1 },
        username = { type = "string" },
        password = { type = "string" },
        pool_size = pool_size,
        dial_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 5 },
        read_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 3 },
        write_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 3 },
        idle_timeout = { type = "integer", minimum = 1, maximum = 2147483647, default = 1 },
      },
    },
    fill_lock = {
      -- internal/fill_lock.go Config 미러 — 기본 on(Go와 동일).
      type = "object",
      additionalProperties = false,
      properties = {
        enabled = { type = "boolean", default = true },
        ttl_ms = { type = "integer", minimum = 0, maximum = 2147483647, default = 3000 },
        wait_ms = { type = "integer", minimum = 0, maximum = 2147483647, default = 200 },
      },
      default = { enabled = true, ttl_ms = 3000, wait_ms = 200 },
    },
  },
  -- Kong-Lua보다 엄격하다(명시 차이): 클러스터 주소 누락을 요청 시점이 아니라 설정 로드에서 거부한다.
  ["if"] = { properties = { strategy = { const = "redis-cluster" } }, required = { "strategy" } },
  ["then"] = { required = { "redis_cluster" }, properties = { redis_cluster = { required = { "addrs" } } } },
}

local _M = {
  version = 0.1,
  -- 권장 커스텀 대역 (0,100)의 상단: Hit은 이보다 낮은 우선순위 access 플러그인보다 먼저 단락한다.
  -- 인증 플러그인은 rewrite 단계에서 consumer를 확정하므로 consumer_name은 이미 있다.
  priority = 99,
  name = plugin_name,
  schema = schema,
  encrypt_fields = { "redis.password", "redis_cluster.password" },
}

function _M.check_schema(conf)
  return core.schema.check(schema, conf)
end

local function first(v)
  if type(v) == "table" then return v[1] end
  return v
end

-- flow env 계약(lua/sonic_boom/flow.lua 머리 주석). ctx는 APISIX api_ctx.
local ENV = {
  method = function() return ngx.req.get_method() end,
  req_header = function(ctx, h) return first(core.request.header(ctx, h)) end,
  path = function(ctx) return ctx.var.uri end,
  path_with_query = function(ctx)
    local args = ctx.var.args
    if args and args ~= "" then
      return ctx.var.uri .. "?" .. args
    end
    return ctx.var.uri
  end,
  raw_body = function(ctx) return core.request.get_body(nil, ctx) end,
  ids = function(ctx)
    return ctx.consumer_name or "", ctx.service_id or "", ctx.route_id or ""
  end,
  set_header = function(n, v) core.response.set_header(n, v) end,
  status = function() return ngx.status end,
  resp_header = function(n) return first(ngx.header[n]) end,
  -- 기본 인자: 소문자 키 조회가 보장된다(raw 모드는 대소문자를 보존해 set-cookie 게이트가 깨진다).
  resp_headers = function() return ngx.resp.get_headers() end,
  log = {
    err = function(...) core.log.error(...) end,
    notice = function(...) core.log.notice(...) end,
  },
}

-- ctx는 전 플러그인이 공유하므로 고유 키를 쓴다. 같은 라우트에 global_rule과 라우트 플러그인을
-- 동시에 거는 구성은 시그널이 겹쳐 미지원이다.
local SIGNAL = "sonic_boom_signal"

function _M.access(conf, ctx)
  local exit, sb = flow.access(conf, ENV, ctx)
  ctx[SIGNAL] = sb
  if exit then
    if exit.headers then
      -- clean_for_replay가 Content-Length를 떼어 낸다. Kong exit와 달리 APISIX는 재계산하지 않는다.
      exit.headers["Content-Length"] = #exit.body
      core.response.set_header(exit.headers)
    end
    -- access에서 core.response.exit를 직접 부르지 않는다 — 엔진이 반환값으로 단락한다.
    return exit.status, exit.body
  end
end

function _M.header_filter(conf, ctx)
  ctx[SIGNAL] = flow.header_filter(conf, ctx[SIGNAL], ENV, ctx)
end

function _M.body_filter(conf, ctx)
  ctx[SIGNAL] = flow.body_filter(conf, ctx[SIGNAL], ENV)
end

function _M.log(conf, ctx)
  local sb = ctx[SIGNAL]
  if not sb then return end
  ctx[SIGNAL] = nil
  flow.log(conf, sb)
end

return _M
