# Feature: TRA-218 Backend GET /api/reports/assets/:id/history Endpoint

## Origin

This specification implements sub-issue TRA-218 of TRA-140 (Add basic asset location reporting). Provides the backend API for Report 2: Asset Location History - enabling NADA customer to see where a specific asset has been over time.

## Outcome

A new paginated API endpoint that returns the location history for a single asset, with date range filtering and duration calculation between scans.

## User Story

As a **warehouse manager**
I want **to query an API for an asset's location history**
So that **the frontend can display an "Asset Location History" report showing where equipment has been**

## Context

**Discovery (from TRA-140)**:
- NADA customer needs to track where AV equipment has been
- Data source is `asset_scans` hypertable (populated by TRA-137 trigger)
- Uses window function `LEAD()` to calculate duration at each location
- RLS automatically handles org filtering via `app.current_org_id`

**Current State**:
- `asset_scans` is a **TimescaleDB hypertable** with 7-day chunks and 365-day retention policy
- Indexes exist for `(org_id, timestamp DESC)`, `(asset_id, timestamp DESC)`, `(location_id, timestamp DESC)`
- Reports API now exists with `GET /api/v1/reports/current-locations` (TRA-217)
- Existing pagination pattern uses `limit`/`offset` (not `page`/`per_page`)

**TimescaleDB Considerations**:
- Hypertable partitions by `timestamp` - queries benefit from time constraints
- Window functions like `LEAD()` work across all chunks
- Date range filters enable chunk skipping for efficient queries

**Desired State**:
- New endpoint: `GET /api/v1/reports/assets/:id/history`
- Returns paginated location history for a single asset
- Supports date range filtering (`start_date`, `end_date`)
- Calculates duration at each location using window function

## Technical Requirements

### 1. Endpoint Definition

```
GET /api/v1/reports/assets/:id/history
```

**Path Parameters**:

| Param | Type | Description |
|-------|------|-------------|
| `id` | int | Asset ID to get history for |

**Query Parameters**:

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `start_date` | ISO datetime | 30 days ago | Filter scans after this time |
| `end_date` | ISO datetime | now | Filter scans before this time |
| `limit` | int | 100 | Results per page (max 500) |
| `offset` | int | 0 | Pagination offset |

### 2. Response Schema

```json
{
  "asset": {
    "id": 12345,
    "name": "Laptop-001",
    "identifier": "E2003412..."
  },
  "data": [
    {
      "timestamp": "2025-12-15T10:30:00Z",
      "location_id": 67890,
      "location_name": "Warehouse A - Rack 12",
      "duration_seconds": 3600
    }
  ],
  "count": 45,
  "offset": 0,
  "total_count": 45
}
```

**Notes**:
- Response includes asset metadata for display purposes
- `duration_seconds` is time until next scan (null for most recent entry)
- `location_id` and `location_name` may be null (scan without location)
- Results ordered by timestamp descending (newest first)

### 3. SQL Query Pattern

```sql
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
```

**Notes**:
- Window function `LEAD()` computes next timestamp for duration calculation
- Duration is `NULL` for the most recent scan (no next timestamp)
- Order by timestamp DESC for most recent first
- Date range filters apply to window function scope

**Count Query** (for total_count):
```sql
SELECT COUNT(*)
FROM trakrf.asset_scans s
WHERE s.asset_id = $1
  AND s.org_id = $2
  AND ($3::timestamptz IS NULL OR s.timestamp >= $3)
  AND ($4::timestamptz IS NULL OR s.timestamp <= $4)
```

**Asset Query** (for response header):
```sql
SELECT
    a.id,
    a.name,
    COALESCE(
        (SELECT i.value FROM trakrf.identifiers i
         WHERE i.asset_id = a.id AND i.is_active = true LIMIT 1),
        ''
    ) AS identifier
FROM trakrf.assets a
WHERE a.id = $1 AND a.org_id = $2
```

### 4. File Structure

Add to existing reports package:

```
backend/internal/
├── handlers/reports/
│   ├── current_locations.go      # Existing (TRA-217)
│   └── asset_history.go          # NEW: Handler for this endpoint
├── storage/
│   └── reports.go                # Add new methods
└── models/report/
    ├── current_location.go       # Existing (TRA-217)
    └── asset_history.go          # NEW: DTOs for history
```

### 5. Handler Implementation

```go
// backend/internal/handlers/reports/asset_history.go
package reports

type AssetInfo struct {
    ID         int    `json:"id"`
    Name       string `json:"name"`
    Identifier string `json:"identifier"`
}

type AssetHistoryItem struct {
    Timestamp       time.Time `json:"timestamp"`
    LocationID      *int      `json:"location_id"`       // nullable
    LocationName    *string   `json:"location_name"`     // nullable
    DurationSeconds *int      `json:"duration_seconds"`  // nullable (most recent)
}

type AssetHistoryResponse struct {
    Asset      AssetInfo          `json:"asset"`
    Data       []AssetHistoryItem `json:"data"`
    Count      int                `json:"count"`
    Offset     int                `json:"offset"`
    TotalCount int                `json:"total_count"`
}
```

