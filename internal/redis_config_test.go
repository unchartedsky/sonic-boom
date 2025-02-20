package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/go-playground/validator/v10"
)

func TestRedisConfig_Validation(t *testing.T) {
	validate := validator.New()

	tests := []struct {
		name    string
		config  RedisConfig
		wantErr bool
	}{
		{
			name: "valid config with minimum values",
			config: RedisConfig{
				Host:     "localhost",
				Port:     6379,
				PoolSize: 1,
			},
			wantErr: false,
		},
		{
			name: "valid config with all fields",
			config: RedisConfig{
				Host:              "redis.example.com",
				Port:             6379,
				DBNumber:         0,
				Username:         "user",
				Password:         "pass",
				PoolSize:         10,
				MaxRetries:       3,
				MinRetryBackoffMs: 8,
				MaxRetryBackoffMs: 512,
				DialTimeout:      5,
				ReadTimeout:      3,
				WriteTimeout:     3,
				PoolTimeout:      5,
				IdleTimeout:      1,
			},
			wantErr: false,
		},
		{
			name: "invalid - negative db number",
			config: RedisConfig{
				Host:     "localhost",
				Port:     6379,
				DBNumber: -1,
				PoolSize: 1,
			},
			wantErr: true,
		},
		{
			name: "invalid - pool size zero",
			config: RedisConfig{
				Host:     "localhost",
				Port:     6379,
				PoolSize: 0,
			},
			wantErr: true,
		},
		{
			name: "valid - negative max retries (disables retries)",
			config: RedisConfig{
				Host:       "localhost",
				Port:       6379,
				PoolSize:   1,
				MaxRetries: -1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRedisClusterConfig_Validation(t *testing.T) {
	validate := validator.New()

	tests := []struct {
		name    string
		config  RedisClusterConfig
		wantErr bool
	}{
		{
			name: "valid config with minimum values",
			config: RedisClusterConfig{
				Addrs:    []string{"localhost:6379"},
				PoolSize: 1,
			},
			wantErr: false,
		},
		{
			name: "valid config with all fields",
			config: RedisClusterConfig{
				Addrs:             []string{"redis1:6379", "redis2:6379"},
				Username:          "user",
				Password:          "pass",
				PoolSize:          10,
				MaxRetries:        3,
				MinRetryBackoffMs: 8,
				MaxRetryBackoffMs: 512,
				DialTimeout:       5,
				ReadTimeout:       3,
				WriteTimeout:      3,
				PoolTimeout:       5,
				IdleTimeout:       1,
			},
			wantErr: false,
		},
		{
			name: "invalid - pool size zero",
			config: RedisClusterConfig{
				Addrs:    []string{"localhost:6379"},
				PoolSize: 0,
			},
			wantErr: true,
		},
		{
			name: "valid - negative max retries (disables retries)",
			config: RedisClusterConfig{
				Addrs:      []string{"localhost:6379"},
				PoolSize:   1,
				MaxRetries: -1,
			},
			wantErr: false,
		},
		{
			name: "valid - negative backoff (disables backoff)",
			config: RedisClusterConfig{
				Addrs:             []string{"localhost:6379"},
				PoolSize:          1,
				MinRetryBackoffMs: -1,
				MaxRetryBackoffMs: -1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
