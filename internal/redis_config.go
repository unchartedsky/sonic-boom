package internal

// RedisConfig 는 [github.com/go-redis/redis] 의 Option을 Wrapping 하였습니다.
type RedisConfig struct {
	Host     string `json:"host" validate:"" default:"localhost"`
	Port     int    `json:"port" validate:"gte=0,lte=65536" default:"6379"`
	DBNumber int    `json:"db_number" validate:"gte=0" default:"0"`
	PoolSize int    `json:"pool_size" validate:"gt=0" default:"10"`

	// -1 disables retries.
	MaxRetries int `json:"max_retries" validate:"gte=-1" default:"3"`
	// -1 disables backoff.
	MinRetryBackoffMs int `json:"min_retry_backoff_ms" validate:"gte=-1" default:"8"`
	// -1 disables backoff.
	MaxRetryBackoffMs int `json:"max_retry_backoff_ms" validate:"gte=-1" default:"512"`

	Username string `json:"username"`
	Password string `json:"password"`

	DialTimeout  int `json:"dial_timeout" validate:"gte=-1" default:"5"`
	ReadTimeout  int `json:"read_timeout" validate:"gte=-1" default:"3"`
	WriteTimeout int `json:"write_timeout" validate:"gte=-1" default:"3"`
	PoolTimeout  int `json:"pool_timeout" validate:"gte=-1" default:"5"`
	IdleTimeout  int `json:"idle_timeout" validate:"gte=-1" default:"1"`
}
