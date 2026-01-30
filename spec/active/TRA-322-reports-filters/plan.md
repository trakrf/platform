# Implementation Plan: TRA-322 Reports Filters

Generated: 2026-01-30
Specification: spec.md

## Understanding

Add functional filter dropdowns to the Current Locations tab in Reports:
1. **Location Filter** - Searchable dropdown to filter by location (server-side via `location_id` param)
2. **Time Range Filter** - Simple dropdown to filter by freshness status (client-side)

Both filters follow the "no logic in TSX" pattern - all state and filtering logic lives in hooks.

---

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/hooks/reports/useAssetSelector.ts` - Searchable dropdown hook pattern
- `frontend/src/components/reports/AssetSelector.tsx` - Searchable dropdown component pattern
- `frontend/src/hooks/locations/useLocations.ts` - Location fetching pattern
- `frontend/src/lib/reports/utils.ts:187-195` - `getFreshnessStatus()` for time filtering

**Files to Create**:
- `frontend/src/hooks/reports/useLocationFilter.ts` - Location dropdown state/logic
- `frontend/src/hooks/reports/useReportsFilters.ts` - Combined filter state + data filtering
- `frontend/src/components/reports/LocationFilter.tsx` - Location dropdown (pure presenter)
- `frontend/src/components/reports/TimeRangeFilter.tsx` - Time range dropdown (pure presenter)

**Files to Modify**:
- `frontend/src/components/ReportsScreen.tsx` (lines ~165-179) - Replace placeholder filters
- `frontend/src/hooks/reports/index.ts` - Export new hooks
- `frontend/src/lib/reports/utils.ts` - Add time range filter types

---

## Architecture Impact

- **Subsystems affected**: Frontend only
- **New dependencies**: None (uses existing hooks and utilities)
- **Breaking changes**: None

---

## Task Breakdown

### Task 1: Add Time Range Types to Utils

**File**: `frontend/src/lib/reports/utils.ts`
**Action**: MODIFY
**Pattern**: Follow existing `DATE_RANGE_OPTIONS` pattern (lines 12-18)

**Implementation**:
```typescript
// Add after FreshnessStatus imports
export type TimeRangeFilter = 'all' | 'live' | 'today' | 'week' | 'stale';

export const TIME_RANGE_OPTIONS: { value: TimeRangeFilter; label: string }[] = [
  { value: 'all', label: 'All Time' },
  { value: 'live', label: 'Live (< 15min)' },
  { value: 'today', label: 'Today' },
  { value: 'week', label: 'Last 7 days' },
  { value: 'stale', label: 'Stale (> 7 days)' },
];

/**
 * Check if an item matches the time range filter
 */
export function matchesTimeRange(lastSeen: string, filter: TimeRangeFilter): boolean {
  if (filter === 'all') return true;
  const status = getFreshnessStatus(lastSeen);
  if (filter === 'week') return status !== 'stale'; // live, today, or recent
  return status === filter;
}
```

**Validation**: `pnpm typecheck`

---

### Task 2: Create useLocationFilter Hook

**File**: `frontend/src/hooks/reports/useLocationFilter.ts`
**Action**: CREATE
**Pattern**: Mirror `useAssetSelector.ts` structure

**Implementation**:
```typescript
import { useState, useRef, useEffect, useMemo, useCallback } from 'react';
import type { Location } from '@/types/locations';

interface UseLocationFilterProps {
  value: number | null;
  onChange: (locationId: number | null) => void;
  locations: Location[];
}

interface UseLocationFilterReturn {
  isOpen: boolean;
  search: string;
  containerRef: React.RefObject<HTMLDivElement>;
  inputRef: React.RefObject<HTMLInputElement>;
  filteredLocations: Location[];
  selectedLocation: Location | undefined;
  handleSelect: (locationId: number | null) => void;
  handleInputClick: () => void;
  handleSearchChange: (value: string) => void;
}

export function useLocationFilter({
  value,
  onChange,
  locations,
}: UseLocationFilterProps): UseLocationFilterReturn {
  // State, refs, effects - mirror useAssetSelector pattern
  // Include "All Locations" option (null value)
}
```

**Validation**: `pnpm typecheck`

---

### Task 3: Create useReportsFilters Hook

**File**: `frontend/src/hooks/reports/useReportsFilters.ts`
**Action**: CREATE
**Pattern**: Combine filter state with useCurrentLocations data

**Implementation**:
```typescript
import { useState, useMemo, useCallback } from 'react';
import { useLocations } from '@/hooks/locations/useLocations';
import { useCurrentLocations } from './useCurrentLocations';
import { useDebounce } from '@/hooks/useDebounce';
import { matchesTimeRange, type TimeRangeFilter } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';

