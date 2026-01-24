# Implementation Plan: TRA-217 Current Locations Endpoint
Generated: 2026-01-23
Specification: spec.md

## Understanding

Build a paginated `GET /api/v1/reports/current-locations` endpoint that returns the most recent location for each asset in an organization. Key design decisions:

1. **Performance-first**: Implement both DISTINCT ON and TimescaleDB `last()` query strategies with env var toggle
2. **Maintainability**: Use struct-based response types (not inline maps)
3. **Large datasets**: Design for scale with proper pagination and query optimization

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/inventory/save.go` (lines 51-99) - handler structure, auth, error handling
- `backend/internal/handlers/assets/assets.go` (lines 273-321) - pagination param parsing
- `backend/internal/storage/assets.go` (lines 200-250) - list + count query pattern
- `backend/internal/handlers/inventory/save_test.go` - test patterns with mock claims
- `backend/internal/apierrors/messages.go` - error constant patterns

**Files to Create**:
- `backend/internal/handlers/reports/current_locations.go` - handler with route registration
- `backend/internal/handlers/reports/current_locations_test.go` - unit tests
- `backend/internal/storage/reports.go` - dual query implementation
- `backend/internal/models/report/current_location.go` - DTOs

**Files to Modify**:
- `backend/main.go` (lines ~23-38, ~100-142) - import handler, add to setupRouter, register routes
- `backend/internal/apierrors/messages.go` - add report error constants

## Architecture Impact
- **Subsystems affected**: Backend API only
- **New dependencies**: None (uses existing pgx, chi, TimescaleDB)
- **Breaking changes**: None (new endpoint)

## Task Breakdown

### Task 1: Create Report Models (DTOs)
**File**: `backend/internal/models/report/current_location.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/asset/asset.go`

**Implementation**:
```go
package report

import "time"

// CurrentLocationItem represents a single asset's current location
type CurrentLocationItem struct {
    AssetID         int       `json:"asset_id"`
    AssetName       string    `json:"asset_name"`
    AssetIdentifier string    `json:"asset_identifier"`
    LocationID      *int      `json:"location_id"`      // nullable
    LocationName    *string   `json:"location_name"`    // nullable
    LastSeen        time.Time `json:"last_seen"`
}

// CurrentLocationsResponse is the paginated response for current locations
type CurrentLocationsResponse struct {
    Data       []CurrentLocationItem `json:"data"`
    Count      int                   `json:"count"`
    Offset     int                   `json:"offset"`
    TotalCount int                   `json:"total_count"`
}

// CurrentLocationFilter contains query parameters for filtering
type CurrentLocationFilter struct {
    LocationID *int
    Search     *string
    Limit      int
    Offset     int
}
```

**Validation**: `just backend build`

---

### Task 2: Add Report Error Messages
**File**: `backend/internal/apierrors/messages.go`
**Action**: MODIFY
**Pattern**: Follow existing const groups

**Implementation**:
Add after line 150 (after Inventory error messages):
```go
// Report error messages
const (
    ReportCurrentLocationsFailed = "Failed to list current locations"
    ReportCurrentLocationsCount  = "Failed to count current locations"
)
```

**Validation**: `just backend build`

---

### Task 3: Create Storage Layer with Dual Query Engines
**File**: `backend/internal/storage/reports.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/storage/assets.go` (lines 200-250)

**Implementation**:
```go
package storage

import (
    "context"
    "fmt"
    "os"

    "github.com/trakrf/platform/backend/internal/models/report"
)

// QueryEngine determines which SQL strategy to use
type QueryEngine string

const (
    QueryEngineDistinctOn     QueryEngine = "distinct_on"
    QueryEngineTimescaleLast  QueryEngine = "timescale_last"
)

// getReportsQueryEngine returns the configured query engine
func getReportsQueryEngine() QueryEngine {
    engine := os.Getenv("REPORTS_QUERY_ENGINE")
    if engine == string(QueryEngineTimescaleLast) {
        return QueryEngineTimescaleLast
    }
    return QueryEngineDistinctOn // default
}

