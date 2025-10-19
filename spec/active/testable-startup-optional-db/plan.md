# Implementation Plan: Testable Router Setup + Dev Commands

Generated: 2025-10-19
Specification: spec.md
Phase: 1 of 2 (Testable Router + Dev Workflow)

## Understanding

This phase extracts router setup into a testable function and adds development workflow commands. The backend service will:

1. **Catch route registration panics in tests** - Extract `setupRouter()` from `main()` for unit testing
2. **Verify critical routes exist** - Test that expected routes are registered
3. **Support multiple deployment modes** - Justfile commands for local/container/cloud databases
4. **Catch startup panics in CI** - Smoke test verifies binary starts without panic

**Key design decision:**
- Simple extraction of existing code into testable function
- No behavior changes (pure refactoring)
- Environment config already complete (PG_URL_LOCAL, PG_URL_CLOUD from earlier work)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/health_test.go:11-54` - Table-driven test pattern with httptest
- `backend/middleware_test.go` - Testing middleware in isolation
- `backend/justfile:10-14` - Existing dev command pattern

**Files to Modify**:
- `backend/main.go:52-107` - Extract router setup into `setupRouter()`
- `backend/justfile` - Add dev, dev-cloud, smoke-test commands

**Files to Create**:
- `backend/main_test.go` - New unit tests for router setup

## Architecture Impact

- **Subsystems affected**: HTTP routing, Development workflow
- **New dependencies**: None
- **Breaking changes**: None (pure refactoring)

## Task Breakdown

### Task 1: Extract setupRouter() Function
**File**: `backend/main.go`
**Action**: MODIFY (refactor)
**Pattern**: Extract method refactoring

**Implementation**:

Add after imports, before `main()`:
```go
// setupRouter creates and configures the chi router with all middleware and routes
// Extracted for testability - panics during route registration are caught by tests
func setupRouter() *chi.Mux {
    r := chi.NewRouter()

    // Apply middleware stack
    r.Use(requestIDMiddleware)
    r.Use(recoveryMiddleware)
    r.Use(corsMiddleware)
    r.Use(contentTypeMiddleware)

    // ========================================================================
    // Frontend & Static Asset Routes
    // ========================================================================
    // IMPORTANT: Static assets must be registered BEFORE API routes to prevent
    // the catch-all SPA handler from intercepting API requests

    frontendHandler := serveFrontend()

    // Static assets (public, no auth required)
    // These are served directly from the embedded filesystem with long cache TTLs
    r.Handle("/assets/*", frontendHandler)
    r.Handle("/favicon.ico", frontendHandler)
    r.Handle("/icon-*.png", frontendHandler) // All icon sizes
    r.Handle("/logo.png", frontendHandler)
    r.Handle("/manifest.json", frontendHandler)
    r.Handle("/og-image.png", frontendHandler)

    // ========================================================================
    // Health Check Routes (K8s liveness/readiness)
    // ========================================================================
    r.Get("/healthz", healthzHandler)
    r.Get("/readyz", readyzHandler)
    r.Get("/health", healthHandler)

    // Register API routes
    // Public endpoints (no auth required)
    registerAuthRoutes(r) // POST /api/v1/auth/signup, /api/v1/auth/login

    // Protected endpoints (require valid JWT)
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware) // Apply auth middleware to this group

        registerAccountRoutes(r)     // All /api/v1/accounts/* routes
        registerUserRoutes(r)        // All /api/v1/users/* routes
        registerAccountUserRoutes(r) // All /api/v1/account_users/* routes
    })

    // ========================================================================
    // SPA Catch-All Handler (must be LAST)
    // ========================================================================
    // Serve index.html for all remaining routes to enable React Router
    // React will handle:
    //   - Public routes: /, /login, /register (inventory without auth)
    //   - Protected routes: /dashboard, /assets, /settings (redirects to login)
    r.HandleFunc("/*", spaHandler)

    return r
}
```

Modify `main()` to use `setupRouter()` (replace lines 52-107):
```go
func main() {
    startTime = time.Now()
    // Setup structured JSON logging to stdout (12-factor)
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    slog.SetDefault(logger)

    // Config from environment (12-factor)
    port := os.Getenv("BACKEND_PORT")
    if port == "" {
        port = "8080"
    }

    // Initialize database connection pool
    ctx := context.Background()
    if err := initDB(ctx); err != nil {
        slog.Error("Failed to initialize database", "error", err)
        os.Exit(1)
    }
    slog.Info("Database connection pool initialized")

    // Initialize repositories
    initAccountRepo()
    initUserRepo()
    initAccountUserRepo()
    slog.Info("Repositories initialized")

    // Initialize authentication service
    initAuthService()
    slog.Info("Auth service initialized")

    // Setup chi router (extracted for testability)
    r := setupRouter()
    slog.Info("Routes registered")

    // HTTP server with timeouts
    server := &http.Server{
        Addr:         ":" + port,
        Handler:      r,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    // Start server in goroutine
    go func() {
        slog.Info("Server starting", "port", port, "version", version)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("Server failed", "error", err)
            os.Exit(1)
        }
    }()

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    // Graceful shutdown (Railway/K8s requirement)
    slog.Info("Shutting down gracefully...")
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        slog.Error("Shutdown error", "error", err)
    }

    // Close database connection pool
    closeDB()
    slog.Info("Database connection pool closed")

    slog.Info("Server stopped")
}
```

**Validation**:
```bash
cd backend
just lint     # Verify formatting
go build .    # Verify compiles
```

---

### Task 2: Create Router Unit Tests
**File**: `backend/main_test.go`
**Action**: CREATE
**Pattern**: Reference health_test.go table-driven tests

**Implementation**:
```go
package main

