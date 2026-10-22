-- 골든 벡터 자기검증(Nginx 런타임 전용 — bare luajit엔 ngx.re·openssl FFI가 없다).
-- 기대 해시는 구현과 별개로 파이썬 hashlib가 계산한 값이다(2026-10-21).
local key = require "sonic_boom.key"
local filter = require "sonic_boom.filter"
local entry = require "sonic_boom.entry"

local _M = {}

local function check(results, name, cond, detail)
  if cond then
    results[#results + 1] = "ok   " .. name
  else
    results[#results + 1] = "FAIL " .. name .. (detail and (" — " .. tostring(detail)) or "")
  end
  return cond
end

local GOLDEN = {
  {
    name = "V1 vary+query",
    input = {
      consumer = "c-uuid", service = "svc-1", route = "r-1", method = "GET",
      path = "/api/users", query = { page = { "2" } },
      vary = { Authorization = "Basic abc" }, ttl = 15, body = "",
    },
    hash = "7755d9d6a845bb1447b8ab2a4c4858d76b6264335c44d50789f8f7d8a3fb2dfb",
  },
  {
    name = "V2 body+POST",
    input = {
      method = "POST", path = "/api/login", query = {}, vary = {}, ttl = 30,
      body = '{"u":"a","p":"b"}',
    },
    hash = "8a72300872bff19cb7cceefe917402bb9bc2db28bfe0fef8574a9cacd1773e45",
  },
  {
    name = "V3 multi-query",
    input = {
      consumer = "anon", service = "svc", route = "r", method = "GET",
      path = "/x", query = { a = { "1", "2" }, b = { "3" } }, vary = {}, ttl = 300,
      body = "",
    },
    hash = "f171325fa9fd6e8b222d130a69da3f9514d4373fce7c02be967c692ebdbdbf1e",
  },
}

function _M.run()
  local r = {}
  local ok = true

  for _, g in ipairs(GOLDEN) do
    local derived = key.derive(g.input, "0.1.0")
    ok = check(r, "key " .. g.name, derived == "sbl:0.1.0:" .. g.hash, derived) and ok
  end

  -- 제품별 키 접두어: 접두어는 해시 입력에 들지 않는다 — 같은 입력이면 해시가 같고 접두어만 갈린다.
  local sba = key.derive(GOLDEN[1].input, "0.1.0", "sba")
  ok = check(r, "key 접두어 sba", sba == "sba:0.1.0:" .. GOLDEN[1].hash, sba) and ok

  local req = { path_with_query = "/api/users?page=2", headers = { Authorization = "Basic abc" } }
  local c, ttl = filter.match({
    { name = "f1", cache_ttl = 30, rules = {
        { regexp = "page=2" },
        { header = "Authorization", regexp = "Basic" },
    } },
  }, 15, req)
  ok = check(r, "filter 모든 룰 매치 → 필터 TTL", c == true and ttl == 30) and ok

  c, ttl = filter.match({
    { name = "f1", cache_ttl = 30, rules = { { header = "Authorization", regexp = "^Bearer" } } },
  }, 15, req)
  ok = check(r, "filter 헤더 룰 실패 → 필터 탈락", c == false and ttl == 0) and ok

  c, ttl = filter.match({
    { name = "inherit", cache_ttl = 0, rules = { { regexp = "users" } } },
  }, 15, req)
  ok = check(r, "filter ttl 0 → 전역 TTL 상속", c == true and ttl == 15) and ok

  c, ttl = filter.match({}, 15, req)
  ok = check(r, "filter 없음 → 전역", c == true and ttl == 15) and ok

  local e = {
    status = 200,
    headers = { ["content-type"] = "application/json", ["x-multi"] = { "a", "b" } },
    body = "line1\nline2\0bin",
    timestamp = 1761000000,
    version = "0.1.0",
    ttl = 15,
  }
  local dec = entry.decode(entry.encode(e))
  ok = check(r, "entry 왕복 보존",
    dec and dec.status == 200 and dec.ttl == 15 and dec.version == "0.1.0"
      and dec.headers["content-type"] == "application/json"
      and table.concat(dec.headers["x-multi"]) == "ab"
      and dec.body == e.body and dec.timestamp == 1761000000) and ok

  -- fill-lock: 락 키 프리픽스는 Go("sb-lock:")와 공유된다 — 락 공간 상호운용 불변량.
  local fill_lock = require "sonic_boom.fill_lock"
  ok = check(r, "fill-lock 락 키 프리픽스", fill_lock.lock_key("abc") == "sb-lock:abc") and ok

  -- fill-lock 계약 벡터: fake store로 internal/fill_lock_test.go의 핵심 분기를 미러링한다.
  local function fake_store(tbl, rec)
    tbl = tbl or {}
    rec = rec or {}
    return {
      tbl = tbl, rec = rec,
      setnx = function(k, v, ttl)
        rec.ttl = ttl
        if tbl[k] then return false, "exists" end
        tbl[k] = v
        return true
      end,
      get = function(k) return tbl[k] end,
      delete = function(k) tbl[k] = nil end,
    }
  end

  local ft = {}
  local frec = {}
  local fs = fake_store(ft, frec)
  local won, tok = fill_lock.acquire(fs, "k1", 1)
  ok = check(r, "fill-lock acquire 승리", won == true and tok ~= "") and ok
  fill_lock.release(fs, "k1", "other-token")
  ok = check(r, "fill-lock release 토큰 불일치 유지", ft[fill_lock.lock_key("k1")] == tok) and ok
  fill_lock.release(fs, "k1", tok)
  ok = check(r, "fill-lock release 토큰 일치 해제", ft[fill_lock.lock_key("k1")] == nil) and ok

  local w2, f2, t2 = fill_lock.or_wait(fake_store(), "k2", { enabled = false })
  ok = check(r, "fill-lock 비활성은 무조건 승자 통과", w2 == true and f2 == nil and t2 == "") and ok

  local ft3 = { ["k3"] = "RAW" } -- 미스 조회와 락 획득 사이 다른 요청이 채운 값
  local w3, f3, t3 = fill_lock.or_wait(fake_store(ft3), "k3", { enabled = true, ttl_ms = 3000, wait_ms = 1 })
  ok = check(r, "fill-lock 승자 더블체크는 자기 락을 풀고 값을 서빙",
    w3 == false and f3 == "RAW" and t3 == "" and ft3[fill_lock.lock_key("k3")] == nil) and ok

  local ft4 = {}
  ft4[fill_lock.lock_key("k4")] = "other" -- 남이 락 보유 중
  local w4, f4, t4 = fill_lock.or_wait(fake_store(ft4), "k4", { enabled = true, ttl_ms = 3000, wait_ms = 1 })
  ok = check(r, "fill-lock 패자 기한 초과는 nil 통과", w4 == false and f4 == nil and t4 == "") and ok

  -- or_wait가 ttl_ms<=0을 기본 3000ms로 폴백하는지(초 단위 3으로 변환돼 setnx에 전달)
  local _w5, _f5, _t5 = fill_lock.or_wait(fake_store(ft, frec), "k5", { enabled = true, ttl_ms = 0, wait_ms = 1 })
  ok = check(r, "fill-lock ttl_ms 0은 기본 3000ms 폴백", frec.ttl == 3) and ok

  return ok, table.concat(r, "\n")
end

return _M
