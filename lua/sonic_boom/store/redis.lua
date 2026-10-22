-- redis 저장소(lua-resty-redis vendored — kong 이미지 번들 아님 실측).
-- 연결은 worker별 keepalive 풀, 타임아웃은 설정 초→ms 매핑. 조회 실패는
-- fail-open(Miss), 저장 실패는 경고 로그 — 재시도는 없다(KTD4).
local redis_lib = require "sonic_boom.vendor.redis"

local PREFIX = "sonic-boom-lua product=kong-lua: "

local _M = {}
_M.sync = false  -- cosocket이 header/body_filter에서 금지라 저장은 log 단계 timer가 담당

local function connect(conf)
  local red = redis_lib:new()
  local dial = (conf.redis.dial_timeout or 5) * 1000
  local send = (conf.redis.write_timeout or 3) * 1000
  local read = (conf.redis.read_timeout or 3) * 1000
  red:set_timeouts(dial, send, read)

  local ok, err = red:connect(conf.redis.host, conf.redis.port, {
    pool = "sbl-redis:" .. tostring(conf.redis.host) .. ":" .. tostring(conf.redis.port)
      .. ":" .. tostring(conf.redis.db_number or 0)
      .. ":" .. tostring(conf.redis.username or ""),
    username = conf.redis.username,
    password = (conf.redis.password ~= "" and conf.redis.password) or nil,
    database = conf.redis.db_number,
  })
  if not ok then
    return nil, err
  end
  ngx.log(ngx.DEBUG, PREFIX, "redis reused times: ", tostring(red:get_reused_times()))
  return red
end

local function release(conf, red)
  red:set_keepalive((conf.redis.idle_timeout or 1) * 1000, conf.redis.pool_size or 10)
end

function _M.new(conf)
  local store = {}

  function store.get(key)
    local red, err = connect(conf)
    if not red then
      ngx.log(ngx.ERR, PREFIX .. "redis connect failed (fail-open, miss): ", err)
      return nil
    end
    local val, gerr = red:get(key)
    release(conf, red)
    if not val then
      if gerr then
        ngx.log(ngx.ERR, PREFIX .. "redis get failed (fail-open, miss): ", gerr)
      end
      return nil
    end
    if val == ngx.null then
      return nil
    end
    return val
  end

  function store.set(key, value, ttl)
    local red, err = connect(conf)
    if not red then
      ngx.log(ngx.WARN, PREFIX .. "redis connect failed, not storing: ", err)
      return nil, err
    end
    local ok, serr
    if ttl > 0 then
      ok, serr = red:set(key, value, "EX", ttl)
    else
      ok, serr = red:set(key, value)
    end
    release(conf, red)
    if not ok then
      ngx.log(ngx.WARN, PREFIX .. "redis set failed, not storing: ", tostring(serr))
    end
    return ok, serr
  end

  function store.delete(key)
    local red, err = connect(conf)
    if not red then
      ngx.log(ngx.WARN, PREFIX .. "redis connect failed, not deleting: ", err)
      return nil, err
    end
    local ok, derr = red:del(key)
    release(conf, red)
    if not ok then
      ngx.log(ngx.WARN, PREFIX .. "redis del failed: ", tostring(derr))
    end
    return ok
  end

  -- setnx: SET key value EX ttl NX. 획득은 "OK", 경합 패배는 ngx.null, 오류는 nil, err.
  function store.setnx(key, value, ttl)
    local red, err = connect(conf)
    if not red then
      ngx.log(ngx.WARN, PREFIX .. "redis connect failed, lock fail-open: ", err)
      return nil, err
    end
    local res, serr = red:set(key, value, "EX", ttl, "NX")
    release(conf, red)
    if res == "OK" then return true end
    if res == ngx.null then return false end
    return nil, serr
  end

  return store
end

return _M
