# Implementation Plan: Backend Logging — Production-Grade Structured Logging

Generated: 2025-11-02
Specification: spec.md
Scope: Phase 1+2 + Minimal Sanitization (MVP)

## Understanding

This implementation adds production-grade structured logging to the Go backend with:
- **Environment detection**: Auto-adapts format/level based on dev/staging/prod
- **Structured logging**: Uses zerolog for zero-allocation JSON logging
- **Request correlation**: Leverages existing `X-Request-ID` middleware
- **Minimal sanitization**: Redacts Authorization headers to prevent token leaks
- **Docker-ready**: Logs to stdout for Docker log collection

**Key decisions**:
- Reuse existing `middleware.RequestID` for request correlation (already checks `X-Request-ID` header)
- Replace current `slog` usage with `zerolog`
- Auto-detect service name from Go module path with `SERVICE_NAME` env override
- Use testify/assert for cleaner test assertions (already in dependencies)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/middleware/middleware.go` (lines 22-34) - RequestID middleware pattern to integrate with
- `backend/internal/middleware/middleware_test.go` (lines 10-161) - Table-driven test pattern with httptest
- `backend/main.go` (lines 94-97) - Current slog initialization to replace
- `backend/main.go` (lines 57-60) - Middleware chain where logging middleware will be added

**Files to Create**:
- `backend/internal/logger/config.go` - Environment detection and configuration
- `backend/internal/logger/logger.go` - Core logger initialization and utilities
- `backend/internal/logger/middleware.go` - HTTP logging middleware for request/response logging
- `backend/internal/logger/sanitize.go` - Minimal sanitization (Authorization header redaction)
- `backend/internal/logger/config_test.go` - Environment detection tests
- `backend/internal/logger/logger_test.go` - Logger initialization tests
- `backend/internal/logger/middleware_test.go` - HTTP middleware tests
- `backend/internal/logger/sanitize_test.go` - Sanitization tests

**Files to Modify**:
- `backend/go.mod` - Add `github.com/rs/zerolog` dependency
- `backend/main.go` (lines 94-97) - Replace slog with zerolog logger initialization
- `backend/main.go` (lines 57-60) - Add logging middleware to chi router middleware chain
- `backend/internal/middleware/middleware.go` (lines 40-54) - Update Recovery middleware to use new logger

## Architecture Impact

- **Subsystems affected**: Logger (new), HTTP/API (middleware integration)
- **New dependencies**: `github.com/rs/zerolog v1.33.0`
- **Breaking changes**: None (replaces slog internally, same logging calls work)

## Task Breakdown

### Task 1: Add zerolog dependency
**File**: `backend/go.mod`
**Action**: MODIFY
**Pattern**: Standard Go dependency addition

**Implementation**:
```bash
cd backend
go get github.com/rs/zerolog@v1.33.0
go mod tidy
```

**Validation**:
```bash
cd backend
just lint    # Verify go.mod is formatted correctly
grep "rs/zerolog" go.mod  # Confirm dependency added
```

---

### Task 2: Create logger configuration
**File**: `backend/internal/logger/config.go`
**Action**: CREATE
**Pattern**: Environment detection with smart defaults

**Implementation**:
```go
package logger

import (
	"os"
	"strings"
)

// Environment represents deployment environment
type Environment string

const (
	EnvDev     Environment = "dev"
	EnvStaging Environment = "staging"
	EnvProd    Environment = "prod"
)

// Config holds logger configuration
type Config struct {
	Environment     Environment
	ServiceName     string
	Level           string
	Format          string // "json" or "console"
	IncludeStack    bool
	IncludeCaller   bool
	ColorOutput     bool
	SanitizeEmails  bool
	SanitizeIPs     bool
	MaxBodySize     int
	Version         string
}

// DetectEnvironment determines environment from env vars or defaults to dev
func DetectEnvironment() Environment {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	switch env {
	case "staging":
		return EnvStaging
	case "prod", "production":
		return EnvProd
	default:
		return EnvDev
	}
}

// DetectServiceName extracts service name from module path or env var
func DetectServiceName() string {
	// Check env var first
	if name := os.Getenv("SERVICE_NAME"); name != "" {
		return name
	}

	// Default: extract from module path
	// "github.com/trakrf/platform/backend" -> "platform-backend"
	return "platform-backend"
}

