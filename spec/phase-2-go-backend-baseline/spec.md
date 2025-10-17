# Phase 2: Go Backend Baseline

**Linear Issue**: TRA-76
**Status**: In Progress
**Version Target**: 0.1.0-dev

## Objective

Create a minimal viable Go HTTP server that proves the backend infrastructure works. Health check endpoint demonstrates the server is running and responds correctly.

## Success Criteria

- [ ] Go HTTP server starts and listens on configurable port
- [ ] Three health endpoints (K8s ready): `/healthz`, `/readyz`, `/health`
- [ ] Version string (0.1.0-dev) embedded at build time
- [ ] Graceful shutdown on SIGTERM/SIGINT
- [ ] Structured logging with slog (JSON to stdout)
- [ ] 12-factor app compliant (env vars, stateless, etc)
- [ ] Just commands for run/build/test/lint/docker
- [ ] Unit tests for all three health endpoints
- [ ] Server runs locally and in Docker
- [ ] Dockerfile (production) and Dockerfile.dev (hot reload)
- [ ] Backend service added to docker-compose.yml
- [ ] Hot reload working in Docker with air

## Architecture Decisions

### 12-Factor App Compliance

**Why**: 12-factor principles ensure the backend works seamlessly across local docker-compose, Railway, and GKE without modification.

| Factor | Implementation | Status |
|--------|---------------|--------|
| **I. Codebase** | Monorepo, git-tracked | ✅ Phase 0 |
| **II. Dependencies** | go.mod/go.sum explicit deps | ✅ Phase 2 |
| **III. Config** | Environment variables only | ✅ Phase 2 |
| **IV. Backing services** | DATABASE_URL, MQTT via env | ✅ Phase 3 |
| **V. Build/release/run** | Multi-stage Dockerfile | ✅ Phase 2 |
| **VI. Processes** | Stateless, no shared state | ✅ Phase 2 |
| **VII. Port binding** | Self-contained HTTP server | ✅ Phase 2 |
| **VIII. Concurrency** | Horizontal scaling ready | ✅ Phase 2 |
| **IX. Disposability** | Fast startup, graceful shutdown | ✅ Phase 2 |
| **X. Dev/prod parity** | Docker for both environments | ✅ Phase 2 |
| **XI. Logs** | Stdout/stderr streams (slog) | ✅ Phase 2 |
| **XII. Admin processes** | Migrations as one-off tasks | ✅ Phase 3 |

**Key Points**:
- No config files (violates factor III)
- No local filesystem writes except logs to stdout
- No in-memory sessions (violates factor VI)
- Database connection via DATABASE_URL (factor IV)
- Migrations via separate `migrate` command (factor XII)

### Project Structure

**Decision**: Start simple, optimize later

```
backend/
├── main.go              # Entry point, server setup
├── health.go            # Health check handler
├── health_test.go       # Tests
├── go.mod               # Dependencies
└── go.sum               # Checksums
```

**Rationale**:
- No users = can refactor easily
- Avoid premature abstraction
- Move to `cmd/server/` + `internal/` when complexity demands it

### HTTP Library

**Decision**: Use stdlib `net/http` only

**Rationale**:
- Phase 4 will choose API framework
- Don't pre-optimize
- Stdlib is production-ready for basic server

### Health Check Endpoints (K8s Ready)

**Why Three Endpoints**: Kubernetes distinguishes between liveness (restart), readiness (traffic routing), and human-friendly health checks.

#### `/healthz` - Liveness Probe

**Purpose**: Kubernetes uses this to know if the process should be restarted.

**Response**: Simple plaintext
```
ok
```

**Logic**: Process is alive? Return 200. Always returns 200 unless process is completely broken.

**Status Codes**:
- `200 OK` - Process is alive

#### `/readyz` - Readiness Probe

**Purpose**: Kubernetes uses this to know if the pod should receive traffic.

**Response**: Simple plaintext
```
ok
```

**Logic Phase 2**: Always return 200 (no dependencies yet)

**Logic Phase 3**: Check database connectivity
```go
if err := db.Ping(); err != nil {
    return 503 // Remove from load balancer
}
return 200 // Ready to serve traffic
```

**Status Codes**:
- `200 OK` - Ready to serve traffic
- `503 Service Unavailable` - Not ready (database down, etc)

