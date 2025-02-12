package internal

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_convertRedisTimeout(t *testing.T) {
	type args struct {
		timeout  int
		timeUnit time.Duration
	}
	tests := []struct {
		name string
		args args
		want time.Duration
	}{
		// TODO: Add test cases.
		{
			name: "timeout value is zero.",
			args: args{
				timeout:  0,
				timeUnit: time.Second,
			},
			want: 0,
		},
		{
			name: "normal test",
			args: args{
				timeout:  1,
				timeUnit: time.Second,
			},
			want: time.Duration(1) * time.Second,
		},
		{
			name: "negative timeout value is -1",
			args: args{
				timeout:  -1,
				timeUnit: time.Second,
			},
			want: -1,
		},
		{
			name: "negative timeout value is -1",
			args: args{
				timeout:  -100,
				timeUnit: time.Second,
			},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, convertRedisTimeout(tt.args.timeout, tt.args.timeUnit), "convertRedisTimeout(%v, %v)", tt.args.timeout, tt.args.timeUnit)
		})
	}
}
