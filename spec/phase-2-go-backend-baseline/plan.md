# Implementation Plan: Phase 2A - Go Backend Core

Generated: 2025-10-17
Specification: spec.md
Phase: 2A of 2 (Core Go server, Docker integration deferred to Phase 2B)

## Understanding

Create a minimal, production-ready Go HTTP server with three Kubernetes-ready health endpoints. This establishes the backend foundation following 12-factor app principles and Go idioms. The server uses stdlib only (no frameworks), implements graceful shutdown, structured logging with slog, and is fully tested.

**Phase Split Rationale**: Validating Go implementation in isolation before adding Docker complexity. Clear test→build→run→curl validation workflow. Can deploy to Railway immediately (Railway supports Go natively).

## Relevant Files

**Reference Patterns** (none - first Go code in monorepo):
- No existing Go patterns to follow
- Will establish patterns for future backend work
- Following Go stdlib idioms and 12-factor principles

**Files to Create**:
- `backend/main.go` - HTTP server, graceful shutdown, logging middleware, signal handling
- `backend/health.go` - Three health handlers (/healthz, /readyz, /health)
- `backend/health_test.go` - Comprehensive test coverage for all endpoints
- `backend/go.mod` - Module definition and dependencies
- `backend/go.sum` - Dependency checksums (auto-generated)
- `backend/server` - Compiled binary (gitignored, created by build)

**Files to Modify**:
- `justfile` (lines ~8-20) - Update backend commands to match actual implementation
  - Modify `backend-build` to use correct output path and version injection
  - Add `backend-run` command
  - Keep existing lint/test commands (already correct)

## Architecture Impact

- **Subsystems affected**: Backend (new), Development tooling (justfile)
- **New dependencies**: None (stdlib only)
- **Breaking changes**: None (new code)
- **12-factor compliance**: ENV vars (PORT), logging to stdout, stateless, graceful shutdown

## Task Breakdown

### Task 1: Initialize Go Module
**File**: `backend/go.mod`
**Action**: CREATE

**Implementation**:
```bash
cd backend
go mod init github.com/trakrf/platform/backend
```

**Creates**:
- go.mod with module path
- Empty dependencies list (stdlib only for Phase 2A)

**Validation**:
```bash
# Verify go.mod exists and is valid
test -f backend/go.mod && echo "✅ go.mod created"
cd backend && go list -m && echo "✅ Module path valid"
```

**Expected**: Module github.com/trakrf/platform/backend created

---

### Task 2: Implement Main Server
**File**: `backend/main.go`
**Action**: CREATE

**Pattern**: Go stdlib HTTP server with graceful shutdown

**Implementation**:
```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
)

var version = "dev" // Overridden at build time via -ldflags

func main() {
    // Setup structured JSON logging to stdout (12-factor)
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    slog.SetDefault(logger)

    // Config from environment (12-factor)
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // Setup routes
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", healthzHandler)  // K8s liveness
    mux.HandleFunc("/readyz", readyzHandler)    // K8s readiness
    mux.HandleFunc("/health", healthHandler)    // Human-friendly

    // HTTP server with timeouts
    server := &http.Server{
        Addr:         ":" + port,
        Handler:      loggingMiddleware(mux),
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
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        slog.Error("Shutdown error", "error", err)
    }

    slog.Info("Server stopped")
}

// loggingMiddleware wraps handler with request logging
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        slog.Info("Request",
            "method", r.Method,
            "path", r.URL.Path,
            "duration", time.Since(start),
        )
    })
}
```

**Key Decisions**:
- Use `slog` (Go 1.21+) for structured logging - stdlib, production-ready
- JSON output to stdout - 12-factor, works with Railway/K8s/docker logs
- Graceful shutdown with 30s timeout - K8s best practice
- Timeouts on server - prevent resource exhaustion
- Logging middleware - observability from day 1

**Validation**:
```bash
cd backend && go fmt ./...  # Format
cd backend && go vet ./...  # Lint
cd backend && go build .    # Compile check
```

**Expected**: Clean compilation, no errors

---

### Task 3: Implement Health Handlers
**File**: `backend/health.go`
**Action**: CREATE

**Pattern**: K8s-ready health checks (liveness, readiness, detailed status)

