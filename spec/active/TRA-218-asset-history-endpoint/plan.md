# Implementation Plan: TRA-218 Asset History Endpoint
Generated: 2026-01-23
Specification: spec.md

## Understanding

Build a paginated `GET /api/v1/reports/assets/{id}/history` endpoint that returns location history for a single asset with duration calculations. Key design decisions:

1. **Match existing patterns**: Use `limit`/`offset` pagination with default=50, max=100 (aligned with TRA-217)
2. **Performance protection**: Enforce 30-day default date range to prevent expensive unbounded queries
3. **Reuse existing storage**: Use `GetAssetByID` + `GetIdentifiersByAssetID` for asset validation
4. **Window functions**: Use `LEAD()` to calculate duration between scans

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/reports/current_locations.go` (lines 46-113) - handler structure, auth, pagination
- `backend/internal/handlers/reports/current_locations_test.go` - unit test patterns
- `backend/internal/handlers/assets/assets_integration_test.go` - integration test patterns
- `backend/internal/storage/reports.go` (lines 29-84) - query + count pattern
- `backend/internal/models/report/current_location.go` - DTO patterns
- `backend/internal/apierrors/messages.go` (lines 152-156) - error constant pattern

**Files to Create**:
- `backend/internal/handlers/reports/asset_history.go` - handler with route registration
- `backend/internal/handlers/reports/asset_history_test.go` - unit tests
- `backend/internal/handlers/reports/asset_history_integration_test.go` - integration tests
- `backend/internal/models/report/asset_history.go` - DTOs

**Files to Modify**:
- `backend/internal/handlers/reports/current_locations.go` (lines 16-19) - move constants to shared location
- `backend/internal/storage/reports.go` - add history query methods
- `backend/internal/apierrors/messages.go` - add asset history error constants

## Architecture Impact
- **Subsystems affected**: Backend API only
- **New dependencies**: None
- **Breaking changes**: None (new endpoint)

## Task Breakdown

### Task 1: Create Asset History Models (DTOs)
**File**: `backend/internal/models/report/asset_history.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/report/current_location.go`

**Implementation**:
```go
package report

import "time"

// AssetInfo contains asset metadata for the response header
type AssetInfo struct {
    ID         int    `json:"id"`
    Name       string `json:"name"`
    Identifier string `json:"identifier"`
}

// AssetHistoryItem represents a single scan in the asset's history
type AssetHistoryItem struct {
    Timestamp       time.Time `json:"timestamp"`
    LocationID      *int      `json:"location_id"`
    LocationName    *string   `json:"location_name"`
    DurationSeconds *int      `json:"duration_seconds"`
}

// AssetHistoryResponse is the paginated response for asset history
type AssetHistoryResponse struct {
    Asset      AssetInfo          `json:"asset"`
    Data       []AssetHistoryItem `json:"data"`
    Count      int                `json:"count"`
    Offset     int                `json:"offset"`
    TotalCount int                `json:"total_count"`
}

// AssetHistoryFilter contains query parameters for filtering
type AssetHistoryFilter struct {
    StartDate *time.Time
    EndDate   *time.Time
    Limit     int
    Offset    int
}
```

**Validation**: `just backend build`

---

### Task 2: Add Asset History Error Messages
**File**: `backend/internal/apierrors/messages.go`
**Action**: MODIFY
**Pattern**: Follow existing Report error messages (lines 152-156)

**Implementation**:
Add to the Report error messages const block:
```go
// Report error messages
const (
    ReportCurrentLocationsFailed = "Failed to list current locations"
    ReportCurrentLocationsCount  = "Failed to count current locations"
    ReportAssetHistoryFailed     = "Failed to get asset history"
    ReportAssetHistoryCount      = "Failed to count asset history"
    ReportAssetNotFound          = "Asset not found"
    ReportInvalidAssetID         = "Invalid asset ID: %s"
    ReportInvalidDateFormat      = "Invalid date format"
)
```

**Validation**: `just backend build`

---

### Task 3: Add Storage Methods for Asset History
**File**: `backend/internal/storage/reports.go`
**Action**: MODIFY
**Pattern**: Follow `ListCurrentLocations`/`CountCurrentLocations` pattern (lines 29-84)

**Implementation**:
```go
// ListAssetHistory returns paginated location history for a single asset
func (s *Storage) ListAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) ([]report.AssetHistoryItem, error) {
    query := `
        WITH scans AS (
            SELECT
                s.timestamp,
                s.location_id,
                l.name AS location_name,
                LEAD(s.timestamp) OVER (ORDER BY s.timestamp) AS next_timestamp
            FROM trakrf.asset_scans s
            LEFT JOIN trakrf.locations l ON l.id = s.location_id
            WHERE s.asset_id = $1
              AND s.org_id = $2
              AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
              AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
        )
        SELECT
            timestamp,
            location_id,
            location_name,
            EXTRACT(EPOCH FROM (next_timestamp - timestamp))::INT AS duration_seconds
        FROM scans
        ORDER BY timestamp DESC
        LIMIT $5 OFFSET $6
    `
    // ... scan rows into []report.AssetHistoryItem
}

// CountAssetHistory returns total count for pagination
func (s *Storage) CountAssetHistory(ctx context.Context, assetID, orgID int, filter report.AssetHistoryFilter) (int, error) {
    query := `
        SELECT COUNT(*)
        FROM trakrf.asset_scans s
        WHERE s.asset_id = $1
          AND s.org_id = $2
          AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
          AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
    `
    // ... return count
}
```

**Validation**: `just backend build`

---

### Task 4: Create Asset History Handler
**File**: `backend/internal/handlers/reports/asset_history.go`
**Action**: CREATE
**Pattern**: Reference `current_locations.go` (lines 46-113)

**Implementation**:
```go
package reports

