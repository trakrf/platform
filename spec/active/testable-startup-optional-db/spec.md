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

## Technical Requirements

### 1. Testable Router Setup
- **Extract** route registration from `main()` into `setupRouter() *chi.Mux`
- **Function scope**:
  - Create chi.Mux
  - Apply all middleware (requestID, recovery, CORS, contentType)
  - Register all routes (frontend assets, health checks, API routes)
  - Return configured router
- **Keep in main()**: Server lifecycle, signal handling, graceful shutdown

### 2. Unit Tests for Router
**File**: `backend/main_test.go`

```go
func TestRouterSetup(t *testing.T) {
    // Initialize test dependencies
    initTestDB(t)
    initAccountRepo()
    initUserRepo()
    initAccountUserRepo()
    initAuthService()

    // Verify no panic during setup
    assert.NotPanics(t, func() {
        r := setupRouter()
        assert.NotNil(t, r)
    })
}

func TestRouterRegistration(t *testing.T) {
    r := setupRouter()

    // Verify critical routes exist
    testCases := []struct {
        method string
        path   string
    }{
        {"GET", "/healthz"},
        {"GET", "/readyz"},
        {"POST", "/api/v1/auth/signup"},
        {"POST", "/api/v1/auth/login"},
        {"GET", "/api/v1/accounts"},
    }

    for _, tc := range testCases {
        rctx := chi.NewRouteContext()
        req := httptest.NewRequest(tc.method, tc.path, nil)
        if !r.Match(rctx, tc.method, tc.path) {
            t.Errorf("Route not found: %s %s", tc.method, tc.path)
        }
    }
}
```

### 3. Optional Database with Graceful Degradation

**Database initialization:**
```go
var (
    dbAvailable atomic.Bool  // Thread-safe flag
    dbMutex     sync.RWMutex
)

func main() {
    // ...
    ctx := context.Background()
    if err := initDB(ctx); err != nil {
        slog.Warn("Database unavailable - starting in degraded mode",
            "error", err,
            "mode", "offline-rfid-only")
        dbAvailable.Store(false)
        go dbReconnectLoop(ctx)  // Start background healing
    } else {
        slog.Info("Database connected")
        dbAvailable.Store(true)
    }

    // Continue startup regardless
    // ...
}
```

**Middleware for database-dependent endpoints:**
```go
func requireDBMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !dbAvailable.Load() {
            http.Error(w,
                `{"error":"database unavailable","mode":"degraded"}`,
                http.StatusServiceUnavailable)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// Apply to protected routes
r.Group(func(r chi.Router) {
    r.Use(authMiddleware)
    r.Use(requireDBMiddleware)  // 503 if database down

    registerAccountRoutes(r)
    registerUserRoutes(r)
    registerAccountUserRoutes(r)
})
```

### 4. Self-Healing Database Connection

**File**: `backend/db.go`

```go
func dbReconnectLoop(ctx context.Context) {
    backoff := 1 * time.Second
    maxBackoff := 60 * time.Second

    for {
        select {
        case <-ctx.Done():
            return
        case <-time.After(backoff):
            slog.Info("Attempting database reconnection", "backoff", backoff)

            if err := initDB(ctx); err != nil {
                slog.Warn("Database reconnection failed",
                    "error", err,
                    "next_retry", backoff*2)

                // Exponential backoff with cap
                backoff *= 2
                if backoff > maxBackoff {
                    backoff = maxBackoff
                }
                continue
            }

            // Success - run migrations
            slog.Info("Database reconnected - running migrations")
            if err := runMigrations(); err != nil {
                slog.Error("Migration failed after reconnect", "error", err)
                closeDB()
                continue
            }

            // Update status
            dbAvailable.Store(true)
            slog.Info("Database fully recovered")
            return  // Exit reconnection loop
        }
    }
}
```

**Backoff schedule:**
- Initial: 1s
- Subsequent: 2s, 4s, 8s, 16s, 32s
- Max: 60s (cap at 1 minute)

### 5. Updated Health Checks

```go
func healthzHandler(w http.ResponseWriter, r *http.Request) {
    // Kubernetes liveness - always return 200 if process is alive
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "alive",
    })
}

func readyzHandler(w http.ResponseWriter, r *http.Request) {
    // Kubernetes readiness - 200 only if fully functional
    if !dbAvailable.Load() {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]any{
            "status": "not_ready",
            "database": "unavailable",
        })
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]any{
        "status": "ready",
        "database": "available",
    })
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    // Detailed health for monitoring
    uptime := time.Since(startTime)

    health := map[string]any{
        "status": "operational",
        "version": version,
        "uptime_seconds": uptime.Seconds(),
        "database": map[string]any{
            "available": dbAvailable.Load(),
        },
    }

    if dbAvailable.Load() {
        health["database"].(map[string]any)["status"] = "connected"
    } else {
        health["database"].(map[string]any)["status"] = "disconnected"
        health["status"] = "degraded"
    }

    status := http.StatusOK
    if !dbAvailable.Load() {
        status = http.StatusServiceUnavailable
    }

    w.WriteHeader(status)
    json.NewEncoder(w).Encode(health)
}
```

### 6. Integration Tests

**File**: `backend/integration_test.go`

