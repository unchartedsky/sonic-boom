package test

import (
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/log"
	"net"
)

func MockLogDefault() log.Log {
	server, client := net.Pipe()
	server.Close()
	client.Close()
	return log.Log{
		PdkBridge: bridge.New(client),
	}
}
