-- Hit 재생용 헤더 정리(게이트웨이 중립) — Go sonic_boom.go의 hop-by-hop·삭제 목록 이식.
-- RFC 2616 13.5.1 + content-length(Kong exit이 다시 계산) + rate limit 누출 방지.
local HOP = {
  connection = true, ["keep-alive"] = true, ["proxy-authenticate"] = true,
  ["proxy-authorization"] = true, te = true, trailers = true,
  ["transfer-encoding"] = true, upgrade = true, ["content-length"] = true,
}
local DROP = { ["x-cache-status"] = true, age = true }

local _M = {}

function _M.clean_for_replay(headers)
  local out = {}
  for name, v in pairs(headers or {}) do
    local lower = name:lower()
    if not HOP[lower] and not DROP[lower] and not lower:find("ratelimit-remaining", 1, true) then
      out[name] = v
    end
  end
  return out
end

return _M
