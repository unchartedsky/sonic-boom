-- 요청 단위 캐시 상태기계(게이트웨이 중립). 어댑터는 PDK 게터·세터를 담은 env 테이블과
-- 시그널(sb) 저장소, Hit·selfcheck 단락 방식만 소유한다 — 판정·저장·fill-lock은 전부 여기.
--
-- env 계약(정적 테이블, per-request ctx는 인자로 받는다 — access가 cosocket에서 yield하므로
-- ctx를 env 필드에 들고 있으면 요청 간 경합이 난다):
--   method(ctx)            요청 메서드
--   req_header(ctx, h)     요청 헤더 단일 값
--   path(ctx)              정규화 경로(쿼리 제외)
--   path_with_query(ctx)   필터 매치 대상(경로+쿼리)
--   raw_body(ctx)          body, err
--   ids(ctx)               consumer, service, route 식별자(없으면 "")
--   set_header(n, v)       응답 헤더 세팅
--   status()               응답 상태 코드(header/body_filter)
--   resp_header(n)         응답 헤더 단일 값
--   resp_headers()         응답 헤더 전체(키는 소문자 조회 가능해야 한다)
--   log.err(...), log.notice(...)
--
-- 시그널(sb) 모양은 fill-lock 계약의 일부다 — 승자 {key,ttl,lock_token}, 패자 {key,ttl},
-- 비저장 경로는 {key,ttl,lock_token,release_only=true}, 비동기 저장은 body·status·version·
-- timestamp를 구워 둔다. 각 페이즈는 다음 페이즈로 넘길 sb를 반환하고 어댑터가 ctx에 저장한다.
local core_key = require "sonic_boom.key"
local core_filter = require "sonic_boom.filter"
local core_entry = require "sonic_boom.entry"
local core_headers = require "sonic_boom.headers"
local core_fill_lock = require "sonic_boom.fill_lock"
local store_memory = require "sonic_boom.store.memory"
local store_redis = require "sonic_boom.store.redis"
local store_rediscluster = require "sonic_boom.store.rediscluster_store"
local product = require "sonic_boom.product"

local _M = {}

-- 저장소 팩토리. nil이면 조회가 항상 Miss다.
function _M.new_store(conf)
  if conf.strategy == "in-memory" then
    return store_memory.new(conf)
  end
  if conf.strategy == "redis" then
    return store_redis.new(conf)
  end
  if conf.strategy == "redis-cluster" then
    return store_rediscluster.new(conf)
  end
  return nil
end

-- fill-lock 해제: 소유 토큰이 있을 때만 store를 만들어 해제한다. store 생성 실패는
-- TTL 만료에 위탁된다(기본 3초).
local function release_if_owned(conf, sb)
  if not sb or not sb.lock_token or sb.lock_token == "" then return end
  local store = _M.new_store(conf)
  if store then
    core_fill_lock.release(store, sb.key, sb.lock_token)
  end
end

-- 캐시 버전: Go cacheVersion()와 동일 — 설정 > CACHE_VERSION env > 플러그인 버전
function _M.cache_version(conf)
  if conf.cache_version and conf.cache_version ~= "" then
    return conf.cache_version
  end
  local from_env = os.getenv("CACHE_VERSION")
  if from_env and from_env ~= "" then
    return from_env
  end
  return product.version
end
local cache_version = _M.cache_version

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

local function snapshot_headers(conf, env, ctx)
  local snap = {}
  for h in pairs(header_names(conf)) do
    local v = env.req_header(ctx, h)
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

