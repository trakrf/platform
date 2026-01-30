# Implementation Plan: TRA-323 Movement History Tab

Generated: 2026-01-30
Specification: spec.md

## Understanding

Implement the **Asset History** tab (renamed from "Movement History") in the Reports screen. This is an asset-centric view that allows users to:
1. Select any asset from a searchable dropdown
2. Set custom date range with From/To date pickers
3. View movement timeline (reuse existing MovementTimeline component)
4. See summary stats (locations visited, time tracked, current location)
5. Export data as CSV

The tab is standalone - no side panel interaction.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `hooks/reports/useAssetDetailPanel.ts` (lines 34-117) - Hook pattern for managing timeline state, pagination, data accumulation
- `components/reports/AssetDetailPanel.tsx` (lines 13-189) - Pure presentational component pattern
- `components/locations/LocationParentSelector.tsx` (lines 73-88) - Native select styling pattern
- `components/locations/LocationForm.tsx` (lines ~140-150) - Native date input styling
- `lib/reports/utils.ts` - All formatting/grouping utilities

**Files to Create**:
- `frontend/src/components/reports/AssetHistoryTab.tsx` - Main tab container
- `frontend/src/components/reports/AssetSelector.tsx` - Searchable asset dropdown
- `frontend/src/components/reports/AssetSummaryCard.tsx` - Stats summary card
- `frontend/src/components/reports/DateRangeInputs.tsx` - From/To date pickers
- `frontend/src/components/reports/ExportCsvButton.tsx` - CSV export button
- `frontend/src/hooks/reports/useAssetHistoryTab.ts` - Main tab state hook
- `frontend/src/hooks/reports/useExportCsv.ts` - CSV export hook
- `frontend/src/lib/reports/exportCsv.ts` - CSV generation utilities

**Files to Modify**:
- `frontend/src/components/ReportsScreen.tsx` (line 86, 239-245) - Replace placeholder, rename tab
- `frontend/src/hooks/reports/index.ts` - Export new hooks

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create CSV Export Utilities
**File**: `frontend/src/lib/reports/exportCsv.ts`
**Action**: CREATE
**Pattern**: Pure utility functions

**Implementation**:
```typescript
// Generate CSV content from history data
export function generateHistoryCsv(
  data: AssetHistoryItem[],
  assetName: string
): string {
  const headers = ['Asset', 'Timestamp', 'Location', 'Duration'];
  const rows = data.map(item => [
    assetName,
    item.timestamp,
    item.location_name || 'Unknown',
    item.duration_seconds ? formatDuration(item.duration_seconds) : 'ongoing'
  ]);
  return [headers, ...rows].map(row => row.join(',')).join('\n');
}

// Trigger browser download
export function downloadCsv(content: string, filename: string): void {
  const blob = new Blob([content], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
```

**Validation**: `just frontend typecheck`

---

### Task 2: Create useExportCsv Hook
**File**: `frontend/src/hooks/reports/useExportCsv.ts`
**Action**: CREATE
**Pattern**: Reference `useAssetDetailPanel.ts` for hook structure

**Implementation**:
```typescript
export function useExportCsv() {
  const [isExporting, setIsExporting] = useState(false);

  const exportToCsv = useCallback((data: AssetHistoryItem[], assetName: string) => {
    setIsExporting(true);
    try {
      const csv = generateHistoryCsv(data, assetName);
      const filename = `${assetName.replace(/\s+/g, '-')}-history-${new Date().toISOString().split('T')[0]}.csv`;
      downloadCsv(csv, filename);
    } finally {
      setTimeout(() => setIsExporting(false), 500); // Brief delay for UX
    }
  }, []);

  return { exportToCsv, isExporting };
}
```

**Validation**: `just frontend typecheck`

---

### Task 3: Create useAssetHistoryTab Hook
**File**: `frontend/src/hooks/reports/useAssetHistoryTab.ts`
**Action**: CREATE
**Pattern**: Reference `useAssetDetailPanel.ts` (lines 34-117) for pagination and data accumulation