**Handler Logic**:
1. Extract JWT claims, validate org context
2. Parse path param: `id` (asset ID)
3. Parse query params: `limit` (default 100, max 500), `offset` (default 0), `start_date`, `end_date`
4. Validate asset exists and belongs to org (return 404 if not)
5. Call storage layer for history data
6. Return JSON response with asset metadata

### 6. Storage Layer

Add to existing `backend/internal/storage/reports.go`:

```go
type AssetHistoryFilter struct {
    StartDate *time.Time
    EndDate   *time.Time
    Limit     int
    Offset    int
}

func (s *Storage) GetAssetInfo(ctx context.Context, assetID, orgID int) (*report.AssetInfo, error)

func (s *Storage) ListAssetHistory(ctx context.Context, assetID, orgID int, filter AssetHistoryFilter) ([]report.AssetHistoryItem, error)

func (s *Storage) CountAssetHistory(ctx context.Context, assetID, orgID int, filter AssetHistoryFilter) (int, error)
```

### 7. Route Registration

Add to `RegisterRoutes` in reports handler:
```go
r.Get("/api/v1/reports/assets/{id}/history", h.GetAssetHistory)
```

## Edge Cases

| Scenario | Expected Behavior |
|----------|-------------------|
| Asset not found | Return 404 with "Asset not found" message |
| Asset belongs to different org | Return 404 (same as not found for security) |
| Asset has no scans | Return empty data array, total_count: 0 |
| No scans in date range | Return empty data array, total_count: 0 |
| Invalid asset_id format | Return 400 "Invalid asset ID" |
| Invalid date format | Return 400 "Invalid date format" |
| limit > 500 | Cap at 500 |
| offset beyond total | Return empty data array |
| Most recent scan | `duration_seconds` is null |
| Scan without location | Include with null location_id/location_name |

## Implementation Notes

- **Asset Validation**: Always check asset exists AND belongs to org before querying history
- **Window Function Scope**: The `LEAD()` function operates within the date-filtered CTE, so duration is relative to filtered results
- **Performance**: The `idx_asset_scans_asset_time` index efficiently supports single-asset queries
- **Default Date Range**: If no dates provided, default to last 30 days to prevent unbounded queries
- **Nullable Duration**: Most recent scan has no "next" timestamp, so duration is null
- **Timestamp Ordering**: Return newest first (DESC) for typical UI display

## Files to Create/Modify

1. **Create** `backend/internal/handlers/reports/asset_history.go`
   - `GetAssetHistory` handler method
   - Add route to `RegisterRoutes`

2. **Create** `backend/internal/models/report/asset_history.go`
   - `AssetInfo` DTO
   - `AssetHistoryItem` DTO
   - `AssetHistoryResponse` DTO
   - `AssetHistoryFilter` struct

3. **Modify** `backend/internal/storage/reports.go`
   - Add `GetAssetInfo()` method
   - Add `ListAssetHistory()` method
   - Add `CountAssetHistory()` method

4. **Create** `backend/internal/handlers/reports/asset_history_test.go`
   - Unit tests for handler

## Validation Criteria

- [ ] Endpoint returns 401 without valid JWT
- [ ] Endpoint returns 401 without org context in JWT
- [ ] Endpoint returns 404 for non-existent asset
- [ ] Endpoint returns 404 for asset in different org
- [ ] Endpoint returns paginated location history for asset
- [ ] Default pagination: limit=100, offset=0
- [ ] Limit capped at 500
- [ ] `start_date` filter works correctly
- [ ] `end_date` filter works correctly
- [ ] Default date range is last 30 days
- [ ] `duration_seconds` calculated correctly using window function
- [ ] Most recent scan has null `duration_seconds`
- [ ] Response includes asset metadata (id, name, identifier)
- [ ] Empty results return empty array (not null)
- [ ] `total_count` reflects filtered count
- [ ] Query uses `idx_asset_scans_asset_time` index efficiently

## References

- **Parent Issue**: TRA-140 (Add basic asset location reporting)
- **Related Backend**: TRA-217 (Current Locations endpoint) - same package
- **Related Frontend**: TRA-220 (Frontend: Asset History view)
- **Codebase Patterns**:
  - `backend/internal/handlers/reports/current_locations.go` - handler pattern
  - `backend/internal/storage/reports.go` - storage pattern
  - `backend/internal/handlers/assets/assets.go:130-170` - path param parsing
- **TimescaleDB Docs**:
  - [Window functions](https://docs.timescale.com/use-timescale/latest/query-data/advanced-analytic-queries/) - LEAD() usage