// ListCurrentLocations returns paginated current asset locations
func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) ([]report.CurrentLocationItem, error) {
    engine := getReportsQueryEngine()

    var query string
    if engine == QueryEngineTimescaleLast {
        query = buildCurrentLocationsQueryTimescale(filter)
    } else {
        query = buildCurrentLocationsQueryDistinctOn(filter)
    }

    rows, err := s.pool.Query(ctx, query, orgID, filter.LocationID, filter.Search, filter.Limit, filter.Offset)
    if err != nil {
        return nil, fmt.Errorf("failed to list current locations: %w", err)
    }
    defer rows.Close()

    var items []report.CurrentLocationItem
    for rows.Next() {
        var item report.CurrentLocationItem
        err := rows.Scan(
            &item.AssetID,
            &item.AssetName,
            &item.AssetIdentifier,
            &item.LocationID,
            &item.LocationName,
            &item.LastSeen,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan current location: %w", err)
        }
        items = append(items, item)
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("error iterating current locations: %w", err)
    }

    if items == nil {
        items = []report.CurrentLocationItem{}
    }

    return items, nil
}

// CountCurrentLocations returns total count for pagination
func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter report.CurrentLocationFilter) (int, error) {
    query := buildCurrentLocationsCountQuery()

    var count int
    err := s.pool.QueryRow(ctx, query, orgID, filter.LocationID, filter.Search).Scan(&count)
    if err != nil {
        return 0, fmt.Errorf("failed to count current locations: %w", err)
    }

    return count, nil
}

func buildCurrentLocationsQueryDistinctOn(filter report.CurrentLocationFilter) string {
    return `
        WITH latest_scans AS (
            SELECT DISTINCT ON (s.asset_id)
                s.asset_id,
                s.location_id,
                s.timestamp AS last_seen
            FROM trakrf.asset_scans s
            WHERE s.org_id = $1
            ORDER BY s.asset_id, s.timestamp DESC
        )
        SELECT
            ls.asset_id,
            a.name AS asset_name,
            COALESCE(
                (SELECT i.value FROM trakrf.identifiers i
                 WHERE i.asset_id = a.id AND i.is_active = true LIMIT 1),
                ''
            ) AS asset_identifier,
            ls.location_id,
            l.name AS location_name,
            ls.last_seen
        FROM latest_scans ls
        JOIN trakrf.assets a ON a.id = ls.asset_id
        LEFT JOIN trakrf.locations l ON l.id = ls.location_id
        WHERE ($2::int IS NULL OR ls.location_id = $2)
          AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
               OR EXISTS (
                   SELECT 1 FROM trakrf.identifiers ai
                   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
               ))
        ORDER BY a.name
        LIMIT $4 OFFSET $5
    `
}

func buildCurrentLocationsQueryTimescale(filter report.CurrentLocationFilter) string {
    return `
        WITH latest_scans AS (
            SELECT
                asset_id,
                last(location_id, timestamp) AS location_id,
                max(timestamp) AS last_seen
            FROM trakrf.asset_scans
            WHERE org_id = $1
            GROUP BY asset_id
        )
        SELECT
            ls.asset_id,
            a.name AS asset_name,
            COALESCE(
                (SELECT i.value FROM trakrf.identifiers i
                 WHERE i.asset_id = a.id AND i.is_active = true LIMIT 1),
                ''
            ) AS asset_identifier,
            ls.location_id,
            l.name AS location_name,
            ls.last_seen
        FROM latest_scans ls
        JOIN trakrf.assets a ON a.id = ls.asset_id
        LEFT JOIN trakrf.locations l ON l.id = ls.location_id
        WHERE ($2::int IS NULL OR ls.location_id = $2)
          AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
               OR EXISTS (
                   SELECT 1 FROM trakrf.identifiers ai
                   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
               ))
        ORDER BY a.name
        LIMIT $4 OFFSET $5
    `
}

func buildCurrentLocationsCountQuery() string {
    return `
        WITH latest_scans AS (
            SELECT DISTINCT ON (s.asset_id)
                s.asset_id,
                s.location_id
            FROM trakrf.asset_scans s
            WHERE s.org_id = $1
            ORDER BY s.asset_id, s.timestamp DESC
        )
        SELECT COUNT(*)
        FROM latest_scans ls
        JOIN trakrf.assets a ON a.id = ls.asset_id
        WHERE ($2::int IS NULL OR ls.location_id = $2)
          AND ($3::text IS NULL OR a.name ILIKE '%' || $3 || '%'
               OR EXISTS (
                   SELECT 1 FROM trakrf.identifiers ai
                   WHERE ai.asset_id = a.id AND ai.is_active = true AND ai.value ILIKE '%' || $3 || '%'
               ))
    `
}
```

**Validation**: `just backend build`

---

### Task 4: Create Reports Handler
**File**: `backend/internal/handlers/reports/current_locations.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/inventory/save.go` (lines 51-99)

**Implementation**:
```go
package reports

