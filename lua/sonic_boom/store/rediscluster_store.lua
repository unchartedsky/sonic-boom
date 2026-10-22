-- redis-cluster 저장소(Kong fork vendored). 슬롯 잠금용 shared dict(redis_cluster_slot_locks)가
-- compose에 선언돼 있어야 한다. TTL은 fork의 set이 EX를 받지 않아 set→expire 2단계(비원자,
-- PoC 수용 — 실패 시 expire 오류만 로그). 조회 실패 fail-open, 저장 실패 경고(KTD4).
local rediscluster = require "sonic_boom.vendor.rediscluster"

local product = require "sonic_boom.product"

local _M = {}
_M.sync = false  -- 저장은 log 단계 timer

function _M.new(conf)
  local serv_list = {}
  for _, addr in ipairs(conf.redis_cluster.addrs) do
    local host, port = addr:match("^(.+):(%d+)$")
    serv_list[#serv_list + 1] = { ip = host, port = tonumber(port) }
  end

  local rc = rediscluster:new({
    name = "sonic_boom_cluster",
    serv_list = serv_list,
    username = conf.redis_cluster.username,
    password = (conf.redis_cluster.password ~= "" and conf.redis_cluster.password) or nil,
    connect_timeout = (conf.redis_cluster.dial_timeout or 5) * 1000,
    read_timeout = (conf.redis_cluster.read_timeout or 3) * 1000,
    send_timeout = (conf.redis_cluster.write_timeout or 3) * 1000,
    keepalive_timeout = (conf.redis_cluster.idle_timeout or 1) * 1000,
    keepalive_cons = conf.redis_cluster.pool_size or 10,
  })
  if not rc then
    return nil, "rediscluster init failed"
  end

  local store = {}

  function store.get(key)
    local ok, val = pcall(function() return rc:get(key) end)
    if not ok then
      ngx.log(ngx.ERR, product.label, "cluster get error (fail-open, miss): ", tostring(val))
      return nil
    end
    if val == nil or val == ngx.null then
      return nil
    end
    return val
  end

  function store.set(key, value, ttl)
    local ok1, err1 = rc:set(key, value)
    if not ok1 then
      ngx.log(ngx.WARN, product.label, "cluster set failed, not storing: ", tostring(err1))
      return nil, err1
    end
    if ttl and ttl > 0 then
      local ok2, err2 = rc:expire(key, ttl)
      if not ok2 then
        ngx.log(ngx.WARN, product.label, "cluster expire failed (무기한 잔존 위험): ", tostring(err2))
      end
    end
    return true
  end

  function store.delete(key)
    local ok, err = rc:del(key)
    if not ok then
      ngx.log(ngx.WARN, product.label, "cluster del failed: ", tostring(err))
    end
    return ok
  end

  -- setnx: SET EX NX를 fork 범용 커맨드 디스패치로 시도한다.
  -- shortcut: cluster SET NX via fork generic dispatch, verified by e2e round-trip;
  -- fallback = fail-open, when NX runtime verification fails(smoke_lua_rediscluster).
  function store.setnx(key, value, ttl)
    local ok, res, err = pcall(function() return rc:set(key, value, "EX", ttl, "NX") end)
    if not ok then
      ngx.log(ngx.WARN, product.label, "cluster setnx error (fail-open): ", tostring(res))
      return nil, tostring(res)
    end
    if res == "OK" then return true end
    if res == ngx.null or res == false then return false end
    return nil, err
  end

  return store
end

return _M
