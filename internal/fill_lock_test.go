package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 스탬피드 fill-lock 단언(통합 — compose redis 필요).
// 설계: 승자(락 획득)만 업스트림 fill, 패자는 지터 재폴링으로 기다렸다가
// 값이 채워지면 그 값을 돌려받고, 기한 내 안 채워지면 통과(오늘의 동작).

func Test_FillLock_SingleWinnerAmongConcurrent(t *testing.T) {
	cfg := newRedisConfigForTest()
	cm, _, err := cfg.newCacheManager(10)
	require.NoError(t, err)

	const n = 32
	key := "fill-lock-winner-key"
	wins := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if acquireFillLock(cm, key, 3*time.Second) {
				wins <- i
			}
		}(i)
	}
	wg.Wait()
	close(wins)
	require.Len(t, wins, 1, "동시 획득 시도 %d회 중 정확히 1승자", n)
}

func Test_FillLock_LoserWaitsAndServesFilledValue(t *testing.T) {
	cfg := newRedisConfigForTest()
	cm, marshal, err := cfg.newCacheManager(10)
	require.NoError(t, err)

	key := "fill-lock-wait-key"
	require.True(t, acquireFillLock(cm, key, 3*time.Second), "첫 획득자는 승자")

	go func() {
		time.Sleep(60 * time.Millisecond)
		cv := &CacheValue{Status: 200, Headers: map[string][]string{}, Body: []byte("filled"), TTL: 10, Version: "1.0"}
		_ = marshal.Set(context.Background(), key, cv)
	}()

	got := waitForFill(marshal, key, 2*time.Second)
	require.NotNil(t, got, "기한 안에 채워진 값을 돌려받아야 한다")
	require.Equal(t, 200, got.Status)
	require.Equal(t, []byte("filled"), got.Body)
}

func Test_FillLock_WaitCapDegradesToPassThrough(t *testing.T) {
	cfg := newRedisConfigForTest()
	cm, marshal, err := cfg.newCacheManager(10)
	require.NoError(t, err)

	key := "fill-lock-cap-key"
	require.True(t, acquireFillLock(cm, key, 3*time.Second))

	start := time.Now()
	got := waitForFill(marshal, key, 150*time.Millisecond)
	elapsed := time.Since(start)

	require.Nil(t, got, "아무도 채우지 않으면 nil(통과)을 돌려준다")
	require.GreaterOrEqual(t, elapsed, 140*time.Millisecond, "기한까지 기다린다")
	require.Less(t, elapsed, 2*time.Second, "기한을 크게 넘기지 않는다")

	// WaitMs=0은 기다림 없이 즉시 통과한다.
	t0 := time.Now()
	require.Nil(t, waitForFill(marshal, "fill-lock-zero-wait", 0))
	require.Less(t, time.Since(t0), 20*time.Millisecond)
}

func Test_FillLock_ReleasedAfterDelete(t *testing.T) {
	cfg := newRedisConfigForTest()
	cm, _, err := cfg.newCacheManager(10)
	require.NoError(t, err)

	key := "fill-lock-release-key"
	require.True(t, acquireFillLock(cm, key, 3*time.Second))
	releaseFillLock(cm, key) // Response에서 fill 완료 후 해제
	require.True(t, acquireFillLock(cm, key, 3*time.Second), "해제 뒤 재획득 가능")
}

func Test_FillLock_Defaults(t *testing.T) {
	cfg := configDefault()
	require.True(t, cfg.FillLock.Enabled, "기본 켜짐")
	require.Equal(t, 3000, cfg.FillLock.TTLMs)
	require.Equal(t, 200, cfg.FillLock.WaitMs)
}