interface UseReportsFiltersProps {
  pageSize: number;
  currentPage: number;
}

interface UseReportsFiltersReturn {
  // Filter state
  selectedLocationId: number | null;
  setSelectedLocationId: (id: number | null) => void;
  selectedTimeRange: TimeRangeFilter;
  setSelectedTimeRange: (range: TimeRangeFilter) => void;
  search: string;
  setSearch: (search: string) => void;

  // Location data for dropdown
  locations: Location[];
  isLoadingLocations: boolean;

  // Filtered results
  filteredData: CurrentLocationItem[];
  totalCount: number;
  isLoading: boolean;
  error: Error | null;

  // Filter helpers
  hasActiveFilters: boolean;
  clearFilters: () => void;
  activeFilterDescription: string; // For empty state message
}

export function useReportsFilters(props: UseReportsFiltersProps): UseReportsFiltersReturn {
  // 1. Filter state
  const [selectedLocationId, setSelectedLocationId] = useState<number | null>(null);
  const [selectedTimeRange, setSelectedTimeRange] = useState<TimeRangeFilter>('all');
  const [search, setSearch] = useState('');

  // 2. Fetch locations for dropdown
  const { locations, isLoading: isLoadingLocations } = useLocations();

  // 3. Fetch current locations with server-side location filter
  const debouncedSearch = useDebounce(search, 300);
  const { data, totalCount, isLoading, error } = useCurrentLocations({
    search: debouncedSearch || undefined,
    location_id: selectedLocationId || undefined,
    limit: props.pageSize,
    offset: (props.currentPage - 1) * props.pageSize,
  });

  // 4. Apply client-side time range filter
  const filteredData = useMemo(() => {
    if (selectedTimeRange === 'all') return data;
    return data.filter(item => matchesTimeRange(item.last_seen, selectedTimeRange));
  }, [data, selectedTimeRange]);

  // 5. Helper functions
  const hasActiveFilters = selectedLocationId !== null || selectedTimeRange !== 'all' || search !== '';

  const clearFilters = useCallback(() => {
    setSelectedLocationId(null);
    setSelectedTimeRange('all');
    setSearch('');
  }, []);

  const activeFilterDescription = useMemo(() => {
    const parts: string[] = [];
    if (selectedLocationId) {
      const loc = locations.find(l => l.id === selectedLocationId);
      if (loc) parts.push(`at "${loc.name}"`);
    }
    if (selectedTimeRange !== 'all') {
      const labels: Record<TimeRangeFilter, string> = {
        all: '', live: 'live', today: 'seen today', week: 'seen this week', stale: 'stale'
      };
      parts.push(labels[selectedTimeRange]);
    }
    if (search) parts.push(`matching "${search}"`);
    return parts.join(' and ');
  }, [selectedLocationId, selectedTimeRange, search, locations]);

  return { /* all values */ };
}
```

**Validation**: `pnpm typecheck`

---

### Task 4: Create LocationFilter Component

**File**: `frontend/src/components/reports/LocationFilter.tsx`
**Action**: CREATE
**Pattern**: Mirror `AssetSelector.tsx` - pure presenter

**Implementation**:
```typescript
import { ChevronDown, Loader2, MapPin } from 'lucide-react';
import { useLocationFilter } from '@/hooks/reports/useLocationFilter';
import type { Location } from '@/types/locations';

interface LocationFilterProps {
  value: number | null;
  onChange: (locationId: number | null) => void;
  locations: Location[];
  isLoading: boolean;
  className?: string;
}

export function LocationFilter({
  value,
  onChange,
  locations,
  isLoading,
  className = '',
}: LocationFilterProps) {
  const {
    isOpen,
    search,
    containerRef,
    inputRef,
    filteredLocations,
    selectedLocation,
    handleSelect,
    handleInputClick,
    handleSearchChange,
  } = useLocationFilter({ value, onChange, locations });

  return (
    // Searchable dropdown with "All Locations" as first option
    // Similar structure to AssetSelector
  );
}
```

**Validation**: `pnpm typecheck && pnpm lint`

---

### Task 5: Create TimeRangeFilter Component

**File**: `frontend/src/components/reports/TimeRangeFilter.tsx`
**Action**: CREATE
**Pattern**: Simple native select (no search needed)

**Implementation**:
```typescript
import { TIME_RANGE_OPTIONS, type TimeRangeFilter } from '@/lib/reports/utils';

interface TimeRangeFilterProps {
  value: TimeRangeFilter;
  onChange: (range: TimeRangeFilter) => void;
  className?: string;
}

export function TimeRangeFilter({
  value,
  onChange,
  className = '',
}: TimeRangeFilterProps) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as TimeRangeFilter)}
      className={`px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg
        bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm
        focus:outline-none focus:ring-2 focus:ring-blue-500 ${className}`}
    >
      {TIME_RANGE_OPTIONS.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}
