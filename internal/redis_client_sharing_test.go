package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// redis 클라이언트 공유 단언: 같은 설정이면 프로세스에 클라이언트가 하나다.
// go-redis 문서("Client is meant to be long-lived and shared between many
// goroutines")에 맞춰, 매 요청 newCacheManager가 불려도 다이얼·핸드셰이크가
// 요청마다 반복되지 않는다(요청당 생성 방식 실측 9.3배 지연, 2026-10-19).
// in-memory 전략의 ristrettoClients 공유와 같은 규율이다.

func Test_Redis_ClientSharedPerConfig(t *testing.T) {
	cfg := newRedisConfigForTest()

	_, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	v1, ok1 := redisClients.Load(cfg.Redis)
	require.True(t, ok1, "첫 호출 뒤 공유 맵에 있어야 한다")

	_, _, err = cfg.newCacheManager(5)
	require.NoError(t, err)

	v2, ok2 := redisClients.Load(cfg.Redis)
	require.True(t, ok2)
	require.Same(t, v1, v2, "같은 RedisConfig는 같은 클라이언트 인스턴스를 써야 한다")

	// RedisConfig는 클라이언트 공유 맵의 키다 — 비교 불가능 필드가 추가되면
	// 이 비교가 컴파일을 실패시킨다.
	require.True(t, cfg.Redis == cfg.Redis) //nolint:staticcheck //SA4000 // 비교 가능성 컴파일 가드

	// 다른 설정은 다른 클라이언트(격리)
	other := newRedisConfigForTest()
	other.Redis.DBNumber = 9
	_, _, err = other.newCacheManager(5)
	require.NoError(t, err)
	v3, ok3 := redisClients.Load(other.Redis)
	require.True(t, ok3)
	require.NotSame(t, v1, v3, "다른 RedisConfig는 다른 클라이언트여야 한다")
}

func Test_RedisCluster_ClientSharedPerConfig(t *testing.T) {
	cfg := newRedisClusterConfigForTest()

	_, _, err := cfg.newCacheManager(5)
	require.NoError(t, err)

	// 호출 사이에 로드를 끼워 '매 호출 Store 덮어쓰기' 회귀를 잡는다.
	v1, ok1 := redisClusterClients.Load(redisClusterKeyOf(cfg.RedisCluster))
	require.True(t, ok1, "첫 호출 뒤 공유 맵에 있어야 한다")

	_, _, err = cfg.newCacheManager(5)
	require.NoError(t, err)

	v2, ok2 := redisClusterClients.Load(redisClusterKeyOf(cfg.RedisCluster))
	require.True(t, ok2)
	require.Same(t, v1, v2, "같은 RedisClusterConfig는 같은 클러스터 클라이언트여야 한다")

	// 다른 addr 세트는 다른 클라이언트
	other := newRedisClusterConfigForTest()
	other.RedisCluster.Addrs = []string{"redis-cluster:7000", "172.30.0.12:7000"}
	_, _, err = other.newCacheManager(5)
	require.NoError(t, err)
	v3, ok3 := redisClusterClients.Load(redisClusterKeyOf(other.RedisCluster))
	require.True(t, ok3)
	require.NotSame(t, v1, v3)
}
