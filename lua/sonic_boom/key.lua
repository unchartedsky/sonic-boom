-- 캐시 키 파생(게이트웨이 중립 — ngx.*/resty.* 만 사용).
-- 직렬화는 모든 필드와 query·vary의 키/값 하나하나를 "길이:값"으로 인코딩해 붙인다:
-- 값 안에 어떤 바이트가 와도 필드·값 경계가 흐려지지 않는다(구분자 주입 불가 —
-- `?x=a,b` ≡ `?x=a&x=b` 충돌 방지). "sbk2"은 포맷 자체 버전이다.
local digest = require "resty.openssl.digest"
local sha256_hex, serialize

local function field(v)
  return #v .. ":" .. v
end

-- query: {k={v1,v2}} 정렬된 키, 값은 주어진 순서 유지
local function query_string(query)
  if not query then return "" end
  local keys = {}
  for k in pairs(query) do keys[#keys + 1] = k end
  table.sort(keys)
  local parts = {}
  for _, k in ipairs(keys) do
    for _, v in ipairs(query[k]) do
      parts[#parts + 1] = field(k) .. field(v)
    end
  end
  return table.concat(parts)
end

-- vary: {k=v} 정렬, 값 있는 헤더만(빈 값 제외는 어댑터 책임)
local function vary_string(vary)
  if not vary then return "" end
  local keys = {}
  for k in pairs(vary) do keys[#keys + 1] = k end
  table.sort(keys)
  local parts = {}
  for _, k in ipairs(keys) do
    parts[#parts + 1] = field(k) .. field(vary[k])
  end
  return table.concat(parts)
end

function serialize(i)
  return "sbk2"
    .. field(i.consumer or "")
    .. field(i.service or "")
    .. field(i.route or "")
    .. field(i.method)
    .. field(i.path)
    .. field(query_string(i.query))
    .. field(vary_string(i.vary))
    .. field(tostring(i.ttl))
    .. field(i.body or "")
end

function sha256_hex(s)
  local d = assert(digest.new("sha256"))
  local bin = assert(d:final(s))
  return (bin:gsub(".", function(c) return string.format("%02x", c:byte()) end))
end

-- k: {consumer, service, route, method, path, query, vary, ttl, body}
-- version: cache_version(설정). prefix: 제품별 키 접두(기본 sbl, APISIX는 sba) — 해시 입력에 들지 않는다.
-- 결과 형식은 R3: {prefix}:{version}:{sha256-hex}
local _M = {}
function _M.derive(k, version, prefix)
  return (prefix or "sbl") .. ":" .. version .. ":" .. sha256_hex(serialize(k))
end

_M.serialize = serialize  -- selfcheck가 골든 직렬화 단얩에 쓴다
return _M
