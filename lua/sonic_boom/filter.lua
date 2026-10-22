-- 필터 판정(게이트웨이 중립). 정규식은 ngx.re "jo"(PCRE JIT + 패턴 캐시)로
-- 요청마다 같은 패턴 문자열이 컴파일을 재사용한다 — Go판 c28b548와 같은 원칙.
local find = ngx.re.find

local _M = {}

-- filters: {{name=, rules={{header="", regexp=...}, ...}, cache_ttl=0}, ...}
-- req: {path_with_query=string, headers={name=value}}  (헤더는 어댑터가 스냅샷)
-- 반환: cacheable(bool), ttl(int)
function _M.match(filters, global_ttl, req)
  if not filters or #filters == 0 then
    return true, global_ttl
  end
  for _, f in ipairs(filters) do
    local all = true
    for _, rule in ipairs(f.rules or {}) do
      local subject
      if rule.header and rule.header ~= "" then
        subject = req.headers[rule.header]
      else
        subject = req.path_with_query
      end
      if not subject or not find(subject, rule.regexp, "jo") then
        all = false
        break
      end
    end
    if all then
      return true, f.cache_ttl > 0 and f.cache_ttl or global_ttl
    end
  end
  return false, 0
end

return _M