#### `/health` - Human-Friendly Status

**Purpose**: Detailed JSON response for humans, dashboards, monitoring.

**Response Format**:
```json
{
  "status": "ok",
  "version": "0.1.0-dev",
  "timestamp": "2025-10-17T14:45:00Z"
}
```

**Phase 3 Enhancement**:
```json
{
  "status": "ok",
  "version": "0.1.0-dev",
  "timestamp": "2025-10-17T14:45:00Z",
  "database": "connected",
  "uptime": "1h23m45s"
}
```

**Status Codes**:
- `200 OK` - All systems operational
- `503 Service Unavailable` - Degraded (Phase 3+)

### Version Management

**Build-Time Injection**:
```bash
go build -ldflags "-X main.version=0.1.0-dev" -o bin/server backend/main.go
```

**Variable Declaration**:
```go
var version = "dev" // overridden at build time
```

### Configuration

**Environment Variables**:
- `PORT` - HTTP listen port (default: 8080)
- `LOG_LEVEL` - Logging level (default: info)

**No Config Files**:
- Keep it simple
- 12-factor app principles
- Config files in Phase 4+ if needed

### Logging

**Library**: `log/slog` (Go 1.21+ stdlib)

**Format**: JSON (production-ready)

**Request Logging**:
```
INFO: Server starting port=8080 version=0.1.0-dev
INFO: Request method=GET path=/health status=200 duration=1ms
INFO: Shutting down gracefully...
INFO: Server stopped
```

### Graceful Shutdown

**Signal Handling**:
- Catch SIGTERM (Railway/Kubernetes)
- Catch SIGINT (Ctrl+C local dev)

**Shutdown Process**:
1. Stop accepting new connections
2. Wait for in-flight requests (max 30s)
3. Close server
4. Log completion

**Critical For**:
- Railway deployments
- Zero-downtime deployments
- Proper resource cleanup

### GKE/Kubernetes Migration Patterns

**Why Plan Now**: These patterns cost ~50 lines of code but enable smooth GKE migration when scaling to 100k+ MRR.

#### Self-Healing (Implemented in Phase 2)

✅ **Liveness probe** (`/healthz`): K8s restarts pod if unhealthy
✅ **Readiness probe** (`/readyz`): K8s removes from service if not ready
✅ **Graceful shutdown**: Drains requests before stopping
✅ **Fast startup**: Minimal initialization time

#### Future K8s Considerations

**When Migrating to GKE** (post-100k MRR):

**Deployment Manifest** (example for Phase 7+):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: trakrf-backend
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: backend
        image: backend:0.1.0
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
        lifecycle:
          preStop:
            exec:
              command: ["/bin/sh", "-c", "sleep 5"]
```

**MQTT Scaling Considerations**:

MQTT requires sticky connections, which affects K8s architecture:

**Option A (Simpler)**: StatefulSet + single MQTT pod
- Good until: ~10k concurrent MQTT connections
- Tradeoff: Single point of failure for MQTT (but HTTP scales)

**Option B (Scale)**: Separate MQTT ingestion service
- MQTT ingestion → Message queue → Stateless HTTP workers
- Horizontal scaling for HTTP/API
- Single or replicated MQTT pods with sticky routing

**Current Phase 2 Decision**: Monolith is correct until 100k MRR. These patterns enable both options without refactor.

## Implementation Plan

### Step 1: Initialize Go Module

```bash
cd backend
go mod init github.com/trakrf/platform/backend
```

### Step 2: Implement Main Server

**File**: `backend/main.go`

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

var version = "dev" // injected at build time

func main() {
    // Setup structured logging
    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))
    slog.SetDefault(logger)

    // Get port from env
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // Setup HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", healthzHandler)  // K8s liveness probe
    mux.HandleFunc("/readyz", readyzHandler)    // K8s readiness probe
    mux.HandleFunc("/health", healthHandler)    // Human-friendly status

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

    // Graceful shutdown
    slog.Info("Shutting down gracefully...")
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        slog.Error("Shutdown error", "error", err)
    }

    slog.Info("Server stopped")
}

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

### Step 3: Implement Health Handlers (K8s Ready)

**File**: `backend/health.go`

```go
package main

import (
    "encoding/json"
    "net/http"
    "time"
)

