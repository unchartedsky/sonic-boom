package internal

import (
	"reflect"
	"testing"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/bridge/bridgetest"
	"github.com/Kong/go-pdk/log"
	"github.com/Kong/go-pdk/request"
	"github.com/stretchr/testify/assert"
)

func defaultLogger() *Logger {
	return NewLogger(&LogConfig{
		LogLevel:              "info",
		ConsoleLoggingEnabled: true,
		FileLogConf: &FileLogConfig{
			Enabled: false,
		},
	})
}

func TestConfig_Access(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
			}
			conf.Access(tt.args.kong)
		})
	}
}

func TestConfig_Response(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
			}
			conf.Response(tt.args.kong)
		})
	}
}

// See https://github.com/Kong/go-pdk/blob/master/log/log_test.go
func mockLog(t *testing.T, s []bridgetest.MockStep) log.Log {
	return log.Log{PdkBridge: bridge.New(bridgetest.Mock(t, s))}
}

func mockLogDefault(t *testing.T) log.Log {
	return mockLog(t, []bridgetest.MockStep{
		{Method: "kong.log.alert"},
		{Method: "kong.log.crit"},
		{Method: "kong.log.err"},
		{Method: "kong.log.warn"},
		{Method: "kong.log.notice"},
		{Method: "kong.log.info"},
		{Method: "kong.log.debug"},
	})
}

// See https://github.com/Kong/go-pdk/blob/master/request/request_test.go
func mockRequest(t *testing.T, s []bridgetest.MockStep) request.Request {
	return request.Request{PdkBridge: bridge.New(bridgetest.Mock(t, s))}
}

