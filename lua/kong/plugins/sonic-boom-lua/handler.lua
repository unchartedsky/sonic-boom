-- sonic-boom-lua: sonic-boom(Go, go-pdk)의 Kong 네이티브 Lua 포팅.
-- 어댑터는 게이트웨이 API(PDK 호출·단락)만 담당 — 판정·키·직렬화·저장은 lua/sonic_boom/flow.
local flow = require "sonic_boom.flow"
local product = require "sonic_boom.product"

local plugin = {
  PRIORITY = 100,  -- Go판 pluginserver 우선선위(1)와 같은 위치감 — 인증 아래, 변환 위는 U6 재생 비교에서 재조정
  VERSION = "0.1.0",
}

-- 제품 식별: 코어 로그 라벨은 이 어댑터의 것을 쓴다(키 접두는 기본 sbl 유지).
product.label = "sonic-boom-lua product=kong-lua: "
product.version = plugin.VERSION

-- PDK 게터·세터. 클로저로 두는 이유: 모듈 로드 시점에 kong PDK 서브테이블이 아직 없을 수 있다.
-- Kong PDK는 요청 문맥을 자체로 잡으므로 ctx 인자는 쓰지 않는다.
local ENV = {
  method = function() return kong.request.get_method() end,
  req_header = function(_, h) return kong.request.get_header(h) end,
  path = function() return kong.request.get_path() end,
  path_with_query = function() return kong.request.get_path_with_query() end,
  raw_body = function() return kong.request.get_raw_body() end,
  ids = function()
    local consumer = kong.client.get_consumer()
    local service = kong.router.get_service()
    local route = kong.router.get_route()
    return consumer and consumer.id or "",
      service and service.id or "",
      route and route.id or ""
  end,
  set_header = function(n, v) return kong.response.set_header(n, v) end,
  status = function() return kong.response.get_status() end,
  resp_header = function(n) return kong.response.get_header(n) end,
  resp_headers = function()
    -- 상한 초과로 잘린 헤더는 nil로 돌려 저장 게이트에서 실패시킨다(fail-closed)
    local h, err = kong.response.get_headers(1000)
    if err then return nil end
    return h
  end,
  log = {
    err = function(...) return kong.log.err(...) end,
    notice = function(...) return kong.log.notice(...) end,
  },
}

function plugin:access(conf)
  local exit, sb = flow.access(conf, ENV)
  kong.ctx.plugin.sb_signal = sb
  if exit then
    return kong.response.exit(exit.status, exit.body, exit.headers)
  end
end

function plugin:header_filter(conf)
  kong.ctx.plugin.sb_signal = flow.header_filter(conf, kong.ctx.plugin.sb_signal, ENV)
end

function plugin:body_filter(conf)
  kong.ctx.plugin.sb_signal = flow.body_filter(conf, kong.ctx.plugin.sb_signal, ENV)
end

function plugin:log(conf)
  local sb = kong.ctx.plugin.sb_signal
  if not sb then return end
  kong.ctx.plugin.sb_signal = nil
  flow.log(conf, sb)
end

return plugin
