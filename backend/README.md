# Backend (Go)

Go HTTP server for TrakRF platform.

## Current Status: Phase 2A Complete

Minimal production-ready HTTP server with Kubernetes-ready health endpoints. Stdlib only, fully tested, 12-factor compliant.

## Structure

```
backend/
├── main.go         # HTTP server entrypoint (graceful shutdown, logging)
├── health.go       # Health check handlers (/healthz, /readyz, /health)
├── health_test.go  # Comprehensive test suite
├── go.mod          # Go module definition
└── server          # Compiled binary (created by build)
```

## Features

### Health Endpoints (K8s Ready)
- **GET /healthz** - Liveness probe (returns "ok" if process alive)
- **GET /readyz** - Readiness probe (returns "ok" if ready for traffic)
- **GET /health** - Detailed JSON health status (version, timestamp)

### Production Features
- ✅ Graceful shutdown (SIGTERM/SIGINT handling)
- ✅ Structured JSON logging (slog)
- ✅ Environment-based configuration (PORT)
- ✅ HTTP timeouts (read, write, idle)
- ✅ Request logging middleware
- ✅ Version injection via build flags
- ✅ 12-factor app compliant

## Development

### Prerequisites
- Go 1.21+
- Just (task runner)

### Quick Start
```bash
# Run all checks (lint, test, build)
just backend

# Start dev server
just backend-run

# Or run directly
cd backend && go run .

# Test endpoints
curl localhost:8080/healthz   # "ok"
curl localhost:8080/readyz    # "ok"
curl localhost:8080/health    # JSON response
```

### Environment Variables
- `PORT` - HTTP server port (default: 8080)

## Validation

### From project root (via Just):
```bash
just backend-lint   # Format (go fmt) + Lint (go vet)
just backend-test   # Run tests with verbose output
just backend-build  # Build binary with version injection
just backend-run    # Start development server
just backend        # Run all checks
```

### From backend/ directory:
```bash
# Lint
go fmt ./...
go vet ./...

# Test
go test -v ./...
go test -race ./...   # Race detection
go test -cover ./...  # Coverage report

# Build
go build -ldflags "-X main.version=0.1.0-dev" -o server .

# Run
./server
PORT=9000 ./server  # Custom port
```

## Testing

**Test Suite:** 10 tests (4 test functions, 10 sub-tests)
- Table-driven tests for all handlers
- Method validation (GET, POST, PUT, DELETE)
- JSON decoding validation
- Timestamp validation
- 40.4% code coverage
- No race conditions

```bash
# Run tests
just backend-test

# With race detection
cd backend && go test -race ./...

# With coverage
cd backend && go test -cover ./...
```

## Architecture

### Current (Phase 2A)
- **Stdlib only** - No external dependencies
- **Single package** - Flat structure for simplicity
- **HTTP handlers** - Standard net/http patterns
- **Table-driven tests** - Go community best practice

### Future Phases
- **Phase 2B**: Docker integration (Dockerfile, compose)
- **Phase 3**: Database migrations (go-migrate)
- **Phase 4**: REST API framework (chi/echo/gorilla)
- **Phase 5**: Authentication (JWT, session management)

## Deployment

### Railway (Current Host)
Railway auto-detects Go projects and runs:
```bash
go build -o server .
./server
```

Version injection happens via justfile:
```bash
just backend-build  # Creates backend/server with version
```

### Kubernetes
Health endpoints are K8s-ready:
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
```

## Logging

Structured JSON logs to stdout (12-factor):
```json
{"time":"2025-10-17T12:00:00Z","level":"INFO","msg":"Server starting","port":"8080","version":"0.1.0-dev"}
{"time":"2025-10-17T12:00:01Z","level":"INFO","msg":"Request","method":"GET","path":"/health","duration":125000}
```

## Version Management

Version injected at build time via ldflags:
```bash
go build -ldflags "-X main.version=0.1.0-dev" -o server .
```

Default version in code: `"dev"`

## Phase 2A Success Metrics

✅ **All metrics achieved (100%)**
- Server starts successfully
- All 3 health endpoints respond correctly
- Graceful shutdown works
- Custom PORT works
- Version injection works
- All tests pass
- Clean code quality (no debug statements)