```

**Validation**: `pnpm typecheck && pnpm lint`

---

### Task 6: Update Hooks Index

**File**: `frontend/src/hooks/reports/index.ts`
**Action**: MODIFY

**Implementation**:
```typescript
// Add exports
export { useLocationFilter } from './useLocationFilter';
export { useReportsFilters } from './useReportsFilters';
export type { TimeRangeFilter } from '@/lib/reports/utils';
```

**Validation**: `pnpm typecheck`

---

### Task 7: Integrate Filters into ReportsScreen

**File**: `frontend/src/components/ReportsScreen.tsx`
**Action**: MODIFY (lines ~18-36, ~153-180, ~184-198)

**Changes**:
1. Replace local filter state with `useReportsFilters` hook
2. Replace placeholder `<select>` elements with new filter components
3. Update empty state to be filter-aware with clear filters button
4. Remove duplicate state (search, pagination now in hook)

**Implementation**:
```typescript
// 1. Import new components and hook
import { LocationFilter } from '@/components/reports/LocationFilter';
import { TimeRangeFilter } from '@/components/reports/TimeRangeFilter';
import { useReportsFilters } from '@/hooks/reports';

// 2. Replace state management
const [currentPage, setCurrentPage] = useState(1);
const [pageSize, setPageSize] = useState(10);

const {
  selectedLocationId,
  setSelectedLocationId,
  selectedTimeRange,
  setSelectedTimeRange,
  search,
  setSearch,
  locations,
  isLoadingLocations,
  filteredData,
  totalCount,
  isLoading,
  error,
  hasActiveFilters,
  clearFilters,
  activeFilterDescription,
} = useReportsFilters({ pageSize, currentPage });

// Reset page when filters change
useEffect(() => {
  setCurrentPage(1);
}, [selectedLocationId, selectedTimeRange, search]);

// 3. Replace placeholder filters (lines 166-179)
<div className="flex gap-2">
  <LocationFilter
    value={selectedLocationId}
    onChange={setSelectedLocationId}
    locations={locations}
    isLoading={isLoadingLocations}
  />
  <TimeRangeFilter
    value={selectedTimeRange}
    onChange={setSelectedTimeRange}
  />
</div>

// 4. Update empty state for filter-aware message
{!isLoading && filteredData.length === 0 && hasActiveFilters && (
  <EmptyState
    icon={Search}
    title="No Results"
    description={`No assets found ${activeFilterDescription}.`}
    action={
      <button
        onClick={clearFilters}
        className="mt-2 text-blue-600 hover:text-blue-700 text-sm font-medium"
      >
        Clear filters
      </button>
    }
  />
)}
```

**Validation**: `pnpm typecheck && pnpm lint && pnpm build`

---

## Risk Assessment

- **Risk**: Time range filter may cause pagination inconsistency (server returns 10 items, client filters to 3)
  **Mitigation**: Document that time range is approximate when paginated. For accurate counts, consider fetching all data for time filtering (like stats query does).

- **Risk**: Location dropdown may have many items
  **Mitigation**: Searchable dropdown pattern handles this well (already proven with AssetSelector).

---

## Integration Points

- **Store updates**: None - using existing hooks
- **Route changes**: None - filters are session-only
- **Config updates**: None

---

## VALIDATION GATES (MANDATORY)

After EVERY task, run from `frontend/` directory:

```bash
pnpm typecheck    # Gate 1: Type Safety
pnpm lint         # Gate 2: Syntax & Style
pnpm test         # Gate 3: Unit Tests
```

**Final validation**:
```bash
pnpm build        # Gate 4: Build succeeds
```

If ANY gate fails → Fix immediately → Re-run → Loop until pass.

---

## Validation Sequence

| Task | Validation |
|------|------------|
| 1 | `pnpm typecheck` |
| 2 | `pnpm typecheck` |
| 3 | `pnpm typecheck` |
| 4 | `pnpm typecheck && pnpm lint` |
| 5 | `pnpm typecheck && pnpm lint` |
| 6 | `pnpm typecheck` |
| 7 | `pnpm typecheck && pnpm lint && pnpm build` |

---

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found: `useAssetSelector.ts`, `AssetSelector.tsx`
✅ All clarifying questions answered
✅ Existing `useLocations` hook ready to use
✅ Existing `getFreshnessStatus()` utility for time filtering
✅ Backend already supports `location_id` param

**Assessment**: Straightforward feature using established patterns. All building blocks exist.

**Estimated one-pass success probability**: 90%

**Reasoning**: Well-defined patterns to follow, no new dependencies, minimal risk areas. Main uncertainty is ensuring the filter-aware empty state integrates cleanly with existing empty state logic.
