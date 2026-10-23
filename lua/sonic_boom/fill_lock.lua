-- fill-lock: 캐시 스탬피드 보호 — internal/fill_lock.go 계약을 미러링한다.
-- acquire(승자 선출)·release(소유 토큰 비교 해제)·wait(패자 기한 폴링)·
-- or_wait(승자 더블체크를 포함한 미스 진입부). 락 키 프리픽스 "sb-lock:"은 Go와
-- 같아서 같은 redis db를 공유하는 배포에서 락 공간이 공유된다.
local _M = {}

local LOCK_PREFIX = "sb-lock:"
local product = require "sonic_boom.product"

local seq = 0
local function new_token()
  -- Go newFillLockToken(crypto/rand hex) 대비 약한 토큰이다 — 소유 비교용이라
  -- TTL 창 안에서의 유일성만 필요하다(같은 워커는 카운터, 워커 간은 pid로 갈린다).
  seq = seq + 1
  return string.format("%x-%x-%x", ngx.worker.pid(), ngx.now() * 1000, seq)
end

local function lock_key(key)
  return LOCK_PREFIX .. key
end
_M.lock_key = lock_key -- selfcheck 골든 벡터용 노출(프리픽스는 Go와 공유 불변량)

local function ttl_seconds(ms)
  return math.max(1, math.ceil(ms / 1000))
end

-- acquire: sb-lock:<key>에 토큰을 놓고 승자 여부를 돌려준다. store의 락 원시형이
-- 오류(nil, err)면 fail-open으로 통과시킨다(won=true, token="") — 대기자로 만들면
-- redis 장애 때 모든 요청이 wait_ms만큼 늘어난다(Go acquireFillLock 주석 미러).
function _M.acquire(store, key, ttl_s)
  local token = new_token()
  local ok, err = store.setnx(lock_key(key), token, ttl_s)
  if ok == nil then
    ngx.log(ngx.WARN, product.label .. "fill-lock acquire failed, fail-open: ", tostring(err))
    return true, ""
  end
  return ok, ok and token or ""
end

-- release: 락 값이 자기 토큰과 일치할 때만 삭제한다 — TTL 만료로 후속 승자가
-- 이어받은 락을 지우지 않는다. GET+DEL 경쟁 창은 Go와 같이 남는다.
function _M.release(store, key, token)
  if not token or token == "" then return end
  local cur = store.get(lock_key(key))
  if cur and cur ~= ngx.null and cur == token then
    store.delete(lock_key(key))
  end
end

-- wait: 패자가 기한 안에 값이 채워지기를 폴링한다. 재폴링 간격에 지터를 섞어
-- 대기자들이 같은 박자로 저장소를 두드리지 않게 한다. 수면이 먼저다(Go와 동일).
-- 편차(Go 미러 불일치): store.get에는 기한을 못 건다 — 마지막 get은 저장소
-- dial/read 타임아웃(최대 약 8s)까지만 묶인다. 루프는 기한 경과 후 get을
-- 발행하지 않고 수면을 남은 시간으로 캡해 초과를 한 번의 get으로 제한한다.
function _M.wait(store, key, wait_ms)
  local deadline = ngx.now() + wait_ms / 1000
  while true do
    local now = ngx.now()
    if now >= deadline then return nil end
    ngx.sleep(math.min(0.02 + math.random() * 0.02, deadline - now))
    if ngx.now() >= deadline then return nil end
    local v = store.get(key)
    if v then return v end
    -- 값 폴링 다음에 락 소멸을 감시한다(수면 먼저·지터·마지막 값 재조회 유지):
    -- 비저장 승자는 값을 채우지 않고 락만 푸니까, 패자가 wait_ms를 통째로 태우지 않게.
    local lk = store.get(lock_key(key))
    if not lk or lk == ngx.null then
      -- 락 해제 직후 채움 경합 창을 닫는다 — 최종 값 재조회 1회, nil이면 통과.
      return store.get(key)
    end
  end
end

-- or_wait: 미스 판정 진입부. 승자는 (true, nil, token) — token=""은 fail-open
-- 통과다. 패자는 (false, 채워진 값|nil, "")를 돌려받는다. 승자는 더블체크로 미스
-- 조회와 락 획득 사이에 다른 요청이 채운 값이 있으면 자기 락을 풀고 그 값을
-- 돌려준다(이중 fill 방지).
function _M.or_wait(store, key, fl)
  if not fl or not fl.enabled then
    return true, nil, ""
  end
  local ttl_ms = fl.ttl_ms
  if not ttl_ms or ttl_ms <= 0 then ttl_ms = 3000 end
  local won, token = _M.acquire(store, key, ttl_seconds(ttl_ms))
  if won then
    local raw = nil
    if token ~= "" then
      -- fail-open 승자(token="")는 store가 방금 실패했음을 알기에 더블체크 get을 건너뛴다
      -- (실패 직후 재조회는 3번째 dial 낭비 — 최대 +5s blackhole).
      raw = store.get(key)
      if raw and raw ~= ngx.null then
        _M.release(store, key, token)
        return false, raw, ""
      end
    end
    return true, nil, token
  end
  return false, _M.wait(store, key, fl.wait_ms or 200), ""
end

return _M