**Implementation**:
```typescript
const PAGE_SIZE = 20;

export function useAssetHistoryTab() {
  // Asset selection
  const [selectedAssetId, setSelectedAssetId] = useState<number | null>(null);

  // Date range (YYYY-MM-DD format for native inputs)
  const [fromDate, setFromDate] = useState(() => {
    const d = new Date();
    d.setDate(d.getDate() - 30);
    return d.toISOString().split('T')[0];
  });
  const [toDate, setToDate] = useState(() => new Date().toISOString().split('T')[0]);

  // Pagination state
  const [offset, setOffset] = useState(0);
  const [accumulatedData, setAccumulatedData] = useState<AssetHistoryItem[]>([]);
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Fetch asset list
  const { data: assetsData, isLoading: isLoadingAssets } = useCurrentLocations({ limit: 1000 });

  // Transform to AssetOption[]
  const assetOptions = useMemo(() =>
    assetsData.map(a => ({ id: a.asset_id, name: a.asset_name, identifier: a.asset_identifier })),
    [assetsData]
  );

  // Fetch timeline
  const historyParams = useMemo(() => ({
    limit: PAGE_SIZE,
    offset,
    start_date: new Date(fromDate).toISOString(),
    end_date: new Date(toDate + 'T23:59:59').toISOString(),
  }), [fromDate, toDate, offset]);

  const { data: historyData, totalCount, isLoading } = useAssetHistory(selectedAssetId, historyParams);

  // Data accumulation effects (encapsulated in hook, not in component)
  useEffect(() => { /* accumulate data */ }, [historyData, offset]);
  useEffect(() => { /* reset on filter change */ }, [selectedAssetId, fromDate, toDate]);

  // Calculate stats
  const stats = useMemo(() => {
    if (accumulatedData.length === 0) return null;
    const uniqueLocations = new Set(accumulatedData.filter(d => d.location_id).map(d => d.location_id));
    const totalSeconds = accumulatedData.reduce((sum, d) => sum + (d.duration_seconds || 0), 0);
    return {
      locationsVisited: uniqueLocations.size,
      timeTracked: formatDuration(totalSeconds),
      currentLocation: accumulatedData[0]?.location_name || null,
    };
  }, [accumulatedData]);

  // Selected asset info
  const selectedAsset = useMemo(() =>
    assetOptions.find(a => a.id === selectedAssetId) || null,
    [assetOptions, selectedAssetId]
  );

  const handleLoadMore = useCallback(() => {
    setIsLoadingMore(true);
    setOffset(prev => prev + PAGE_SIZE);
  }, []);

  return {
    selectedAssetId, setSelectedAssetId,
    assetOptions, isLoadingAssets,
    fromDate, toDate, setFromDate, setToDate,
    timelineData: accumulatedData,
    isLoadingTimeline: isLoading && offset === 0,
    hasMore: accumulatedData.length < totalCount,
    isLoadingMore, handleLoadMore,
    stats, selectedAsset,
  };
}
```

**Validation**: `just frontend typecheck`

---

### Task 4: Update Hooks Index
**File**: `frontend/src/hooks/reports/index.ts`
**Action**: MODIFY
**Pattern**: Match existing exports

**Implementation**:
```typescript
// Add exports:
export { useAssetHistoryTab } from './useAssetHistoryTab';
export { useExportCsv } from './useExportCsv';
```

**Validation**: `just frontend typecheck`

---

### Task 5: Create AssetSelector Component
**File**: `frontend/src/components/reports/AssetSelector.tsx`
**Action**: CREATE
**Pattern**: Reference `LocationParentSelector.tsx` for select styling, add search input

**Implementation**:
```typescript
interface AssetSelectorProps {
  value: number | null;
  onChange: (assetId: number | null) => void;
  assets: AssetOption[];
  isLoading: boolean;
  className?: string;
}

// Searchable dropdown: text input + filtered select
// - Search input filters assets by name/identifier
// - Select shows filtered results
// - Display format: "{name} ({identifier})"
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 6: Create DateRangeInputs Component
**File**: `frontend/src/components/reports/DateRangeInputs.tsx`
**Action**: CREATE
**Pattern**: Reference `LocationForm.tsx` (lines ~140-150) for date input styling

**Implementation**:
```typescript
interface DateRangeInputsProps {
  fromDate: string;  // YYYY-MM-DD
  toDate: string;    // YYYY-MM-DD
  onFromDateChange: (date: string) => void;
  onToDateChange: (date: string) => void;
}

// Two labeled date inputs: "From" and "To"
// Native <input type="date"> with consistent Tailwind styling
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 7: Create ExportCsvButton Component
**File**: `frontend/src/components/reports/ExportCsvButton.tsx`
**Action**: CREATE
**Pattern**: Button with loading spinner

**Implementation**:
```typescript
interface ExportCsvButtonProps {
  data: AssetHistoryItem[];
  assetName: string;
  disabled?: boolean;
}

// Uses useExportCsv hook
// Shows Download icon + "Export CSV" text
// Shows Loader2 spinner when exporting
// Disabled when no data or disabled prop
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 8: Create AssetSummaryCard Component
**File**: `frontend/src/components/reports/AssetSummaryCard.tsx`
**Action**: CREATE
**Pattern**: Reference mockup layout

**Implementation**:
```typescript
interface AssetSummaryCardProps {
  assetName: string;
  assetIdentifier: string;
  locationsVisited: number;
  timeTracked: string;
  currentLocation: string | null;
}

