# Backend (Go)

Go HTTP server for TrakRF platform.

## Current Status: Phase 6 Complete

Production-ready HTTP server with embedded React frontend, database migrations, and authentication. Single binary deployment with integrated frontend serving.

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

#### Development Mode (with hot reload)

**Option 1: Using Just (recommended)**
```bash
# Terminal 1: Frontend dev server (from project root)
just frontend-dev        # http://localhost:5173

# Terminal 2: Backend server (from project root)
just backend-run         # http://localhost:8080
# CORS enabled automatically for frontend dev
```

**Option 2: Manual**
```bash
# Terminal 1: Frontend dev server
cd frontend && pnpm dev  # http://localhost:5173

# Terminal 2: Backend server
cd backend && go run .   # http://localhost:8080
# CORS enabled automatically for frontend dev
```

#### Production Mode (integrated)

**Option 1: Using build script (recommended)**
```bash
# Build everything (frontend + backend)
./scripts/build.sh

# Run integrated server
cd backend && ./bin/trakrf
# Full app on http://localhost:8080
```

**Option 2: Using Just**
```bash
# Build and validate everything
just build

# Run integrated server
cd backend && ./bin/trakrf
# Full app on http://localhost:8080
```

#### Test Endpoints

```bash
curl localhost:8080/healthz           # Health check
curl localhost:8080/api/v1/health     # API health
curl localhost:8080/                  # React frontend
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKEND_PORT` | HTTP server port | `8080` |
| `BACKEND_CORS_ORIGIN` | CORS allowed origin | `*` (dev mode) |
| `JWT_SECRET` | JWT signing secret | `dev-secret-change-in-production` |
| `DATABASE_URL` | PostgreSQL connection string | Required |

**Phase 6 Notes**:
- Set `BACKEND_CORS_ORIGIN=disabled` when using embedded frontend (production)
- Default `*` allows frontend dev server to access API during development

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

### Current (Phase 6)
- **Chi router** - Production-grade HTTP router
- **Embedded frontend** - React app served from Go binary via `embed` package
- **JWT authentication** - Secure API with token-based auth
- **TimescaleDB** - PostgreSQL with time-series extensions
- **Multi-tenant** - Account-based data isolation
- **Cache headers** - Optimized frontend asset caching

### Frontend Integration (Phase 6)

The backend embeds the built React frontend as a single deployable artifact:

```
backend/
├── frontend.go           # Frontend serving logic
├── frontend_test.go      # Cache header tests
├── frontend/dist/        # Embedded React build (copied from ../frontend/dist)
└── bin/trakrf           # Single binary with embedded frontend
```

**Routing Order**:
1. Health checks (`/healthz`, `/readyz`, `/health`)
2. Static assets (`/assets/*`, `/favicon.ico`, icons, manifest)
3. API routes (`/api/v1/*` - protected by JWT)
4. SPA catch-all (`/*` - serves index.html for React Router)

**Cache Strategy**:
- `index.html`: `no-cache, no-store, must-revalidate` (always fresh)
- Hashed assets (`/assets/*`): `max-age=31536000, immutable` (1 year)
- Other static files: `max-age=3600` (1 hour)

**Build Process**:
```bash
# Automated (recommended)
./scripts/build.sh

# Manual
cd frontend && pnpm build
cp -r dist ../backend/frontend/
cd ../backend && go build -o bin/trakrf .
```

### Completed Phases
- **Phase 2A**: ✅ HTTP server with health endpoints
- **Phase 2B**: ✅ Docker integration (multi-stage Dockerfile, docker-compose, Air hot-reload)
- **Phase 3**: ✅ Database migrations (golang-migrate, TimescaleDB schema, Just commands)
- **Phase 4**: ✅ REST API framework (chi router, Phase 4A endpoints)
- **Phase 5**: ✅ Authentication (JWT, auth middleware, signup/login)
- **Phase 6**: ✅ Frontend integration (embedded React, SPA routing, cache headers)

### Future Phases
- **Phase 7**: Railway/GKE deployment
- **Phase 8**: MQTT ingestion pipeline

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