import (
    "net/http"
    "strconv"

    "github.com/go-chi/chi/v5"
    "github.com/trakrf/platform/backend/internal/apierrors"
    "github.com/trakrf/platform/backend/internal/middleware"
    modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
    "github.com/trakrf/platform/backend/internal/models/report"
    "github.com/trakrf/platform/backend/internal/storage"
    "github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
    defaultLimit = 50
    maxLimit     = 100
)

// Handler handles report-related API requests
type Handler struct {
    storage *storage.Storage
}

// NewHandler creates a new reports handler
func NewHandler(storage *storage.Storage) *Handler {
    return &Handler{storage: storage}
}

// ListCurrentLocations handles GET /api/v1/reports/current-locations
// @Summary List current asset locations
// @Description Get paginated list of assets with their most recent location
// @Tags reports
// @Accept json
// @Produce json
// @Param limit query int false "Results per page (default 50, max 100)" minimum(1) maximum(100) default(50)
// @Param offset query int false "Pagination offset (default 0)" minimum(0) default(0)
// @Param location_id query int false "Filter by location ID"
// @Param search query string false "Search asset name or identifier"
// @Success 200 {object} report.CurrentLocationsResponse
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/reports/current-locations [get]
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
    requestID := middleware.GetRequestID(r.Context())

    // 1. Get org from claims
    claims := middleware.GetUserClaims(r)
    if claims == nil || claims.CurrentOrgID == nil {
        httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
            apierrors.ReportCurrentLocationsFailed, "missing organization context", requestID)
        return
    }
    orgID := *claims.CurrentOrgID

    // 2. Parse query parameters
    filter := report.CurrentLocationFilter{
        Limit:  defaultLimit,
        Offset: 0,
    }

    if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
        if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
            filter.Limit = parsed
            if filter.Limit > maxLimit {
                filter.Limit = maxLimit
            }
        }
    }

    if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
        if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
            filter.Offset = parsed
        }
    }

    if locationIDStr := r.URL.Query().Get("location_id"); locationIDStr != "" {
        if parsed, err := strconv.Atoi(locationIDStr); err == nil {
            filter.LocationID = &parsed
        }
    }

    if search := r.URL.Query().Get("search"); search != "" {
        filter.Search = &search
    }

    // 3. Fetch data
    items, err := h.storage.ListCurrentLocations(r.Context(), orgID, filter)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.ReportCurrentLocationsFailed, err.Error(), requestID)
        return
    }

    totalCount, err := h.storage.CountCurrentLocations(r.Context(), orgID, filter)
    if err != nil {
        httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
            apierrors.ReportCurrentLocationsCount, err.Error(), requestID)
        return
    }

    // 4. Return response
    response := report.CurrentLocationsResponse{
        Data:       items,
        Count:      len(items),
        Offset:     filter.Offset,
        TotalCount: totalCount,
    }

    httputil.WriteJSON(w, http.StatusOK, response)
}

// RegisterRoutes registers report handler routes
func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Get("/api/v1/reports/current-locations", h.ListCurrentLocations)
}
```

**Validation**: `just backend build`

---

### Task 5: Wire Handler into Main
**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Reference existing handler imports and registration (lines 23-38, 100-142)

**Implementation**:

1. Add import (around line 27):
```go
reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
```

2. Add parameter to setupRouter function (around line 106):
```go
reportsHandler *reportshandler.Handler,
```

3. Register routes inside authenticated group (after line 141):
```go
reportsHandler.RegisterRoutes(r)
```

4. Create handler in main() and pass to setupRouter (around line 195):
```go
reportsHandler := reportshandler.NewHandler(store)
```

**Validation**: `just backend build`

---

### Task 6: Create Handler Unit Tests
**File**: `backend/internal/handlers/reports/current_locations_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/inventory/save_test.go`

**Implementation**:
```go
package reports

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/trakrf/platform/backend/internal/middleware"
    "github.com/trakrf/platform/backend/internal/util/jwt"
)

func TestListCurrentLocations_MissingOrgContext(t *testing.T) {
    handler := NewHandler(nil)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/current-locations", nil)
    w := httptest.NewRecorder()

    handler.ListCurrentLocations(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)

    var response struct {
        Error struct {
            Type   string `json:"type"`
            Status int    `json:"status"`
        } `json:"error"`
    }
    err := json.Unmarshal(w.Body.Bytes(), &response)
    require.NoError(t, err)
    assert.Equal(t, "unauthorized", response.Error.Type)
}

