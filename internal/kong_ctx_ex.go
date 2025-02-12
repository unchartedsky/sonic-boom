package internal

import (
	"encoding/base64"
	"encoding/json"
	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"google.golang.org/protobuf/types/known/structpb"
)

func SetPluginEx(kong *pdk.PDK, k string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return SetPlugin(kong, k, data)
}

func GetPluginAnyEx(kong *pdk.PDK, k string, v any) error {
	data, err := GetPluginString(kong, k)
	if err != nil {
		return err
	}
	bytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(bytes, &v); err != nil {
		return err
	}
	return nil
}

// NOTE kong.ctx.plugin 을 구현해서 쓰려다 잘 안 되어서 임시로 kong.ctx.shared 의 구현을 베껴놨다.
func SetPlugin(kong *pdk.PDK, k string, value interface{}) error {
	v, err := structpb.NewValue(value)
	if err != nil {
		return err
	}

	return kong.Ctx.Ask(`kong.ctx.shared.set`, &kong_plugin_protocol.KV{K: k, V: v}, nil)
}

func GetPluginAny(kong *pdk.PDK, k string) (interface{}, error) {
	return kong.Ctx.AskValue(`kong.ctx.shared.get`, bridge.WrapString(k))
}

// kong.Ctx.GetSharedString() returns a string value from the `kong.ctx.shared` request context table.
func GetPluginString(kong *pdk.PDK, k string) (string, error) {
	v, err := GetPluginAny(kong, k)
	if err != nil {
		return "", err
	}

	s, ok := v.(string)
	if ok {
		return s, nil
	}

	return "", bridge.ReturnTypeError("string")
}

// kong.Ctx.GetSharedFloat() returns a float value from the `kong.ctx.shared` request context table.
func GetPluginFloat(kong *pdk.PDK, k string) (float64, error) {
	v, err := GetPluginAny(kong, k)
	if err != nil {
		return 0, err
	}

	f, ok := v.(float64)
	if ok {
		return f, nil
	}

	return 0, bridge.ReturnTypeError("number")
}
