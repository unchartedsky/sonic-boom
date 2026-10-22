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
  store.sync = true -- shdict은 cosocket 제약이 없다 — header·body_filter 동기 저장·해제 가능

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

  -- setnx: shdict add의 원자적 설정. 키가 있으면 (false, "exists")로 진 패배,
  -- 그 외 오류("no memory" 등)는 (nil, err)로 돌려 acquire의 fail-open이 처리한다.
  function store.setnx(key, value, exptime)
    local ok, err = dict():add(key, value, exptime or 0)
    if ok then
      return true
    end
    if err ~= "exists" then
      ngx.log(ngx.WARN, "sonic-boom-lua product=kong-lua: shared dict setnx failed key=",
        key, " err=", err)
      return nil, err
    end
    return false, err
  end

  function store.delete(key)
    dict():delete(key)
  end

  return store
end

return _M