// NewConfig creates config with environment-specific defaults
func NewConfig(version string) *Config {
	env := DetectEnvironment()

	cfg := &Config{
		Environment: env,
		ServiceName: DetectServiceName(),
		Version:     version,
	}

	// Apply environment-specific defaults
	switch env {
	case EnvDev:
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "debug")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "console")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", true)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", true)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", true)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", false)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", false)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 1000)

	case EnvStaging:
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "info")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "json")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", false)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", false)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", false)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", true)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", true)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 0) // disabled

	case EnvProd:
		cfg.Level = getEnvOrDefault("LOG_LEVEL", "warn")
		cfg.Format = getEnvOrDefault("LOG_FORMAT", "json")
		cfg.IncludeStack = getBoolEnv("LOG_INCLUDE_STACK", false)
		cfg.IncludeCaller = getBoolEnv("LOG_INCLUDE_CALLER", false)
		cfg.ColorOutput = getBoolEnv("LOG_COLOR", false)
		cfg.SanitizeEmails = getBoolEnv("LOG_SANITIZE_EMAILS", true)
		cfg.SanitizeIPs = getBoolEnv("LOG_SANITIZE_IPS", true)
		cfg.MaxBodySize = getIntEnv("LOG_MAX_BODY_SIZE", 0) // disabled
	}

	return cfg
}

// Helper functions for env var parsing
func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getBoolEnv(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1"
}

func getIntEnv(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	var result int
	if _, err := fmt.Sscanf(val, "%d", &result); err != nil {
		return defaultVal
	}
	return result
}
```

**Validation**:
```bash
cd backend
just lint      # Verify code formatting
just test      # Will pass once tests are added
```

---

### Task 3: Create logger initialization
**File**: `backend/internal/logger/logger.go`
**Action**: CREATE
**Pattern**: Zerolog initialization with environment-aware formatting

**Implementation**:
```go
package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var globalLogger *zerolog.Logger

// Initialize sets up the global logger based on config
func Initialize(cfg *Config) *zerolog.Logger {
	// Set global log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Configure output format
	var logger zerolog.Logger
	if cfg.Format == "console" {
		// Development: human-readable console output
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    !cfg.ColorOutput,
		}
		logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		// Staging/Prod: JSON output
		logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	// Add caller info if enabled (dev only typically)
	if cfg.IncludeCaller {
		logger = logger.With().Caller().Logger()
	}

	// Add global fields
	logger = logger.With().
		Str("service", cfg.ServiceName).
		Str("env", string(cfg.Environment)).
		Str("version", cfg.Version).
		Logger()

	// Store stack trace setting for error logging
	if cfg.IncludeStack {
		logger = logger.With().Stack().Logger()
	}

	globalLogger = &logger
	log.Logger = logger // Set as default zerolog logger

	return &logger
}

// Get returns the global logger instance
func Get() *zerolog.Logger {
	if globalLogger == nil {
		// Fallback: create default logger if not initialized
		cfg := NewConfig("unknown")
		return Initialize(cfg)
	}
	return globalLogger
}

// parseLevel converts string level to zerolog.Level
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
```

**Validation**:
```bash
cd backend
just lint
just test
```

---

### Task 4: Create sanitization utilities
**File**: `backend/internal/logger/sanitize.go`
**Action**: CREATE
**Pattern**: Minimal redaction for Authorization header

**Implementation**:
```go
package logger

import (
	"net/http"
	"strings"
)

// SanitizeHeaders redacts sensitive headers (Authorization, etc.)
func SanitizeHeaders(headers http.Header) map[string]string {
	sanitized := make(map[string]string)

	for key, values := range headers {
		lowerKey := strings.ToLower(key)

		// Redact Authorization header
		if lowerKey == "authorization" {
			// Check if it's a Bearer token
			if len(values) > 0 && strings.HasPrefix(values[0], "Bearer ") {
				sanitized[key] = "Bearer <redacted>"
			} else {
				sanitized[key] = "<redacted>"
			}
			continue
		}

		// Keep other headers as-is (for MVP)
		if len(values) > 0 {
			sanitized[key] = values[0]
		}
	}

	return sanitized
}
```

**Validation**:
```bash
cd backend
just lint
just test
```

---

### Task 5: Create HTTP logging middleware
**File**: `backend/internal/logger/middleware.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/middleware/middleware.go` for chi middleware pattern

**Implementation**:
```go
package logger

