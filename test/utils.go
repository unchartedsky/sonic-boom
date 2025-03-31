package test

import (
	"net"

	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/log"
)

func MockLogDefault() log.Log {
	server, client := net.Pipe()
	server.Close() //nolint directives: gosimple
	client.Close() //nolint directives: gosimple
	return log.Log{
		PdkBridge: bridge.New(client),
	}
}