-- access: exit(단락할 응답 {status, body, headers}) 또는 nil, 그리고 다음 페이즈로 넘길 sb.
function _M.access(conf, env, ctx)
  if conf.selfcheck then
    local sc = require "sonic_boom.selfcheck"
    local ok, report = sc.run()
    if ok then
      return { status = 200, body = "SELFCHECK OK\n" .. report .. "\n" }, nil
    end
    return { status = 500, body = "SELFCHECK FAIL\n" .. tostring(report) .. "\n" }, nil
  end

  local method = env.method(ctx)
  if not cacheable_method(conf, method) then
    env.set_header("X-Cache-Status", "Bypass")
    return nil, nil
  end

  local snap = snapshot_headers(conf, env, ctx)
  local path_with_query = env.path_with_query(ctx)
  local cacheable, ttl = core_filter.match(conf.filters, conf.cache_ttl,
    { path_with_query = path_with_query, headers = snap })
  if not cacheable then
    env.set_header("X-Cache-Status", "Bypass")
    return nil, nil
  end

  local body, err = env.raw_body(ctx)
  if err then
    -- Go 관측 동작: 본문 읽기 실패는 헤더 없이 업스트림 진행(빈 body 키 생성 금지)
    env.log.err(product.label .. "raw body read failed: ", err)
    return nil, nil
  end

  local consumer, service, route = env.ids(ctx)

  local vary = {}
  for _, h in ipairs(conf.vary_headers or {}) do
    local v = snap[h]
    if v then vary[h] = v end
  end

  local version = cache_version(conf)
  local key = core_key.derive({
    consumer = consumer,
    service = service,
    route = route,
    method = method,
    path = env.path(ctx),
    query = normalized_query(),
    vary = vary,
    ttl = ttl,
    body = body or "",
  }, version, product.key_prefix)
  env.set_header("X-Cache-Key", key)

  -- ---- 저장소 조회 ----
  local store = _M.new_store(conf)
  local raw = store and store.get(key)

  if not raw then
    -- fill-lock: 미스 시점 승자 선출과 패자 대기(internal/fill_lock.go 계약 미러).
    -- 패자가 기한 내 값을 받으면 같은 판정 블록으로 흘러간다.
    local won, filled, tok = true, nil, ""
    if store then
      won, filled, tok = core_fill_lock.or_wait(store, key, conf.fill_lock)
    end
    if filled then
      raw = filled -- 더블체크·패자 대기 성공 — 아래 공용 판정 블록으로
    elseif won then
      -- 승자: 해제 책임은 sb.lock_token으로 응답 페이즈에 있다
      env.set_header("X-Cache-Status", "Miss")
      return nil, { key = key, ttl = ttl, lock_token = tok }
    else
      -- 패자 기한 초과 통과 — 저장은 하되 해제는 없다(Go Winner=false와 동일)
      env.set_header("X-Cache-Status", "Miss")
      return nil, { key = key, ttl = ttl }
    end
  end

  if raw then
    local entry, derr = core_entry.decode(raw)
    if entry then
      if entry.version ~= version then
        -- Go(dc9c909): 버전 불일치는 purge 후 Miss 경로로 재수급
        if store.delete then store.delete(key) end
      elseif (ngx.time() - entry.timestamp) > ttl then
        -- Go(dc9c909): stale은 Refresh 표시 후 업스트림 진행·재저장(경합 창 상태)
        env.set_header("X-Cache-Status", "Refresh")
        return nil, { key = key, ttl = ttl }
      else
        local headers = core_headers.clean_for_replay(entry.headers)
        headers["Age"] = tostring(ngx.time() - entry.timestamp)
        headers["X-Cache-Status"] = "Hit"
        -- 단락은 어댑터 소유: Kong은 PDK exit, APISIX는 return status, body(KTD3)
        return { status = entry.status, body = entry.body, headers = headers }, nil
      end
    else
      env.log.err(product.label .. "entry decode failed: ", tostring(derr))
    end
  end

  env.set_header("X-Cache-Status", "Miss")
  return nil, { key = key, ttl = ttl }
end

-- ---- 응답 저장: header_filter(판정) + body_filter(수집·저장) ----

-- R4 명시 차이: Set-Cookie·private/no-store 응답은 미저장(개인화 응답 오염 방지).
-- content_type은 목록 매치 실패 시 저장하지 않는다(dc9c909 폴스루 수정본과 동일).
local function storable_response(conf, env)
  local status = env.status()
  local status_ok = false
  for _, s in ipairs(conf.response_code) do
    if s == status then status_ok = true break end
  end
  if not status_ok then
    return false, "status " .. tostring(status)
  end

  local ct = env.resp_header("Content-Type")
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

  local hdrs = env.resp_headers()
  if hdrs["set-cookie"] then
    return false, "set-cookie"
  end
  local cc = hdrs["cache-control"]
  if type(cc) == "string" then cc = { cc } end
  for _, v in ipairs(cc or {}) do
    local lower = v:lower()
    if lower:find("private", 1, true) or lower:find("no-store", 1, true) or lower:find("no-cache", 1, true) then
      return false, "cache-control " .. v
    end
  end
  return true
end

local function store_entry(conf, sb, body, env)
  local store = _M.new_store(conf)
  if not store then return end
  store.set(sb.key, core_entry.encode({
    status = env.status(),
    headers = sb.headers or {},
    body = body,
    timestamp = ngx.time(),
    version = cache_version(conf),
    ttl = sb.ttl,
  }), sb.ttl)
end