import (
	"net/http"
	"time"

	"github.com/trakrf/platform/backend/internal/middleware"
)

// Middleware logs HTTP requests and responses
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get request ID from context (set by middleware.RequestID)
		requestID := middleware.GetRequestID(r.Context())

		// Create request logger with context
		logger := Get().With().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_ip", r.RemoteAddr).
			Logger()

		// Log incoming request
		logger.Debug().
			Interface("headers", SanitizeHeaders(r.Header)).
			Msg("Request received")

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Calculate duration
		duration := time.Since(start)

		// Log request completion
		logEvent := logger.Info().
			Int("status", wrapped.statusCode).
			Dur("duration_ms", duration).
			Int64("duration_ms_int", duration.Milliseconds())

		if wrapped.statusCode >= 400 {
			logEvent = logger.Warn().
				Int("status", wrapped.statusCode).
				Dur("duration_ms", duration).
				Int64("duration_ms_int", duration.Milliseconds())
		}

		logEvent.Msg("Request completed")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
```

**Validation**:
```bash
cd backend
just lint
just test
```

---

### Task 6: Update Recovery middleware to use new logger
**File**: `backend/internal/middleware/middleware.go`
**Action**: MODIFY (lines 36-54)
**Pattern**: Replace slog calls with zerolog

**Implementation**:
Replace the Recovery function:
```go
// Recovery catches panics and returns a 500 error response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := GetRequestID(r.Context())

				// Use zerolog instead of slog
				logger.Get().Error().
					Interface("error", err).
					Str("request_id", requestID).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("Panic recovered")

				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal server error", "", requestID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
```

Add import at top:
```go
import (
	// ... existing imports ...
	"github.com/trakrf/platform/backend/internal/logger"
)
```

**Validation**:
```bash
cd backend
just lint
just test
```

---

### Task 7: Update main.go to use zerolog
**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Replace slog initialization and add logging middleware

**Step 7a - Replace logger initialization (lines 94-97)**:
```go
// Initialize structured logger
loggerCfg := logger.NewConfig(version)
logger.Initialize(loggerCfg)
log := logger.Get()

log.Info().Msg("Logger initialized")
```

**Step 7b - Add logging middleware to chi router (after line 57)**:
In `setupRouter` function, add after `r.Use(middleware.RequestID)`:
```go
r.Use(middleware.RequestID)
r.Use(logger.Middleware)  // Add this line
r.Use(middleware.Recovery)
```

**Step 7c - Update existing log calls**:
Replace:
- `slog.Info(...)` → `log.Info().Msg(...)`
- `slog.Error(...)` → `log.Error().Err(err).Msg(...)`

**Step 7d - Update imports**:
Remove:
```go
"log/slog"
```

Add:
```go
"github.com/trakrf/platform/backend/internal/logger"
```

**Validation**:
```bash
cd backend
just lint
just build   # Verify compiles
just test    # Verify tests pass
```

---

### Task 8: Add config tests
**File**: `backend/internal/logger/config_test.go`
**Action**: CREATE
**Pattern**: Table-driven tests with testify/assert

**Implementation**:
```go
package logger

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected Environment
	}{
		{"dev default", "", EnvDev},
		{"dev explicit", "dev", EnvDev},
		{"staging", "staging", EnvStaging},
		{"prod", "prod", EnvProd},
		{"production alias", "production", EnvProd},
		{"unknown defaults to dev", "unknown", EnvDev},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var
			if tt.envValue != "" {
				os.Setenv("ENVIRONMENT", tt.envValue)
			} else {
				os.Unsetenv("ENVIRONMENT")
			}
			defer os.Unsetenv("ENVIRONMENT")

			result := DetectEnvironment()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewConfig_EnvironmentDefaults(t *testing.T) {
	tests := []struct {
		env              string
		expectLevel      string
		expectFormat     string
		expectCaller     bool
		expectColorOut   bool
	}{
		{"dev", "debug", "console", true, true},
		{"staging", "info", "json", false, false},
		{"prod", "warn", "json", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			os.Setenv("ENVIRONMENT", tt.env)
			defer os.Unsetenv("ENVIRONMENT")

			cfg := NewConfig("test-version")

			assert.Equal(t, tt.expectLevel, cfg.Level)
			assert.Equal(t, tt.expectFormat, cfg.Format)
			assert.Equal(t, tt.expectCaller, cfg.IncludeCaller)
			assert.Equal(t, tt.expectColorOut, cfg.ColorOutput)
			assert.Equal(t, "test-version", cfg.Version)
		})
	}
}