**Implementation**:
```go
package main

import (
    "encoding/json"
    "net/http"
    "time"
)

// healthzHandler - K8s liveness probe
// Returns 200 if process is alive
// K8s will restart pod if this fails
func healthzHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

// readyzHandler - K8s readiness probe
// Returns 200 if ready to serve traffic
// K8s will remove from service if this fails
// Phase 2A: Simple check (no dependencies yet)
// Phase 3: Will add db.Ping() check
func readyzHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // TODO Phase 3: Add database connectivity check
    // if err := db.Ping(r.Context()); err != nil {
    //     w.WriteHeader(http.StatusServiceUnavailable)
    //     w.Write([]byte("database unavailable"))
    //     return
    // }

    w.Header().Set("Content-Type", "text/plain; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

// HealthResponse - JSON response for /health endpoint
type HealthResponse struct {
    Status    string    `json:"status"`
    Version   string    `json:"version"`
    Timestamp time.Time `json:"timestamp"`
    // Phase 3 additions:
    // Database string `json:"database,omitempty"`
    // Uptime   string `json:"uptime,omitempty"`
}

// healthHandler - Human-friendly health check with details
// Returns JSON with status, version, timestamp
// Used by humans, dashboards, monitoring tools
func healthHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    resp := HealthResponse{
        Status:    "ok",
        Version:   version,
        Timestamp: time.Now().UTC(),
    }

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}
```

**Key Decisions**:
- Separate liveness vs readiness - K8s best practice
- Plaintext for probes, JSON for human endpoint - convention
- Explicit Content-Type headers - proper HTTP
- Method checking - defense in depth
- TODO comments for Phase 3 - clear extension points

**Validation**:
```bash
cd backend && go fmt ./...
cd backend && go vet ./...
cd backend && go build .
```

**Expected**: Clean compilation, no errors

---

### Task 4: Implement Comprehensive Tests
**File**: `backend/health_test.go`
**Action**: CREATE

**Pattern**: Table-driven tests (Go idiom), httptest for HTTP handlers

**Implementation**:
```go
package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthzHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
        wantBody   string
    }{
        {
            name:       "GET returns 200 ok",
            method:     "GET",
            wantStatus: http.StatusOK,
            wantBody:   "ok",
        },
        {
            name:       "POST returns 405",
            method:     "POST",
            wantStatus: http.StatusMethodNotAllowed,
            wantBody:   "",
        },
        {
            name:       "PUT returns 405",
            method:     "PUT",
            wantStatus: http.StatusMethodNotAllowed,
            wantBody:   "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/healthz", nil)
            w := httptest.NewRecorder()

            healthzHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
            }

            if tt.wantBody != "" && w.Body.String() != tt.wantBody {
                t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
            }
        })
    }
}

func TestReadyzHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
        wantBody   string
    }{
        {
            name:       "GET returns 200 ok",
            method:     "GET",
            wantStatus: http.StatusOK,
            wantBody:   "ok",
        },
        {
            name:       "POST returns 405",
            method:     "POST",
            wantStatus: http.StatusMethodNotAllowed,
            wantBody:   "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/readyz", nil)
            w := httptest.NewRecorder()

            readyzHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
            }

            if tt.wantBody != "" && w.Body.String() != tt.wantBody {
                t.Errorf("body = %q, want %q", w.Body.String(), tt.wantBody)
            }
        })
    }
}

func TestHealthHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
    }{
        {
            name:       "GET returns 200",
            method:     "GET",
            wantStatus: http.StatusOK,
        },
        {
            name:       "POST returns 405",
            method:     "POST",
            wantStatus: http.StatusMethodNotAllowed,
        },
        {
            name:       "DELETE returns 405",
            method:     "DELETE",
            wantStatus: http.StatusMethodNotAllowed,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/health", nil)
            w := httptest.NewRecorder()

            healthHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
            }
        })
    }
}

func TestHealthResponse(t *testing.T) {
    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()

    healthHandler(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
    }

    var resp HealthResponse
    if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
        t.Fatalf("failed to decode JSON: %v", err)
    }

    if resp.Status != "ok" {
        t.Errorf("status = %q, want %q", resp.Status, "ok")
    }

    if resp.Version != version {
        t.Errorf("version = %q, want %q", resp.Version, version)
    }

    if resp.Timestamp.IsZero() {
        t.Error("timestamp is zero")
    }

    // Verify timestamp is recent (within 1 second)
    if time.Since(resp.Timestamp) > time.Second {
        t.Errorf("timestamp too old: %v", resp.Timestamp)
    }
}
```

