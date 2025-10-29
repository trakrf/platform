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

## API Routes

> **📚 Full API Documentation:** Visit http://localhost:8080/swagger/index.html when server is running

**Quick Summary:**
- ✅ **Public:** Health checks, Auth (signup/login), Swagger docs
- ✅ **Protected:** Assets (full CRUD + bulk import), Users (full CRUD)
- 🚧 **TODO:** Organizations, Org Users (registered but not implemented)

**Base URL:** `http://localhost:8080`

### Public Routes (No Authentication Required)

#### Health & Status
| Method | Endpoint | Description | Response |
|--------|----------|-------------|----------|
| GET | `/healthz` | Liveness probe | `200 OK` - Plain text "ok" |
| GET | `/readyz` | Readiness probe | `200 OK` - Plain text "ok" |
| GET | `/health` | Detailed health check | `200 OK` - JSON with version, uptime, database status |

#### Authentication
| Method | Endpoint | Description | Request Body | Response |
|--------|----------|-------------|--------------|----------|
| POST | `/api/v1/auth/signup` | Register new user | `{email, password}` | `201` - User + JWT token |
| POST | `/api/v1/auth/login` | Authenticate user | `{email, password}` | `200` - User + JWT token |

#### Documentation
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/swagger/*` | Swagger API documentation |

### Protected Routes (Authentication Required)

**Authentication:** All routes below require `Authorization: Bearer <token>` header

#### Assets (CRUD + Bulk Import)
| Method | Endpoint | Description | Request Body | Response |
|--------|----------|-------------|--------------|----------|
| GET | `/api/v1/assets` | List all assets (paginated) | - | `202` - Array of assets |
| GET | `/api/v1/assets/{id}` | Get asset by ID | - | `202` - Single asset |
| POST | `/api/v1/assets` | Create new asset | Asset object | `201` - Created asset |
| PUT | `/api/v1/assets/{id}` | Update asset | Asset update object | `202` - Updated asset |
| DELETE | `/api/v1/assets/{id}` | Soft delete asset | - | `202` - `{deleted: true}` |
| POST | `/api/v1/assets/bulk` | Upload CSV for bulk import | CSV file | `200` - Job ID |
| GET | `/api/v1/assets/bulk/{jobId}` | Check bulk import job status | - | `200` - Job status |

**Asset Object:**
```json
{
  "org_id": 1,
  "identifier": "ASSET-001",
  "name": "Device Name",
  "type": "device",
  "description": "Description",
  "valid_from": "2025-01-01T00:00:00Z",
  "valid_to": "2026-01-01T00:00:00Z",
  "is_active": true
}
```

#### Users (CRUD)
| Method | Endpoint | Description | Query Params | Request Body | Response |
|--------|----------|-------------|--------------|--------------|----------|
| GET | `/api/v1/users` | List users | `?page=1&per_page=20` | - | `200` - Users + pagination |
| GET | `/api/v1/users/{id}` | Get user by ID | - | - | `200` - Single user |
| POST | `/api/v1/users` | Create new user | - | User object | `201` - Created user |
| PUT | `/api/v1/users/{id}` | Update user | - | User update object | `200` - Updated user |
| DELETE | `/api/v1/users/{id}` | Soft delete user | - | - | `204` - No content |

**User Object:**
```json
{
  "email": "user@example.com",
  "display_name": "John Doe",
  "is_active": true
}
```

#### Organizations (TODO - Not Implemented)
| Method | Endpoint | Description | Status |
|--------|----------|-------------|--------|
| GET | `/api/v1/organizations` | List organizations | `501 Not Implemented` |
| GET | `/api/v1/organizations/{id}` | Get organization | `501 Not Implemented` |
| POST | `/api/v1/organizations` | Create organization | `501 Not Implemented` |
| PUT | `/api/v1/organizations/{id}` | Update organization | `501 Not Implemented` |
| DELETE | `/api/v1/organizations/{id}` | Delete organization | `501 Not Implemented` |

> **Note:** Organization CRUD endpoints are registered but not implemented. Auth flow creates organizations via direct SQL queries.

#### Organization Users (TODO - Not Implemented)
| Method | Endpoint | Description | Status |
|--------|----------|-------------|--------|
| GET | `/api/v1/org_users` | List org users | `501 Not Implemented` |
| GET | `/api/v1/org_users/{orgId}/{userId}` | Get org user | `501 Not Implemented` |
| POST | `/api/v1/org_users` | Add user to org | `501 Not Implemented` |
| PUT | `/api/v1/org_users/{orgId}/{userId}` | Update org user | `501 Not Implemented` |
| DELETE | `/api/v1/org_users/{orgId}/{userId}` | Remove user from org | `501 Not Implemented` |

### Frontend Routes (SPA)
| Path | Description |
|------|-------------|
| `/assets/*` | Frontend static assets (JS, CSS, images) - 1 year cache |
| `/favicon.ico` | Favicon |
| `/icon-*` | PWA icons |
| `/logo.png` | Logo image |
| `/manifest.json` | PWA manifest |
| `/og-image.png` | OpenGraph image |
| `/*` | Catch-all for React Router (serves index.html) - no cache |

### Error Responses

All endpoints return consistent error format:

```json
{
  "error": "error_code",
  "message": "Human-readable error message",
  "details": "Additional error details (optional)",
  "request_id": "unique-request-id",
  "timestamp": "2025-10-28T15:50:34Z"
}
```

**Common Error Codes:**
- `400 Bad Request` - Invalid JSON or validation error
- `401 Unauthorized` - Missing or invalid JWT token
- `403 Forbidden` - Insufficient permissions
- `404 Not Found` - Resource not found
- `409 Conflict` - Duplicate resource (e.g., email already exists)
- `500 Internal Server Error` - Server error
- `501 Not Implemented` - Endpoint not yet implemented

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

**Option 1: From workspace directory (recommended)**
```bash
# Terminal 1: Frontend
cd frontend && just dev  # http://localhost:5173

# Terminal 2: Backend
cd backend && just dev   # http://localhost:8080
# CORS enabled automatically for frontend dev
```

**Option 2: From root with delegation**
```bash
# Terminal 1: Frontend
just frontend dev        # http://localhost:5173

# Terminal 2: Backend
just backend dev         # http://localhost:8080
```

**Option 3: Parallel local development**
```bash
just dev-local           # Starts both in parallel
# Frontend: http://localhost:5173
# Backend: http://localhost:8080
# Press Ctrl+C to stop both
```

**Option 4: Docker orchestration**
```bash
just dev                 # Full stack with database
```

#### Production Mode (integrated)

```bash
# Build everything (frontend + backend)
./scripts/build.sh

# Or via Just
just build              # Builds both workspaces

# Run integrated server
cd backend && ./bin/trakrf
# Full app on http://localhost:8080
```

#### Test Endpoints

```bash
# Health checks
curl localhost:8080/healthz
curl localhost:8080/health

# Authentication
curl -X POST localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}'

curl -X POST localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"SecurePass123!"}'

# Protected endpoints (requires JWT token)
TOKEN="your-jwt-token-here"

curl localhost:8080/api/v1/users \
  -H "Authorization: Bearer $TOKEN"

curl localhost:8080/api/v1/assets \
  -H "Authorization: Bearer $TOKEN"

# Create asset
curl -X POST localhost:8080/api/v1/assets \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"org_id":1,"identifier":"ASSET-001","name":"Test Asset","type":"device"}'

# Frontend
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

### From project root (delegation pattern):
```bash
just backend validate  # All backend checks (lint + test + build)
just backend lint      # Format (go fmt) + Lint (go vet)
just backend test      # Run tests with verbose output
just backend build     # Build binary with version injection
just backend dev       # Start development server
```

### From backend/ directory:
```bash
# All checks
just validate          # Lint + test + build

# Individual checks
just lint              # go fmt + go vet
just test              # go test -v
just build             # go build with version injection
just dev               # go run .

# Manual commands (without Just)
go fmt ./...
go vet ./...
go test -v ./...
go test -race ./...    # Race detection
go test -cover ./...   # Coverage report
go build -ldflags "-X main.version=0.1.0-dev" -o bin/trakrf .
./bin/trakrf
BACKEND_PORT=9000 ./bin/trakrf  # Custom port
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
# Run tests (from root)
just backend test

# Run tests (from backend/)
just test

# With race detection
just test-race

# With coverage
just test-coverage

# Manual (without Just)
go test -v ./...
go test -race ./...
go test -cover ./...
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
