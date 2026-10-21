-- sonic-boom-lua: sonic-boom(Go, go-pdk)의 Kong 네이티브 Lua 포팅.
-- 어댑터는 게이트웨이 API(PDK 호출·단락)만 담당 — 판정·키·직렬화는 lua/sonic_boom/ 코어.
local core_key = require "sonic_boom.key"
local core_filter = require "sonic_boom.filter"
local core_entry = require "sonic_boom.entry"
local core_headers = require "sonic_boom.headers"
local store_memory = require "sonic_boom.store.memory"

local plugin = {
  PRIORITY = 100,  -- Go판 pluginserver 우선선위(1)와 같은 위치감 — 인증 아래, 변환 위는 U6 재생 비교에서 재조정
  VERSION = "0.1.0",
}

local PREFIX = "sonic-boom-lua product=kong-lua: "

-- 저장소 팩토리: U5(redis)·U7(redis-cluster)가 나머지를 채운다.
-- nil이면 조회가 항상 Miss다(개발 중 단계적 장착).
plugin.new_store = function(conf)
  if conf.strategy == "in-memory" then
    return store_memory.new()
  end
  return nil
end

-- 캐시 버전: Go cacheVersion()와 동일 — 설정 > CACHE_VERSION env > 플러그인 버전
local function cache_version(conf)
  if conf.cache_version and conf.cache_version ~= "" then
    return conf.cache_version
  end
  local from_env = os.getenv("CACHE_VERSION")
  if from_env and from_env ~= "" then
    return from_env
  end
  return plugin.VERSION
end

-- vary + 필터 룰이 참조하는 헤더만 스냅샷(요청 헤더 전체 읽기 회피)
local function header_names(conf)
  local set = {}
  for _, h in ipairs(conf.vary_headers or {}) do set[h] = true end
  for _, f in ipairs(conf.filters or {}) do
    for _, r in ipairs(f.rules or {}) do
      if r.header and r.header ~= "" then set[r.header] = true end
    end
  end
  return set
end

local function snapshot_headers(conf)
  local snap = {}
  for h in pairs(header_names(conf)) do
    local v = kong.request.get_header(h)
    if v and v ~= "" then snap[h] = v end
  end
  return snap
end

-- ngx.req.get_uri_args: 단일 값은 문자열, 복수는 배열, 값 없는 인자(?enc1)는 true.
-- 코어 형태(k→배열)로 정규화하되 값 없는 인자는 빈 문자열 값으로 둔다 —
-- Go(net/url ParseQuery)가 "?enc1"을 {"enc1":[""]}로 다루는 것과 같은 의미다.
local function normalized_query()
  local args = ngx.req.get_uri_args(1000) or {}
  local out = {}
  for k, v in pairs(args) do
    if type(v) == "table" then
      for i, item in ipairs(v) do
        if item == true then v[i] = "" end
      end
      out[k] = v
    elseif v == true then
      out[k] = { "" }
    else
      out[k] = { v }
    end
  end
  return out
end

local function cacheable_method(conf, method)
  local lower = method:lower()
  for _, m in ipairs(conf.request_method) do
    if m:lower() == lower then return true end
  end
  return false
end

function plugin:access(conf)
  if conf.selfcheck then
    local sc = require "sonic_boom.selfcheck"
    local ok, report = sc.run()
    if ok then
      return kong.response.exit(200, "SELFCHECK OK\n" .. report .. "\n")
    end
    return kong.response.exit(500, "SELFCHECK FAIL\n" .. tostring(report) .. "\n")
  end

  local method = kong.request.get_method()
  if not cacheable_method(conf, method) then
    kong.response.set_header("X-Cache-Status", "Bypass")
    return
  end

  local snap = snapshot_headers(conf)
  local path_with_query = kong.request.get_path_with_query()
  local cacheable, ttl = core_filter.match(conf.filters, conf.cache_ttl,
    { path_with_query = path_with_query, headers = snap })
  if not cacheable then
    kong.response.set_header("X-Cache-Status", "Bypass")
    return
  end

  local body, err = kong.request.get_raw_body()
  if err then
    -- Go 관측 동작: 본문 읽기 실패는 헤더 없이 업스트림 진행(빈 body 키 생성 금지)
    kong.log.err(PREFIX .. "raw body read failed: ", err)
    return
  end

  local consumer = kong.client.get_consumer()
  local service = kong.router.get_service()
  local route = kong.router.get_route()

  local vary = {}
  for _, h in ipairs(conf.vary_headers or {}) do
    local v = snap[h]
    if v then vary[h] = v end
  end

  local version = cache_version(conf)
  local key = core_key.derive({
    consumer = consumer and consumer.id or "",
    service = service and service.id or "",
    route = route and route.id or "",
    method = method,
    path = kong.request.get_path(),
    query = normalized_query(),
    vary = vary,
    ttl = ttl,
    body = body or "",
  }, version)
  kong.response.set_header("X-Cache-Key", key)

  -- ---- 저장소 조회(U4 장착 전까지 store=nil → 항상 Miss) ----
  local store = plugin.new_store(conf)
  local raw = store and store.get(key)

  if raw then
    local entry, derr = core_entry.decode(raw)
    if entry then
      if entry.version ~= version then
        -- Go(dc9c909): 버전 불일치는 purge 후 Miss 경로로 재수급
        if store.delete then store.delete(key) end
      elseif (ngx.time() - entry.timestamp) > ttl then
        -- Go(dc9c909): stale은 Refresh 표시 후 업스트림 진행·재저장(경합 창 상태)
        kong.response.set_header("X-Cache-Status", "Refresh")
        kong.ctx.plugin.sb_signal = { key = key, ttl = ttl }
        return
      else
        local headers = core_headers.clean_for_replay(entry.headers)
        headers["Age"] = tostring(ngx.time() - entry.timestamp)
        headers["X-Cache-Status"] = "Hit"
        -- 단락: Kong은 kong.response.exit — APISIX 어댑터는 return status, body(KTD3)
        return kong.response.exit(entry.status, entry.body, headers)
      end
    else
      kong.log.err(PREFIX .. "entry decode failed: ", tostring(derr))
    end
  end

  kong.ctx.plugin.sb_signal = { key = key, ttl = ttl }
  kong.response.set_header("X-Cache-Status", "Miss")
