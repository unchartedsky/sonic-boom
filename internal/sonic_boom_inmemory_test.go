package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newInMemoryConfigForTest() *Config {
	cfg := configDefault()
	cfg.Strategy = "in-memory"
	cfg.InMemory.MaxCost = 1024
	cfg.InMemory.NumCounters = 1024
	cfg.InMemory.BufferItems = 64
	return cfg
}

func cloneInMemoryConfig(c *Config) *Config {
	n := *c
	n.InMemory = c.InMemory
	return &n
}

func Test_newCacheManager_InMemory_Success(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	cm, m, err := cfg.newCacheManager(1)
	require.NoError(t, err)
	require.NotNil(t, cm)
	require.NotNil(t, m)
}

func Test_InMemory_PutGet_RoundTrip(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

    key := "roundtrip-key"
    val := []byte("hello")
    require.NoError(t, cm.Set(context.Background(), key, val))
    // ristretto는 비동기 쓰기이므로 짧은 리트라이 루프를 둔다
    var got []byte
    var err error
    for i := 0; i < 10; i++ {
        got, err = cm.Get(context.Background(), key)
        if err == nil {
            break
        }
        time.Sleep(20 * time.Millisecond)
    }
    require.NoError(t, err)
    require.Equal(t, val, got)
}

func Test_InMemory_TTL_Expires(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(1) // 1s
	require.NoError(t, err)

	key := "ttl-key"
	val := []byte("x")
    require.NoError(t, cm.Set(context.Background(), key, val))
    // 쓰기 확정 대기 후 TTL 만료까지 대기
    time.Sleep(200 * time.Millisecond)
    time.Sleep(1500 * time.Millisecond)

    // 만료 확인은 리트라이 불필요(이미 TTL 경과)
    _, err = cm.Get(context.Background(), key)
    require.Error(t, err)
}

func Test_InMemory_Concurrency_NoRace(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	cm, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	keys := []string{"k1", "k2", "k3", "k4", "k5"}

	t.Run("writers", func(t *testing.T) {
		t.Parallel()
		for _, k := range keys {
			require.NoError(t, cm.Set(context.Background(), k, []byte(k)))
		}
	})

	t.Run("readers", func(t *testing.T) {
		t.Parallel()
		for _, k := range keys {
			_, _ = cm.Get(context.Background(), k)
		}
	})
}

func Test_InMemory_Eviction_MinimalGuarantee(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	cfg.InMemory.MaxCost = 256
	cm, _, err := cfg.newCacheManager(60)
	require.NoError(t, err)

	n := 200
	for i := 0; i < n; i++ {
		require.NoError(t, cm.Set(context.Background(), fmt.Sprintf("k-%d", i), []byte("0123456789")))
	}
	misses := 0
	for i := 0; i < n; i++ {
		_, err := cm.Get(context.Background(), fmt.Sprintf("k-%d", i))
		if err != nil {
			misses++
		}
	}
	require.Greater(t, misses, 0)
}

func Test_InMemory_Marshal_Unmarshal_CacheValue(t *testing.T) {
	cfg := newInMemoryConfigForTest()
	_, m, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	cv := CacheValue{
		Status:    200,
		Headers:   map[string][]string{"Content-Type": {"text/plain"}},
		Body:      []byte("hello"),
		BodyLen:   5,
		Timestamp: time.Now().Unix(),
		TTL:       5,
		Version:   "1.0",
		ReqBody:   []byte{},
	}

	key := "marshal-key"
    require.NoError(t, m.Set(context.Background(), key, cv))
    var out CacheValue
    // marshaler도 내부적으로 store를 사용하므로 비동기 쓰기 보정
    for i := 0; i < 10; i++ {
        found, err := m.Get(context.Background(), key, &out)
        if err == nil && found {
            break
        }
        time.Sleep(20 * time.Millisecond)
        if i == 9 {
            require.NoError(t, err)
            require.True(t, found)
        }
    }
	require.Equal(t, cv.Status, out.Status)
	require.Equal(t, cv.BodyLen, out.BodyLen)
	require.Equal(t, cv.Version, out.Version)
}

func Test_InMemory_LoadOrStore_ReusesInstance(t *testing.T) {
	// 동일한 InMemoryConfig를 사용하면 같은 store/manager가 재사용되어야 한다
	cfg := newInMemoryConfigForTest()
	cm1, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)
	cm2, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	// 캐시 히트를 통해 간접 확인
	key := "reuse"
	require.NoError(t, cm1.Set(context.Background(), key, []byte("v")))
	got, err := cm2.Get(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, []byte("v"), got)
}

func Test_InMemory_DifferentConfig_IsolatedInstances(t *testing.T) {
	// 서로 다른 설정 키면 분리된 인스턴스이어야 한다
	cfg1 := newInMemoryConfigForTest()
	cfg2 := cloneInMemoryConfig(cfg1)
	cfg2.InMemory.MaxCost = cfg1.InMemory.MaxCost + 1 // 키가 달라지도록

	cm1, _, err := cfg1.newCacheManager(60)
	require.NoError(t, err)
	cm2, _, err := cfg2.newCacheManager(60)
	require.NoError(t, err)

	// 서로 간섭하지 않는지 확인(키 충돌 방지용 접두)
    require.NoError(t, cm1.Set(context.Background(), "isolate-1", []byte("a")))
    require.NoError(t, cm2.Set(context.Background(), "isolate-2", []byte("b")))

    // 비동기 쓰기 보정
    time.Sleep(100 * time.Millisecond)

    v1, err := cm1.Get(context.Background(), "isolate-1")
    require.NoError(t, err)
    require.Equal(t, []byte("a"), v1)

    v2, err := cm2.Get(context.Background(), "isolate-2")
    require.NoError(t, err)
    require.Equal(t, []byte("b"), v2)
}

func Test_Unknown_Strategy_Error(t *testing.T) {
	cfg := configDefault()
	cfg.Strategy = "unknown"
	cm, m, err := cfg.newCacheManager(5)
	require.Error(t, err)
	require.Nil(t, cm)
	require.Nil(t, m)
}
