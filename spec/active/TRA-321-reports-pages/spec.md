# Feature: TRA-321 Reports Pages - Routes, Data & UI

## Origin

Sub-issue of TRA-219 (Frontend: Reports page + Current Locations table). Implements the Reports section in the frontend with routing, data fetching, and UI for Current Locations and Asset History views.

## Outcome

A new Reports section in the app navigation with two views:
1. **Current Locations** (`#reports`) - Table showing all assets with their current location
2. **Asset History** (`#reports-history?id=X`) - Single asset's movement timeline

## User Story

As a **warehouse manager**
I want **to view where all my assets currently are and see their movement history**
So that **I can track equipment locations and audit their movements over time**

---

## Backend Contracts

### Endpoint 1: Current Locations (TRA-217 - MERGED)

```
GET /api/v1/reports/current-locations
```

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 50 | Results per page (max 100) |
| `offset` | int | 0 | Pagination offset |
| `location_id` | int | - | Filter by location ID |
| `search` | string | - | Search asset name/identifier |

**Response Schema:**
```json
{
  "data": [
    {
      "asset_id": 123,
      "asset_name": "Projector A1",
      "asset_identifier": "AST-001",
      "location_id": 1,
      "location_name": "Room 101",
      "last_seen": "2025-01-27T10:30:00Z"
    }
  ],
  "count": 1,
  "offset": 0,
  "total_count": 247
}
```

**Backend Model Reference:** `backend/internal/models/report/current_location.go`
```go
type CurrentLocationItem struct {
    AssetID         int       `json:"asset_id"`
    AssetName       string    `json:"asset_name"`
    AssetIdentifier string    `json:"asset_identifier"`
    LocationID      *int      `json:"location_id"`   // nullable
    LocationName    *string   `json:"location_name"` // nullable
    LastSeen        time.Time `json:"last_seen"`
}

type CurrentLocationsResponse struct {
    Data       []CurrentLocationItem `json:"data"`
    Count      int                   `json:"count"`
    Offset     int                   `json:"offset"`
    TotalCount int                   `json:"total_count"`
}
```

---

### Endpoint 2: Asset History (TRA-218 - MERGED)

```
GET /api/v1/reports/assets/:id/history
```

**Path Parameters:**
| Param | Type | Description |
|-------|------|-------------|
| `id` | int | Asset ID |

**Query Parameters:**
| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `limit` | int | 100 | Results per page (max 500) |
| `offset` | int | 0 | Pagination offset |
| `start_date` | ISO datetime | 30 days ago | Filter scans after this time |
| `end_date` | ISO datetime | now | Filter scans before this time |

**Response Schema:**
```json
{
  "asset": {
    "id": 123,
    "name": "Projector A1",
    "identifier": "AST-001"
  },
  "data": [
    {
      "timestamp": "2025-01-27T10:30:00Z",
      "location_id": 1,
      "location_name": "Room 101",
      "duration_seconds": 3600
    }
  ],
  "count": 45,
  "offset": 0,
  "total_count": 45
}
```

**Backend Model Reference:** `backend/internal/models/report/asset_history.go`
```go
type AssetInfo struct {
    ID         int    `json:"id"`
    Name       string `json:"name"`
    Identifier string `json:"identifier"`
}

type AssetHistoryItem struct {
    Timestamp       time.Time `json:"timestamp"`
    LocationID      *int      `json:"location_id"`
    LocationName    *string   `json:"location_name"`
    DurationSeconds *int      `json:"duration_seconds"`
}

type AssetHistoryResponse struct {
    Asset      AssetInfo          `json:"asset"`
    Data       []AssetHistoryItem `json:"data"`
    Count      int                `json:"count"`
    Offset     int                `json:"offset"`
    TotalCount int                `json:"total_count"`
}
```

**Error Responses:**
- `404` - Asset not found or belongs to different org
- `400` - Invalid asset ID or date format

---

## Pre-existing Frontend Schemas

### Location (from `frontend/src/types/locations/index.ts`)
Used for location filter dropdown.

```typescript
interface Location {
  id: number;
  name: string;
  identifier: string;
  // ... other fields
}

interface ListLocationsResponse {
  data: Location[];
  count: number;
  offset: number;
  total_count: number;
}
```