func TestDetectServiceName(t *testing.T) {
	// With env var
	os.Setenv("SERVICE_NAME", "custom-service")
	defer os.Unsetenv("SERVICE_NAME")

	result := DetectServiceName()
	assert.Equal(t, "custom-service", result)

	// Without env var
	os.Unsetenv("SERVICE_NAME")
	result = DetectServiceName()
	assert.Equal(t, "platform-backend", result)
}
```

**Validation**:
```bash
cd backend
just test
```

---

### Task 9: Add logger tests
**File**: `backend/internal/logger/logger_test.go`
**Action**: CREATE
**Pattern**: Test logger initialization with different configs

**Implementation**:
```go
package logger

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestInitialize_JSONFormat(t *testing.T) {
	cfg := &Config{
		Environment:   EnvProd,
		ServiceName:   "test-service",
		Level:         "info",
		Format:        "json",
		IncludeCaller: false,
		ColorOutput:   false,
		Version:       "1.0.0",
	}

	logger := Initialize(cfg)
	assert.NotNil(t, logger)

	// Verify global level was set
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
}

func TestInitialize_ConsoleFormat(t *testing.T) {
	cfg := &Config{
		Environment:   EnvDev,
		ServiceName:   "test-service",
		Level:         "debug",
		Format:        "console",
		IncludeCaller: true,
		ColorOutput:   true,
		Version:       "dev",
	}

	logger := Initialize(cfg)
	assert.NotNil(t, logger)

	// Verify global level was set
	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"unknown", zerolog.InfoLevel}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLevel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGet_InitializesDefaultIfNotSet(t *testing.T) {
	// Reset global logger
	globalLogger = nil
	os.Setenv("ENVIRONMENT", "dev")
	defer os.Unsetenv("ENVIRONMENT")

	logger := Get()
	assert.NotNil(t, logger)
	assert.NotNil(t, globalLogger)
}
```

**Validation**:
```bash
cd backend
just test
```

---

### Task 10: Add middleware tests
**File**: `backend/internal/logger/middleware_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/middleware/middleware_test.go` for httptest pattern

**Implementation**:
```go
package logger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/trakrf/platform/backend/internal/middleware"
)

func TestMiddleware_LogsRequest(t *testing.T) {
	// Initialize logger
	os.Setenv("ENVIRONMENT", "dev")
	defer os.Unsetenv("ENVIRONMENT")
	cfg := NewConfig("test")
	Initialize(cfg)

	// Create test handler
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create request with request ID in context
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Wrap with middleware
	handler := Middleware(nextHandler)
	handler.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
}

func TestMiddleware_CapturesStatusCode(t *testing.T) {
	os.Setenv("ENVIRONMENT", "dev")
	defer os.Unsetenv("ENVIRONMENT")
	cfg := NewConfig("test")
	Initialize(cfg)

	tests := []struct {
		name           string
		handlerStatus  int
		expectedStatus int
	}{
		{"200 OK", http.StatusOK, http.StatusOK},
		{"404 Not Found", http.StatusNotFound, http.StatusNotFound},
		{"500 Internal Error", http.StatusInternalServerError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.handlerStatus)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-id")
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			handler := Middleware(nextHandler)
			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}
```

**Validation**:
```bash
cd backend
just test
```

---

### Task 11: Add sanitization tests
**File**: `backend/internal/logger/sanitize_test.go`
**Action**: CREATE
**Pattern**: Test header sanitization

**Implementation**:
```go
package logger

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeHeaders_Authorization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Bearer token",
			input:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
			expected: "Bearer <redacted>",
		},
		{
			name:     "Basic auth",
			input:    "Basic dXNlcjpwYXNz",
			expected: "<redacted>",
		},
		{
			name:     "Custom auth",
			input:    "CustomScheme token123",
			expected: "<redacted>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("Authorization", tt.input)

			sanitized := SanitizeHeaders(headers)

			assert.Equal(t, tt.expected, sanitized["Authorization"])
		})
	}
}

