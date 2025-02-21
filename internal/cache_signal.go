package internal

// CacheSignal represents a signal for cache operations
type CacheSignal struct {
	CacheKeyID string `json:"cache_key_id" validate:"required"`
	CacheTTL   int    `json:"cache_ttl" validate:"gte=0" default:"0"`
}

// NewCacheSignal creates a new CacheSignal instance
func NewCacheSignal(cacheKeyID string, cacheTTL int) *CacheSignal {
	return &CacheSignal{
		CacheKeyID: cacheKeyID,
		CacheTTL:   cacheTTL,
	}
}
