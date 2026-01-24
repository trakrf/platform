# Feature: TRA-217 Backend GET /api/reports/current-locations Endpoint

## Origin

This specification implements sub-issue TRA-217 of TRA-140 (Add basic asset location reporting). Provides the backend API for Report 1: Current Asset Locations - enabling NADA customer to see where AV equipment is currently located.

## Outcome

A new paginated API endpoint that returns the current (most recent) location for each asset in the organization, with filtering by location and search capabilities.

## User Story

As a **warehouse manager**
I want **to query an API for current asset locations**
So that **the frontend can display a "Current Asset Locations" report**

## Context

**Discovery (from TRA-140)**:
- NADA customer needs to see where AV equipment is located
- Data source is `asset_scans` hypertable (populated by TRA-137 trigger)
- Uses DISTINCT ON pattern to get latest scan per asset
- RLS automatically handles org filtering via `app.current_org_id`

**Current State**:
- `asset_scans` is a **TimescaleDB hypertable** with 7-day chunks and 365-day retention policy
- Indexes exist for `(org_id, timestamp DESC)`, `(asset_id, timestamp DESC)`, `(location_id, timestamp DESC)`
- No reports API exists yet
- Existing pagination pattern uses `limit`/`offset` (not `page`/`per_page`)

**TimescaleDB Considerations**:
- Hypertable partitions by `timestamp` - queries benefit from time constraints
- For "latest scan per asset", consider `last()` aggregate or `DISTINCT ON` pattern
- Future optimization: continuous aggregate for `current_asset_locations` (out of scope for MVP)

**Desired State**:
- New endpoint: `GET /api/v1/reports/current-locations`
- Returns paginated list of assets with their most recent location
- Supports filtering by location_id and text search

## Technical Requirements

### 1. Endpoint Definition

```
GET /api/v1/reports/current-locations
```

**Query Parameters**:

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `location_id` | int | - | Filter by specific location |
| `search` | string | - | Search asset name/identifier (ILIKE) |
| `limit` | int | 50 | Results per page (max 100) |
| `offset` | int | 0 | Pagination offset |

### 2. Response Schema

```json
{
  "data": [
    {
      "asset_id": 12345,
      "asset_name": "Laptop-001",
      "asset_identifier": "E2003412...",
      "location_id": 67890,
      "location_name": "Warehouse A - Rack 12",
      "last_seen": "2025-12-15T10:30:00Z"
    }
  ],
  "count": 50,
  "offset": 0,
  "total_count": 127
}
```

**Notes**:
- Response format aligns with existing `ListAssetsResponse` pattern
- `location_id` and `location_name` may be null (assets not yet scanned at a location)
- `asset_identifier` is the primary tag identifier for the asset

### 3. SQL Query Pattern

**Option A: DISTINCT ON (standard PostgreSQL)**
```sql
-- CTE to get latest scan per asset, then paginate
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
```

**Option B: TimescaleDB `last()` aggregate (potentially more efficient)**
```sql
-- Uses TimescaleDB's last() function for latest value per group
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
    -- ... rest of query same as Option A
```

**Recommendation**: Start with Option A (DISTINCT ON) for simplicity. Profile with `EXPLAIN ANALYZE` on production-scale data. Switch to Option B if needed - TimescaleDB's `last()` can be more efficient for hypertables with many chunks.

**Tested on preview** (7 rows, 2 chunks):
- DISTINCT ON: 0.26ms execution, uses `idx_asset_scans_org_time`
- last(): 0.44ms execution, uses `idx_asset_scans_org_time` with Finalize HashAggregate

**Count Query** (for total_count):
```sql
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
```

### 4. File Structure

Create new files following existing patterns:

```
backend/internal/
├── handlers/reports/
│   └── current_locations.go    # Handler with validation & response
├── storage/
│   └── reports.go              # Query methods
└── models/report/
    └── current_location.go     # DTOs
```

### 5. Handler Implementation

```go
// backend/internal/handlers/reports/current_locations.go
package reports

type CurrentLocationItem struct {
    AssetID         int       `json:"asset_id"`
    AssetName       string    `json:"asset_name"`
    AssetIdentifier string    `json:"asset_identifier"`
    LocationID      *int      `json:"location_id"`       // nullable
    LocationName    *string   `json:"location_name"`     // nullable
    LastSeen        time.Time `json:"last_seen"`
}

type CurrentLocationsResponse struct {
    Data       []CurrentLocationItem `json:"data"`
    Count      int                   `json:"count"`
    Offset     int                   `json:"offset"`
    TotalCount int                   `json:"total_count"`
}
```