// Horizontal card layout:
// Left: Asset name (bold) + identifier (gray below)
// Center: Two stat columns - "4 Locations Visited" and "3d 5h Time Tracked"
// Right: Green dot + "Studio A Current Location"
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 9: Create AssetHistoryTab Component
**File**: `frontend/src/components/reports/AssetHistoryTab.tsx`
**Action**: CREATE
**Pattern**: Reference `AssetDetailPanel.tsx` for pure presentational pattern

**Implementation**:
```typescript
export function AssetHistoryTab() {
  const {
    selectedAssetId, setSelectedAssetId,
    assetOptions, isLoadingAssets,
    fromDate, toDate, setFromDate, setToDate,
    timelineData, isLoadingTimeline,
    hasMore, isLoadingMore, handleLoadMore,
    stats, selectedAsset,
  } = useAssetHistoryTab();

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Controls Row */}
      <div className="flex flex-wrap items-end gap-4 mb-4">
        <AssetSelector ... />
        <DateRangeInputs ... />
        <ExportCsvButton ... />
      </div>

      {/* Summary Card (when asset selected and has stats) */}
      {selectedAsset && stats && <AssetSummaryCard ... />}

      {/* Empty state when no asset selected */}
      {!selectedAssetId && <EmptyState ... />}

      {/* Timeline (when asset selected) */}
      {selectedAssetId && (
        <div className="flex-1 min-h-0 overflow-auto ...">
          <MovementTimeline ... />
        </div>
      )}
    </div>
  );
}
```

**Rules enforced**:
- NO helper functions in component body
- NO useEffect in component body
- All logic delegated to useAssetHistoryTab hook

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 10: Integrate AssetHistoryTab into ReportsScreen
**File**: `frontend/src/components/ReportsScreen.tsx`
**Action**: MODIFY

**Changes**:
1. Line 86: Rename tab label from "Movement History" to "Asset History"
2. Lines 239-245: Replace EmptyState placeholder with `<AssetHistoryTab />`
3. Add import for AssetHistoryTab

**Before** (line 86):
```typescript
{ id: 'movement', label: 'Movement History' },
```

**After**:
```typescript
{ id: 'movement', label: 'Asset History' },
```

**Before** (lines 239-245):
```tsx
{activeTab === 'movement' && (
  <EmptyState
    icon={FileText}
    title="Coming Soon"
    description="Movement History report will be available in a future update."
  />
)}
```

**After**:
```tsx
{activeTab === 'movement' && <AssetHistoryTab />}
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 11: Visual Testing with Playwright MCP
**Action**: VERIFY

**Test Steps**:
1. Navigate to Reports page
2. Click "Asset History" tab
3. Screenshot: Verify empty state "Select an asset to view movement history"
4. Select asset from dropdown
5. Screenshot: Verify summary card and timeline appear
6. Change date range
7. Screenshot: Verify timeline updates
8. Click Export CSV
9. Screenshot: Verify button shows loading then completes

**Validation**: Visual inspection via Playwright MCP screenshots

---

## Risk Assessment

- **Risk**: Large asset lists may slow down dropdown
  **Mitigation**: Searchable filter reduces visible options; consider virtualization in future if needed

- **Risk**: Date range changes cause multiple refetches
  **Mitigation**: Memoize params, reset offset on filter change to avoid stale pagination

- **Risk**: CSV export with large datasets
  **Mitigation**: Export only accumulated data (what's loaded), not full history

## Integration Points

- **Store updates**: None - uses existing useCurrentLocations and useAssetHistory hooks
- **Route changes**: None - tab is within existing Reports route
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change, run from `frontend/` directory:
- Gate 1: `just lint` - Syntax & Style
- Gate 2: `just typecheck` - Type Safety
- Gate 3: `just test` - Unit Tests (if applicable)

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

Final validation: `just validate`

## Validation Sequence

After each task:
```bash
cd frontend && just typecheck && just lint
```

Final validation:
```bash
cd frontend && just validate
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and mockup
✅ Similar patterns found: useAssetDetailPanel.ts, AssetDetailPanel.tsx
✅ All clarifying questions answered
✅ Existing MovementTimeline component reusable
✅ Existing useAssetHistory hook reusable
✅ All utility functions already exist in lib/reports/utils.ts
✅ Native date inputs - no new dependencies

**Assessment**: High confidence implementation. All patterns exist, just need to compose them into new components.

**Estimated one-pass success probability**: 90%

**Reasoning**: Well-scoped feature with clear patterns to follow. The main components (timeline, hooks, utils) already exist. New work is primarily composing existing pieces and adding the searchable selector + CSV export.
