package internal

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path"
	"strconv"
	"time"
)

type Logger struct {
	*zerolog.Logger
	writers []io.Writer
}

func (l *Logger) Close() {
	for _, w := range l.writers {
		if c, ok := w.(io.Closer); ok {
			_ = c.Close()
		}
	}
}

func newConsoleWriter(config *LogConfig) io.Writer {
	writer := zerolog.ConsoleWriter{Out: os.Stderr}
	if config.DiodeEnabled {
		return diode.NewWriter(writer, 1000, 10*time.Millisecond, func(missed int) {
			fmt.Printf("ConsoleWriter dropped %d messages", missed)
		})
	}
	return writer
}

func newRollingFileWriter(config *LogConfig) io.Writer {
	writer := newRollingFile(config.FileLogConf)
	if config.DiodeEnabled {
		return diode.NewWriter(writer, 1000, 10*time.Millisecond, func(missed int) {
			fmt.Printf("RollingFileWriter dropped %d messages", missed)
		})
	}
	return writer
}

// Configure sets up the logging framework
//
// In production, the container logs will be collected and file logging should be disabled. However,
// during development it's nicer to see logs as text and optionally write to a file when debugging
// problems in the containerized pipeline
//
// The output log file will be located at /var/log/service-xyz/service-xyz.log and
// will be rolled according to configuration set.
func NewLogger(config *LogConfig) *Logger {
	var writers []io.Writer

	if config.ConsoleLoggingEnabled {
		writers = append(writers, newConsoleWriter(config))
	}
	if config.FileLogConf.Enabled {
		writers = append(writers, newRollingFileWriter(config))
	}
	mw := io.MultiWriter(writers...)

	// Add file and line number to log
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
		return file + ":" + strconv.Itoa(line)
	}
	logger := zerolog.New(mw).With().Caller().Timestamp().Logger()

	logLevel, err := zerolog.ParseLevel(config.LogLevel)
	if err != nil {
		panic(err)
	}
	zerolog.SetGlobalLevel(logLevel)

	fileLogConf := config.FileLogConf
	logger.Info().
		Bool("fileLogging", fileLogConf.Enabled).
		Str("logFolder", fileLogConf.Folder).
		Str("fileName", fileLogConf.Filename).
		Int("maxSizeMB", fileLogConf.MaxSize).
		Int("maxBackups", fileLogConf.MaxBackups).
		Int("maxAgeInDays", fileLogConf.MaxAge).
		Str("logLevel", config.LogLevel).
		Msg("logging configured")

	return &Logger{
		Logger:  &logger,
		writers: writers,
	}
}

func newRollingFile(config *FileLogConfig) io.Writer {
	if err := os.MkdirAll(config.Folder, 0744); err != nil {
		panic(err)
	}

	return &lumberjack.Logger{
		Filename:   path.Join(config.Folder, config.Filename),
		MaxBackups: config.MaxBackups, // files
		MaxSize:    config.MaxSize,    // megabytes
		MaxAge:     config.MaxAge,     // days
	}
}

type LogConfig struct {
	LogLevel              string         `json:"log_level" validate:"required" default:"info"`
	ConsoleLoggingEnabled bool           `json:"console_logging_enabled" validate:"" default:"true"`
	FileLogConf           *FileLogConfig `json:"file_log" validate:"required" default:"{}"`
	DiodeEnabled          bool           `json:"diode_enabled" validate:"" default:"true"`
}

type FileLogConfig struct {
	Enabled bool `json:"enabled" validate:"" default:"false"`

	Filename string `json:"filename" validate:"" default:"sonic-boom.log"`
	Folder   string `json:"folder" validate:"" default:"/tmp/logs"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `json:"maxsize" yaml:"maxsize"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `json:"maxage" yaml:"maxage"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `json:"maxbackups" yaml:"maxbackups"`

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time.  The default is to use UTC
	// time.
	LocalTime bool `json:"localtime" yaml:"localtime"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`
}
