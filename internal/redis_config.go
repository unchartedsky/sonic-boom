package internal

// RedisConfig 는 [github.com/go-redis/redis] 의 Option을 Wrapping 하였습니다.
// RedisConfig 는 단일 Redis 인스턴스를 위한 설정입니다.
type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port" default:"6379"`
	DBNumber int    `json:"db_number" validate:"gte=0" default:"0"`
	Username string `json:"username"`
	Password string `json:"password"`

	// Connection settings
	PoolSize int `json:"pool_size" validate:"gt=0" default:"10"`
	// -1 disables retries.
	MaxRetries int `json:"max_retries" validate:"gte=-1" default:"3"`
	// -1 disables backoff.
	MinRetryBackoffMs int `json:"min_retry_backoff_ms" validate:"gte=-1" default:"8"`
	// -1 disables backoff.
	MaxRetryBackoffMs int `json:"max_retry_backoff_ms" validate:"gte=-1" default:"512"`

	// Timeouts
	DialTimeout  int `json:"dial_timeout" validate:"gte=-1" default:"5"`
	ReadTimeout  int `json:"read_timeout" validate:"gte=-1" default:"3"`
	WriteTimeout int `json:"write_timeout" validate:"gte=-1" default:"3"`
	PoolTimeout  int `json:"pool_timeout" validate:"gte=-1" default:"5"`
	IdleTimeout  int `json:"idle_timeout" validate:"gte=-1" default:"1"`

	// ClientSideCache 는 값 구조체다 — RedisConfig가 클라이언트 공유 맵의
	// 키로 쓰여 비교 가능해야 하기 때문.
	ClientSideCache ClientSideCacheConfig `json:"client_side_cache" default:"{}"`
}

// ClientSideCacheConfig 는 go-redis 실험적 클라이언트 사이드 캐싱 설정이다
// (RESP3 CLIENT TRACKING 기반 로컬 L1). 기본 off. standalone DB 0 전용 —
// 위반 시 go-redis가 경고와 함께 비활성화한다. MaxStalenessMs는 캐시 TTL
// 이하로 두는 것이 신선도 계약 보호의 불변식이다.
type ClientSideCacheConfig struct {
	Enabled bool `json:"enabled" default:"false"`
	// 주의: go-pdk 스키마 생성기가 int64를 표현하지 못해 필드가 스키마에서
	// 빠지면 Kong이 설정 파싱을 거부한다(unknown field) — 설정 필드는 int까지만.
	MaxEntries     int `json:"max_entries" validate:"gte=0" default:"0"`      // 0=라이브러리 기본 상한
	MaxMemoryBytes int `json:"max_memory_bytes" validate:"gte=0" default:"0"` // 0=무제한(항목수 상한 우선). 바이트 수, int 범위(≈2GB)까지
	MaxStalenessMs int `json:"max_staleness_ms" validate:"gte=0" default:"0"` // 0=비활성
}

// RedisClusterConfig 는 Redis Cluster를 위한 설정입니다.
type RedisClusterConfig struct {
	Addrs    []string `json:"addrs"`
	Username string   `json:"username"`
	Password string   `json:"password"`

	// Connection settings
	PoolSize   int `json:"pool_size" validate:"gt=0" default:"10"`
	MaxRetries int `json:"max_retries" validate:"gte=-1" default:"3"`
	// -1 disables backoff.
	MinRetryBackoffMs int `json:"min_retry_backoff_ms" validate:"gte=-1" default:"8"`
	// -1 disables backoff.
	MaxRetryBackoffMs int `json:"max_retry_backoff_ms" validate:"gte=-1" default:"512"`

	// Timeouts
	DialTimeout  int `json:"dial_timeout" validate:"gte=-1" default:"5"`
	ReadTimeout  int `json:"read_timeout" validate:"gte=-1" default:"3"`
	WriteTimeout int `json:"write_timeout" validate:"gte=-1" default:"3"`
	PoolTimeout  int `json:"pool_timeout" validate:"gte=-1" default:"5"`
	IdleTimeout  int `json:"idle_timeout" validate:"gte=-1" default:"1"`
}
