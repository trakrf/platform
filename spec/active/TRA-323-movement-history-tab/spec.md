# Feature: TRA-323 Movement History Tab (Asset-Centric View)

## Origin

Sub-issue of TRA-219 (Frontend: Reports page). Implements the Movement History tab that shows where a selected asset has been over time.

## Outcome

A new **Movement History** tab in the Reports screen that provides an asset-centric view: select an asset → see all locations it has been to with a visual timeline.

## User Story

As a **warehouse manager**
I want **to select any asset and see its complete movement history**
So that **I can audit where equipment has been and track its location patterns over time**

---

## Tab Structure Context

The Reports screen has two tabs with inverse views:

| Tab | Select | Shows | Question Answered |
|-----|--------|-------|-------------------|
| Current Locations | Location | All assets at that location | "Which assets were at this location?" |
| **Movement History** | Asset | All locations for that asset | "Where has this asset been?" |

This ticket covers **Tab 2: Movement History**.

---

## Design Reference

**Mockup**: https://trakrf.github.io/platform/mockup-b-asset-history.html

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Reports                                                    [Org Selector ▼] │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Current Locations]  [Asset History]                                        │
│                       ^^^^^^^^^^^^ (active, underlined)                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Select Asset                          From            To                    │
│ [Sony PXW-FX9 Camera (CAM-001) ▼]    [01/20/2026 📅] [01/23/2026 📅] [Export CSV] │
├─────────────────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────────────────┐ │
│ │ Sony PXW-FX9 Camera              4              3d 5h        ● Studio A │ │
│ │ CAM-001                    Locations Visited  Time Tracked  Current Loc │ │
│ └─────────────────────────────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────────┤
│ Movement Timeline                                                           │
│                                                                             │
│ Jan 23, 2026  Today                                                         │
│   ● 10:30 AM  [NOW]                                                         │
│     Studio A                                                                │
│     ████████░░░░░░░░░░░░░░  2h 15m (ongoing)                               │
│                                                                             │
│   ○ 8:00 AM - 10:30 AM                                                      │
│     Equipment Room                                                          │
│     ████████░░░░░░░░░░░░░░  2h 30m                                         │
│                                                                             │
│ Jan 22, 2026  Yesterday                                                     │
│   ○ 2:00 PM - 6:00 PM                                                       │
│     Studio B                                                                │
│     ████████████░░░░░░░░░░  4h 00m                                         │
│                                                                             │
│ [Load more history]                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Backend Contracts (Already Implemented)

### Endpoint 1: Asset List (via Current Locations)

```
GET /api/v1/reports/current-locations?limit=1000
```

Used to populate the asset dropdown selector.

**Response Schema:**
```json
{
  "data": [
    {
      "asset_id": 123,
      "asset_name": "Sony PXW-FX9 Camera",
      "asset_identifier": "CAM-001",
      "location_id": 1,
      "location_name": "Studio A",
      "last_seen": "2026-01-23T10:30:00Z"
    }
  ],
  "count": 50,
  "offset": 0,
  "total_count": 50
}
```

### Endpoint 2: Asset History (TRA-218 - MERGED)

```
GET /api/v1/reports/assets/:id/history
```

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
    "name": "Sony PXW-FX9 Camera",
    "identifier": "CAM-001"
  },
  "data": [
    {
      "timestamp": "2026-01-23T10:30:00Z",
      "location_id": 1,
      "location_name": "Studio A",
      "duration_seconds": null
    },
    {
      "timestamp": "2026-01-23T08:00:00Z",
      "location_id": 2,
      "location_name": "Equipment Room",
      "duration_seconds": 9000
    }
  ],
  "count": 45,
  "offset": 0,
  "total_count": 45
}
```

---

## Pre-existing Frontend Code to Reuse

### Components
| Component | Location | Reuse For |
|-----------|----------|-----------|
| `MovementTimeline.tsx` | `components/reports/` | Timeline display (no changes needed) |
| `FreshnessBadge.tsx` | `components/reports/` | Current location status indicator |

### Hooks
| Hook | Location | Reuse For |
|------|----------|-----------|
| `useAssetHistory` | `hooks/reports/` | Fetching timeline data |
| `useCurrentLocations` | `hooks/reports/` | Getting asset list for dropdown |
| `useAssetDetailPanel` | `hooks/reports/` | Reference for state management pattern |

### Utilities
| Function | Location | Reuse For |
|----------|----------|-----------|
| `formatDuration` | `lib/reports/utils.ts` | Duration formatting |
| `formatDate` | `lib/reports/utils.ts` | Date headers |
| `formatTime` | `lib/reports/utils.ts` | Time display |
| `groupTimelineByDate` | `lib/reports/utils.ts` | Timeline grouping |
| `getAvatarColor` | `lib/reports/utils.ts` | Asset avatar colors |
| `getInitials` | `lib/reports/utils.ts` | Asset avatar initials |

---

## New Components to Create

### 1. AssetHistoryTab.tsx
**Location:** `frontend/src/components/reports/AssetHistoryTab.tsx`

Main container for the Movement History tab content.

**Structure:**
```tsx
<div className="flex-1 flex flex-col min-h-0">
  {/* Controls Row */}
  <div className="flex items-center gap-4 mb-4">
    <AssetSelector ... />
    <DateRangeInputs ... />
    <ExportCsvButton ... />
  </div>

  {/* Summary Card (when asset selected) */}
  {selectedAsset && <AssetSummaryCard ... />}

  {/* Timeline */}
  <MovementTimeline ... />
