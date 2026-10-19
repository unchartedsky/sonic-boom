package internal

// CacheSignal represents a signal for cache operations
type CacheSignal struct {
	CacheKeyID string `json:"cache_key_id" validate:"required"`
	CacheTTL   int    `json:"cache_ttl" validate:"gte=0" default:"0"`
	// Winner 는 이 요청이 fill-lock의 승자(fill 주도권 보유)임을 Access→Response로
	// 전달한다. Response는 승자만 락을 해제한다 — 패자가 남의 락을 지우지 못하게.
	Winner bool `json:"winner" validate:"" default:"false"`
}

// NewCacheSignal creates a new CacheSignal instance
func NewCacheSignal(cacheKeyID string, cacheTTL int) *CacheSignal {
	return &CacheSignal{
		CacheKeyID: cacheKeyID,
		CacheTTL:   cacheTTL,
	}
}
