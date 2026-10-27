package internal

import (
	"testing"

	"github.com/Kong/go-pdk"
	"github.com/Kong/go-pdk/bridge"
	"github.com/Kong/go-pdk/bridge/bridgetest"
	"github.com/Kong/go-pdk/client"
	"github.com/Kong/go-pdk/request"
	"github.com/Kong/go-pdk/router"
	"github.com/Kong/go-pdk/server/kong_plugin_protocol"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// 캐시 키 결정론성 단언(계획 U1): 같은 CacheKey는 항상 같은 키, 맵 삽입
// 순서와 무관, 필드 하나만 달라도 다른 키. hashstructure 제거 후
// 직렬화+sha256 구현이 이 계약을 지키는지 검증한다.

func Test_GenerateCacheKeyID_Deterministic(t *testing.T) {
	logger := defaultLogger()

	key := func() *CacheKey {
		return &CacheKey{
			Consumer:  "consumer-1",
			Service:   "svc-1",
			Route:     "route-1",
			Method:    "GET",
			URL:       "/api/users",
			QueryArgs: map[string][]string{"page": {"2"}, "size": {"10"}},
			Headers:   map[string]string{"Host": "example.com"},
			Body:      []byte(`{"a":1}`),
			CacheTTL:  15,
		}
	}

	k1, err := generateCacheKeyID(logger, key())
	require.NoError(t, err)
	require.NotEmpty(t, k1)

	k2, err := generateCacheKeyID(logger, key())
	require.NoError(t, err)
	require.Equal(t, k1, k2, "같은 CacheKey는 같은 키")
}

func Test_GenerateCacheKeyID_MapOrderInsensitive(t *testing.T) {
	logger := defaultLogger()

	a, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15,
		QueryArgs: map[string][]string{"a": {"1"}, "b": {"2"}, "c": {"3"}},
		Headers:   map[string]string{"H1": "v1", "H2": "v2"},
	})
	require.NoError(t, err)

	b, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15,
		QueryArgs: map[string][]string{"c": {"3"}, "a": {"1"}, "b": {"2"}},
		Headers:   map[string]string{"H2": "v2", "H1": "v1"},
	})
	require.NoError(t, err)

	require.Equal(t, a, b, "맵 삽입 순서가 달라도 같은 키")
}

func Test_GenerateCacheKeyID_NilMapEqualsEmpty(t *testing.T) {
	logger := defaultLogger()

	a, err := generateCacheKeyID(logger, &CacheKey{Method: "GET", URL: "/x", CacheTTL: 15})
	require.NoError(t, err)

	b, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15,
		QueryArgs: map[string][]string{}, Headers: map[string]string{},
	})
	require.NoError(t, err)

	require.Equal(t, a, b, "nil 맵과 빈 맵은 같은 키")
}

func Test_GenerateCacheKeyID_NulSingleVsMultiValue(t *testing.T) {
	logger := defaultLogger()

	// PE3: 단일 값 "a\x00b"와 다중 값 ["a","b"]는 이전 NUL join 직렬화에서 같은
	// 키로 충돌했다. 값 단위 길이 접두로 구분돼야 한다.
	single, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15,
		QueryArgs: map[string][]string{"x": {"a\x00b"}},
	})
	require.NoError(t, err)

	multi, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15,
		QueryArgs: map[string][]string{"x": {"a", "b"}},
	})
	require.NoError(t, err)

	require.NotEqual(t, single, multi, "NUL 포함 단일 값과 다중 값은 다른 키여야 한다")
}

