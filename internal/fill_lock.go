package internal

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	lib_store "github.com/eko/gocache/lib/v4/store"
)

const fillLockKeyPrefix = "sb-lock:"

// store가 SetIfNotExists를 지원하지 않으면(gocache v4.4 선택 인터페이스 —
// in-memory ristretto store가 이에 해당) 오류로 돌아온다. 이때와 일시
// 오류 때는 잠금 없이 통과시킨다 — 대기자로 만들면 지원하지 않는 전략과
// redis 장애 시 모든 요청이 WaitMs만큼 늘어난다.
func acquireFillLock(cm *cache.Cache[any], cacheKeyID string, ttl time.Duration) bool {
	set, err := cm.SetIfNotExists(context.Background(), fillLockKeyPrefix+cacheKeyID, "1",
		lib_store.WithExpiration(ttl))
	if err != nil {
		return true
	}
	return set
}

func releaseFillLock(cm *cache.Cache[any], cacheKeyID string) {
	_ = cm.Delete(context.Background(), fillLockKeyPrefix+cacheKeyID)
}

// waitForFill 은 패자가 기한 안에 값이 채워지기를 기다린다. 재폴링 간격에
// 지터를 섞어 대기자들이 같은 박자로 redis를 두드리지 않게 한다.
func waitForFill(m *marshaler.Marshaler, cacheKeyID string, wait time.Duration) *CacheValue {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		time.Sleep(fillLockPollInterval())
		if v, err := m.Get(context.Background(), cacheKeyID, new(CacheValue)); err == nil && v != nil {
			if cv, ok := v.(*CacheValue); ok {
				return cv
			}
		}
	}
	return nil
}

func fillLockPollInterval() time.Duration {
	return 20*time.Millisecond + time.Duration(rand.Int64N(int64(20*time.Millisecond)))
}

// fillLockOrWait 는 Miss 판정 지점의 진입부다. winner가 true면 이 요청이
// 업스트림 fill을 주도한다. 패자는 기한 안에 채워진 값 또는 nil(통과)을
// 돌려받는다.
func (conf *Config) fillLockOrWait(cm *cache.Cache[any], m *marshaler.Marshaler, cacheKeyID string) (winner bool, filled *CacheValue) {
	if !conf.FillLock.Enabled {
		return true, nil
	}
	lockTTL := convertRedisTimeout(conf.FillLock.TTLMs, time.Millisecond)
	if lockTTL <= 0 {
		lockTTL = defaultFillLockTTLMs * time.Millisecond
	}
	if acquireFillLock(cm, cacheKeyID, lockTTL) {
		return true, nil
	}
	return false, waitForFill(m, cacheKeyID, convertRedisTimeout(conf.FillLock.WaitMs, time.Millisecond))
}