```go
func TestServiceStartsWithoutDatabase(t *testing.T) {
    // Set invalid database URL
    os.Setenv("PG_URL", "postgresql://invalid:invalid@localhost:9999/invalid")

    // Service should start
    r := setupRouter()
    ts := httptest.NewServer(r)
    defer ts.Close()

    // Healthz should return 200 (liveness)
    resp, err := http.Get(ts.URL + "/healthz")
    require.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)

    // Readyz should return 503 (not ready)
    resp, err = http.Get(ts.URL + "/readyz")
    require.NoError(t, err)
    assert.Equal(t, 503, resp.StatusCode)

    // Protected endpoints should return 503
    resp, err = http.Get(ts.URL + "/api/v1/accounts")
    require.NoError(t, err)
    assert.Equal(t, 503, resp.StatusCode)
}

func TestDatabaseReconnection(t *testing.T) {
    // Start with invalid DB
    os.Setenv("PG_URL", "postgresql://invalid:invalid@localhost:9999/invalid")

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Start reconnection loop
    go dbReconnectLoop(ctx)

    // Verify still disconnected
    time.Sleep(100 * time.Millisecond)
    assert.False(t, dbAvailable.Load())

    // Fix database connection
    os.Setenv("PG_URL", "postgresql://postgres:postgres@localhost:5432/test")

    // Wait for reconnection (with timeout)
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        if dbAvailable.Load() {
            break
        }
        time.Sleep(100 * time.Millisecond)
    }

    assert.True(t, dbAvailable.Load(), "Database should reconnect")
}
```

### 7. CI Smoke Test

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

## Use Cases

### Use Case 1: RFID-Only Deployment
**Scenario**: Developer wants to use embedded SPA for RFID operations without database

```bash
# Start without database
POSTGRES_PASSWORD=ignored PG_URL=postgresql://invalid@invalid/invalid ./backend

# Output:
# {"level":"warn","msg":"Database unavailable - starting in degraded mode"}
# {"level":"info","msg":"Server starting","port":"8080"}

# RFID operations work (no DB required)
# Auth/account operations return 503
```

### Use Case 2: Transient Database Failure
**Scenario**: Database container restarts during operation

```bash
# Service running normally
GET /api/v1/accounts -> 200 OK

# Database goes down
docker restart timescaledb

# During downtime
GET /api/v1/accounts -> 503 Service Unavailable
GET /healthz -> 200 OK (liveness)
GET /readyz -> 503 Not Ready

# After 1s, 2s, 4s attempts...
# {"level":"info","msg":"Database reconnected"}

# Service recovered
GET /api/v1/accounts -> 200 OK
```

### Use Case 3: Route Panic Detection
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

## Validation Criteria

### Environment Configuration
- [ ] `.env.local` updated with `PG_URL_LOCAL` and `PG_URL_CLOUD`
- [ ] `.env.local.example` updated with `PG_URL_LOCAL` and renamed `CLOUD_PG_URL` → `PG_URL_CLOUD`
- [ ] Bash substitution works: `PG_URL_LOCAL="${PG_URL/timescaledb/localhost}"`
- [ ] Three deployment modes documented in comments

### Unit Tests
- [ ] `TestRouterSetup` passes without panic
- [ ] `TestRouterRegistration` verifies all critical routes exist
- [ ] Tests run in < 1 second (fast feedback)

### Integration Tests
- [ ] `TestServiceStartsWithoutDatabase` verifies graceful degradation
- [ ] `TestDatabaseReconnection` verifies auto-healing works
- [ ] Protected endpoints return 503 when database unavailable
- [ ] Health endpoints reflect accurate status

### CI Pipeline
- [ ] `just backend validate` includes smoke test
- [ ] Smoke test catches route panics before merge
- [ ] Build succeeds, binary runs without panic

### Backend Justfile
- [ ] `just backend dev` runs with `PG_URL_LOCAL` (host → docker)
- [ ] `just backend dev-cloud` runs with `PG_URL_CLOUD` (host → cloud)
- [ ] Commands are self-documenting with echo statements

### Operational
- [ ] Service starts and serves traffic without database
- [ ] Logs clearly indicate degraded mode
- [ ] Database reconnection succeeds after transient failure
- [ ] Migrations run automatically on reconnection
- [ ] Exponential backoff prevents thundering herd
- [ ] All three deployment modes work correctly

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

### File Changes Required
```
backend/
├── main.go              # Extract setupRouter(), optional DB
├── main_test.go         # NEW: Router setup tests
├── integration_test.go  # NEW: Startup and reconnection tests
├── db.go                # Add dbReconnectLoop(), dbAvailable flag
├── middleware.go        # Add requireDBMiddleware
└── justfile             # Add dev, dev-cloud, smoke-test recipes

.env.local               # Add PG_URL_LOCAL, PG_URL_CLOUD
.env.local.example       # Add PG_URL_LOCAL, rename CLOUD_PG_URL → PG_URL_CLOUD
```

### Migration Strategy
1. **Phase 1**: Extract and test router setup + dev commands (non-breaking)
2. **Phase 2**: Add self-healing connection (start HTTP first, background DB retry)

### Backwards Compatibility
- **Breaking**: Service no longer exits on database failure
- **Migration path**: Update deployment checks to use `/readyz` not `/healthz`
- **Kubernetes**: Update readinessProbe to tolerate 503 during startup

### Performance Considerations
- Reconnection loop uses minimal resources (sleep-based)
- `atomic.Bool` for lock-free status checks
- No performance impact on hot path

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