// healthzHandler - Liveness probe for Kubernetes
// Returns 200 if process is alive
// K8s will restart pod if this fails
func healthzHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

// readyzHandler - Readiness probe for Kubernetes
// Returns 200 if ready to serve traffic
// K8s will remove from service if this fails
// Phase 2: Simple check (no dependencies)
// Phase 3: Add db.Ping() check
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

    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

// HealthResponse - JSON response for human-friendly health endpoint
type HealthResponse struct {
    Status    string    `json:"status"`
    Version   string    `json:"version"`
    Timestamp time.Time `json:"timestamp"`
    // Phase 3: Add Database string `json:"database,omitempty"`
    // Phase 3: Add Uptime   string `json:"uptime,omitempty"`
}

// healthHandler - Human-friendly health check with details
// Returns JSON with status, version, timestamp
// Phase 3: Will add database status and uptime
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

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}
```

### Step 4: Write Tests

**File**: `backend/health_test.go`

```go
package main

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

// TestHealthzHandler - Test K8s liveness probe
func TestHealthzHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
        wantBody   string
    }{
        {"GET returns 200 ok", "GET", http.StatusOK, "ok"},
        {"POST returns 405", "POST", http.StatusMethodNotAllowed, ""},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/healthz", nil)
            w := httptest.NewRecorder()

            healthzHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
            }

            if tt.wantBody != "" && w.Body.String() != tt.wantBody {
                t.Errorf("got body %q, want %q", w.Body.String(), tt.wantBody)
            }
        })
    }
}

// TestReadyzHandler - Test K8s readiness probe
func TestReadyzHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
        wantBody   string
    }{
        {"GET returns 200 ok", "GET", http.StatusOK, "ok"},
        {"POST returns 405", "POST", http.StatusMethodNotAllowed, ""},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/readyz", nil)
            w := httptest.NewRecorder()

            readyzHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
            }

            if tt.wantBody != "" && w.Body.String() != tt.wantBody {
                t.Errorf("got body %q, want %q", w.Body.String(), tt.wantBody)
            }
        })
    }
}

// TestHealthHandler - Test human-friendly health endpoint
func TestHealthHandler(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        wantStatus int
    }{
        {"GET returns 200", "GET", http.StatusOK},
        {"POST returns 405", "POST", http.StatusMethodNotAllowed},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest(tt.method, "/health", nil)
            w := httptest.NewRecorder()

            healthHandler(w, req)

            if w.Code != tt.wantStatus {
                t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
            }
        })
    }
}

// TestHealthResponse - Test JSON structure of /health endpoint
func TestHealthResponse(t *testing.T) {
    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()

    healthHandler(w, req)

    var resp HealthResponse
    if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
        t.Fatalf("failed to decode response: %v", err)
    }

    if resp.Status != "ok" {
        t.Errorf("got status %q, want %q", resp.Status, "ok")
    }

    if resp.Version != version {
        t.Errorf("got version %q, want %q", resp.Version, version)
    }

    if resp.Timestamp.IsZero() {
        t.Error("timestamp is zero")
    }
}
```

### Step 5: Add Just Commands

**File**: `justfile` (add to root)

```makefile
# Backend commands
backend-run:
    cd backend && go run .

backend-build:
    go build -ldflags "-X main.version=0.1.0-dev" -o bin/server backend/main.go

backend-test:
    cd backend && go test -v ./...

backend-lint:
    cd backend && golangci-lint run

backend-fmt:
    cd backend && go fmt ./...

# Alias for convenience
run: backend-run
build: backend-build
test: backend-test
```

### Step 6: Manual Testing (Local)

```bash
# Terminal 1: Start server
just backend-run

# Terminal 2: Test all health endpoints

# K8s liveness probe
curl http://localhost:8080/healthz
# Expected: ok

# K8s readiness probe
curl http://localhost:8080/readyz
# Expected: ok

# Human-friendly health check
curl http://localhost:8080/health
# Expected response:
{
  "status": "ok",
  "version": "0.1.0-dev",
  "timestamp": "2025-10-17T14:45:00Z"
}

