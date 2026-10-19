package internal

import ()

// InMemoryConfig represents the configuration for in-memory cache using Ristretto
type InMemoryConfig struct {
	MaxCost     int `json:"max_cost" validate:"gte=0" default:"1000000"`
	NumCounters int `json:"num_counters" validate:"gte=0" default:"1000000"`
	BufferItems int `json:"buffer_items" validate:"gte=0" default:"64"`
}

// Validate 는 InMemoryConfig 검증이다 — 프로세스 공유 검증기(configValidator,
// sonic_boom.go)를 쓴다. validator.New()는 등록 비용이 커서 인스턴스마다 만들지
// 않는다(테스트 호출자 1건이 유일한 소비자다).
func Validate(config *InMemoryConfig) error {
	return configValidator.Struct(config)
}
