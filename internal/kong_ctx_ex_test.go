package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/bridge/bridgetest"
	"github.com/Kong/go-pdk/ctx"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	"testing"
)

func mockCtx(t *testing.T, s []bridgetest.MockStep) ctx.Ctx {
	return ctx.Ctx{PdkBridge: bridge.New(bridgetest.Mock(t, s))}
}

func TestSetPluginEx(t *testing.T) {
	k := "key"
	v := map[string]int{"foo": 1, "bar": 2}

	jsonData, err := json.Marshal(v)
	assert.NoError(t, err)

	data, err := structpb.NewValue(jsonData)
	assert.NoError(t, err)

	type args struct {
		kong  *pdk.PDK
		k     string
		value interface{}
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		// TODO: Add test cases.
		{
			name: "SetPluginEx",
			args: args{
				kong: &pdk.PDK{
					Ctx: mockCtx(t, []bridgetest.MockStep{
						{Method: "kong.ctx.shared.set", Args: &kong_plugin_protocol.KV{K: k, V: data}},
					}),
				},
				k:     k,
				value: v,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(
				t,
				SetPluginEx(tt.args.kong, tt.args.k, tt.args.value),
				fmt.Sprintf("SetPluginEx(%v, %v, %v)", tt.args.kong, tt.args.k, tt.args.value),
			)
		})
	}
}

func TestGetPluginAnyEx(t *testing.T) {
	k := "key"
	v := map[string]int{"foo": 1, "bar": 2}

	jsonData, err := json.Marshal(v)
	assert.NoError(t, err)

	//data, err := structpb.NewValue(jsonData)
	//assert.NoError(t, err)

	base64Str := base64.StdEncoding.EncodeToString(jsonData)

	type args struct {
		kong *pdk.PDK
		k    string
		v    any
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		// TODO: Add test cases.
		{
			name: "GetPluginAnyEx",
			args: args{
				kong: &pdk.PDK{
					Ctx: mockCtx(t, []bridgetest.MockStep{
						{Method: "kong.ctx.shared.get", Args: bridge.WrapString(k), Ret: structpb.NewStringValue(base64Str)},
					}),
				},
				k: k,
				v: v,
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vv := map[string]int{}
			err := GetPluginAnyEx(tt.args.kong, tt.args.k, &vv)
			tt.wantErr(t, err, fmt.Sprintf("GetPluginAnyEx(%v, %v, %v)", tt.args.kong, tt.args.k, tt.args.v))
			assert.Equal(t, tt.args.v, vv)
		})
	}
}
