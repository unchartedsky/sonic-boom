package internal

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

// Custom validator for body length consistency
func validateBodyLength(sl validator.StructLevel) {
	cv := sl.Current().Interface().(CacheValue)
	if cv.Body == nil {
		if cv.BodyLen != 0 {
			sl.ReportError(cv.BodyLen, "BodyLen", "BodyLen", "bodyLenMatch", "")
		}
	} else if len(cv.Body) != cv.BodyLen {
		sl.ReportError(cv.BodyLen, "BodyLen", "BodyLen", "bodyLenMatch", "")
	}
}

func newValidator() *validator.Validate {
	validate := validator.New()
	validate.RegisterStructValidation(validateBodyLength, CacheValue{})
	return validate
}

func TestCacheValue_Validation(t *testing.T) {
	validate := newValidator()
	now := time.Now().Unix()

	tests := []struct {
		name    string
		value   CacheValue
		wantErr bool
	}{
		{
			// 모든 필수 필드가 유효한 값으로 설정된 경우 검증 통과
			name: "valid cache value with minimum fields",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{"Content-Type": {"text/plain"}},
				Body:      []byte("Hello"),
				BodyLen:   5,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: false,
		},
		{
			// 빈 본문(empty body)의 경우 BodyLen도 0이어야 함
			name: "valid - empty body with zero length",
			value: CacheValue{
				Status:    204,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: false,
		},
		{
			// nil 본문의 경우 BodyLen도 0이어야 함
			name: "valid - nil body with zero length",
			value: CacheValue{
				Status:    204,
				Headers:   map[string][]string{},
				Body:      nil,
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: false,
		},
		{
			// Status가 음수인 경우 검증 실패
			name: "invalid - negative status",
			value: CacheValue{
				Status:    -1,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// BodyLen이 음수인 경우 검증 실패
			name: "invalid - negative body length",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   -1,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// TTL이 음수인 경우 검증 실패
			name: "invalid - negative TTL",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: now,
				TTL:       -1,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// Version이 빈 문자열인 경우 검증 실패
			name: "invalid - empty version",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// ReqBody가 누락된 경우 검증 실패
			name: "invalid - missing required ReqBody",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
			},
			wantErr: true,
		},
		{
			// Timestamp가 음수인 경우 검증 실패
			name: "invalid - negative timestamp",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte{},
				BodyLen:   0,
				Timestamp: -1,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// Body의 실제 길이와 BodyLen이 일치하지 않는 경우 검증 실패
			name: "invalid - inconsistent body length",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte("hello"),
				BodyLen:   10,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// nil 본문과 함께 BodyLen이 0이 아닌 경우 검증 실패
			name: "invalid - nil body with non-zero length",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      nil,
				BodyLen:   5,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
		{
			// 비어있지 않은 본문과 함께 BodyLen이 일치하지 않는 경우 검증 실패
			name: "invalid - inconsistent body length with non-empty body",
			value: CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      []byte("hello"),
				BodyLen:   0,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCacheValue_String(t *testing.T) {
	// CacheValue 구조체의 문자열 표현이 예상대로 포맷팅되는지 검증
	now := time.Now().Unix()
	cv := CacheValue{
		Status:    200,
		Headers:   map[string][]string{"Content-Type": {"text/plain"}},
		Body:      []byte("Hello"),
		BodyLen:   5,
		Timestamp: now,
		TTL:       300,
		Version:   "1.0",
		ReqBody:   []byte{},
	}

	expected := "CacheValue{Status: 200, Headers: map[Content-Type:[text/plain]], BodyLen: 5, " +
		"Timestamp: %d, TTL: 300, Version: 1.0}"

	assert.Equal(t,
		cv.String(),
		fmt.Sprintf(expected, now),
		"String representation should match expected format",
	)
}

func TestCacheValue_BodyLenConsistency(t *testing.T) {
	validate := newValidator()

	tests := []struct {
		name    string
		body    []byte
		bodyLen int
		wantErr bool
	}{
		{
			// Body의 실제 길이와 BodyLen이 일치하는 경우 검증 통과
			name:    "consistent body length",
			body:    []byte("hello"),
			bodyLen: 5,
			wantErr: false,
		},
		{
			// Body의 실제 길이와 BodyLen이 일치하지 않는 경우 검증 실패
			name:    "inconsistent body length",
			body:    []byte("hello"),
			bodyLen: 10,
			wantErr: true,
		},
		{
			// 빈 본문(empty body)의 경우 BodyLen도 0이어야 함
			name:    "valid - empty body with zero length",
			body:    []byte{},
			bodyLen: 0,
			wantErr: false,
		},
		{
			// nil 본문의 경우 BodyLen도 0이어야 함
			name:    "valid - nil body with zero length",
			body:    nil,
			bodyLen: 0,
			wantErr: false,
		},
		{
			// nil 본문과 함께 BodyLen이 0이 아닌 경우 검증 실패
			name:    "invalid - nil body with non-zero length",
			body:    nil,
			bodyLen: 5,
			wantErr: true,
		},
		{
			// 비어있지 않은 본문과 함께 BodyLen이 일치하지 않는 경우 검증 실패
			name:    "invalid - inconsistent body length with non-empty body",
			body:    []byte("hello"),
			bodyLen: 0,
			wantErr: true,
		},
	}

	now := time.Now().Unix()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := CacheValue{
				Status:    200,
				Headers:   map[string][]string{},
				Body:      tt.body,
				BodyLen:   tt.bodyLen,
				Timestamp: now,
				TTL:       300,
				Version:   "1.0",
				ReqBody:   []byte{},
			}

			err := validate.Struct(cv)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(cv.Body), cv.BodyLen,
					"BodyLen should match actual body length")
			}
		})
	}
}