end

-- ---- 응답 저장: header_filter(판정) + body_filter(수집·저장) ----

-- R4 명시 차이: Set-Cookie·private/no-store 응답은 미저장(개인화 응답 오염 방지).
-- content_type은 목록 매치 실패 시 저장하지 않는다(dc9c909 폴스루 수정본과 동일).
local function storable_response(conf)
  local status = kong.response.get_status()
  local status_ok = false
  for _, s in ipairs(conf.response_code) do
    if s == status then status_ok = true break end
  end
  if not status_ok then
    return false, "status " .. tostring(status)
  end

  local ct = kong.response.get_header("Content-Type")
  if not ct or ct == "" then
    return false, "content_type empty"
  end
  local ct_ok = false
  for _, t in ipairs(conf.content_type) do
    if t:lower() == ct:lower() then ct_ok = true break end
  end
  if not ct_ok then
    return false, "content_type " .. ct
  end

  local hdrs = kong.response.get_headers()
  if hdrs["set-cookie"] then
    return false, "set-cookie"
  end
  local cc = hdrs["cache-control"]
  if type(cc) == "string" then cc = { cc } end
  for _, v in ipairs(cc or {}) do
    local lower = v:lower()
    if lower:find("private", 1, true) or lower:find("no-store", 1, true) then
      return false, "cache-control " .. v
    end
  end
  return true
end

local function store_entry(conf, sb, body)
  local store = plugin.new_store(conf)
  if not store then return end
  store.set(sb.key, core_entry.encode({
    status = kong.response.get_status(),
    headers = sb.headers or {},
    body = body,
    timestamp = ngx.time(),
    version = cache_version(conf),
    ttl = sb.ttl,
  }), sb.ttl)
end

function plugin:header_filter(conf)
  local sb = kong.ctx.plugin.sb_signal
  if not sb then return end

  local ok, reason = storable_response(conf)
  if not ok then
    kong.ctx.plugin.sb_signal = nil
    kong.response.set_header("X-Cache-Status", "Bypass")
    kong.log.notice(PREFIX .. "response not storable, bypassing store: ", reason)
    return
  end

  sb.headers = kong.response.get_headers()

  -- HEAD는 본문 청크가 흐르지 않아 body_filter eof가 올 것을 보장하지 못한다.
  -- 빈 본문 엔트리를 여기서 저장한다(body_filter도 오면 같은 내용으로 덮는다).
  if kong.request.get_method():upper() == "HEAD" then
    store_entry(conf, sb, "")
  end
end

function plugin:body_filter(conf)
  local sb = kong.ctx.plugin.sb_signal
  if not sb then return end

  local chunk, eof = ngx.arg[1], ngx.arg[2]
  if chunk and #chunk > 0 then
    local size = (sb.size or 0) + #chunk
    if conf.cacheable_body_max_size > 0 and size > conf.cacheable_body_max_size then
      -- Go와 같은 '>' 경계. 초과 시 수집을 중단하고 저장하지 않는다(잘린 body 저장 금지).
      kong.ctx.plugin.sb_signal = nil
      ngx.log(ngx.INFO, PREFIX .. "body exceeds cacheable_body_max_size, not storing key=", sb.key)
      return
    end
    sb.buf = sb.buf or {}
    sb.buf[#sb.buf + 1] = chunk
    sb.size = size
  end

  if eof then
    store_entry(conf, sb, sb.buf and table.concat(sb.buf) or "")
    kong.ctx.plugin.sb_signal = nil
  end
end

return plugin