### Shared Types (from `frontend/src/types/shared/identifier.ts`)
```typescript
type IdentifierType = 'rfid';

interface TagIdentifier {
  id: number;
  type: IdentifierType;
  value: string;
  is_active: boolean;
}
```

---

## New Frontend Schemas (to create)

### File: `frontend/src/types/reports/index.ts`

```typescript
/**
 * Report Types
 *
 * Type definitions for report endpoints.
 * Backend: backend/internal/models/report/
 */

// ============ Current Locations (TRA-217) ============

/**
 * Single asset's current location
 * Backend: report.CurrentLocationItem
 */
export interface CurrentLocationItem {
  asset_id: number;
  asset_name: string;
  asset_identifier: string;
  location_id: number | null;
  location_name: string | null;
  last_seen: string; // ISO 8601
}

/**
 * Paginated response for current locations
 * Backend: report.CurrentLocationsResponse
 */
export interface CurrentLocationsResponse {
  data: CurrentLocationItem[];
  count: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for current locations
 */
export interface CurrentLocationsParams {
  limit?: number;
  offset?: number;
  location_id?: number;
  search?: string;
}

// ============ Asset History (TRA-218) ============

/**
 * Asset summary in history response
 * Backend: report.AssetInfo
 */
export interface AssetInfo {
  id: number;
  name: string;
  identifier: string;
}

/**
 * Single history entry for an asset
 * Backend: report.AssetHistoryItem
 */
export interface AssetHistoryItem {
  timestamp: string; // ISO 8601
  location_id: number | null;
  location_name: string | null;
  duration_seconds: number | null;
}

/**
 * Paginated response for asset history
 * Backend: report.AssetHistoryResponse
 */
export interface AssetHistoryResponse {
  asset: AssetInfo;
  data: AssetHistoryItem[];
  count: number;
  offset: number;
  total_count: number;
}

/**
 * Query params for asset history
 */
export interface AssetHistoryParams {
  limit?: number;
  offset?: number;
  start_date?: string; // ISO datetime
  end_date?: string; // ISO datetime
}

// ============ UI Types ============

/**
 * Freshness status derived from last_seen
 * Used for status badges in UI
 */
export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
```

---

## Implementation Plan

### Phase 1: Types & API Client

1. **Create** `frontend/src/types/reports/index.ts` - TypeScript types
2. **Update** `frontend/src/types/index.ts` - Export reports types
3. **Create** `frontend/src/lib/api/reports/index.ts` - API client methods
4. **Create** `frontend/src/lib/reports/utils.ts` - Helper functions (freshness, duration formatting)
5. **Create** `frontend/src/lib/reports/mocks.ts` - Mock data for testing

### Phase 2: React Query Hooks

1. **Create** `frontend/src/hooks/reports/useCurrentLocations.ts`
2. **Create** `frontend/src/hooks/reports/useAssetHistory.ts`
3. **Create** `frontend/src/hooks/reports/index.ts` - Exports

### Phase 3: Routing & Navigation

1. **Edit** `frontend/src/stores/uiStore.ts` - Add `reports`, `reports-history` to TabType
2. **Edit** `frontend/src/App.tsx` - Add screen imports, VALID_TABS, tabComponents
3. **Edit** `frontend/src/components/TabNavigation.tsx` - Add Reports NavItem (BarChart3 icon)

### Phase 4: UI Components

1. **Create** `frontend/src/components/reports/FreshnessBadge.tsx` - Status badge
2. **Create** `frontend/src/components/reports/CurrentLocationsTable.tsx` - Table using DataTable
3. **Create** `frontend/src/components/reports/AssetHistoryTable.tsx` - History table
4. **Create** `frontend/src/components/ReportsScreen.tsx` - Current Locations page
5. **Create** `frontend/src/components/ReportsHistoryScreen.tsx` - Asset History page

---

## Mock Data (for testing without backend data)

