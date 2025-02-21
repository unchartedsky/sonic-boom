package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/go-playground/validator.v9"
)

func TestNewCacheSignal(t *testing.T) {
	tests := []struct {
		name      string
		cacheKeyID string
		cacheTTL   int
		want      *CacheSignal
	}{
		{
			name:      "기본 캐시 시그널 생성",
			cacheKeyID: "test-key",
			cacheTTL:   3600,
			want: &CacheSignal{
				CacheKeyID: "test-key",
				CacheTTL:   3600,
			},
		},
		{
			name:      "TTL이 0인 캐시 시그널",
			cacheKeyID: "test-key-2",
			cacheTTL:   0,
			want: &CacheSignal{
				CacheKeyID: "test-key-2",
				CacheTTL:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCacheSignal(tt.cacheKeyID, tt.cacheTTL)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCacheSignal_Validation(t *testing.T) {
	tests := []struct {
		name    string
		signal  *CacheSignal
		wantErr bool
	}{
		{
			name: "유효한 캐시 시그널",
			signal: &CacheSignal{
				CacheKeyID: "test-key",
				CacheTTL:   3600,
			},
			wantErr: false,
		},
		{
			name: "캐시 키 ID가 없는 경우",
			signal: &CacheSignal{
				CacheKeyID: "",
				CacheTTL:   3600,
			},
			wantErr: true,
		},
		{
			name: "음수 TTL",
			signal: &CacheSignal{
				CacheKeyID: "test-key",
				CacheTTL:   -1,
			},
			wantErr: true,
		},
	}

	validate := validator.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.signal)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