func TestConfig_cacheableRequestMethod(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
		{
			name: "test matched http method",
			fields: fields{
				RequestMethods: []string{"GET", "HEAD"},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_method", Ret: bridge.WrapString("GET")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want: true,
		},
		{
			name: "test unmatched http method",
			fields: fields{
				RequestMethods: []string{"GET", "HEAD"},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_method", Ret: bridge.WrapString("POST")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//t.Parallel()
			conf := Config{
				ResponseCodes:  tt.fields.ResponseCodes,
				RequestMethods: tt.fields.RequestMethods,
				ContentTypes:   tt.fields.ContentTypes,
				VaryHeaders:    tt.fields.VaryHeaders,
				CacheTTL:       tt.fields.CacheTTL,
				CacheControl:   tt.fields.CacheControl,
				Strategy:       tt.fields.Strategy,
				Redis:          tt.fields.Redis,
			}
			if got := conf.cacheableRequestMethod(tt.args.kong); got != tt.want {
				t.Errorf("cacheableRequestMethod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheableResponse(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
			}
			if got := conf.cacheableResponse(tt.args.kong); got != tt.want {
				t.Errorf("cacheableResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheableResponseContentType(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
			}
			if got := conf.cacheableResponseContentType(tt.args.kong); got != tt.want {
				t.Errorf("cacheableResponseContentType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheableResponseStatus(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		Strategy             string
		Redis                RedisConfig
		Box                  map[string]any
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conf := Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
			}
			if got := conf.cacheableResponseStatus(tt.args.kong); got != tt.want {
				t.Errorf("cacheableResponseStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

//func TestConfig_checkConfig(t *testing.T) {
//	type fields struct {
//		ResponseCodes        []int
//		RequestMethods       []string
//		ContentTypes         []string
//		VaryHeaders          []string
//		CacheTTL             int
//		CacheControl         bool
//		CacheableBodyMaxSize int
//		Strategy             string
//		Redis                RedisConfig
//		Box                  map[string]any
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//		{
//			name: "checkConfig",
//			fields: fields{
//				ResponseCodes:        []int{200, 301, 404},
//				RequestMethods:       []string{"GET", "HEAD"},
//				ContentTypes:         []string{"text/plain", "application/json", "application/json; charset=utf-8"},
//				VaryHeaders:          []string{},
//				CacheTTL:             0,
//				CacheControl:         false,
//				CacheableBodyMaxSize: 10000,
//				Strategy:             "redis",
//			},
//			wantErr: false,
//		},
//		{
//			name: "checkConfigWhenInvalidCacheTTL",
//			fields: fields{
//				ResponseCodes:        []int{200, 301, 404},
//				RequestMethods:       []string{"GET", "HEAD"},
//				ContentTypes:         []string{"text/plain", "application/json", "application/json; charset=utf-8"},
//				VaryHeaders:          []string{},
//				CacheTTL:             -1,
//				CacheControl:         false,
//				CacheableBodyMaxSize: 10000,
//				Strategy:             "redis",
//			},
//			wantErr: true,
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			t.Parallel()
//			conf := Config{
//				ResponseCodes:        tt.fields.ResponseCodes,
//				RequestMethods:       tt.fields.RequestMethods,
//				ContentTypes:         tt.fields.ContentTypes,
//				VaryHeaders:          tt.fields.VaryHeaders,
//				CacheTTL:             tt.fields.CacheTTL,
//				CacheControl:         tt.fields.CacheControl,
//				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
//				Strategy:             tt.fields.Strategy,
//				Redis:                tt.fields.Redis,
//			}
//			if err := conf.checkConfig(); (err != nil) != tt.wantErr {
//				t.Errorf("checkConfig() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}

func configDefault() *Config {
	return &Config{
		ResponseCodes:        []int{200, 301, 404},
		RequestMethods:       []string{"GET", "HEAD"},
		ContentTypes:         []string{"text/plain", "application/json", "application/json; charset=utf-8"},
		VaryHeaders:          []string{},
		Filters:              []Filter{},
		CacheTTL:             0,
		CacheControl:         false,
		CacheableBodyMaxSize: 0,
		CacheVersion:         "",
		Strategy:             "redis",

		InMemory: InMemoryConfig{
			MaxCost:     1000000,
			NumCounters: 1000000,
			BufferItems: 64,
		},

		Redis: RedisConfig{
			// Host:              "localhost",
			Port:              6379,
			DBNumber:          0,
			PoolSize:          10,
			MaxRetries:        3,
			MinRetryBackoffMs: 8,
			MaxRetryBackoffMs: 512,
			DialTimeout:       5,
			ReadTimeout:       3,
			WriteTimeout:      3,
			PoolTimeout:       5,
			IdleTimeout:       1,
		},

		RedisCluster: RedisClusterConfig{
			// Addrs:             []string{"localhost:6379"},
			PoolSize:          10,
			MaxRetries:        3,
			MinRetryBackoffMs: 8,
			MaxRetryBackoffMs: 512,
			DialTimeout:       5,
			ReadTimeout:       3,
			WriteTimeout:      3,
			PoolTimeout:       5,
			IdleTimeout:       1,
		},

		LogConf: LogConfig{
			LogLevel:              "info",
			ConsoleLoggingEnabled: true,
			FileLogConf: &FileLogConfig{
				Enabled:  false,
				Filename: "sonic-boom.log",
				Folder:   "/tmp/logs",
			},
			DiodeEnabled: true,
		},
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		want interface{}
	}{
		// TODO: Add test cases.
		{
			name: "test default values",
			want: configDefault(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := New(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New() = %v\n want %v", got, tt.want)
			}
		})
	}
}

func Test_overwritableHeader(t *testing.T) {
	type args struct {
		header string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overwritableHeader(tt.args.header); got != tt.want {
				t.Errorf("overwritableHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_serviceResponseRawBody(t *testing.T) {
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serviceResponseRawBody(tt.args.kong)
			if (err != nil) != tt.wantErr {
				t.Errorf("serviceResponseRawBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serviceResponseRawBody() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheablePath(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		Filters              []Filter
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		CacheVersion         string
		Strategy             string
		Redis                RedisConfig
		LogConf              LogConfig
		Box                  map[string]any
		logger               *Logger
	}
	type args struct {
		kong *pdk.PDK
		rule Rule
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
		{
			name: "test cacheable path",
			fields: fields{
				logger: defaultLogger(),
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
					}),
					Log: mockLog(t, []bridgetest.MockStep{
						{Method: "kong.log.debug"},
					}),
				},
				rule: Rule{
					Regexp: ".*ref.*",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				Filters:              tt.fields.Filters,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				CacheVersion:         tt.fields.CacheVersion,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
				LogConf:              tt.fields.LogConf,
				logger:               tt.fields.logger,
			}
			assert.Equalf(t, tt.want, conf.cacheablePath(tt.args.kong, tt.args.rule), "cacheablePath(%v, %v)", tt.args.kong, tt.args.rule)
		})
	}
}

func TestConfig_cacheableHeader(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		Filters              []Filter
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		CacheVersion         string
		Strategy             string
		Redis                RedisConfig
		LogConf              LogConfig
		Box                  map[string]any
		logger               *Logger
	}
	type args struct {
		kong *pdk.PDK
		rule Rule
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
		{
			name: "test cacheable header",
			fields: fields{
				logger: defaultLogger(),
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
				rule: Rule{
					Header: "Authorization",
					Regexp: ".*a29yZWFpb6aaaaa29yZWFpbnYwMjI4.*",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				Filters:              tt.fields.Filters,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				CacheVersion:         tt.fields.CacheVersion,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
				LogConf:              tt.fields.LogConf,
				logger:               tt.fields.logger,
			}
			assert.Equalf(t, tt.want, conf.cacheableHeader(tt.args.kong, tt.args.rule), "cacheableHeader(%v, %v)", tt.args.kong, tt.args.rule)
		})
	}
}

func TestConfig_filtered(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		Filters              []Filter
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		CacheVersion         string
		Strategy             string
		Redis                RedisConfig
		LogConf              LogConfig
		logger               *Logger
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
		want1  int
	}{
		// TODO: Add test cases.
		{
			name: "test with empty rules",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules:    []Rule{},
					},
				},
				CacheTTL: 2,
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{}),
					Log:     mockLogDefault(t),
				},
			},
			want:  true,
			want1: 1,
		},
		{
			name: "test cacheable path",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Regexp: ".*ref.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  true,
			want1: 1,
		},
		{
			name: "test cacheable path",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Regexp: ".*testing.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  false,
			want1: 0,
		},
		{
			name: "test cacheable header",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Header: "Authorization",
								Regexp: "Basic a29yZWFpb6aaaaa.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						//{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  true,
			want1: 1,
		},
		{
			name: "test cacheable header",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Header: "Authorization",
								Regexp: "Basic ABCD.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						//{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  false,
			want1: 0,
		},
		{
			name: "test cacheable header and path",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Regexp: ".*goodluck.*",
							},
							{
								Header: "Authorization",
								Regexp: "Basic a29yZWFpb6aaaaa.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  true,
			want1: 1,
		},
		{
			name: "test cacheable header and path",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Regexp: ".*testing.*",
							},
							{
								Header: "Authorization",
								Regexp: "Basic a29yZWFpb6aaaaa.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  false,
			want1: 0,
		},
		{
			name: "test cacheable header and path",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules: []Rule{
							{
								Regexp: ".*goodluck.*",
							},
							{
								Header: "Authorization",
								Regexp: "Basic ABCD.*",
							},
						},
					},
				},
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_path_with_query", Ret: bridge.WrapString("/login/goodluck?ref=wayback")},
						{Method: "kong.request.get_header", Args: bridge.WrapString("Authorization"), Ret: bridge.WrapString("Basic a29yZWFpb6aaaaa29yZWFpbnYwMjI4")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  false,
			want1: 0,
		},
	}
	for _, tt := range tests {
		//t.Parallel()
		t.Run(tt.name, func(t *testing.T) {
			conf := &Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				Filters:              tt.fields.Filters,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				CacheVersion:         tt.fields.CacheVersion,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
				LogConf:              tt.fields.LogConf,
				logger:               tt.fields.logger,
			}
			got, got1 := conf.filtered(tt.args.kong)
			assert.Equalf(t, tt.want, got, "filtered(%v)", tt.args.kong)
			assert.Equalf(t, tt.want1, got1, "filtered(%v)", tt.args.kong)
		})
	}
}

func TestConfig_cacheableRequest(t *testing.T) {
	type fields struct {
		ResponseCodes        []int
		RequestMethods       []string
		ContentTypes         []string
		VaryHeaders          []string
		Filters              []Filter
		CacheTTL             int
		CacheControl         bool
		CacheableBodyMaxSize int
		CacheVersion         string
		Strategy             string
		Redis                RedisConfig
		LogConf              LogConfig
		logger               *Logger
	}
	type args struct {
		kong *pdk.PDK
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
		want1  int
	}{
		// TODO: Add test cases.
		{
			name: "test with empty filters",
			fields: fields{
				logger:         defaultLogger(),
				RequestMethods: []string{"GET", "HEAD"},
				Filters:        []Filter{},
				CacheTTL:       2,
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_method", Ret: bridge.WrapString("GET")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  true,
			want1: 2,
		},
		{
			name: "test with empty rules",
			fields: fields{
				logger:         defaultLogger(),
				RequestMethods: []string{"GET", "HEAD"},
				Filters: []Filter{
					{
						Name:     "Test",
						CacheTTL: 1,
						Rules:    []Rule{},
					},
				},
				CacheTTL: 2,
			},
			args: args{
				kong: &pdk.PDK{
					Request: mockRequest(t, []bridgetest.MockStep{
						{Method: "kong.request.get_method", Ret: bridge.WrapString("GET")},
					}),
					Log: mockLogDefault(t),
				},
			},
			want:  true,
			want1: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &Config{
				ResponseCodes:        tt.fields.ResponseCodes,
				RequestMethods:       tt.fields.RequestMethods,
				ContentTypes:         tt.fields.ContentTypes,
				VaryHeaders:          tt.fields.VaryHeaders,
				Filters:              tt.fields.Filters,
				CacheTTL:             tt.fields.CacheTTL,
				CacheControl:         tt.fields.CacheControl,
				CacheableBodyMaxSize: tt.fields.CacheableBodyMaxSize,
				CacheVersion:         tt.fields.CacheVersion,
				Strategy:             tt.fields.Strategy,
				Redis:                tt.fields.Redis,
				LogConf:              tt.fields.LogConf,
				logger:               tt.fields.logger,
			}
			got, got1 := conf.cacheableRequest(tt.args.kong)
			assert.Equalf(t, tt.want, got, "cacheableRequest(%v)", tt.args.kong)
			assert.Equalf(t, tt.want1, got1, "cacheableRequest(%v)", tt.args.kong)
		})
	}
}