**Key Decisions**:
- Table-driven tests - Go idiom, easy to extend
- Test method validation for all endpoints - security
- Test JSON decoding for /health - integration check
- Test timestamp validity - catch time zone issues
- Use httptest.ResponseRecorder - stdlib, no mocks needed

**Validation**:
```bash
cd backend && go test -v ./...
cd backend && go test -race ./...  # Race detection
cd backend && go test -cover ./... # Coverage report
```

**Expected**: All tests pass, no races, >90% coverage

---

### Task 5: Update Justfile Commands
**File**: `justfile` (root)
**Action**: MODIFY (lines ~8-20, backend section)

**Implementation**:
Replace existing placeholder backend commands with:

```makefile
# Backend validation commands
backend-lint:
    cd backend && go fmt ./...
    cd backend && go vet ./...

backend-test:
    cd backend && go test -v ./...

backend-build:
    cd backend && go build -ldflags "-X main.version=0.1.0-dev" -o server .

backend-run:
    cd backend && go run .

# Run all backend checks
backend: backend-lint backend-test backend-build
```

**Changes from existing**:
- `backend-build`: Add version injection via ldflags, output to `backend/server`
- `backend-run`: New command for local development
- Keep `backend-lint`, `backend-test`, `backend` as-is (already correct)

**Why these commands**:
- `backend-run` - Fast iteration (go run compiles in memory)
- `backend-build` - Production binary with version
- `backend-lint` - Format + vet (Go standard)
- `backend-test` - Run tests with verbose output
- `backend` - Run all checks (used by CI/validation)

**Validation**:
```bash
just backend-lint   # Should format and vet
just backend-test   # Should run tests
just backend-build  # Should create backend/server
test -f backend/server && echo "✅ Binary created"
just backend-run &  # Should start server
sleep 2
curl localhost:8080/healthz  # Should return "ok"
pkill -f "go run"  # Stop server
```

**Expected**: All commands work, server responds to curl

---

### Task 6: Manual End-to-End Validation
**Action**: VALIDATE (no files created)

**Purpose**: Verify complete workflow before marking phase done

**Test Sequence**:

```bash
# 1. Lint check
just backend-lint
# Expected: "go fmt" and "go vet" complete with no output

# 2. Run tests
just backend-test
# Expected: All tests pass, no failures

# 3. Build binary
just backend-build
# Expected: backend/server created

# 4. Test binary with version injection
./backend/server &
SERVER_PID=$!
sleep 2

# 5. Test all three endpoints
curl -s http://localhost:8080/healthz
# Expected: "ok"

curl -s http://localhost:8080/readyz
# Expected: "ok"

curl -s http://localhost:8080/health | jq .
# Expected: JSON with status="ok", version="0.1.0-dev", timestamp

# 6. Test method validation
curl -X POST http://localhost:8080/healthz
# Expected: 405 Method Not Allowed

# 7. Test graceful shutdown
kill -TERM $SERVER_PID
# Expected: Logs show "Shutting down gracefully..." and "Server stopped"

# 8. Test with custom port
PORT=9000 ./backend/server &
sleep 2
curl http://localhost:9000/healthz
# Expected: "ok"
pkill -f server

# 9. Run full validation suite
just backend
# Expected: All checks pass
```

**Success Criteria**:
- ✅ All tests pass
- ✅ Code formatted and vetted
- ✅ Binary builds with version
- ✅ All three endpoints respond correctly
- ✅ Method validation works (405 on POST)
- ✅ Graceful shutdown logs appear
- ✅ Custom PORT works
- ✅ JSON response valid
- ✅ Timestamp is recent UTC

**If any fail**: Fix immediately, re-run validation

---

## Risk Assessment

**Low Risk** ✅
- **Reason**: Stdlib only, no external dependencies
- **Mitigation**: N/A - minimal complexity

**Medium Risk** ⚠️
- **Risk**: Version injection via ldflags might fail silently
  **Mitigation**: Task 6 validates version in JSON response

- **Risk**: Graceful shutdown might not work on Windows
  **Mitigation**: SIGTERM/SIGINT work on Windows Go runtime

**No High Risks** - This is a straightforward Go HTTP server

## Integration Points