function _M.header_filter(conf, sb, env, ctx)
  if not sb then return nil end

  local ok, reason = storable_response(conf, env)
  if not ok then
    -- 해제는 log 타이머로 미룬다 — header_filter에선 cosocket이 금지돼 있다.
    -- 소유 토큰만 남은 release-only 신호로 바꿔 놓는다(body_filter가 저장을 시도하지 않게).
    local next_sb = nil
    if sb.lock_token and sb.lock_token ~= "" then
      next_sb = { key = sb.key, ttl = sb.ttl, lock_token = sb.lock_token, release_only = true }
    end
    env.set_header("X-Cache-Status", "Bypass")
    env.log.notice(product.label .. "response not storable, bypassing store: ", reason)
    return next_sb
  end

  sb.headers = env.resp_headers()

  -- HEAD는 본문 청크가 흐르지 않아 body_filter eof가 올 것을 보장하지 못한다.
  -- 동기 저장소(in-memory)면 여기서 빈 본문 엔트리를 굽고, 비동기 저장소(redis·cluster)는
  -- cosocket이 header_filter에서도 금지라 log 타이머로 넘긴다.
  if env.method(ctx):upper() == "HEAD" then
    local store = _M.new_store(conf)
    if store and store.sync then
      store_entry(conf, sb, "", env)
      release_if_owned(conf, sb) -- sync store는 shdict뿐이라 header_filter에서도 해제 가능
      return nil
    else
      sb.body = ""
      sb.status = env.status()
      sb.version = cache_version(conf)
      sb.timestamp = ngx.time()
    end
  end
  return sb
end

function _M.body_filter(conf, sb, env)
  if not sb or sb.release_only then return sb end

  local chunk, eof = ngx.arg[1], ngx.arg[2]
  if chunk and #chunk > 0 then
    local size = (sb.size or 0) + #chunk
    if conf.cacheable_body_max_size > 0 and size > conf.cacheable_body_max_size then
      -- Go와 같은 '>' 경계. 초과 시 수집을 중단하고 저장하지 않는다(잘린 body 저장 금지).
      -- 해제는 log 타이머로 — body_filter에선 cosocket이 금지돼 있다.
      local next_sb = nil
      if sb.lock_token and sb.lock_token ~= "" then
        next_sb = { key = sb.key, ttl = sb.ttl, lock_token = sb.lock_token, release_only = true }
      end
      ngx.log(ngx.INFO, product.label .. "body exceeds cacheable_body_max_size, not storing key=", sb.key)
      return next_sb
    end
    sb.buf = sb.buf or {}
    sb.buf[#sb.buf + 1] = chunk
    sb.size = size
  end

  if eof then
    local store = _M.new_store(conf)
    if store and store.sync then
      store_entry(conf, sb, sb.buf and table.concat(sb.buf) or "", env)
      release_if_owned(conf, sb)
      return nil
    else
      -- redis/cluster: cosocket이 body_filter에서 금지라 log 단계 timer로 저장을 미룬다.
      -- 엔트리 필드를 신호에 미리 굽는다(타임스탬프는 응답 시점 = Go Response 관측과 근사).
      sb.body = sb.buf and table.concat(sb.buf) or ""
      sb.status = env.status()
      sb.version = cache_version(conf)
      sb.timestamp = ngx.time()
    end
  end
  return sb
end

-- log: 어댑터가 ctx의 sb를 비운 뒤 호출한다.
function _M.log(conf, sb)
  if not sb then return end

  -- 본문 저장(redis/cluster의 log 단계 timer 저장)과 fill-lock 해제를 한 타이머로
  -- 처리한다. 해제는 저장이 보인 뒤에 — 비워 둔 창을 패자 폴링이 미스로 보지 않게.
  -- release-only 신호(비저장 경로)는 해제만 수행한다.
  local payload = {
    key = sb.key, ttl = sb.ttl, body = sb.body, headers = sb.headers,
    status = sb.status, version = sb.version, timestamp = sb.timestamp,
    lock_token = sb.lock_token, store_it = sb.body ~= nil,
  }
  if not payload.store_it and not (payload.lock_token and payload.lock_token ~= "") then
    return
  end

  local ok, err = ngx.timer.at(0, function(premature)
    if premature then return end
    local store = _M.new_store(conf)
    if not store then return end
    if payload.store_it then
      store.set(payload.key, core_entry.encode({
        status = payload.status, headers = payload.headers or {}, body = payload.body,
        timestamp = payload.timestamp, version = payload.version, ttl = payload.ttl,
      }), payload.ttl)
    end
    if payload.lock_token and payload.lock_token ~= "" then
      core_fill_lock.release(store, payload.key, payload.lock_token)
    end
  end)
  if not ok then
    -- 타이머 실패 시 저장·해제 모두 포기 — 락은 TTL 만료가 푼다(log 컨텍스트도
    -- cosocket이 금지라 여기서 직접 해제할 수 없다).
    ngx.log(ngx.WARN, product.label .. "timer creation failed, not storing key=",
      sb.key, " err=", err)
  end
end

return _M