```typescript
export const mockCurrentLocations: CurrentLocationItem[] = [
  {
    asset_id: 1,
    asset_name: 'Projector A1',
    asset_identifier: 'AST-001',
    location_id: 1,
    location_name: 'Room 101',
    last_seen: new Date(Date.now() - 2 * 60 * 1000).toISOString(), // 2 min ago - live
  },
  {
    asset_id: 2,
    asset_name: 'Laptop Cart B',
    asset_identifier: 'AST-002',
    location_id: 2,
    location_name: 'Storage',
    last_seen: new Date(Date.now() - 15 * 60 * 1000).toISOString(), // 15 min ago - today
  },
  // ... more mock data
];

export const mockAssetHistory: AssetHistoryResponse = {
  asset: { id: 1, name: 'Projector A1', identifier: 'AST-001' },
  data: [
    { timestamp: '...', location_id: 1, location_name: 'Room 101', duration_seconds: null },
    { timestamp: '...', location_id: 2, location_name: 'Storage', duration_seconds: 2700 },
  ],
  count: 2,
  offset: 0,
  total_count: 2,
};
```

---

## Testing Strategy

### Unit Tests (Vitest)

1. **Utility functions** - `getFreshnessStatus`, `formatDuration`, `formatRelativeTime`
2. **API client** - Mock axios responses
3. **Hooks** - Mock React Query

### E2E Tests (Playwright MCP)

Test the complete UI flow using Playwright MCP tools:

1. **Navigation Test**
   - Navigate to `#reports` using `playwright_navigate`
   - Take screenshot to verify Reports tab is highlighted
   - Verify page title shows "Current Asset Locations"

2. **Current Locations Table Test**
   - Use `playwright_get_visible_text` to verify table headers
   - Verify data rows render (or empty state if no data)
   - Check for status badges (Live/Today/Recent/Stale)

3. **Row Click Navigation Test**
   - Use `playwright_click` on a table row
   - Verify URL changes to `#reports-history?id=X`
   - Take screenshot to verify asset history page loads

4. **Back Navigation Test**
   - Click back button using `playwright_click`
   - Verify returns to `#reports`

5. **Empty State Test**
   - With no data, verify empty state message using `playwright_get_visible_text`

---

## Files to Create

| File | Purpose |
|------|---------|
| `frontend/src/types/reports/index.ts` | TypeScript types |
| `frontend/src/lib/api/reports/index.ts` | API client |
| `frontend/src/lib/reports/utils.ts` | Helper functions |
| `frontend/src/lib/reports/mocks.ts` | Mock data for testing |
| `frontend/src/hooks/reports/useCurrentLocations.ts` | Query hook |
| `frontend/src/hooks/reports/useAssetHistory.ts` | Query hook |
| `frontend/src/hooks/reports/index.ts` | Hook exports |
| `frontend/src/components/ReportsScreen.tsx` | Current Locations page |
| `frontend/src/components/ReportsHistoryScreen.tsx` | Asset History page |
| `frontend/src/components/reports/CurrentLocationsTable.tsx` | Table component |
| `frontend/src/components/reports/AssetHistoryTable.tsx` | History table |
| `frontend/src/components/reports/FreshnessBadge.tsx` | Status badge |

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/stores/uiStore.ts` | Add `'reports' \| 'reports-history'` to TabType |
| `frontend/src/App.tsx` | Add screen imports, VALID_TABS, tabComponents |
| `frontend/src/components/TabNavigation.tsx` | Add Reports NavItem with BarChart3 icon |
| `frontend/src/types/index.ts` | Export reports types |

---

## Validation Criteria

- [ ] `#reports` route loads Current Locations page
- [ ] Reports tab highlighted in navigation
- [ ] Current Locations table displays data
- [ ] Status badges show correct freshness (Live/Today/Recent/Stale)
- [ ] Search filter works (300ms debounce)
- [ ] Pagination works
- [ ] Row click navigates to `#reports-history?id=X`
- [ ] Asset History page shows asset details and timeline
- [ ] Duration formatted correctly (2h 15m)
- [ ] Back button returns to reports
- [ ] Empty states display appropriately
- [ ] Mobile responsive (cards on mobile)
- [ ] TypeScript types match backend contracts
- [ ] Playwright E2E tests pass

---

## References

- **Parent Issue**: TRA-219 (Frontend: Reports page + Current Locations table)
- **Backend Specs**: `spec/active/TRA-218-asset-history-endpoint/spec.md`
- **Mockup**: https://trakrf.github.io/platform/mockup-c-dashboard.html
- **Frontend Patterns**:
  - `frontend/src/components/AssetsScreen.tsx` - Screen pattern
  - `frontend/src/components/shared/DataTable.tsx` - Table component
  - `frontend/src/hooks/locations/useLocations.ts` - Hook pattern
  - `frontend/src/lib/api/locations/index.ts` - API client pattern