func Test_GenerateCacheKeyID_FieldChangeChangesKey(t *testing.T) {
	logger := defaultLogger()
	base := &CacheKey{
		Consumer: "c", Service: "s", Route: "r", Method: "GET", URL: "/x",
		QueryArgs: map[string][]string{"p": {"1"}}, Headers: map[string]string{"H": "v"},
		Body: []byte("body"), CacheTTL: 15,
	}
	baseKey, err := generateCacheKeyID(logger, base)
	require.NoError(t, err)

	mutations := map[string]func(k *CacheKey){
		"consumer": func(k *CacheKey) { k.Consumer = "c2" },
		"service":  func(k *CacheKey) { k.Service = "s2" },
		"route":    func(k *CacheKey) { k.Route = "r2" },
		"method":   func(k *CacheKey) { k.Method = "POST" },
		"url":      func(k *CacheKey) { k.URL = "/y" },
		"queryarg": func(k *CacheKey) { k.QueryArgs["p"] = []string{"2"} },
		"header":   func(k *CacheKey) { k.Headers["H"] = "v2" },
		"body":     func(k *CacheKey) { k.Body = []byte("body2") },
		"ttl":      func(k *CacheKey) { k.CacheTTL = 30 },
	}
	for name, mutate := range mutations {
		k := &CacheKey{
			Consumer: base.Consumer, Service: base.Service, Route: base.Route,
			Method: base.Method, URL: base.URL, QueryArgs: map[string][]string{},
			Headers: map[string]string{}, Body: base.Body, CacheTTL: base.CacheTTL,
		}
		for kk, vv := range base.QueryArgs {
			k.QueryArgs[kk] = vv
		}
		for kk, vv := range base.Headers {
			k.Headers[kk] = vv
		}
		mutate(k)
		got, err := generateCacheKeyID(logger, k)
		require.NoError(t, err, name)
		require.NotEqual(t, baseKey, got, "필드 %s 변경은 키를 바꿔야 한다", name)
	}
}

// principal 성분(⑥ 요청층 자격증명 vary): consumer 미확정 요청이 Authorization을
// 갖고 있으면 익명 공용 슬롯 대신 자격증명별 슬롯으로 분리된다. Lua sbk3와 쌍대.

func Test_GenerateCacheKeyID_PrincipalComponent(t *testing.T) {
	logger := defaultLogger()

	without, err := generateCacheKeyID(logger, &CacheKey{Method: "GET", URL: "/x", CacheTTL: 15})
	require.NoError(t, err)

	withPrincipal, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/x", CacheTTL: 15, Principal: "Bearer cred-A",
	})
	require.NoError(t, err)

	require.NotEqual(t, withPrincipal, without, "principal 성분은 키에 반영된다")
}

// keyEnv는 NewCacheKey 단위 테스트용 PDK 브리지 목 — 지정 식별자·헤더로 kong.*에 답한다.
type keyEnv struct {
	t        *testing.T
	consumer string
	headers  map[string]string
}

func (e *keyEnv) Handle(method string, args_d []byte) []byte {
	// go-pdk v0.11.2의 AskString 와이어 타입은 kong_plugin_protocol.String이다
	// (structpb.Value 아님 — bridge.AskString 참조).
	answer := func(v string) []byte {
		d, err := proto.Marshal(&kong_plugin_protocol.String{V: v})
		if err != nil {
			e.t.Fatal(err)
		}
		return d
	}
	switch method {
	case "kong.client.get_consumer":
		d, err := proto.Marshal(&kong_plugin_protocol.Consumer{Id: e.consumer})
		if err != nil {
			e.t.Fatal(err)
		}
		return d
	case "kong.router.get_service", "kong.router.get_route":
		return answer("")
	case "kong.request.get_method":
		return answer("GET")
	case "kong.request.get_path":
		return answer("/p")
	case "kong.request.get_header":
		var arg kong_plugin_protocol.String
		if err := proto.Unmarshal(args_d, &arg); err != nil {
			e.t.Fatal(err)
		}
		return answer(e.headers[arg.GetV()])
	case "kong.request.get_query":
		d, err := proto.Marshal(&structpb.Struct{})
		if err != nil {
			e.t.Fatal(err)
		}
		return d
	}
	return nil
}

func (e *keyEnv) Errorf(format string, args ...interface{}) { e.t.Errorf(format, args...) }
func (e *keyEnv) IsRunning() bool                           { return true }
func (e *keyEnv) SubscribeStatusChange(ch chan<- string)    {}

func newKeyPDK(t *testing.T, consumer string, headers map[string]string) *pdk.PDK {
	env := &keyEnv{t: t, consumer: consumer, headers: headers}
	mk := func() bridge.PdkBridge { return bridge.New(bridgetest.MockFunc(env)) }
	return &pdk.PDK{
		Client:  client.Client{PdkBridge: mk()},
		Router:  router.Router{PdkBridge: mk()},
		Request: request.Request{PdkBridge: mk()},
	}
}

