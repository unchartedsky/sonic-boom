-- sonic-boom-lua: sonic-boom(Go, go-pdk)의 Kong 네이티브 Lua 포팅.
-- U1은 로딩 확인용 no-op — 단계 구현은 U3(access)·U4(header/body_filter)부터.
local plugin = {
  PRIORITY = 100,  -- Go판 pluginserver 우선순위(1)와 같은 위치감: 인증 아래, 변환 위는 U3에서 재조정
  VERSION = "0.1.0",
}

return plugin
