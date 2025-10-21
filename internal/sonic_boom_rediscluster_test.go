package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newRedisClusterConfigForTest() *Config {
	cfg := configDefault()
	cfg.Strategy = "redis-cluster"
	cfg.RedisCluster.Addrs = []string{"127.0.0.1:7000", "127.0.0.1:7001", "127.0.0.1:7002"}
	cfg.RedisCluster.Password = ""
	cfg.RedisCluster.MaxRetries = 3
	cfg.RedisCluster.DialTimeout = 5
	cfg.RedisCluster.ReadTimeout = 3
	cfg.RedisCluster.WriteTimeout = 3
	cfg.RedisCluster.PoolSize = 10
	cfg.RedisCluster.IdleTimeout = 1
	return cfg
}

func Test_newCacheManager_RedisCluster_Success(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cm, m, err := cfg.newCacheManager(5)
	require.NoError(t, err)
	require.NotNil(t, cm)
	require.NotNil(t, m)
}

func Test_RedisCluster_PutGet_RoundTrip(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cm, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	key := "cluster-roundtrip-key"
	val := []byte("hello cluster")
	require.NoError(t, cm.Set(context.Background(), key, val))

	// Redis Cluster는 네트워크 지연이 있을 수 있으므로 짧은 대기
	time.Sleep(100 * time.Millisecond)

	gotAny, err := cm.Get(context.Background(), key)
	require.NoError(t, err)
	got, ok := gotAny.([]byte)
	require.True(t, ok)
	require.Equal(t, val, got)
}

func Test_RedisCluster_TTL_Expires(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cm, _, err := cfg.newCacheManager(1) // 1s
	require.NoError(t, err)

	key := "cluster-ttl-key"
	val := []byte("x")
	require.NoError(t, cm.Set(context.Background(), key, val))

	// TTL 만료까지 대기
	time.Sleep(1500 * time.Millisecond)

	_, err = cm.Get(context.Background(), key)
	require.Error(t, err)
}

func Test_RedisCluster_Concurrency_NoRace(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cm, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	keys := []string{"cluster-k1", "cluster-k2", "cluster-k3", "cluster-k4", "cluster-k5"}

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

func Test_RedisCluster_Marshal_Unmarshal_CacheValue(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	_, m, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	cv := CacheValue{
		Status:    200,
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Body:      []byte("{\"cluster\":\"data\"}"),
		BodyLen:   18,
		Timestamp: time.Now().Unix(),
		TTL:       5,
		Version:   "1.0",
		ReqBody:   []byte{},
	}

	key := "cluster-marshal-key"
	require.NoError(t, m.Set(context.Background(), key, cv))

	// Redis Cluster 네트워크 지연 보정
	time.Sleep(100 * time.Millisecond)

	var out CacheValue
	var found any
	for i := 0; i < 10; i++ {
		found, err = m.Get(context.Background(), key, &out)
		if err == nil && found != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, cv.Status, out.Status)
	require.Equal(t, cv.BodyLen, out.BodyLen)
	require.Equal(t, cv.Version, out.Version)
}

func Test_RedisCluster_LoadOrStore_ReusesInstance(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cm1, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)
	cm2, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	// Redis Cluster는 외부 서비스이므로 실제로는 같은 연결을 재사용
	key := "cluster-reuse"
	require.NoError(t, cm1.Set(context.Background(), key, []byte("v")))
	time.Sleep(100 * time.Millisecond)

	gotAny, err := cm2.Get(context.Background(), key)
	require.NoError(t, err)
	got, ok := gotAny.([]byte)
	require.True(t, ok)
	require.Equal(t, []byte("v"), got)
}

func Test_RedisCluster_DifferentConfig_IsolatedInstances(t *testing.T) {
	cfg1 := newRedisClusterConfigForTest()
	cfg2 := newRedisClusterConfigForTest()
	cfg2.RedisCluster.Addrs = []string{"localhost:8000", "localhost:8001", "localhost:8002"} // 다른 포트

	cm1, _, err := cfg1.newCacheManager(60)
	require.NoError(t, err)
	cm2, _, err := cfg2.newCacheManager(60)
	require.NoError(t, err)

	// 서로 다른 클러스터이므로 간섭하지 않아야 함
	require.NoError(t, cm1.Set(context.Background(), "cluster-isolate-1", []byte("a")))
	require.NoError(t, cm2.Set(context.Background(), "cluster-isolate-2", []byte("b")))

	time.Sleep(100 * time.Millisecond)

	v1Any, err := cm1.Get(context.Background(), "cluster-isolate-1")
	require.NoError(t, err)
	v1, ok := v1Any.([]byte)
	require.True(t, ok)
	require.Equal(t, []byte("a"), v1)

	v2Any, err := cm2.Get(context.Background(), "cluster-isolate-2")
	require.NoError(t, err)
	v2, ok := v2Any.([]byte)
	require.True(t, ok)
	require.Equal(t, []byte("b"), v2)
}

func Test_RedisCluster_Connection_Error(t *testing.T) {
	cfg := newRedisClusterConfigForTest()
	cfg.RedisCluster.Addrs = []string{"nonexistent-host:9999"}
	cfg.RedisCluster.DialTimeout = 1 // 짧은 타임아웃으로 빠른 실패 유도

	cm, m, err := cfg.newCacheManager(5)
	// Redis Cluster 클라이언트는 지연 연결을 사용하므로 초기 생성 시에는 에러가 발생하지 않을 수 있음
	// 실제 연결 시도는 첫 번째 작업에서 발생
	if err != nil {
		require.Nil(t, cm)
		require.Nil(t, m)
		return
	}

	// 실제 작업을 시도하여 연결 에러 확인
	err = cm.Set(context.Background(), "test", []byte("value"))
	require.Error(t, err)
}
