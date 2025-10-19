# Feature: Testable Router Setup with Optional Database and Self-Healing Connection

## Origin
This specification emerged from discovering a chi router panic at runtime that was never caught by tests or CI. The panic (`wildcard '*' must be the last value in a route`) only manifested when running the service, exposing a critical testing gap.

## Outcome
The backend service will:
1. **Catch startup panics in tests** - Route registration errors are detected before deployment
2. **Auto-heal database connection** - Automatic reconnection with exponential backoff
3. **Start HTTP immediately** - Frontend accessible even during DB connection attempts
4. **Support multiple deployment modes** - Explicit environment config for local/container/cloud

## User Story
**As a** developer deploying the backend service
**I want** startup failures caught in tests and automatic database recovery
**So that** I can deploy with confidence and handle transient failures

**As a** developer working locally
**I want** explicit commands for different deployment modes
**So that** I can easily switch between local/container/cloud databases

## Context

### Discovery
While troubleshooting docker-compose postgres authentication, discovered:
```
panic: chi: wildcard '*' must be the last value in a route
```
This panic happens during route registration in `main()`, which is:
- Not tested
- Not caught by CI
- Only discovered at runtime in production/staging

### Current State
**Backend startup (`backend/main.go`):**
- Router setup in `main()` (lines 52-107) - untestable
- Database connection failure = fatal exit (line 37-39)
- No reconnection logic
- No graceful degradation

**Testing gaps:**
- No unit tests for route registration
- No integration tests that start the server
- No CI smoke tests that run the binary

### Desired State
**Testable architecture:**
```go
// Extracted, testable router setup
func setupRouter() *chi.Mux {
    r := chi.NewRouter()
    // ... all middleware and routes
    return r
}

func main() {
    r := setupRouter()  // Panic caught by tests
    // ... server startup
}
```

**Self-healing connection:**
```go
// HTTP server starts first (frontend always accessible)
go startHTTPServer(setupRouter())

// Database connection with automatic retry
go connectDBWithRetry(ctx) // Exponential backoff, runs in background
```

**Key simplifications:**
- No degraded mode complexity (no atomic flags, no 503 middleware)
- HTTP starts immediately (frontend routes always work)
- DB retries in background (handles both startup and runtime failures)
- Natural DB errors for API calls during connection (acceptable for dev/exception case)

## Technical Requirements (Phase 1)

### 1. Testable Router Setup
- **Extract** route registration from `main()` into `setupRouter() *chi.Mux`
- **Function scope**:
  - Create chi.Mux
  - Apply all middleware (requestID, recovery, CORS, contentType)
  - Register all routes (frontend assets, health checks, API routes)
  - Return configured router
- **Keep in main()**: Server lifecycle, signal handling, graceful shutdown, database init

### 2. Unit Tests for Router
**File**: `backend/main_test.go`

```go
func TestRouterSetup(t *testing.T) {
    // Verify no panic during router setup
    // This catches route registration errors (e.g., invalid wildcard patterns)
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

func TestRouterRegistration(t *testing.T) {
    r := setupRouter()

    // Verify critical routes exist
    tests := []struct {
        method string
        path   string
    }{
        {"GET", "/healthz"},
        {"GET", "/readyz"},
        {"GET", "/health"},
        {"POST", "/api/v1/auth/signup"},
        {"POST", "/api/v1/auth/login"},
        {"GET", "/api/v1/accounts"},
        {"GET", "/api/v1/users"},
        {"GET", "/assets/index.js"},
        {"GET", "/favicon.ico"},
        {"GET", "/"},
    }

    for _, tt := range tests {
        rctx := chi.NewRouteContext()
        req := httptest.NewRequest(tt.method, tt.path, nil)
        if !r.Match(rctx, tt.method, tt.path) {
            t.Errorf("Route not found: %s %s", tt.method, tt.path)
        }
    }
}
```

### 3. Development Workflow Commands
**File**: `backend/justfile`

Add commands for different deployment modes:

```just
# Local development (backend on host, postgres in docker)
dev:
    @echo "🚀 Backend on host → postgres in docker (localhost:5432)"
    @if [ -z "$$PG_URL_LOCAL" ]; then \
        echo "⚠️  PG_URL_LOCAL not set - using PG_URL"; \
        go run .; \
    else \
        PG_URL="$$PG_URL_LOCAL" go run .; \
    fi

# Cloud development (backend on host, postgres in cloud)
dev-cloud:
    @echo "☁️  Backend on host → cloud postgres"
    @if [ -z "$$PG_URL_CLOUD" ]; then \
        echo "❌ PG_URL_CLOUD not set in .env.local"; \
        exit 1; \
    fi
    PG_URL="$$PG_URL_CLOUD" go run .
```

### 4. CI Smoke Test

**Add to**: `backend/justfile`

```just
# Smoke test - verifies binary starts without panic
smoke-test:
    @echo "🔥 Running smoke test..."
    @just build
    @timeout 5s ./bin/server > /tmp/smoke-test.log 2>&1 & SERVER_PID=$! ; \
    sleep 2 ; \
    curl -f http://localhost:8080/healthz || (cat /tmp/smoke-test.log && exit 1) ; \
    kill $SERVER_PID 2>/dev/null || true ; \
    echo "✅ Smoke test passed"

# Add to validate recipe
validate: lint test build smoke-test
```