- **Justfile**: Backend commands integrated with existing monorepo tooling
- **Docker**: Ready for Phase 2B (Dockerfile will reference backend/server binary)
- **Railway**: Can deploy immediately (Railway auto-detects Go and runs `go build`)
- **Phase 3**: Database migrations will connect from this server

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are BLOCKING gates, not suggestions.

After EVERY code change:

**Gate 1: Formatting & Linting**
```bash
cd backend && go fmt ./...
cd backend && go vet ./...
```
- ✅ Pass → Continue
- ❌ Fail → Fix and re-run

**Gate 2: Tests**
```bash
cd backend && go test ./...
cd backend && go test -race ./...
```
- ✅ Pass → Continue
- ❌ Fail → Fix tests or code, re-run

**Gate 3: Build**
```bash
cd backend && go build .
```
- ✅ Pass → Continue
- ❌ Fail → Fix compilation error, re-run

**After 3 failures on same gate**: STOP and ask for help

## Validation Sequence

**After each task (1-5)**:
```bash
just backend-lint   # Gate 1
just backend-test   # Gate 2
just backend-build  # Gate 3
```

**Final validation (Task 6)**:
```bash
just backend        # All gates
# Plus manual curl tests
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (MEDIUM-LOW) ✅
- 6 subtasks (well under 13 subtask threshold)
- Single subsystem (backend)
- Zero external dependencies
- Stdlib patterns only

**Confidence Score**: 9/10 (HIGH) ✅

**Confidence Factors**:
✅ Standard Go HTTP server pattern - well documented
✅ Stdlib only - no dependency uncertainty
✅ Table-driven tests - Go community standard
✅ Clear validation gates at each step
✅ Can test incrementally (build → run → curl)
✅ No integration with other subsystems yet
⚠️ First Go code in monorepo - no existing patterns to follow (but establishing them)

**Assessment**: High confidence implementation. Standard Go patterns, stdlib only, clear validation path. The split from Docker reduces complexity significantly. Each task is independently validatable.

**Estimated one-pass success probability**: 90%

**Reasoning**:
- Go stdlib HTTP is battle-tested and well-documented
- No external dependencies = no integration surprises
- Tests validate behavior before manual testing
- Incremental validation catches issues early
- Only risk is typos/syntax, caught by go fmt/vet/build gates
- 10% risk accounts for potential environment issues (Go version, PATH, etc.)

## Phase 2A Definition of Done

### Functional Requirements
- [ ] Go server starts successfully with `just backend-run`
- [ ] GET /healthz returns 200 "ok" (K8s liveness)
- [ ] GET /readyz returns 200 "ok" (K8s readiness)
- [ ] GET /health returns 200 JSON with status, version, timestamp
- [ ] POST to any endpoint returns 405 Method Not Allowed
- [ ] Version "0.1.0-dev" appears in /health response
- [ ] Timestamp in /health is valid UTC ISO 8601
- [ ] Ctrl+C triggers graceful shutdown with logs
- [ ] Custom PORT environment variable works

### Quality Requirements
- [ ] `just backend-lint` passes (gofmt + go vet)
- [ ] `just backend-test` passes (all tests green)
- [ ] `just backend-test -race` passes (no race conditions)
- [ ] `just backend-build` creates backend/server binary
- [ ] Binary runs standalone: `./backend/server`
- [ ] Logs are JSON structured to stdout

### Integration Requirements
- [ ] Justfile commands work from root directory
- [ ] Can deploy to Railway (Go auto-detected)
- [ ] Ready for Phase 2B (Docker can build this)
- [ ] Ready for Phase 3 (can add DB connection)

### Documentation Requirements
- [ ] Code comments explain K8s probe purposes
- [ ] TODO comments mark Phase 3 extension points
- [ ] Justfile commands are clear and follow existing patterns

## Next Steps After Phase 2A

1. **Test Railway deployment** (optional but recommended):
   ```bash
   # Railway will auto-detect Go and build
   railway up
   ```

2. **Begin Phase 2B** (Docker Integration):
   - Dockerfile (production)
   - Dockerfile.dev (hot reload)
   - docker-compose.yml updates
   - Air configuration

3. **Phase 3 Preview** (Database Migrations):
   - Will add db.Ping() to readyzHandler
   - Will add DATABASE_URL env var
   - Backend container → db connectivity validated
