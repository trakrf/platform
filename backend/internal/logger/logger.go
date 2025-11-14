package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var globalLogger *zerolog.Logger

func Initialize(cfg *Config) *zerolog.Logger {
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	var logger zerolog.Logger
	if cfg.Format == "console" {
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    !cfg.ColorOutput,
		}
		logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	if cfg.IncludeCaller {
		logger = logger.With().Caller().Logger()
	}

	logger = logger.With().
		Str("service", cfg.ServiceName).
		Str("env", string(cfg.Environment)).
		Str("version", cfg.Version).
		Logger()

	if cfg.IncludeStack {
		logger = logger.With().Stack().Logger()
	}

	globalLogger = &logger
	log.Logger = logger

	return &logger
}

func Get() *zerolog.Logger {
	if globalLogger == nil {
		cfg := NewConfig("unknown")
		return Initialize(cfg)
	}
	return globalLogger
}

func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}