## Use Cases (Phase 1)

### Use Case 1: Route Panic Detection
**Scenario**: Developer adds invalid route pattern

```go
// Bad route added
r.Handle("/assets/*/extra", handler)  // Invalid wildcard
```

```bash
# Caught in unit tests
just backend test
# TestRouterSetup FAIL: panic during route setup

# Caught in CI
# ❌ Tests failed - fix before merge
```

## Validation Criteria (Phase 1)

### Environment Configuration
- [x] `.env.local` updated with `PG_URL_LOCAL` and `PG_URL_CLOUD` ✅ Already complete
- [x] `.env.local.example` updated with `PG_URL_LOCAL` and renamed `CLOUD_PG_URL` → `PG_URL_CLOUD` ✅ Already complete
- [x] Bash substitution works: `PG_URL_LOCAL="${PG_URL/timescaledb/localhost}"` ✅ Already complete
- [x] Three deployment modes documented in comments ✅ Already complete

### Unit Tests
- [ ] `TestRouterSetup` passes without panic
- [ ] `TestRouterRegistration` verifies all critical routes exist
- [ ] Tests run in < 1 second (fast feedback)

### CI Pipeline
- [ ] `just backend validate` includes smoke test
- [ ] Smoke test catches route panics before merge
- [ ] Build succeeds, binary runs without panic

### Backend Justfile
- [ ] `just backend dev` runs with `PG_URL_LOCAL` (host → docker)
- [ ] `just backend dev-cloud` runs with `PG_URL_CLOUD` (host → cloud)
- [ ] Commands are self-documenting with echo statements

## Environment Configuration

### Database Connection URLs

Update `.env.local` and `.env.local.example` to support three deployment modes:

```bash
# Three deployment modes:
# - PG_URL: Container-to-container (docker-compose)
# - PG_URL_LOCAL: Host-to-container (local development with `go run`)
# - PG_URL_CLOUD: Host-to-cloud (production testing)
POSTGRES_PASSWORD="rfidCollect#1"
POSTGRES_DB=postgres
PG_URL=postgresql://postgres:rfidCollect%231@timescaledb:5432/postgres?options=-c%20search_path%3Dtrakrf,public
PG_URL_LOCAL="${PG_URL/timescaledb/localhost}"
PG_URL_CLOUD=postgres://user:password@your-cloud-host.tsdb.cloud.timescale.com:12345/tsdb?sslmode=require&options=-c%20search_path%3Dtrakrf,public
```

**Rationale:**
- `PG_URL_LOCAL` uses bash substitution to stay DRY (credentials sync automatically)
- `PG_URL_CLOUD` is fully explicit (too different for substitution)
- No direnv magic or detection - explicit mode selection via justfile

### Backend Justfile Commands

Update `backend/justfile` to support explicit deployment mode selection:

```just
# Local development (backend on host, postgres in docker)
dev:
    @echo "🚀 Backend on host → postgres in docker (localhost:5432)"
    PG_URL="$$PG_URL_LOCAL" go run .

# Cloud testing (backend on host, postgres in cloud)
dev-cloud:
    @echo "☁️  Backend on host → cloud postgres"
    PG_URL="$$PG_URL_CLOUD" go run .

# Container mode: Use root justfile instead
# just dev  (runs docker-compose with PG_URL=timescaledb)
```

**Usage:**
```bash
# Container-to-container (full stack docker)
just dev                # Uses PG_URL (timescaledb hostname)

# Host-to-container (local backend dev)
just backend dev        # Uses PG_URL_LOCAL (localhost)

# Host-to-cloud (production testing)
just backend dev-cloud  # Uses PG_URL_CLOUD
```

## Implementation Notes

### File Changes Required (Phase 1)
```
backend/
├── main.go         # Extract setupRouter() from main()
├── main_test.go    # NEW: Router setup tests
└── justfile        # Add dev, dev-cloud, smoke-test commands

.env.local          # ✅ Already complete
.env.local.example  # ✅ Already complete
```

### Migration Strategy
1. **Phase 1**: Extract and test router setup + dev commands
2. **Phase 2**: Add self-healing connection (start HTTP first, background DB retry)

## Conversation References

**Key insight**:
> "we should have a unit test that catches that the built version of the service panics. yes?"

**Use case**:
> "there are some cases where i might want to only run the RFID functions in the UI which i could do without the database while still taking advantage of the single file bundle"

**Decision**:
> "how hard would it be to add a self healing db connection with exponential backoff?"

**Root problem**:
> "panic: chi: wildcard '*' must be the last value in a route" - discovered at runtime, not caught by tests

## Success Metrics
- ✅ Zero startup panics reach production
- ✅ Service uptime increases (survives transient DB failures)
- ✅ RFID-only deployments possible without database
- ✅ Developer confidence in deployment (caught by tests)
