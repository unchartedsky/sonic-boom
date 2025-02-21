package internal

import (
	"encoding/json"
	"testing"

	"github.com/creasty/defaults"
	"github.com/stretchr/testify/assert"
)

func TestInMemoryConfig_Defaults(t *testing.T) {
	config := &InMemoryConfig{}
	err := defaults.Set(config)
	assert.NoError(t, err)

	assert.Equal(t, 1000000, config.MaxCost)
	assert.Equal(t, 1000000, config.NumCounters)
	assert.Equal(t, 64, config.BufferItems)
}

func TestInMemoryConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  InMemoryConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: InMemoryConfig{
				MaxCost:     1000000,
				NumCounters: 1000000,
				BufferItems: 64,
			},
			wantErr: false,
		},
		{
			name: "invalid max cost",
			config: InMemoryConfig{
				MaxCost:     -1,
				NumCounters: 1000000,
				BufferItems: 64,
			},
			wantErr: true,
		},
		{
			name: "invalid num counters",
			config: InMemoryConfig{
				MaxCost:     1000000,
				NumCounters: -1,
				BufferItems: 64,
			},
			wantErr: true,
		},
		{
			name: "invalid buffer items",
			config: InMemoryConfig{
				MaxCost:     1000000,
				NumCounters: 1000000,
				BufferItems: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInMemoryConfig_JSON(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantConfig InMemoryConfig
		wantErr    bool
	}{
		{
			name: "valid json",
			json: `{
				"max_cost": 2000000,
				"num_counters": 3000000,
				"buffer_items": 128
			}`,
			wantConfig: InMemoryConfig{
				MaxCost:     2000000,
				NumCounters: 3000000,
				BufferItems: 128,
			},
			wantErr: false,
		},
		{
			name: "missing fields should use defaults",
			json: `{}`,
			wantConfig: InMemoryConfig{
				MaxCost:     1000000,
				NumCounters: 1000000,
				BufferItems: 64,
			},
			wantErr: false,
		},
		{
			name: "invalid json",
			json: `{
				"max_cost": "invalid"
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config InMemoryConfig
			err := json.Unmarshal([]byte(tt.json), &config)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			err = defaults.Set(&config)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantConfig, config)
		})
	}
}
