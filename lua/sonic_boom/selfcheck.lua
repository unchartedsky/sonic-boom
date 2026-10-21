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

  return ok, table.concat(r, "\n")
end

return _M
