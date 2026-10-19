package internal

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/marshaler"
	lib_store "github.com/eko/gocache/lib/v4/store"
)

// 스탬피드 완화용 분산 fill-lock(로드맵 U3). 캐시 만료 순간 동시 미스가
// 몰리면 첫 요청만 업스트림에 가고 나머지는 짧게 기다렸다가 갓 채워진
// 값을 받는다. 락 키는 sb-lock: 접두사의 별도 임시 키 — 기존 캐시 키
// 공간과 직렬화는 건드리지 않는다.
const fillLockKeyPrefix = "sb-lock:"

// acquireFillLock 는 SET NX로 fill 주도권을 1회만 배정한다.
// store가 SetIfNotExists를 지원하지 않으면(in-memory ristretto store,
// gocache v4.4 선택 인터페이스) 항상 true — 오늘의 동작(전원 통과)을 유지한다.
func acquireFillLock(cm *cache.Cache[any], cacheKeyID string, ttl time.Duration) bool {
	set, err := cm.SetIfNotExists(context.Background(), fillLockKeyPrefix+cacheKeyID, "1",
		lib_store.WithExpiration(ttl))
	if err != nil {
		return false // 일시 오류면 통과가 안전하다(락 없이 진행)
	}
	return set
}

// releaseFillLock 은 fill 완료(Response 저장 직후) 최선 해제다.
// 실패해도 락 TTL이 상한을 못 박는다.
func releaseFillLock(cm *cache.Cache[any], cacheKeyID string) {
	_ = cm.Delete(context.Background(), fillLockKeyPrefix+cacheKeyID)
}

// waitForFill 은 패자가 기한 안에 값이 채워지기를 기다린다. 대기자 폭풍을
// 막으려 재폴링 간격에 지터를 섞는다. 못 기다리면 nil(통과).
func waitForFill(m *marshaler.Marshaler, cacheKeyID string, wait time.Duration) *CacheValue {
	deadline := time.Now().Add(wait)
	for {
		if v, err := m.Get(context.Background(), cacheKeyID, new(CacheValue)); err == nil && v != nil {
			if cv, ok := v.(*CacheValue); ok {
				return cv
			}
		}
		if !time.Now().Before(deadline) {
			return nil
		}
		time.Sleep(fillLockPollInterval())
	}
}

// 20ms 기본 + [0, 20ms) 지터 — 대기자들이 같은 박자로 redis를 두드리지 않게.
func fillLockPollInterval() time.Duration {
	return 20*time.Millisecond + time.Duration(rand.Int64N(int64(20*time.Millisecond)))
}

// fillLockOrWait 는 Miss 판정 지점의 진입부다. 승자는 nil을 받아 프록시로
// fill하고, 패자는 기한 안에 채워진 값 또는 nil(통과)을 받는다.
func (conf *Config) fillLockOrWait(cm *cache.Cache[any], m *marshaler.Marshaler, cacheKeyID string) *CacheValue {
	if !conf.FillLock.Enabled {
		return nil
	}
	lockTTL := time.Duration(conf.FillLock.TTLMs) * time.Millisecond
	if lockTTL <= 0 {
		lockTTL = 3 * time.Second
	}
	if acquireFillLock(cm, cacheKeyID, lockTTL) {
		return nil // 승자: 업스트림 fill 진행
	}
	wait := time.Duration(conf.FillLock.WaitMs) * time.Millisecond
	return waitForFill(m, cacheKeyID, wait)
}