# Test graceful shutdown (Terminal 1: Ctrl+C)
# Should see:
# INFO: Shutting down gracefully...
# INFO: Server stopped
```

## Testing Checklist

### Local Testing
- [ ] Server starts without errors (`just backend-run`)
- [ ] `/healthz` endpoint returns 200 OK with "ok"
- [ ] `/readyz` endpoint returns 200 OK with "ok"
- [ ] `/health` endpoint returns 200 OK with JSON
- [ ] Health response includes status, version, timestamp
- [ ] POST to health endpoints returns 405 Method Not Allowed
- [ ] Version appears in /health response
- [ ] Timestamp is valid ISO 8601
- [ ] Ctrl+C triggers graceful shutdown
- [ ] Shutdown log messages appear
- [ ] All unit tests pass (`just backend-test`)
- [ ] Build produces binary (`just backend-build`)
- [ ] Binary runs standalone (`./bin/server`)
- [ ] Linting passes (`just backend-lint`)

### Docker Testing
- [ ] Production Dockerfile builds (`docker build -f backend/Dockerfile backend/`)
- [ ] Dev Dockerfile builds (`docker build -f backend/Dockerfile.dev backend/`)
- [ ] Stack starts (`just stack-up`)
- [ ] Backend container is healthy (`docker ps`)
- [ ] `/healthz` accessible at http://localhost:8080/healthz
- [ ] `/readyz` accessible at http://localhost:8080/readyz
- [ ] `/health` accessible at http://localhost:8080/health with JSON
- [ ] Hot reload works (edit main.go, see rebuild in logs)
- [ ] Backend logs visible (`just docker-logs`)
- [ ] Tests pass in container (`just docker-test`)
- [ ] Stack stops cleanly (`just stack-down`)

### Step 7: Docker Integration

**Why Required**: Phase 3 (database migrations) requires backend container to connect to TimescaleDB container. Better to establish this now than retrofit later.

#### Dockerfile (Production Build)

**File**: `backend/Dockerfile`

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with version injection
ARG VERSION=0.1.0-dev
RUN go build -ldflags "-X main.version=${VERSION}" -o server .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
```

#### Dockerfile.dev (Development with Hot Reload)

**File**: `backend/Dockerfile.dev`

```dockerfile
FROM golang:1.21-alpine

WORKDIR /app

# Install air for hot reload
RUN go install github.com/cosmtrek/air@latest

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source (volume mount will override in docker-compose)
COPY . .

# Expose port
EXPOSE 8080

# Run with air
CMD ["air", "-c", ".air.toml"]
```

#### Air Configuration

**File**: `backend/.air.toml`

```toml
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -ldflags '-X main.version=0.1.0-dev' -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
```

#### Update docker-compose.yml

**Add to existing `docker-compose.yml`**:

```yaml
services:
  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile.dev
    ports:
      - "8080:8080"
    volumes:
      - ./backend:/app
      - /app/tmp  # Don't sync tmp dir
    environment:
      - PORT=8080
      - LOG_LEVEL=info
    depends_on:
      - db
    networks:
      - trakrf

  db:
    # ... existing db config ...
    networks:
      - trakrf

networks:
  trakrf:
    driver: bridge
```

#### Update Just Commands

**Add to `justfile`**:

```makefile
# Docker commands
docker-up:
    docker-compose up -d

docker-down:
    docker-compose down

docker-logs:
    docker-compose logs -f backend

docker-restart:
    docker-compose restart backend

docker-build:
    docker-compose build backend

docker-test:
    docker-compose exec backend go test -v ./...

# Full stack
stack-up: docker-up
    @echo "✅ Stack running:"
    @echo "   Backend: http://localhost:8080"
    @echo "   Database: localhost:5432"

stack-down: docker-down

stack-logs:
    docker-compose logs -f
```

### Step 8: golangci-lint Configuration

**File**: `backend/.golangci.yml`

```yaml
run:
  timeout: 5m
  tests: true

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - typecheck
    - unused
    - gofmt
    - goimports

linters-settings:
  errcheck:
    check-blank: true

  govet:
    enable-all: true

issues:
  exclude-use-default: false
  max-issues-per-linter: 0
  max-same-issues: 0
```

### Step 9: Manual Testing (Docker)

