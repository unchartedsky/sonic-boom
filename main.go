package main

import (
	"github.com/Kong/go-pdk/server"
	"github.com/unchartedsky/sonic-boom/internal"
)

func main() {
	internal.New()
	err := server.StartServer(internal.New, internal.Version, internal.Priority)
	if err != nil {
		panic(err)
	}
}
