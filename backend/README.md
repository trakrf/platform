# Backend (Go)

Go HTTP server for TrakRF platform.

## Current Status: Phase 3 Complete

Production-ready HTTP server with Docker integration and database migrations. Stdlib only, fully tested, 12-factor compliant, TimescaleDB backend with golang-migrate.

## Structure

```
backend/
├── main.go         # HTTP server entrypoint (graceful shutdown, logging)
├── health.go       # Health check handlers (/healthz, /readyz, /health)
├── health_test.go  # Comprehensive test suite
├── go.mod          # Go module definition
├── Dockerfile      # Multi-stage build (dev with Air, prod standalone)
├── .air.toml       # Air hot-reload configuration
└── server          # Compiled binary (created by build)

../database/migrations/  # SQL migrations (golang-migrate)
├── 000001_prereqs.up.sql
├── 000001_prereqs.down.sql
└── ... (24 total migration files)
```

## Features

### Health Endpoints (K8s Ready)
- **GET /healthz** - Liveness probe (returns "ok" if process alive)
- **GET /readyz** - Readiness probe (returns "ok" if ready for traffic)
- **GET /health** - Detailed JSON health status (version, timestamp)

### Database Migrations (golang-migrate)
- **12 migrations** - Complete schema from TimescaleDB extensions to sample data
- **Versioned** - Sequential 6-digit numbering (000001-000012)
- **Reversible** - Up/down pairs with CASCADE cleanup
- **Auto-migration** - Runs automatically on `just dev` startup
- **CLI included** - migrate v4.17.0 installed in Docker image

**Schema includes:**
- Accounts, users, RBAC (multi-tenant foundation)
- Locations, devices, antennas (hardware topology)
- Assets and tags (tracking entities)
- Events hypertable (time-series location events)
- Messages hypertable (MQTT ingestion with auto-processing trigger)
- Sample data (development fixtures)

### Production Features
- ✅ Graceful shutdown (SIGTERM/SIGINT handling)
- ✅ Structured JSON logging (slog)
- ✅ Environment-based configuration (BACKEND_PORT)
- ✅ HTTP timeouts (read, write, idle)
- ✅ Request logging middleware
- ✅ Version injection via build flags
- ✅ 12-factor app compliant

## Development

### Prerequisites
- Go 1.25+ (required for Air hot-reload)
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
- `BACKEND_PORT` - HTTP server port (default: 8080)
- `BACKEND_LOG_LEVEL` - Log level: debug, info, warn, error (default: info, currently hard-coded)
- `PG_URL` - PostgreSQL connection string with URL-encoded password

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
BACKEND_PORT=9000 ./server  # Custom port
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

### Completed Phases
- **Phase 2A**: ✅ HTTP server with health endpoints
- **Phase 2B**: ✅ Docker integration (multi-stage Dockerfile, docker-compose, Air hot-reload)
- **Phase 3**: ✅ Database migrations (golang-migrate, TimescaleDB schema, Just commands)

### Future Phases
- **Phase 4**: REST API framework (chi/fiber/echo)
- **Phase 5**: Authentication (JWT, session management)
- **Phase 6**: MQTT ingestion pipeline

## Database Migrations

Managed via golang-migrate CLI (included in Docker image).

### Commands (from project root)
```bash
just db-migrate-up         # Apply all pending migrations
just db-migrate-down       # Rollback last migration
just db-migrate-status     # Show current migration version
just db-migrate-create foo # Create new migration pair
just db-migrate-force 5    # Force version to 5 (recovery)
```

### Creating New Migrations
```bash
# From project root
just db-migrate-create add_widgets

# Creates:
# database/migrations/000013_add_widgets.up.sql
# database/migrations/000013_add_widgets.down.sql

# Edit both files, then apply:
just db-migrate-up
```

### Migration Design Principles
- **Up migrations** create/alter schema, add data
- **Down migrations** use CASCADE drops for clean rollback
- **Sample data** down migration is no-op (cleanup via table drops)
- **Search path** set to `trakrf,public` in all migrations
- **TimescaleDB** hypertables created with retention policies

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
- Custom BACKEND_PORT works
- Version injection works
- All tests pass
- Clean code quality (no debug statements)
