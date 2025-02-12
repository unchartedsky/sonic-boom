package internal

import (
	"fmt"
	"os"
	"testing"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/bridge/bridgetest"
	"github.com/Kong/go-pdk/client"
	kongctx "github.com/Kong/go-pdk/ctx"
	"github.com/Kong/go-pdk/ip"
	"github.com/Kong/go-pdk/nginx"
	"github.com/Kong/go-pdk/node"
	"github.com/Kong/go-pdk/request"
	"github.com/Kong/go-pdk/response"
	"github.com/Kong/go-pdk/router"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"github.com/Kong/go-pdk/service"
	service_request "github.com/Kong/go-pdk/service/request"
	service_response "github.com/Kong/go-pdk/service/response"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/unchartedsky/sonic-boom/test"
)

func mockPdkDefault(t *testing.T) *pdk.PDK {
	q, err := bridge.WrapHeaders(map[string][]string{
		"X-Test": {"wayback"},
	})
	assert.NoError(t, err)

	b := bridge.New(bridgetest.Mock(t, []bridgetest.MockStep{
		{Method: "kong.client.get_consumer", Ret: &kong_plugin_protocol.Consumer{Id: "001", Username: "Jon Doe"}},

		{Method: "kong.router.get_service", Ret: &kong_plugin_protocol.Service{
			Id:       "003:004",
			Name:     "self_service",
			Protocol: "http",
			Path:     "/v0/left",
		}},
		{Method: "kong.router.get_route", Ret: &kong_plugin_protocol.Route{
			Id:        "001:002",
			Name:      "route_66",
			Protocols: []string{"http", "tcp"},
			Paths:     []string{"/v0/left", "/v1/this"},
		}},

		{Method: "kong.request.get_method", Ret: bridge.WrapString("POST")},
		{Method: "kong.request.get_path", Ret: bridge.WrapString("/login/orout")},

		{Method: "kong.request.get_header", Args: bridge.WrapString("Host"), Ret: bridge.WrapString("example.com")},
		{Method: "kong.request.get_query", Args: &kong_plugin_protocol.Int{V: 1000}, Ret: q},
		//{"kong.request.get_path_with_query", nil, bridge.WrapString("/login/orout?ref=wayback")},
	}))
	return &pdk.PDK{

		Client:          client.Client{PdkBridge: b},
		Ctx:             kongctx.Ctx{PdkBridge: b},
		Log:             test.MockLogDefault(),
		Nginx:           nginx.Nginx{PdkBridge: b},
		Request:         request.Request{PdkBridge: b},
		Response:        response.Response{PdkBridge: b},
		Router:          router.Router{PdkBridge: b},
		IP:              ip.Ip{PdkBridge: b},
		Node:            node.Node{PdkBridge: b},
		Service:         service.Service{PdkBridge: b},
		ServiceRequest:  service_request.Request{PdkBridge: b},
		ServiceResponse: service_response.Response{PdkBridge: b},
	}
}

func TestNewCacheKey(t *testing.T) {
	l := zerolog.New(os.Stderr).With().Timestamp().Logger()
	type args struct {
		kong     *pdk.PDK
		conf     *Config
		body     []byte
		cacheTTL int
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "test",
			args: args{
				kong: mockPdkDefault(t),
				conf: &Config{
					logger:      &Logger{Logger: &l},
					VaryHeaders: []string{"Host"},
				},
				body:     []byte("test"),
				cacheTTL: 100,
			},
			want: "16020438735363915618",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewCacheKey(tt.args.kong, tt.args.conf, tt.args.body, tt.args.cacheTTL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCacheKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NewCacheKey() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_generateCacheKeyId(t *testing.T) {
	l := zerolog.New(os.Stderr).With().Timestamp().Logger()
	type args struct {
		logger   *Logger
		cacheKey *CacheKey
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "test",
			args: args{
				logger: &Logger{Logger: &l},
				cacheKey: &CacheKey{
					Method: "GET",
					URL:    "/v0/left",
				},
			},
			want: "12183599531055065935",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateCacheKeyID(tt.args.logger, tt.args.cacheKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateCacheKeyID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("generateCacheKeyID() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCacheKey_String(t *testing.T) {
	type fields struct {
		Consumer  string
		Service   string
		Route     string
		Method    string
		URL       string
		QueryArgs map[string][]string
		Headers   map[string]string
		Body      []byte
		CacheTTL  int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
		{
			name: "test",
			fields: fields{
				Consumer: "consumer",
				Method:   "HEAD",
				URL:      "/v0/left",
				Body:     []byte("test"),
				Headers: map[string]string{
					"Host": "example.com",
				},
				CacheTTL: 100,
			},
			want: "CacheKey{Consumer: consumer, Service: , Route: , Method: HEAD, URL: /v0/left, QueryArgs: map[], Headers: map[Host:example.com], BodyLen: 4, CacheTTL: 100}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CacheKey{
				Consumer:  tt.fields.Consumer,
				Service:   tt.fields.Service,
				Route:     tt.fields.Route,
				Method:    tt.fields.Method,
				URL:       tt.fields.URL,
				QueryArgs: tt.fields.QueryArgs,
				Headers:   tt.fields.Headers,
				Body:      tt.fields.Body,
				CacheTTL:  tt.fields.CacheTTL,
			}
			assert.Equalf(t, tt.want, c.String(), "String()")
			assert.Equalf(t, tt.want, fmt.Sprintf("%v", c), "String()")
			assert.Equalf(t, tt.want, fmt.Sprintf("%s", c), "String()") //nolint:gosimple
		})
	}
}