func TestNewCacheKey_Principal(t *testing.T) {
	logger := defaultLogger()
	conf := &Config{logger: logger, CacheTTL: 15}
	authA := map[string]string{"Authorization": "Bearer cred-A"}
	authB := map[string]string{"Authorization": "Bearer cred-B"}

	// 1) consumer 없+Authorization → principal 성분 반영(정확한 성분 검침)
	kA, err := NewCacheKey(newKeyPDK(t, "", authA), conf, []byte(""), 15)
	require.NoError(t, err)
	expectedA, err := generateCacheKeyID(logger, &CacheKey{
		Principal: "Bearer cred-A", Method: "GET", URL: "/p",
		QueryArgs: map[string][]string{}, Headers: map[string]string{},
		Body: []byte(""), CacheTTL: 15,
	})
	require.NoError(t, err)
	require.Equal(t, expectedA, kA, "consumer 없+Authorization 키는 principal 성분으로만 달라진다")

	// 같은 자격증명 재요청 → 같은 키
	kA2, err := NewCacheKey(newKeyPDK(t, "", authA), conf, []byte(""), 15)
	require.NoError(t, err)
	require.Equal(t, kA, kA2, "같은 자격증명은 같은 슬롯")

	// 2) 값이 다른 자격증명 → 다른 키(자격증명별 슬롯 — 버그 창 차단의 본체)
	kB, err := NewCacheKey(newKeyPDK(t, "", authB), conf, []byte(""), 15)
	require.NoError(t, err)
	require.NotEqual(t, kA, kB, "다른 자격증명은 다른 슬롯")

	// 3) Authorization 없음 → principal 빈 값(공개 슬롯 유지)
	kAnon, err := NewCacheKey(newKeyPDK(t, "", nil), conf, []byte(""), 15)
	require.NoError(t, err)
	expectedAnon, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/p",
		QueryArgs: map[string][]string{}, Headers: map[string]string{},
		Body: []byte(""), CacheTTL: 15,
	})
	require.NoError(t, err)
	require.Equal(t, expectedAnon, kAnon, "공개 트래픽 키는 principal 빈 값")
	require.NotEqual(t, kAnon, kA, "자격증명 요청은 공개 슬롯에 섞이지 않는다")

	// 4) consumer 확정 → principal 미사용(consumer만으로 슬롯 결정)
	kC1, err := NewCacheKey(newKeyPDK(t, "c-1", authA), conf, []byte(""), 15)
	require.NoError(t, err)
	kC2, err := NewCacheKey(newKeyPDK(t, "c-1", authB), conf, []byte(""), 15)
	require.NoError(t, err)
	expectedC, err := generateCacheKeyID(logger, &CacheKey{
		Consumer: "c-1", Method: "GET", URL: "/p",
		QueryArgs: map[string][]string{}, Headers: map[string]string{},
		Body: []byte(""), CacheTTL: 15,
	})
	require.NoError(t, err)
	require.Equal(t, expectedC, kC1, "consumer 확정 시 principal은 빈 값")
	require.Equal(t, kC1, kC2, "consumer 확정 시 자격증명 차이는 슬롯을 나누지 않는다")

	// 5) vary_headers에 Authorization 선언 → principal 건너뛰고 vary 성분만
	confVary := &Config{logger: logger, CacheTTL: 15, VaryHeaders: []string{"Authorization"}}
	kV, err := NewCacheKey(newKeyPDK(t, "", authA), confVary, []byte(""), 15)
	require.NoError(t, err)
	expectedV, err := generateCacheKeyID(logger, &CacheKey{
		Method: "GET", URL: "/p",
		QueryArgs: map[string][]string{},
		Headers:   map[string]string{"Authorization": "Bearer cred-A"},
		Body:      []byte(""), CacheTTL: 15,
	})
	require.NoError(t, err)
	require.Equal(t, expectedV, kV, "vary 선언 Authorization은 vary 성분만 쓰고 principal은 건너뛴다")
}
