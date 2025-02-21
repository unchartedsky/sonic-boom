package internal

import (
	"gopkg.in/go-playground/validator.v9"
)

// InMemoryConfig represents the configuration for in-memory cache using Ristretto
type InMemoryConfig struct {
	MaxCost     int `json:"max_cost" validate:"gte=0" default:"1000000"`
	NumCounters int `json:"num_counters" validate:"gte=0" default:"1000000"`
	BufferItems int `json:"buffer_items" validate:"gte=0" default:"64"`
}

// Validate validates the InMemoryConfig
func Validate(config *InMemoryConfig) error {
	validate := validator.New()
	return validate.Struct(config)
}