import (
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
)

// TestRouterSetup verifies router can be created without panic
// This catches route registration errors (e.g., invalid wildcard patterns)
// before they reach production
func TestRouterSetup(t *testing.T) {
    // This will panic if route registration fails
    // (e.g., "wildcard '*' must be the last value in a route")
    defer func() {
        if r := recover(); r != nil {
            t.Fatalf("setupRouter panicked: %v", r)
        }
    }()

    r := setupRouter()

    if r == nil {
        t.Fatal("setupRouter returned nil")
    }
}

// TestRouterRegistration verifies critical routes are registered
func TestRouterRegistration(t *testing.T) {
    r := setupRouter()

    tests := []struct {
        method string
        path   string
    }{
        // Health endpoints
        {"GET", "/healthz"},
        {"GET", "/readyz"},
        {"GET", "/health"},

        // Auth endpoints
        {"POST", "/api/v1/auth/signup"},
        {"POST", "/api/v1/auth/login"},

        // Protected endpoints (will fail auth, but route should exist)
        {"GET", "/api/v1/accounts"},
        {"GET", "/api/v1/users"},

        // Frontend routes
        {"GET", "/assets/index.js"},
        {"GET", "/favicon.ico"},
        {"GET", "/"},
    }

    for _, tt := range tests {
        t.Run(tt.method+" "+tt.path, func(t *testing.T) {
            rctx := chi.NewRouteContext()
            req := httptest.NewRequest(tt.method, tt.path, nil)

            if !r.Match(rctx, tt.method, tt.path) {
                t.Errorf("Route not found: %s %s", tt.method, tt.path)
            }
        })
    }
}
```

**Validation**:
```bash
cd backend
just lint
just test     # Should pass with new tests
```

---

### Task 3: Add Development Workflow Commands
**File**: `backend/justfile`
**Action**: MODIFY
**Pattern**: Reference existing dev command

**Implementation**:

Update `dev` recipe to use PG_URL_LOCAL:
```just
# Start Go development server (local mode: backend on host → postgres in docker)
dev:
    @echo "🚀 Backend on host → postgres in docker (localhost:5432)"
    @if [ -z "$$PG_URL_LOCAL" ]; then \
        echo "⚠️  PG_URL_LOCAL not set - using PG_URL"; \
        go run .; \
    else \
        PG_URL="$$PG_URL_LOCAL" go run .; \
    fi
```

Add `dev-cloud` command:
```just
# Cloud development (backend on host → postgres in cloud)
dev-cloud:
    @echo "☁️  Backend on host → cloud postgres"
    @if [ -z "$$PG_URL_CLOUD" ]; then \
        echo "❌ PG_URL_CLOUD not set in .env.local"; \
        exit 1; \
    fi
    PG_URL="$$PG_URL_CLOUD" go run .
```

Add smoke test:
```just
# Smoke test - verifies binary starts without panic
smoke-test:
    @echo "🔥 Running smoke test..."
    @just build
    @echo "Starting server with 5s timeout..."
    @timeout 5s ./bin/trakrf > /tmp/trakrf-smoke.log 2>&1 & SERVER_PID=$$! ; \
    sleep 2 ; \
    echo "Testing /healthz endpoint..." ; \
    curl -f http://localhost:8080/healthz > /dev/null 2>&1 || (echo "❌ Smoke test failed" && cat /tmp/trakrf-smoke.log && exit 1) ; \
    kill $$SERVER_PID 2>/dev/null || true ; \
    echo "✅ Smoke test passed"
```

Update validate to include smoke test:
```just
# Run all backend validation checks
validate: lint test build smoke-test
```

Full updated justfile:
```just
# Backend Task Runner (TrakRF Platform)
# Uses fallback to inherit shared recipes from root justfile
set fallback := true

# List all available recipes
default:
    @just --list

# Start Go development server (local mode: backend on host → postgres in docker)
dev:
    @echo "🚀 Backend on host → postgres in docker (localhost:5432)"
    @if [ -z "$$PG_URL_LOCAL" ]; then \
        echo "⚠️  PG_URL_LOCAL not set - using PG_URL"; \
        go run .; \
    else \
        PG_URL="$$PG_URL_LOCAL" go run .; \
    fi