**Handler Logic**:
1. Extract JWT claims, validate org context
2. Parse query params: `limit` (default 50, max 100), `offset` (default 0), `location_id`, `search`
3. Call storage layer
4. Return JSON response

### 6. Storage Layer

```go
// backend/internal/storage/reports.go
package storage

type CurrentLocationFilter struct {
    LocationID *int
    Search     *string
    Limit      int
    Offset     int
}

func (s *Storage) ListCurrentLocations(ctx context.Context, orgID int, filter CurrentLocationFilter) ([]report.CurrentLocationItem, error)

func (s *Storage) CountCurrentLocations(ctx context.Context, orgID int, filter CurrentLocationFilter) (int, error)
```

### 7. Route Registration

Add to `main.go` in the authenticated group:
```go
reportsHandler.RegisterRoutes(r)
```

## Edge Cases

| Scenario | Expected Behavior |
|----------|-------------------|
| Asset never scanned | Not included in results (no asset_scans row) |
| Asset scanned but no location | Include with null location_id/location_name |
| Empty search results | Return empty data array, total_count: 0 |
| Invalid location_id | Return empty results (filter finds nothing) |
| limit > 100 | Cap at 100 |
| offset beyond total | Return empty data array |

## Implementation Notes

- **RLS**: The CTE with DISTINCT ON operates on `asset_scans` which has RLS. Ensure `org_id` filter is applied explicitly (RLS via session var may not work in all contexts).
- **Performance**: The `idx_asset_scans_asset_time` index supports the DISTINCT ON query efficiently.
- **Nullable Fields**: Use pointer types for `location_id` and `location_name` to support null in JSON.
- **TimescaleDB Hypertable**: `asset_scans` is partitioned by timestamp (7-day chunks). The `DISTINCT ON` or `last()` queries work across all chunks. For very large datasets, consider:
  - Adding a time constraint (e.g., last 30 days) if acceptable for "current" definition
  - Future: continuous aggregate materialized view for `current_asset_locations`
- **Query Plan**: Run `EXPLAIN ANALYZE` on production data to verify index usage. TimescaleDB may chunk-skip effectively with org_id filter.

## Files to Create/Modify

1. **Create** `backend/internal/handlers/reports/current_locations.go`
   - Handler struct with storage dependency
   - `ListCurrentLocations` handler method
   - `RegisterRoutes` method

2. **Create** `backend/internal/storage/reports.go`
   - `ListCurrentLocations()` - paginated query
   - `CountCurrentLocations()` - total count

3. **Create** `backend/internal/models/report/current_location.go`
   - `CurrentLocationItem` DTO
   - `CurrentLocationsResponse` DTO
   - `CurrentLocationFilter` struct

4. **Modify** `backend/main.go`
   - Import reports handler
   - Register routes in authenticated group

5. **Create** `backend/internal/handlers/reports/current_locations_test.go`
   - Unit tests for handler

## Validation Criteria

- [ ] Endpoint returns 401 without valid JWT
- [ ] Endpoint returns 401 without org context in JWT
- [ ] Endpoint returns paginated list of current asset locations
- [ ] Default pagination: limit=50, offset=0
- [ ] Limit capped at 100
- [ ] `location_id` filter works correctly
- [ ] `search` filter matches asset name (case-insensitive)
- [ ] `search` filter matches asset identifier (case-insensitive)
- [ ] Response includes asset and location names (not just IDs)
- [ ] Empty results return empty array (not null)
- [ ] `total_count` reflects filtered count
- [ ] Query uses `idx_asset_scans_asset_time` index efficiently (verify with EXPLAIN ANALYZE)

## References

- **Parent Issue**: TRA-140 (Add basic asset location reporting)
- **Data Source**: TRA-137 (Save inventory to database) - populates asset_scans
- **Related Frontend**: TRA-219 (Frontend: Reports page + Current Locations table)
- **Codebase Patterns**:
  - `backend/internal/handlers/assets/assets.go:255-321` - pagination pattern
  - `backend/internal/storage/assets.go:200-250` - list storage pattern
  - `backend/migrations/000011_asset_scans.up.sql` - hypertable schema
- **TimescaleDB Docs**:
  - [last() aggregate](https://docs.timescale.com/api/latest/hyperfunctions/last/) - get last value in ordered set
  - [Continuous aggregates](https://docs.timescale.com/use-timescale/latest/continuous-aggregates/) - future optimization path