</div>
```

**Rules:**
- NO helper functions in component body
- NO useEffect in component body
- Only useState, useMemo, and custom hooks
- Pure presentational - all logic in `useAssetHistoryTab` hook

### 2. AssetSelector.tsx
**Location:** `frontend/src/components/reports/AssetSelector.tsx`

Dropdown to select which asset to view.

**Props:**
```typescript
interface AssetSelectorProps {
  value: number | null;
  onChange: (assetId: number | null) => void;
  assets: AssetOption[];
  isLoading: boolean;
  className?: string;
}

interface AssetOption {
  id: number;
  name: string;
  identifier: string;
}
```

**Display format:** `{name} ({identifier})` e.g., "Sony PXW-FX9 Camera (CAM-001)"

### 3. AssetSummaryCard.tsx
**Location:** `frontend/src/components/reports/AssetSummaryCard.tsx`

Horizontal card showing asset stats.

**Props:**
```typescript
interface AssetSummaryCardProps {
  assetName: string;
  assetIdentifier: string;
  locationsVisited: number;
  timeTracked: string;           // Formatted duration e.g., "3d 5h"
  currentLocation: string | null;
}
```

**Layout (from mockup):**
- Left: Asset name (bold, large) + identifier (gray, below)
- Center: Two stats with labels below
  - Locations Visited (number)
  - Time Tracked (formatted duration)
- Right: Current location with green dot indicator

### 4. DateRangeInputs.tsx
**Location:** `frontend/src/components/reports/DateRangeInputs.tsx`

From/To date picker inputs.

**Props:**
```typescript
interface DateRangeInputsProps {
  fromDate: string;              // YYYY-MM-DD format
  toDate: string;                // YYYY-MM-DD format
  onFromDateChange: (date: string) => void;
  onToDateChange: (date: string) => void;
}
```

**Implementation:** Native HTML date inputs (`<input type="date">`) styled to match design.

### 5. ExportCsvButton.tsx
**Location:** `frontend/src/components/reports/ExportCsvButton.tsx`

Button to export timeline data as CSV.

**Props:**
```typescript
interface ExportCsvButtonProps {
  data: AssetHistoryItem[];
  assetName: string;
  disabled?: boolean;
}
```

---

## New Hooks to Create

### 1. useAssetHistoryTab.ts
**Location:** `frontend/src/hooks/reports/useAssetHistoryTab.ts`

Main hook managing all tab state and logic.

**Returns:**
```typescript
interface UseAssetHistoryTabReturn {
  // Asset selection
  selectedAssetId: number | null;
  setSelectedAssetId: (id: number | null) => void;
  assetOptions: AssetOption[];
  isLoadingAssets: boolean;

  // Date range
  fromDate: string;
  toDate: string;
  setFromDate: (date: string) => void;
  setToDate: (date: string) => void;

  // Timeline data
  timelineData: AssetHistoryItem[];
  isLoadingTimeline: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  handleLoadMore: () => void;

  // Calculated stats
  stats: {
    locationsVisited: number;
    timeTracked: string;
    currentLocation: string | null;
  } | null;

  // Selected asset info
  selectedAsset: AssetOption | null;
}
```

**Logic:**
- Fetches asset list from `useCurrentLocations` (limit: 1000)
- Transforms to `AssetOption[]` format
- Uses existing `useAssetHistory` for timeline data
- Calculates stats from timeline data:
  - `locationsVisited`: `new Set(data.filter(d => d.location_id).map(d => d.location_id)).size`
  - `timeTracked`: Sum of all `duration_seconds`, formatted with `formatDuration`
  - `currentLocation`: First item's `location_name` (most recent)

### 2. useExportCsv.ts
**Location:** `frontend/src/hooks/reports/useExportCsv.ts`

Hook for CSV export functionality.

**Returns:**
```typescript
interface UseExportCsvReturn {
  exportToCsv: (data: AssetHistoryItem[], assetName: string) => void;
  isExporting: boolean;
}
```

---

## New Utility Functions

### lib/reports/exportCsv.ts
**Location:** `frontend/src/lib/reports/exportCsv.ts`

```typescript
/**
 * Generate CSV content from asset history data
 */
export function generateHistoryCsv(
  data: AssetHistoryItem[],
  assetName: string
): string;

/**
 * Trigger browser download of CSV file
 */