# Cloud development (backend on host → postgres in cloud)
dev-cloud:
    @echo "☁️  Backend on host → cloud postgres"
    @if [ -z "$$PG_URL_CLOUD" ]; then \
        echo "❌ PG_URL_CLOUD not set in .env.local"; \
        exit 1; \
    fi
    PG_URL="$$PG_URL_CLOUD" go run .

# Alias for consistency
run: dev

# Lint Go code (formatting + static analysis)
lint:
    go fmt ./...
    go vet ./...

# Run backend tests with verbose output
test:
    go test -v ./...

# Run tests with race detection
test-race:
    go test -race ./...

# Run tests with coverage report
test-coverage:
    go test -cover ./...

# Build backend binary with version injection
build:
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .

# Smoke test - verifies binary starts without panic
smoke-test:
    @echo "🔥 Running smoke test..."
    @just build
    @echo "Starting server with 5s timeout..."
    @timeout 5s ./bin/trakrf > /tmp/trakrf-smoke.log 2>&1 & SERVER_PID=$$! ; \
    sleep 2 ; \
    echo "Testing /healthz endpoint..." ; \
    curl -f http://localhost:8080/healthz > /dev/null 2>&1 || (echo "❌ Smoke test failed" && cat /tmp/trakrf-smoke.log && exit 1) ; \
    kill $$SERVER_PID 2>/dev/null || true ; \
    echo "✅ Smoke test passed"

# Run all backend validation checks
validate: lint test build smoke-test

# Shell access
shell:
    docker compose exec backend sh
```

**Validation**:
```bash
cd backend
just lint
just --list    # Verify new commands appear
```

---

## Risk Assessment

**Risk: Tests don't catch actual route panics**
- **Mitigation**: TestRouterSetup specifically defers recover() to catch panics. Smoke test verifies binary starts.

**Risk: PG_URL_LOCAL not set in environment**
- **Mitigation**: Dev command checks for PG_URL_LOCAL and falls back to PG_URL with warning.

**Risk: Smoke test leaves server running**
- **Mitigation**: Using timeout + explicit kill with SERVER_PID. Cleanup even on failure.

## Integration Points

**Justfile commands:**
- `just backend dev` - Uses PG_URL_LOCAL (host → docker)
- `just backend dev-cloud` - Uses PG_URL_CLOUD (host → cloud)
- `just backend validate` - Now includes smoke test

**Environment variables (already complete):**
- `PG_URL` - Container-to-container (docker-compose)
- `PG_URL_LOCAL` - Host-to-container (bash substitution from PG_URL)
- `PG_URL_CLOUD` - Host-to-cloud (explicit)

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are blocking requirements. If any gate fails → Fix immediately.

After EVERY code change:

**Gate 1: Lint & Format**
```bash
cd backend
just lint
```
Must pass with zero errors.

**Gate 2: Build**
```bash
cd backend
just build
```
Must compile successfully.

**Gate 3: Unit Tests**
```bash
cd backend
just test
```
All tests must pass (existing + new router tests).

**Gate 4: Smoke Test**
```bash
cd backend
just smoke-test
```
Binary must start and respond to /healthz.

**Enforcement**: After 3 failed attempts on any gate → Stop and ask for help.

## Validation Sequence

**After Task 1 (extract setupRouter):**
```bash
cd backend
just lint
go build .      # Verify compiles
```

**After Task 2 (router tests):**
```bash
cd backend
just lint
just test       # Verify new tests pass
```

**After Task 3 (justfile commands):**
```bash
cd backend
just lint
just --list     # Verify commands listed

# Test dev command
just dev &
sleep 2
curl http://localhost:8080/healthz
killall trakrf

# Test smoke test
just smoke-test
```

**Final validation:**
```bash
cd backend
just validate   # Runs lint + test + build + smoke-test

# From project root
just backend validate
```

**Manual verification:**
```bash
# Verify deployment modes work
just backend dev          # Local mode
just backend dev-cloud    # Cloud mode (if PG_URL_CLOUD set)
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)

**Confidence Score**: 9/10 (VERY HIGH)

**Confidence Factors**:
✅ Pure refactoring (extract method pattern)
✅ Simple unit tests following existing patterns
✅ Justfile commands mirror existing structure
✅ No behavior changes (non-breaking)
✅ Environment config already complete
✅ All design decisions validated with user
✅ Standard Go testing practices
✅ Clear validation sequence

**Assessment**: Very high confidence. Straightforward refactoring with well-established patterns. Router extraction is mechanical, tests are simple, justfile follows existing conventions.

**Estimated one-pass success probability**: 95%

**Reasoning**:
- Simple task breakdown (3 tasks, 2 file modifications, 1 new file)
- No architectural changes
- Following existing codebase patterns exactly
- Comprehensive validation gates
- Zero new dependencies
- User confirmed simplified approach eliminates speculative complexity

**Potential friction points**:
1. Smoke test timeout might need adjustment (2s vs 5s)
2. PG_URL_LOCAL check might need refinement for error messages

**Mitigation**: Validation gates after each task catch issues immediately. Total implementation time < 30 minutes.