```bash
# Start full stack
just stack-up

# Verify backend container is running
docker ps | grep backend

# Test all health endpoints

# K8s liveness probe
curl http://localhost:8080/healthz
# Expected: ok

# K8s readiness probe
curl http://localhost:8080/readyz
# Expected: ok

# Human-friendly health check
curl http://localhost:8080/health
# Expected JSON response:
{
  "status": "ok",
  "version": "0.1.0-dev",
  "timestamp": "2025-10-17T14:45:00Z"
}

# Watch logs (in another terminal)
just docker-logs

# Test hot reload:
# 1. Edit backend/main.go (add a comment)
# 2. Watch logs - should see rebuild
# 3. Curl all health endpoints again - should still work

# Test from inside container
docker-compose exec backend sh -c "apk add curl && curl localhost:8080/healthz && curl localhost:8080/readyz"

# Run tests in container
just docker-test

# Stop stack
just stack-down
```

## Definition of Done

### Functional Requirements
- [ ] Go server starts successfully locally
- [ ] K8s liveness probe accessible at GET /healthz (plaintext "ok")
- [ ] K8s readiness probe accessible at GET /readyz (plaintext "ok")
- [ ] Human-friendly health check at GET /health (JSON response)
- [ ] Health JSON includes status, version, timestamp
- [ ] Version string embedded (0.1.0-dev)
- [ ] Graceful shutdown on SIGTERM/SIGINT signals

### Docker Requirements
- [ ] Dockerfile (production) builds successfully
- [ ] Dockerfile.dev builds successfully
- [ ] Backend service runs in docker-compose
- [ ] Hot reload works (air detects changes)
- [ ] Backend can reach database container
- [ ] `just docker-up` starts full stack
- [ ] `just docker-logs` shows backend logs
- [ ] Health endpoint accessible at http://localhost:8080/health

### Quality Requirements
- [ ] Unit tests written and passing
- [ ] Code formatted (gofmt)
- [ ] Linting passes (golangci-lint)
- [ ] Structured logging implemented
- [ ] Tests pass in Docker (`just docker-test`)

### Documentation Requirements
- [ ] Just commands documented
- [ ] Manual testing steps verified
- [ ] Docker commands documented
- [ ] This spec completed

### Integration Requirements
- [ ] Runs on localhost:8080 (both local and Docker)
- [ ] Compatible with Phase 1 (Docker env exists)
- [ ] Ready for Phase 3 (DB migrations can connect to db)

## Open Questions

**Q1: Should we add CORS middleware now?**
**A**: NO - Phase 6 (Serve Frontend Assets) will need it. Don't pre-optimize.

**Q2: Should we structure logs differently?**
**A**: JSON is production-ready. Can add development mode later if needed.

**Q3: Health check on root `/` or `/health`?**
**A**: `/health` is standard. Root will serve frontend in Phase 6.

**Q4: Add metrics (Prometheus) now?**
**A**: NO - No users, no production, defer to post-MVP.

**Q5: Should version be separate `/version` endpoint?**
**A**: NO - Included in /health is sufficient for MVP.

## References

- **Linear Issue**: [TRA-76 - Phase 2: Go Backend Baseline](https://linear.app/trakrf/issue/TRA-76/phase-2-go-backend-baseline)
- **CLAUDE.md**: Go backend standards
- **Epic**: spec/epic.md
- **Phase 1**: Docker environment setup (TRA-75)
- **Phase 3**: Database migrations (TRA-77, next phase)

## Notes

- Keep it simple, avoid premature optimization
- No users = freedom to iterate
- Get to working state fast, refine later
- Foundation for all subsequent phases

### Docker & Deployment
- **Docker integration is required** (not optional) because Phase 3 database migrations need backend container to connect to TimescaleDB container
- Hot reload with air provides good DX without sacrificing production Dockerfile
- Both local (`just backend-run`) and Docker (`just stack-up`) workflows supported

### 12-Factor & Cloud Native
- **12-factor app principles** ensure smooth operation across local docker-compose → Railway → GKE
- No config files (all via environment variables)
- No filesystem writes except stdout/stderr logging
- Stateless design (no in-memory sessions)
- Fast startup + graceful shutdown

### Kubernetes Readiness
- **Three health endpoints** (`/healthz`, `/readyz`, `/health`) enable K8s self-healing
- **Cost**: ~50 lines of code
- **Benefit**: Drop-in K8s deployment manifests when migrating to GKE (post-100k MRR)
- **MQTT consideration**: Current monolith is correct; StatefulSet or separate service when scaling
- Phase 2 establishes patterns, Phase 3 adds database checks to `/readyz`
