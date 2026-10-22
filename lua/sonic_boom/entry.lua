-- 캐시 엔트리 직렬화(게이트웨이 중립): "메타JSON길이\n{메타JSON}{raw body}".
-- body는 어떤 바이너리(\n, \0 포함)든 그대로 붙는다 — 메타 길이 접두가 경계를 준다.
local cjson = require "cjson.safe"

local _M = {}

-- e: {status, headers, body, timestamp, version, ttl}
function _M.encode(e)
  local meta = cjson.encode({
    status = e.status,
    headers = e.headers,
    timestamp = e.timestamp,
    version = e.version,
    ttl = e.ttl,
  })
  return #meta .. "\n" .. meta .. (e.body or "")
end

function _M.decode(s)
  local nl = s:find("\n", 1, true)
  if not nl then return nil, "entry: length prefix missing" end
  local n = tonumber(s:sub(1, nl - 1))
  if not n then return nil, "entry: bad length prefix" end
  local meta = cjson.decode(s:sub(nl + 1, nl + n))
  if not meta then return nil, "entry: meta decode failed" end
  meta.body = s:sub(nl + n + 1)
  return meta
end

return _M
