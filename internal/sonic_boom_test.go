package internal

import (
	"reflect"
	"testing"
	"time"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/bridge/bridgetest"
	"github.com/Kong/go-pdk/log"
	"github.com/Kong/go-pdk/nginx"
	"github.com/Kong/go-pdk/request"
	"github.com/Kong/go-pdk/response"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
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

// Access/Response는 pdk.PDK 오케스트레이션이라 단위 테스트에서 다루지 않는다.
// 이 흐름은 e2e 스모크(make e2e, X-Cache-Status 단언)가 검증한다.

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
	tests := []struct {
		name          string
		responseCodes []int
		contentTypes  []string
		status        int
		contentType   string
		want          bool
	}{
		{
			name:          "status와 content-type 모두 허용",
			responseCodes: []int{200, 301, 404},
			contentTypes:  []string{"application/json", "text/plain"},
			status:        200,
			contentType:   "application/json",
			want:          true,
		},
		{
			name:          "status 미허용이면 false",
			responseCodes: []int{200},
			contentTypes:  []string{"application/json"},
			status:        500,
			contentType:   "application/json",
			want:          false,
		},
		{
			name:          "content-type이 비면 false",
			responseCodes: []int{200},
			contentTypes:  []string{"application/json"},
			status:        200,
			contentType:   "",
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ResponseCodes: tt.responseCodes,
				ContentTypes:  tt.contentTypes,
				logger:        defaultLogger(),
			}
			kong := &pdk.PDK{
				Response: response.Response{PdkBridge: bridge.New(bridgetest.Mock(t, []bridgetest.MockStep{
					{Method: "kong.response.get_status", Ret: &kong_plugin_protocol.Int{V: int32(tt.status)}},
				}))},
				Nginx: nginx.Nginx{PdkBridge: bridge.New(bridgetest.Mock(t, []bridgetest.MockStep{
					{Method: "kong.nginx.get_var", Args: bridge.WrapString("sent_http_content_type"), Ret: bridge.WrapString(tt.contentType)},
				}))},
			}
			if got := conf.cacheableResponse(kong); got != tt.want {
				t.Errorf("cacheableResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheableResponseContentType(t *testing.T) {
	tests := []struct {
		name         string
		contentTypes []string
		contentType  string
		want         bool
	}{
		{name: "설정된 content-type과 일치", contentTypes: []string{"application/json", "text/plain"}, contentType: "application/json", want: true},
		{name: "대소문자 무관 일치", contentTypes: []string{"application/json"}, contentType: "Application/JSON", want: true},
		{name: "content-type 없으면 false", contentTypes: []string{"application/json"}, contentType: "", want: false},
		{name: "미일치 content-type은 캐시하지 않는다", contentTypes: []string{"application/json"}, contentType: "text/xml", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := Config{
				ContentTypes: tt.contentTypes,
				logger:       defaultLogger(),
			}
			kong := &pdk.PDK{
				Nginx: nginx.Nginx{PdkBridge: bridge.New(bridgetest.Mock(t, []bridgetest.MockStep{
					{Method: "kong.nginx.get_var", Args: bridge.WrapString("sent_http_content_type"), Ret: bridge.WrapString(tt.contentType)},
				}))},
			}
			if got := conf.cacheableResponseContentType(kong); got != tt.want {
				t.Errorf("cacheableResponseContentType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_cacheableResponseStatus(t *testing.T) {
	tests := []struct {
		name          string
		responseCodes []int
		status        int
		askFails      bool
		want          bool
	}{
		{name: "설정된 status", responseCodes: []int{200, 301, 404}, status: 200, want: true},
		{name: "설정된 status 두 번째", responseCodes: []int{200, 301, 404}, status: 404, want: true},
		{name: "미설정 status", responseCodes: []int{200, 301, 404}, status: 500, want: false},
		// 빈 mock 목록 → Ask가 EOF로 실패 → 오류 경로에서 false
		{name: "GetStatus 실패", responseCodes: []int{200}, askFails: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conf := Config{
				ResponseCodes: tt.responseCodes,
				logger:        defaultLogger(),
			}
			steps := []bridgetest.MockStep{}
			if !tt.askFails {
				steps = append(steps, bridgetest.MockStep{
					Method: "kong.response.get_status", Ret: &kong_plugin_protocol.Int{V: int32(tt.status)},
				})
			}
			kong := &pdk.PDK{
				Response: response.Response{PdkBridge: bridge.New(bridgetest.Mock(t, steps))},
			}
			if got := conf.cacheableResponseStatus(kong); got != tt.want {
				t.Errorf("cacheableResponseStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_checkConfig(t *testing.T) {
	baseConfig := func() *Config {
		conf := configDefault()
		conf.Redis.Host = "localhost"
		conf.RedisCluster.Addrs = []string{"localhost:7000"}
		return conf
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "redis 전략 유효", mutate: func(c *Config) {}, wantErr: false},
		{
			name: "redis-cluster 전략 유효",
			mutate: func(c *Config) {
				c.Strategy = "redis-cluster"
			},
			wantErr: false,
		},
		{
			name: "in-memory 전략은 redis 설정 불필요",
			mutate: func(c *Config) {
				c.Strategy = "in-memory"
			},
			wantErr: false,
		},
		{
			name: "CacheTTL 음수",
			mutate: func(c *Config) {
				c.CacheTTL = -1
			},
			wantErr: true,
		},
		{
			name: "CacheTTL 0 거절(영구 키 방지)",
			mutate: func(c *Config) {
				c.CacheTTL = 0
			},
			wantErr: true,
		},
		{
			name: "알 수 없는 전략",
			mutate: func(c *Config) {
				c.Strategy = "bogus"
			},
			wantErr: true,
		},
		{
			name: "redis 전략인데 host 없음",
			mutate: func(c *Config) {
				c.Redis.Host = ""
			},
			wantErr: true,
		},
		{
			name: "redis port 0",
			mutate: func(c *Config) {
				c.Redis.Port = 0
			},
			wantErr: true,
		},
		{
			name: "redis-cluster 전략인데 addrs 없음",
			mutate: func(c *Config) {
				c.Strategy = "redis-cluster"
				c.RedisCluster.Addrs = nil
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conf := baseConfig()
			tt.mutate(conf)
			err := conf.checkConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("checkConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// panicGuard가 패닉을 실제로 회복하는지 검증 — 클로저 반환형이므로
// defer panicGuard(kong)() 형태(반환된 클로저 호출)여야 recover가 실행된다.
// 괄호 없는 defer panicGuard(kong) 형태로 퇴행하면 패닉이 전파돼 이 테스트가 죽는다.
func Test_panicGuard_Recovers(t *testing.T) {
	func() {
		defer panicGuard(nil)()
		panic("boom")
	}()
}

func configDefault() *Config {
	return &Config{
		ResponseCodes:        []int{200, 301, 404},
		RequestMethods:       []string{"GET", "HEAD"},
		ContentTypes:         []string{"text/plain", "application/json", "application/json; charset=utf-8"},
		VaryHeaders:          []string{},
		Filters:              []Filter{},
		CacheTTL:             300,
		CacheControl:         false,
		CacheableBodyMaxSize: 0,
		CacheVersion:         "",
		Strategy:             "redis",

		FillLock: FillLockConfig{
			Enabled: true,
			TTLMs:   defaultFillLockTTLMs,
			WaitMs:  200,
		},

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
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "일반 헤더는 덮어쓸 수 있다", header: "Content-Type", want: true},
		{name: "content-encoding은 재생에서 보존된다(gzip 캐시 정합성)", header: "Content-Encoding", want: true},
		{name: "hop-by-hop connection", header: "Connection", want: false},
		{name: "hop-by-hop transfer-encoding", header: "Transfer-Encoding", want: false},
		{name: "hop-by-hop keep-alive", header: "Keep-Alive", want: false},
		{name: "content-length", header: "Content-Length", want: false},
		{name: "ratelimit-remaining 포함 헤더", header: "X-RateLimit-Remaining", want: false},
		{name: "대소문자 무관 ratelimit", header: "ratelimit-remaining", want: false},
		{name: "대소문자 무관 hop-by-hop", header: "CONTENT-LENGTH", want: false},
		{name: "대소문자 무관 content-encoding", header: "CONTENT-ENCODING", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := overwritableHeader(tt.header); got != tt.want {
				t.Errorf("overwritableHeader(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func Test_headerToDelete(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{name: "x-cache-status", header: "X-Cache-Status", want: true},
		{name: "age", header: "Age", want: true},
		{name: "대소문자 무관", header: "x-cache-status", want: true},
		{name: "삭제 대상 아님", header: "X-Custom-Header", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := headerToDelete(tt.header); got != tt.want {
				t.Errorf("headerToDelete(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func Test_convertRedisTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  int
		timeUnit time.Duration
		want     time.Duration
	}{
		{name: "양수는 단위 곱해 초로 변환", timeout: 5, timeUnit: time.Second, want: 5 * time.Second},
		{name: "0은 무동작(기본값 위임)", timeout: 0, timeUnit: time.Second, want: 0},
		{name: "-1은 무한 대기", timeout: -1, timeUnit: time.Second, want: -1},
		{name: "-2 이하는 -1로 클램프", timeout: -2, timeUnit: time.Second, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := convertRedisTimeout(tt.timeout, tt.timeUnit); got != tt.want {
				t.Errorf("convertRedisTimeout(%d, %v) = %v, want %v", tt.timeout, tt.timeUnit, got, tt.want)
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
		{
			name: "unset filter TTL inherits global cache_ttl",
			fields: fields{
				logger: defaultLogger(),
				Filters: []Filter{
					{
						Name: "Test",
						Rules: []Rule{
							{
								Regexp: ".*ref.*",
							},
						},
					},
				},
				CacheTTL: 15,
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
			want1: 15,
		},
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