func TestSanitizeHeaders_PreservesOtherHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Custom-Header", "custom-value")
	headers.Set("Authorization", "Bearer secret-token")

	sanitized := SanitizeHeaders(headers)

	assert.Equal(t, "application/json", sanitized["Content-Type"])
	assert.Equal(t, "custom-value", sanitized["X-Custom-Header"])
	assert.Equal(t, "Bearer <redacted>", sanitized["Authorization"])
}

func TestSanitizeHeaders_EmptyHeaders(t *testing.T) {
	headers := http.Header{}
	sanitized := SanitizeHeaders(headers)

	assert.Empty(t, sanitized)
}
```

**Validation**:
```bash
cd backend
just test
```

---

### Task 12: Final validation and integration test
**Action**: Manual testing with Docker
**Pattern**: Verify logs appear correctly in Docker environment

**Steps**:
1. Build and run backend in Docker:
```bash
cd backend
just build
```

2. Test with different environments:
```bash
# Dev environment (console output)
ENVIRONMENT=dev go run main.go

# Staging environment (JSON output)
ENVIRONMENT=staging go run main.go

# Prod environment (JSON output, warn level)
ENVIRONMENT=prod go run main.go
```

3. Make test HTTP requests and verify logs:
```bash
curl -H "Authorization: Bearer test-token" http://localhost:8080/api/health
```

4. Verify Authorization header is redacted in logs
5. Verify request_id appears in all log lines
6. Verify JSON format in staging/prod, console in dev

**Validation**:
```bash
cd backend
just validate  # Run all validation gates
```

---

## Risk Assessment

**Risk**: Zerolog API differs from slog - need to update all log call sites
**Mitigation**: Complete inventory of slog usage in main.go and middleware. Update systematically.

**Risk**: RequestID middleware may not expose context key publicly
**Mitigation**: Already verified - `middleware.GetRequestID(ctx)` is exported (line 164-170)

**Risk**: Tests may fail if environment variables pollute test state
**Mitigation**: Use `defer os.Unsetenv()` in all tests that set env vars

**Risk**: Missing fmt import in config.go for Sscanf
**Mitigation**: Add `"fmt"` to imports in Task 2

## Integration Points

- **Middleware chain**: Insert logging middleware after RequestID, before Recovery
- **Recovery middleware**: Update to use zerolog instead of slog
- **Main.go**: Replace slog initialization with zerolog
- **Environment variables**: Read from existing env var patterns

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
```bash
cd backend
just lint       # Gate 1: Syntax & Style
just test       # Gate 2: Unit Tests (no typecheck for Go)
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

**After each task**:
```bash
cd backend
just lint
just test
```

**Final validation (Task 12)**:
```bash
cd backend
just validate   # Runs lint + test + build
```

**Manual verification**:
- Run with `ENVIRONMENT=dev` - see colored console logs
- Run with `ENVIRONMENT=prod` - see JSON logs
- Make HTTP request - verify Authorization header redacted
- Check logs contain request_id field

## Plan Quality Assessment

**Complexity Score**: 7/10 (HIGH - AT THRESHOLD, but manageable for MVP scope)

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec (Phases 1+2 well-defined)
✅ Similar patterns found in codebase at `backend/internal/middleware/middleware.go`
✅ All clarifying questions answered
✅ Existing test patterns to follow at `backend/internal/middleware/middleware_test.go`
✅ RequestID middleware already exists - just need to integrate
✅ Zerolog is well-documented and mature
✅ Testify already in dependencies - no new test framework needed
⚠️ Need to update multiple log call sites in main.go (minor risk)
⚠️ Environment detection logic is custom (no existing reference)

**Assessment**: High confidence implementation. Clear path forward with existing patterns to follow. Main complexity is in environment-aware configuration, which is well-specified. Reusing existing RequestID middleware eliminates significant complexity.

**Estimated one-pass success probability**: 85%

**Reasoning**:
- Zerolog is straightforward to integrate
- Middleware pattern already established in codebase
- Tests can use testify/assert (already available)
- Main risk is finding all slog call sites to update, but codebase is small
- Environment detection is custom logic but well-specified in spec
- Minimal sanitization (just Authorization header) reduces scope significantly
