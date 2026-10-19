package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 캐시 키 결정론성 단언(계획 U1): 같은 CacheKey는 항상 같은 키, 맵 삽입
// 순서와 무관, 필드 하나만 달라도 다른 키. hashstructure 제거 후
// 직렬화+xxhash 구현이 이 계약을 지키는지 검증한다.

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
