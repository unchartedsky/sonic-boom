package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// CSC(클라이언트 사이드 캐싱, 로드맵 U4) 설정 plumbing 단언.
// go-redis v9.22+ 실험 기능 — 기본 off, standalone DB 0 전용(라이브러리가
// 위반 시 경고와 함께 비활성화). 활성 시 Protocol 3 + ClientSideCacheConfig.

func Test_Redis_ClientSideCache_OptionsWiring(t *testing.T) {
	// 기본(off): 프로토콜 미지정, CSC 설정 nil
	c := newRedisConfigForTest().Redis
	opts := newRedisClientOptions(c)
	require.Zero(t, opts.Protocol)
	require.Nil(t, opts.ClientSideCacheConfig)

	// on: RESP3 + CSC 설정 전달
	c.ClientSideCache = ClientSideCacheConfig{
		Enabled:        true,
		MaxEntries:     128,
		MaxMemoryBytes: 1 << 20,
		MaxStalenessMs: 5000,
	}
	opts = newRedisClientOptions(c)
	require.Equal(t, 3, opts.Protocol, "CSC는 RESP3가 필요하다")
	require.NotNil(t, opts.ClientSideCacheConfig)
	require.Equal(t, 128, opts.ClientSideCacheConfig.MaxEntries)
	require.EqualValues(t, 1<<20, opts.ClientSideCacheConfig.MaxMemoryBytes)
	require.Equal(t, 5*time.Second, opts.ClientSideCacheConfig.MaxStaleness)
}

func Test_Redis_ClientSideCache_DefaultOff(t *testing.T) {
	cfg := configDefault()
	require.False(t, cfg.Redis.ClientSideCache.Enabled, "CSC는 기본 off")
}

// 실경로 투명성 게이트(베이크오프가 남긴 검증 게이트): CSC 켜진 클라이언트로
// gocache marshal 왕복이 동작하고, 다른 클라이언트의 갱신이 invalidation
// push로 반영돼 영구 stale이 되지 않는다. push는 비동기라 기한 있는 대기로
// 단언한다(라이브러리 문서: eventually consistent).
func Test_Redis_ClientSideCache_FreshAfterExternalWrite(t *testing.T) {
	ctx := context.Background()

	// CSC 클라이언트(MaxEntries 128 — 공유 맵에서 별도 클라이언트를 얻는 식별자)
	cscCfg := newRedisConfigForTest()
	cscCfg.Redis.DBNumber = 0
	cscCfg.Redis.ClientSideCache = ClientSideCacheConfig{
		Enabled:        true,
		MaxEntries:     128,
		MaxStalenessMs: 5000,
	}
	_, cscMarshal, err := cscCfg.newCacheManager(60)
	require.NoError(t, err)

	key := "csc-fresh-key"

	// 1) CSC 경로로 값 저장·조회(왕복 투명성)
	cv1 := &CacheValue{Status: 200, Headers: map[string][]string{}, Body: []byte("v1"), TTL: 60, Version: "1.0"}
	require.NoError(t, cscMarshal.Set(ctx, key, cv1))
	got := waitForFill(cscMarshal, key, 2*time.Second)
	require.NotNil(t, got, "CSC 클라이언트로 marshal 왕복이 동작한다")
	require.Equal(t, []byte("v1"), got.Body)

	// 2) 다른 클라이언트(CSC 없음, MaxEntries 64로 식별)가 갱신
	writerCfg := newRedisConfigForTest()
	writerCfg.Redis.DBNumber = 0
	writerCfg.Redis.ClientSideCache = ClientSideCacheConfig{Enabled: false, MaxEntries: 64}
	_, writerMarshal, err := writerCfg.newCacheManager(60)
	require.NoError(t, err)
	cv2 := &CacheValue{Status: 200, Headers: map[string][]string{}, Body: []byte("v2"), TTL: 60, Version: "1.0"}
	require.NoError(t, writerMarshal.Set(ctx, key, cv2))

	// 3) CSC 클라이언트의 재조회가 갱신을 반영한다(invalidation push, 기한 있음)
	deadline := time.Now().Add(5 * time.Second)
	for {
		v := waitForFill(cscMarshal, key, 200*time.Millisecond)
		if v != nil && string(v.Body) == "v2" {
			break // 갱신 반영 확인
		}
		if time.Now().After(deadline) {
			t.Fatal("invalidation이 5초 내 반영되지 않았다 — 영구 stale 가능성")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