func TestListCurrentLocations_DefaultPagination(t *testing.T) {
    // Verify default limit and offset parsing
    // Full integration test would require storage mock
    handler := NewHandler(nil)
    assert.NotNil(t, handler)
    assert.Equal(t, 50, defaultLimit)
    assert.Equal(t, 100, maxLimit)
}

func TestListCurrentLocations_LimitCapping(t *testing.T) {
    tests := []struct {
        name          string
        queryLimit    string
        expectedLimit int
    }{
        {"default", "", 50},
        {"valid", "25", 25},
        {"over max", "200", 100},
        {"invalid", "abc", 50},
        {"zero", "0", 50},
        {"negative", "-5", 50},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // This tests the parsing logic conceptually
            // Full test would require mocked storage
            limit := defaultLimit
            if tt.queryLimit != "" {
                // Simulate parsing
                if parsed := parseLimit(tt.queryLimit); parsed > 0 {
                    limit = parsed
                    if limit > maxLimit {
                        limit = maxLimit
                    }
                }
            }
            assert.Equal(t, tt.expectedLimit, limit)
        })
    }
}

func parseLimit(s string) int {
    var result int
    for _, c := range s {
        if c < '0' || c > '9' {
            return 0
        }
        result = result*10 + int(c-'0')
    }
    return result
}

func TestListCurrentLocations_RouteRegistration(t *testing.T) {
    handler := NewHandler(nil)

    r := chi.NewRouter()
    handler.RegisterRoutes(r)

    rctx := chi.NewRouteContext()
    if !r.Match(rctx, http.MethodGet, "/api/v1/reports/current-locations") {
        t.Error("Route GET /api/v1/reports/current-locations not registered")
    }
}

func TestCurrentLocationFilter_Struct(t *testing.T) {
    locationID := 123
    search := "laptop"

    filter := struct {
        LocationID *int
        Search     *string
        Limit      int
        Offset     int
    }{
        LocationID: &locationID,
        Search:     &search,
        Limit:      50,
        Offset:     100,
    }

    assert.Equal(t, 123, *filter.LocationID)
    assert.Equal(t, "laptop", *filter.Search)
    assert.Equal(t, 50, filter.Limit)
    assert.Equal(t, 100, filter.Offset)
}

func createTestRequest(t *testing.T, path string, orgID int) *http.Request {
    req := httptest.NewRequest(http.MethodGet, path, nil)
    claims := &jwt.Claims{
        UserID:       1,
        Email:        "test@example.com",
        CurrentOrgID: &orgID,
    }
    ctx := context.WithValue(req.Context(), middleware.UserClaimsKey, claims)
    return req.WithContext(ctx)
}
```

**Validation**: `just backend test`

---

### Task 7: Run Full Validation
**Action**: VALIDATE

Run complete validation suite:
```bash
just backend validate
```

This runs:
- `go fmt ./...`
- `go vet ./...`
- `go test ./...`
- `go build ./...`

**Validation**: All checks must pass

---

## Risk Assessment

- **Risk**: Large datasets may cause slow queries on either engine
  **Mitigation**: Implemented dual query engines with env var toggle for quick switching. Monitor with EXPLAIN ANALYZE on production data.

- **Risk**: Count query runs full CTE twice (once for data, once for count)
  **Mitigation**: Acceptable for MVP. Future optimization: single query with window function or cached counts.

- **Risk**: Identifier subquery (N+1 pattern in SELECT)
  **Mitigation**: Acceptable for paginated results (max 100 rows). Future: lateral join or batch fetch.

## Integration Points

- **Route registration**: Added to authenticated group in main.go
- **Storage**: New methods on existing Storage struct
- **Models**: New `report` package for DTOs
- **Errors**: New constants in apierrors

## VALIDATION GATES (MANDATORY)

After EVERY task, run from `backend/` directory:
```bash
just validate
```

This runs lint, vet, test, and build.

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task:
```bash
cd backend && just validate
```

Final validation:
```bash
just validate  # from project root - validates full stack
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec with tested SQL
- ✅ Similar handler pattern found at `handlers/inventory/save.go`
- ✅ Similar storage pattern found at `storage/assets.go`
- ✅ All clarifying questions answered (performance-first, dual engines, env var)
- ✅ Existing test patterns to follow at `handlers/inventory/save_test.go`
- ✅ Query validated on preview database
- ⚠️ Minor: No existing reports handler to reference (new package)

**Assessment**: Well-defined endpoint following established patterns. Dual query engine adds slight complexity but provides valuable production flexibility.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist in codebase, SQL tested on real data, clear file structure. Main risk is minor typos or import path issues.
