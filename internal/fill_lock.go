package internal

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
func acquireFillLock(cm *cache.Cache[any], cacheKeyID string, ttl time.Duration) (bool, string) {
	token := newFillLockToken()
	set, err := cm.SetIfNotExists(context.Background(), fillLockKeyPrefix+cacheKeyID, token,
		lib_store.WithExpiration(ttl))
	if err != nil {
		// fail-open으로 보호가 풀린 사실을 메트릭에 남긴다 — 미지원 store는
		// 구조적인 것, 그 외 오류는 일시 장애로 라벨을 나눈다.
		reason := "error"
		if errors.Is(err, lib_store.ErrNotSupported) {
			reason = "unsupported"
		}
		recordFillLockDegraded(reason)
		return true, "" // 통과하지만 소유 락은 없다 — 해제도 하지 않는다
	}
	return set, token
}

// newFillLockToken 은 획득마다 유일한 소유 토큰을 만든다.
func newFillLockToken() string {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// releaseFillLockIfOwned 는 락 값이 자기 토큰과 일치할 때만 삭제한다 — TTL
// 만료로 후속 승자가 락을 이어받았다면 값이 달라 삭제되지 않는다. GET과 DEL이
// 원자적이지 않아 극단적 경쟁 창은 남지만, 무조건 DELETE가 'TTL 만료 이후
// 언제나' 남의 락을 지울 수 있던 것을 획득 직후로 좁힌다. 토큰이 빈 문자열
// (소유 없음)이면 아무것도 하지 않는다.
func releaseFillLockIfOwned(cm *cache.Cache[any], cacheKeyID, token string) {
	if token == "" {
		return
	}
	lockKey := fillLockKeyPrefix + cacheKeyID
	v, err := cm.Get(context.Background(), lockKey)
	if err != nil || v == nil {
		return
	}
	if cur, ok := v.(string); ok && cur == token {
		_ = cm.Delete(context.Background(), lockKey)
	}
}

// waitForFill 은 패자가 기한 안에 값이 채워지기를 기다린다. 재폴링 간격에
// 지터를 섞어 대기자들이 같은 박자로 redis를 두드리지 않게 한다.
// waitCtx 가 Get 자체에도 시한을 건다 — redis가 느리면 루프 조건과 무관하게
// 한 폴링이 Dial/Read/PoolTimeout 전부를 태워버리기 때문.
func waitForFill(m *marshaler.Marshaler, cacheKeyID string, wait time.Duration) *CacheValue {
	waitCtx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for {
		select {
		case <-waitCtx.Done():
			return nil
		case <-time.After(fillLockPollInterval()):
		}
		if v, err := m.Get(waitCtx, cacheKeyID, new(CacheValue)); err == nil && v != nil {
			if cv, ok := v.(*CacheValue); ok {
				return cv
			}
		}
	}
}

func fillLockPollInterval() time.Duration {
	return 20*time.Millisecond + time.Duration(rand.Int64N(int64(20*time.Millisecond)))
}

// fillLockOrWait 는 Miss 판정 지점의 진입부다. winner가 true면 이 요청이
// 업스트림 fill을 주도한다. 패자는 기한 안에 채워진 값 또는 nil(통과)을
// 돌려받는다.
func (conf *Config) fillLockOrWait(cm *cache.Cache[any], m *marshaler.Marshaler, cacheKeyID string) (winner bool, filled *CacheValue, lockToken string) {
	if !conf.FillLock.Enabled {
		return true, nil, ""
	}
	lockTTL := convertRedisTimeout(conf.FillLock.TTLMs, time.Millisecond)
	if lockTTL <= 0 {
		lockTTL = defaultFillLockTTLMs * time.Millisecond
	}
	if won, token := acquireFillLock(cm, cacheKeyID, lockTTL); won {
		// 더블체크: 이 요청의 미스 조회와 락 획득 사이에 다른 요청이 값을 채웠을
		// 수 있다. 그 값으로 서빙하고 락을 돌려준다(이중 fill 방지).
		if v, err := m.Get(context.Background(), cacheKeyID, new(CacheValue)); err == nil && v != nil {
			if cv, ok := v.(*CacheValue); ok {
				releaseFillLockIfOwned(cm, cacheKeyID, token)
				return false, cv, ""
			}
		}
		return true, nil, token // token==""면 fail-open 통과(소유 없음)
	}
	return false, waitForFill(m, cacheKeyID, convertRedisTimeout(conf.FillLock.WaitMs, time.Millisecond)), ""
}
