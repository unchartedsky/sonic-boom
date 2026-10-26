-- 골든 벡터 자기검증(Nginx 런타임 전용 — bare luajit엔 ngx.re·openssl FFI가 없다).
-- 기대 해시는 구현과 별개로 파이썬 hashlib가 계산한 값이다(2026-10-23, sbk2 포맷).
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
    hash = "a77103a405e23ded32012fb275bde4f38ff0473dcc43c14a33af78e9d943ede7",
  },
  {
    name = "V2 body+POST",
    input = {
      method = "POST", path = "/api/login", query = {}, vary = {}, ttl = 30,
      body = '{"u":"a","p":"b"}',
    },
    hash = "c70cce71c036ec77fe2f8b24615521e869350ed4eba4b629dd839916c8cf34b6",
  },
  {
    name = "V3 multi-query",
    input = {
      consumer = "anon", service = "svc", route = "r", method = "GET",
      path = "/x", query = { a = { "1", "2" }, b = { "3" } }, vary = {}, ttl = 300,
      body = "",
    },
    hash = "ef20363874d8b1795c002d0f484fd4ec7926b31e66a086441b1b7b7ab0fb53ef",
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

  -- 구분자 주입 회귀: sbk2 길이 접두 덕에 단일 값 "a,b"과 복수 값 a·b가 같은 키로 뭉개지지 않는다.
  local q1 = key.derive({ method = "GET", path = "/q", query = { x = { "a,b" } }, vary = {}, ttl = 1, body = "" }, "0.1.0")
  local q2 = key.derive({ method = "GET", path = "/q", query = { x = { "a", "b" } }, vary = {}, ttl = 1, body = "" }, "0.1.0")
  ok = check(r, "key query 구분자 충돌 없음(sbk2)", q1 ~= q2, q1) and ok

  -- 헤더 정리: ratelimit-remaining은 떼고 일반 헤더는 남긴다(plain find — % 이스케이프 잔재 회귀).
  local headers = require "sonic_boom.headers"
  local cleaned = headers.clean_for_replay({
    ["Content-Type"] = "application/json",
    ["X-RateLimit-Remaining"] = "42",
    ["x-ratelimit-remaining-second"] = "7",
  })
  ok = check(r, "headers ratelimit-remaining 제거·일반 헤더 유지",
    cleaned["Content-Type"] == "application/json"
      and cleaned["X-RateLimit-Remaining"] == nil
      and cleaned["x-ratelimit-remaining-second"] == nil) and ok

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

  -- flow.access 조회 판정: 버전 불일치는 purge 후 Miss(낡은 값을 Hit로 서빙하지 않는다 —
  -- CLAUDE.md "return 시맨틱 보존"), 신선하면 Hit 단락, ttl 초과는 Refresh. e2e로는 닿지 않는 분기다.
  local flow = require "sonic_boom.flow" -- flow가 selfcheck를 지연 require하므로 여기서 불러 순환을 피한다
  local hdr = {}
  local fenv = {
    method = function() return "GET" end,
    req_header = function() return nil end,
    path = function() return "/selfcheck-flow" end,
    path_with_query = function() return "/selfcheck-flow" end,
    raw_body = function() return "", nil end,
    ids = function() return "selfcheck-flow-" .. ngx.now(), "", "" end,
    set_header = function(n, v) hdr[n] = v end,
    log = { err = function() end, notice = function() end },
  }
  local fconf = {
    request_method = { "GET" }, cache_ttl = 60, strategy = "in-memory",
    cache_version = "sc-v2", fill_lock = { enabled = false },
  }
  local cid = fenv.ids()
  fenv.ids = function() return cid, "", "" end -- 같은 키로 네 번 조회
  local mstore = require("sonic_boom.store.memory").new()
  local _e0, sb0 = flow.access(fconf, fenv, nil)
  local fkey = sb0 and sb0.key
  ok = check(r, "flow 빈 저장소 → Miss", hdr["X-Cache-Status"] == "Miss" and fkey ~= nil) and ok
  if fkey then
    local function put(version, age)
      mstore.set(fkey, entry.encode({ status = 200, headers = {}, body = "cached",
        timestamp = ngx.time() - age, version = version, ttl = 60 }), 60)
    end
    put("sc-v1", 0)
    local e1, sb1 = flow.access(fconf, fenv, nil)
    ok = check(r, "flow 버전 불일치 → purge 후 Miss",
      e1 == nil and sb1 ~= nil and hdr["X-Cache-Status"] == "Miss" and mstore.get(fkey) == nil) and ok
    put("sc-v2", 0)
    local e2 = flow.access(fconf, fenv, nil)
    ok = check(r, "flow 신선한 엔트리 → Hit 단락",
      e2 ~= nil and e2.status == 200 and e2.body == "cached" and e2.headers["X-Cache-Status"] == "Hit") and ok
    put("sc-v2", 120)
    local e3, sb3 = flow.access(fconf, fenv, nil)
    ok = check(r, "flow ttl 초과 → Refresh", e3 == nil and sb3 ~= nil and hdr["X-Cache-Status"] == "Refresh") and ok
    mstore.delete(fkey)
  end

  -- fill-lock flow 벡터: 남의 토큰이 락을 선점하고 대기 중 타이머가 값을 채우면, 패자는
  -- 그 값을 Hit로 서빙한다(flow.lua raw = filled 핸드오프 — e2e로는 닿지 않는 분기).
  local tstore = require("sonic_boom.store.memory").new()
  local tid = "sc-fl-" .. ngx.now()
  local tenv = {
    method = function() return "GET" end,
    req_header = function() return nil end,
    path = function() return "/selfcheck-flow" end,
    path_with_query = function() return "/selfcheck-flow" end,
    raw_body = function() return "", nil end,
    ids = function() return tid, "", "" end,
    set_header = function(n, v) hdr[n] = v end,
    log = { err = function() end, notice = function() end },
  }
  local tnolock = {
    request_method = { "GET" }, cache_ttl = 60, strategy = "in-memory",
    cache_version = "sc-fl", fill_lock = { enabled = false },
  }
  local tlock = {
    request_method = { "GET" }, cache_ttl = 60, strategy = "in-memory",
    cache_version = "sc-fl", fill_lock = { enabled = true, ttl_ms = 3000, wait_ms = 500 },
  }
  local _tx, tsb0 = flow.access(tnolock, tenv, nil) -- 키 학습용 1회(Miss, 잔여물 없음)
  local tkey = tsb0 and tsb0.key
  ok = check(r, "flow fill-lock 패자 준비(키 파생)", tkey ~= nil) and ok
  if tkey then
    local lk = fill_lock.lock_key(tkey)
    local held = tstore.setnx(lk, "sc-other-winner", 3)
    local tok, terr = ngx.timer.at(0.05, function()
      tstore.set(tkey, entry.encode({ status = 200, headers = {}, body = "filled-by-other",
        timestamp = ngx.time(), version = "sc-fl", ttl = 60 }), 60)
    end)
    local texit = flow.access(tlock, tenv, nil)
    ok = check(r, "flow fill-lock 패자는 대기 중 채워진 값을 Hit로 서빙",
      held and tok ~= nil and texit ~= nil and texit.status == 200
        and texit.body == "filled-by-other" and texit.headers["X-Cache-Status"] == "Hit") and ok
    tstore.delete(lk)
    tstore.delete(tkey)
  end

  -- cache_control 요청층 가드(Go cacheableRequest 미러 — internal/sonic_boom.go cacheableRequest):
  -- true(기본)면 요청 Cache-Control의 no-store/no-cache(대소문자 무관)와 Authorization 요청을
  -- Bypass, false면 가드 없음. KTD3 — 벡터는 conf에 cache_control을 명시해 코어 판정 경로만 검치고,
  -- 스키마 기본값 경로(필드 생략)는 U2의 e2e가 담당한다.
  local function genv(headers, id)
    local ghdr = {}
    local gid = "sc-guard-" .. id .. "-" .. ngx.now()
    return ghdr, {
      method = function() return "GET" end,
      req_header = function(_, h) return headers[h] end,
      path = function() return "/selfcheck-flow" end,
      path_with_query = function() return "/selfcheck-flow" end,
      raw_body = function() return "", nil end,
      ids = function() return gid, "", "" end,
      set_header = function(n, v) ghdr[n] = v end,
      log = { err = function() end, notice = function() end },
    }
  end
  local function gconf(cc_on)
    return {
      request_method = { "GET" }, cache_ttl = 60, strategy = "in-memory",
      cache_version = "sc-guard", fill_lock = { enabled = false },
      cache_control = cc_on,
    }
  end
  local ghdr_a, genv_a = genv({ Authorization = "Basic dXNlcjpwYXNz" }, "a")
  local ga, gsb_a = flow.access(gconf(true), genv_a, nil)
  ok = check(r, "flow cache_control 가드: Authorization 요청 Bypass",
    ga == nil and gsb_a == nil and ghdr_a["X-Cache-Status"] == "Bypass") and ok

  local ghdr_b, genv_b = genv({ ["Cache-Control"] = "no-store" }, "b")
  local gb, gsb_b = flow.access(gconf(true), genv_b, nil)
  ok = check(r, "flow cache_control 가드: no-store 요청 Bypass",
    gb == nil and gsb_b == nil and ghdr_b["X-Cache-Status"] == "Bypass") and ok

  local ghdr_c, genv_c = genv({ Authorization = "Basic dXNlcjpwYXNz" }, "c")
  local gc, gsb_c = flow.access(gconf(false), genv_c, nil)
  ok = check(r, "flow cache_control 가드: false는 Authorization도 캐시",
    gc == nil and gsb_c ~= nil and gsb_c.key ~= nil
      and ghdr_c["X-Cache-Status"] == "Miss") and ok

  local ghdr_d, genv_d = genv({ ["Cache-Control"] = "max-age=0, No-Cache" }, "d")
  local gd, gsb_d = flow.access(gconf(true), genv_d, nil)
  ok = check(r, "flow cache_control 가드: 대소문자 혼용 no-cache Bypass",
    gd == nil and gsb_d == nil and ghdr_d["X-Cache-Status"] == "Bypass") and ok

  local ghdr_e, genv_e = genv({ ["Cache-Control"] = "max-age=60" }, "e")
  local ge, gsb_e = flow.access(gconf(true), genv_e, nil)
  ok = check(r, "flow cache_control 가드: 무관 디렉티브 정상 캐시",
    ge == nil and gsb_e ~= nil and gsb_e.key ~= nil
      and ghdr_e["X-Cache-Status"] == "Miss") and ok

  return ok, table.concat(r, "\n")
end

return _M
