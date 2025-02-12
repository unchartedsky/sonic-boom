package internal

import (
	"github.com/Kong/go-pdk"
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func Test_newRollingFile(t *testing.T) {
	type args struct {
		config *FileLogConfig
	}
	tests := []struct {
		name       string
		args       args
		want       io.Writer
		wantNotNil bool
	}{
		// TODO: Add test cases.
		{
			name: "test",
			args: args{
				config: &FileLogConfig{
					Enabled:  true,
					Filename: "test.log",
					Folder:   "/tmp/logs",
				},
			},
			wantNotNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantNotNil {
				assert.NotNil(t, newRollingFile(tt.args.config), "newRollingFile(%v)", tt.args.config)
			} else {
				assert.Equalf(t, tt.want, newRollingFile(tt.args.config), "newRollingFile(%v)", tt.args.config)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	type args struct {
		config *LogConfig
		kong   *pdk.PDK
	}
	tests := []struct {
		name       string
		args       args
		want       *Logger
		wantNotNil bool
	}{
		// TODO: Add test cases.
		{
			name: "test",
			args: args{
				config: &LogConfig{
					LogLevel:              "info",
					ConsoleLoggingEnabled: true,
					FileLogConf: &FileLogConfig{
						Enabled:  false,
						Filename: "sonic-boom.log",
						Folder:   "/tmp/logs",
					},
				},
			},
			wantNotNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantNotNil {
				assert.NotNil(t, NewLogger(tt.args.config), "NewLogger(%v, %v)", tt.args.config, tt.args.kong)
			} else {
				assert.Equalf(t, tt.want, NewLogger(tt.args.config), "NewLogger(%v, %v)", tt.args.config, tt.args.kong)
			}
		})
	}
}
