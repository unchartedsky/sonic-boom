-- sonic-boom-lua: sonic-boom(Go, go-pdk)의 Kong 네이티브 Lua 포팅.
-- U2 단계 — access는 selfcheck 진입만 있다. 조회·단락은 U3부터.
local plugin = {
  PRIORITY = 100,  -- Go판 pluginserver 우선순위(1)와 같은 위치감: 인증 아래, 변환 위는 U3에서 재조정
  VERSION = "0.1.0",
}

function plugin:access(conf)
  if conf.selfcheck then
    local sc = require "sonic_boom.selfcheck"
    local ok, report = sc.run()
    if ok then
      return kong.response.exit(200, "SELFCHECK OK\n" .. report .. "\n")
    end
    return kong.response.exit(500, "SELFCHECK FAIL\n" .. tostring(report) .. "\n")
  end
end

return plugin
