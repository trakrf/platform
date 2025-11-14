package logger

import (
	"bytes"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestInitialize(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "Development console logger",
			config: &Config{
				Environment:    EnvDev,
				ServiceName:    "test-service",
				Level:          "debug",
				Format:         "console",
				IncludeStack:   true,
				IncludeCaller:  true,
				ColorOutput:    false,
				SanitizeEmails: false,
				SanitizeIPs:    false,
				MaxBodySize:    1000,
				Version:        "1.0.0",
			},
		},
		{
			name: "Production JSON logger",
			config: &Config{
				Environment:    EnvProd,
				ServiceName:    "prod-service",
				Level:          "warn",
				Format:         "json",
				IncludeStack:   false,
				IncludeCaller:  false,
				ColorOutput:    false,
				SanitizeEmails: true,
				SanitizeIPs:    true,
				MaxBodySize:    0,
				Version:        "2.0.0",
			},
		},
		{
			name: "Staging JSON logger",
			config: &Config{
				Environment:    EnvStaging,
				ServiceName:    "staging-service",
				Level:          "info",
				Format:         "json",
				IncludeStack:   false,
				IncludeCaller:  false,
				ColorOutput:    false,
				SanitizeEmails: true,
				SanitizeIPs:    true,
				MaxBodySize:    0,
				Version:        "1.5.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := Initialize(tt.config)

			assert.NotNil(t, logger, "Initialize should return a non-nil logger")

			// Verify Get() returns the same logger
			got := Get()
			assert.Equal(t, logger, got, "Get() should return the logger created by Initialize()")
		})
	}
}

func TestGet(t *testing.T) {
	t.Run("Returns initialized logger", func(t *testing.T) {
		cfg := &Config{
			Environment:    EnvDev,
			ServiceName:    "test-service",
			Level:          "debug",
			Format:         "console",
			IncludeStack:   true,
			IncludeCaller:  true,
			ColorOutput:    false,
			SanitizeEmails: false,
			SanitizeIPs:    false,
			MaxBodySize:    1000,
			Version:        "1.0.0",
		}

		logger := Initialize(cfg)
		got := Get()

		assert.Equal(t, logger, got, "Get() should return the initialized logger")
	})

	t.Run("Creates default logger if not initialized", func(t *testing.T) {
		// Reset global logger to simulate uninitialized state
		globalLogger = nil

		got := Get()
		assert.NotNil(t, got, "Get() should create a default logger if not initialized")
	})
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected zerolog.Level
	}{
		{
			name:     "Debug level",
			level:    "debug",
			expected: zerolog.DebugLevel,
		},
		{
			name:     "Info level",
			level:    "info",
			expected: zerolog.InfoLevel,
		},
		{
			name:     "Warn level",
			level:    "warn",
			expected: zerolog.WarnLevel,
		},
		{
			name:     "Error level",
			level:    "error",
			expected: zerolog.ErrorLevel,
		},
		{
			name:     "Fatal level",
			level:    "fatal",
			expected: zerolog.FatalLevel,
		},
		{
			name:     "Unknown level defaults to info",
			level:    "unknown",
			expected: zerolog.InfoLevel,
		},
		{
			name:     "Empty level defaults to info",
			level:    "",
			expected: zerolog.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitializeWithDifferentFormats(t *testing.T) {
	t.Run("Console format creates ConsoleWriter", func(t *testing.T) {
		cfg := &Config{
			Environment:   EnvDev,
			ServiceName:   "test",
			Level:         "info",
			Format:        "console",
			ColorOutput:   true,
			IncludeCaller: false,
			IncludeStack:  false,
			Version:       "1.0.0",
		}

		logger := Initialize(cfg)
		assert.NotNil(t, logger)

		// Test that we can write a log
		var buf bytes.Buffer
		testLogger := logger.Output(&buf)
		testLogger.Info().Msg("test message")

		// Verify output exists (ConsoleWriter will format it)
		assert.NotEmpty(t, buf.String())
	})

	t.Run("JSON format creates JSON logger", func(t *testing.T) {
		cfg := &Config{
			Environment:   EnvProd,
			ServiceName:   "test",
			Level:         "info",
			Format:        "json",
			ColorOutput:   false,
			IncludeCaller: false,
			IncludeStack:  false,
			Version:       "1.0.0",
		}

		logger := Initialize(cfg)
		assert.NotNil(t, logger)

		// Test that we can write a JSON log
		var buf bytes.Buffer
		testLogger := logger.Output(&buf)
		testLogger.Info().Msg("test message")

		// Verify JSON output exists
		assert.Contains(t, buf.String(), `"message":"test message"`)
		assert.Contains(t, buf.String(), `"service":"test"`)
		assert.Contains(t, buf.String(), `"env":"prod"`)
		assert.Contains(t, buf.String(), `"version":"1.0.0"`)
	})
}

func TestInitializeWithCallerInfo(t *testing.T) {
	t.Run("IncludeCaller adds caller information", func(t *testing.T) {
		cfg := &Config{
			Environment:   EnvDev,
			ServiceName:   "test",
			Level:         "info",
			Format:        "json",
			IncludeCaller: true,
			IncludeStack:  false,
			Version:       "1.0.0",
		}

		logger := Initialize(cfg)
		assert.NotNil(t, logger)

		var buf bytes.Buffer
		testLogger := logger.Output(&buf)
		testLogger.Info().Msg("test message")

		// Verify caller field exists in JSON output
		assert.Contains(t, buf.String(), `"caller"`)
	})

	t.Run("Without IncludeCaller no caller information", func(t *testing.T) {
		cfg := &Config{
			Environment:   EnvProd,
			ServiceName:   "test",
			Level:         "info",
			Format:        "json",
			IncludeCaller: false,
			IncludeStack:  false,
			Version:       "1.0.0",
		}

		logger := Initialize(cfg)
		assert.NotNil(t, logger)

		var buf bytes.Buffer
		testLogger := logger.Output(&buf)
		testLogger.Info().Msg("test message")

		// Verify no caller field in JSON output
		assert.NotContains(t, buf.String(), `"caller"`)
	})
}

func TestInitializeGlobalFields(t *testing.T) {
	t.Run("Adds service, env, and version fields", func(t *testing.T) {
		cfg := &Config{
			Environment:   EnvStaging,
			ServiceName:   "my-service",
			Level:         "info",
			Format:        "json",
			IncludeCaller: false,
			IncludeStack:  false,
			Version:       "v2.5.0",
		}

		logger := Initialize(cfg)
		assert.NotNil(t, logger)

		var buf bytes.Buffer
		testLogger := logger.Output(&buf)
		testLogger.Info().Msg("test message")

		output := buf.String()
		assert.Contains(t, output, `"service":"my-service"`)
		assert.Contains(t, output, `"env":"staging"`)
		assert.Contains(t, output, `"version":"v2.5.0"`)
	})
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()

	// Exit with test result code
	os.Exit(code)
}