// GetAssetHistory handles GET /api/v1/reports/assets/{id}/history
func (h *Handler) GetAssetHistory(w http.ResponseWriter, r *http.Request) {
    // 1. Get org from claims (same as current_locations.go:49-56)
    // 2. Parse path param: id (asset ID) using chi.URLParam
    // 3. Validate asset exists and belongs to org using storage.GetAssetByID
    //    - Return 404 if not found or wrong org
    // 4. Parse query params: limit, offset, start_date, end_date
    //    - Default limit=50, max=100
    //    - Default date range: last 30 days
    // 5. Fetch identifiers using storage.GetIdentifiersByAssetID
    //    - Find first active identifier for response
    // 6. Call storage.ListAssetHistory and CountAssetHistory
    // 7. Build and return AssetHistoryResponse
}
```

**Key logic**:
- Default date range: `startDate = now - 30 days`, `endDate = now`
- Parse ISO 8601 dates with `time.Parse(time.RFC3339, ...)`
- Use existing `GetAssetByID` to validate asset + get name
- Use existing `GetIdentifiersByAssetID` to get active identifier

**Validation**: `just backend build`

---

### Task 5: Register Asset History Route
**File**: `backend/internal/handlers/reports/current_locations.go`
**Action**: MODIFY
**Pattern**: Add to `RegisterRoutes` function (line 116-118)

**Implementation**:
```go
// RegisterRoutes registers report handler routes
func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Get("/api/v1/reports/current-locations", h.ListCurrentLocations)
    r.Get("/api/v1/reports/assets/{id}/history", h.GetAssetHistory)
}
```

**Validation**: `just backend build && just backend test`

---

### Task 6: Create Unit Tests
**File**: `backend/internal/handlers/reports/asset_history_test.go`
**Action**: CREATE
**Pattern**: Reference `current_locations_test.go`

**Test cases**:
1. `TestGetAssetHistory_MissingOrgContext` - 401 without org
2. `TestGetAssetHistory_InvalidAssetID` - 400 for non-numeric ID
3. `TestGetAssetHistory_DefaultPagination` - verify defaults (50/100)
4. `TestGetAssetHistory_LimitCapping` - verify max 100
5. `TestGetAssetHistory_DefaultDateRange` - verify 30-day default
6. `TestGetAssetHistory_RouteRegistration` - verify route registered
7. `TestAssetHistoryItem_JSON` - verify JSON serialization
8. `TestAssetHistoryItem_NullableFields` - verify null handling
9. `TestAssetHistoryResponse_EmptyData` - verify empty array (not null)

**Validation**: `just backend test`

---

### Task 7: Create Integration Tests
**File**: `backend/internal/handlers/reports/asset_history_integration_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/assets/assets_integration_test.go`

**Test cases**:
1. `TestGetAssetHistory_Success` - full happy path with test data
2. `TestGetAssetHistory_AssetNotFound` - 404 for non-existent asset
3. `TestGetAssetHistory_WrongOrg` - 404 for asset in different org
4. `TestGetAssetHistory_NoScans` - empty data array
5. `TestGetAssetHistory_DateRangeFilter` - verify date filtering
6. `TestGetAssetHistory_Pagination` - verify limit/offset work
7. `TestGetAssetHistory_DurationCalculation` - verify LEAD() works

**Setup requirements**:
- Use `//go:build integration` tag
- Use `testutil.SetupTestDB` for database
- Create test asset with scans at known timestamps
- Verify duration_seconds calculated correctly

**Validation**: `just backend test` (unit) + `go test -tags=integration ./internal/handlers/reports/...` (integration)

---

### Task 8: Final Validation and Cleanup
**Action**: VERIFY

**Checklist**:
- [ ] All unit tests pass: `just backend test`
- [ ] All integration tests pass: `go test -tags=integration ./internal/handlers/reports/...`
- [ ] Lint clean: `just backend lint`
- [ ] Build succeeds: `just backend build`
- [ ] No console.log/debug statements
- [ ] Route responds correctly (manual test with curl if dev server available)

**Validation**: `just backend validate`

---

## Risk Assessment

- **Risk**: Window function `LEAD()` ordering might be wrong (ASC vs DESC)
  **Mitigation**: Integration test specifically validates duration calculation with known timestamps

- **Risk**: Date parsing edge cases (timezones, formats)
  **Mitigation**: Use `time.RFC3339` for parsing, return 400 with clear error message on parse failure

- **Risk**: N+1 query for asset info + identifiers
  **Mitigation**: Acceptable for single-asset endpoint; only 2 extra queries regardless of result size

## Integration Points

- **Storage**: Adds 2 methods to `storage/reports.go`
- **Routes**: Adds 1 route to `RegisterRoutes`
- **Models**: Adds new DTOs in `models/report/`
- **Errors**: Adds constants to `apierrors/messages.go`

## VALIDATION GATES (MANDATORY)

After EVERY code change, run validation from `spec/stack.md`:

**Gate 1**: Lint - `just backend lint`
**Gate 2**: Build - `just backend build`
**Gate 3**: Unit Tests - `just backend test`

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: `just backend lint && just backend build`
After Task 6+: `just backend test`
Final validation: `just backend validate`

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and Linear ticket
- ✅ Directly mirrors TRA-217 pattern (same package, same structure)
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow at `current_locations_test.go`
- ✅ Window function `LEAD()` is standard SQL, well-documented
- ✅ No new dependencies
- ✅ Single subsystem (backend only)

**Assessment**: High confidence - this is a straightforward addition following established patterns from TRA-217.

**Estimated one-pass success probability**: 90%

**Reasoning**: Nearly identical structure to TRA-217 which shipped successfully. Main variable is integration test setup for asset_scans data, which may require some iteration.
