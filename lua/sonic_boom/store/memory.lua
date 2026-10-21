-- in-memory 저장소(lua_shared_dict). 노드 안 모든 worker가 공유한다
-- (Go판 "플러그인 서버 프로세스 하나 = 노드 공유"와 같은 의미).
-- set 실패(no memory·단편화)는 fail-open: 경고 로그만 남기고 요청은 계속 진행.
local DICT_NAME = "sonic_boom_cache"

local _M = {}

local function dict()
  return ngx.shared[DICT_NAME]
end

function _M.new()
  local store = {}

  function store.get(key)
    return dict():get(key)
  end

  function store.set(key, value, ttl)
    local ok, err, forcible = dict():set(key, value, ttl)
    if not ok then
      ngx.log(ngx.WARN, "sonic-boom-lua product=kong-lua: shared dict set failed key=",
        key, " err=", err)
    end
    if forcible then
      ngx.log(ngx.WARN, "sonic-boom-lua product=kong-lua: shared dict evicted live entries (zone too small) key=", key)
    end
    return ok, err
  end

  function store.delete(key)
    dict():delete(key)
  end

  return store
end

return _M