export function downloadCsv(content: string, filename: string): void;
```

**CSV Format:**
```csv
Asset,Timestamp,Location,Duration
Sony PXW-FX9 Camera,2026-01-23T10:30:00Z,Studio A,ongoing
Sony PXW-FX9 Camera,2026-01-23T08:00:00Z,Equipment Room,2h 30m
```

---

## Integration with ReportsScreen.tsx

Replace the placeholder in `frontend/src/components/ReportsScreen.tsx`:

```tsx
// Before (lines 239-245)
{activeTab === 'movement' && (
  <EmptyState
    icon={FileText}
    title="Coming Soon"
    description="Movement History report will be available in a future update."
  />
)}

// After
{activeTab === 'movement' && <AssetHistoryTab />}
```

Also rename the tab label from "Movement History" to "Asset History" to match mockup (line 86).

---

## Files to Create

| File | Purpose |
|------|---------|
| `frontend/src/components/reports/AssetHistoryTab.tsx` | Main tab content |
| `frontend/src/components/reports/AssetSelector.tsx` | Asset dropdown |
| `frontend/src/components/reports/AssetSummaryCard.tsx` | Stats card |
| `frontend/src/components/reports/DateRangeInputs.tsx` | From/To date pickers |
| `frontend/src/components/reports/ExportCsvButton.tsx` | CSV export button |
| `frontend/src/hooks/reports/useAssetHistoryTab.ts` | Main tab hook |
| `frontend/src/hooks/reports/useExportCsv.ts` | CSV export hook |
| `frontend/src/lib/reports/exportCsv.ts` | CSV generation utilities |

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/ReportsScreen.tsx` | Replace placeholder with `<AssetHistoryTab />`, rename tab label |
| `frontend/src/hooks/reports/index.ts` | Export new hooks |

---

## Implementation Rules

1. **No helper functions in .tsx files** - Extract to hooks or utils
2. **No useEffect in .tsx files** - Use hooks that encapsulate effects
3. **Only useState, useMemo in components** - Keep components pure presentational
4. **Follow mockup exactly** - Match layout, spacing, colors, typography
5. **Reuse existing components** - MovementTimeline, FreshnessBadge
6. **Take patterns from AssetDetailPanel** - Similar data flow and state management

---

## Testing Strategy

### Visual Testing (Playwright MCP)

1. **Empty State Test**
   - Navigate to Reports → Asset History tab
   - Screenshot: verify empty state when no asset selected
   - Use `playwright_get_visible_text` to verify placeholder message

2. **Asset Selection Test**
   - Use `playwright_click` to open asset dropdown
   - Screenshot: verify dropdown shows assets with name + identifier
   - Select an asset
   - Screenshot: verify summary card and timeline appear

3. **Summary Card Test**
   - Verify asset name and identifier display
   - Verify stats: locations visited, time tracked, current location
   - Verify current location has green dot indicator

4. **Timeline Test**
   - Verify timeline groups by date with headers
   - Verify current location shows "NOW" badge
   - Verify duration progress bars display
   - Verify "Load more history" button works

5. **Date Range Test**
   - Change From/To dates using `playwright_fill`
   - Verify timeline updates with filtered data

6. **Export CSV Test**
   - Click Export CSV button
   - Verify download triggers (check for file in downloads)

7. **Mobile Responsive Test**
   - Use `playwright_resize` to mobile viewport
   - Screenshot: verify layout adapts appropriately

---

## Validation Criteria

- [ ] Asset History tab loads when clicked
- [ ] Asset dropdown shows all assets with name + identifier format
- [ ] Selecting asset displays summary card with correct stats
- [ ] Locations Visited count is accurate (unique locations)
- [ ] Time Tracked is sum of all durations, formatted correctly
- [ ] Current Location shows with green indicator
- [ ] Timeline displays grouped by date with headers
- [ ] Current location entry shows "NOW" badge
- [ ] Duration progress bars render correctly
- [ ] "Load more history" pagination works
- [ ] Date range inputs filter timeline data
- [ ] Export CSV downloads file with correct data
- [ ] Empty state when no asset selected
- [ ] Loading states during data fetch
- [ ] Mobile responsive layout
- [ ] All .tsx files are pure presentational (no helper functions, no useEffect)
- [ ] TypeScript compiles without errors
- [ ] ESLint passes
- [ ] All existing tests still pass

---

## References

- **Parent Issue**: TRA-219 (Frontend: Reports page)
- **Related**: TRA-321 (Reports Pages - implemented Current Locations tab)
- **Mockup**: https://trakrf.github.io/platform/mockup-b-asset-history.html
- **Frontend Patterns**:
  - `frontend/src/components/reports/AssetDetailPanel.tsx` - Similar data flow
  - `frontend/src/hooks/reports/useAssetDetailPanel.ts` - Hook pattern reference
  - `frontend/src/components/reports/MovementTimeline.tsx` - Reusable timeline
  - `frontend/src/lib/reports/utils.ts` - Utility functions
