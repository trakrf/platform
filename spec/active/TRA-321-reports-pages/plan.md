# Implementation Plan: TRA-321 Reports Pages

Generated: 2026-01-28
Specification: spec.md

## Understanding

Implement the Reports feature with two views:
1. **Current Locations** (`#reports`) - Table showing all assets with their current location and freshness status
2. **Asset History** (`#reports-history?id=X`) - Single asset's movement timeline

This is a read-only reporting feature that uses existing backend endpoints (TRA-217, TRA-218 - already merged).

## Clarifying Decisions

- **Mobile cards**: Include in Phase 2 (CurrentLocationCard, AssetHistoryCard)
- **Search debounce**: Create reusable `useDebounce` hook (300ms default)
- **Error handling**: Toast notifications via react-hot-toast
- **Test coverage**: Unit tests for utils + hook tests with mocked API

---

## PHASE 1: Data Layer Foundation

**Complexity**: 3/10 ✅
**Files**: 7 (6 create, 1 modify)
**Estimated**: 4 subtasks

### Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/lib/api/locations/index.ts` - API client pattern
- `frontend/src/hooks/locations/useLocations.ts` - React Query hook pattern
- `frontend/src/hooks/locations/useLocations.test.ts` - Hook test pattern
- `frontend/src/lib/location/filters.test.ts` - Utility test pattern
- `frontend/src/types/locations/index.ts` - Type definition pattern

**Files to Create**:
- `frontend/src/types/reports/index.ts` - TypeScript interfaces
- `frontend/src/lib/api/reports/index.ts` - API client
- `frontend/src/lib/reports/utils.ts` - Helper functions
- `frontend/src/lib/reports/utils.test.ts` - Utility tests
- `frontend/src/hooks/reports/useCurrentLocations.ts` - Query hook
- `frontend/src/hooks/reports/useAssetHistory.ts` - Query hook
- `frontend/src/hooks/reports/useCurrentLocations.test.ts` - Hook tests
- `frontend/src/hooks/reports/useAssetHistory.test.ts` - Hook tests
- `frontend/src/hooks/reports/index.ts` - Exports
- `frontend/src/hooks/useDebounce.ts` - Reusable debounce hook

**Files to Modify**:
- `frontend/src/types/index.ts` - Add reports export

---

### Task 1.1: Create TypeScript Types

**File**: `frontend/src/types/reports/index.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/types/locations/index.ts`

**Implementation**:
```typescript
// Types matching backend contracts from:
// - backend/internal/models/report/current_location.go
// - backend/internal/models/report/asset_history.go

export interface CurrentLocationItem {
  asset_id: number;
  asset_name: string;
  asset_identifier: string;
  location_id: number | null;
  location_name: string | null;
  last_seen: string; // ISO 8601
}

export interface CurrentLocationsResponse {
  data: CurrentLocationItem[];
  count: number;
  offset: number;
  total_count: number;
}

export interface CurrentLocationsParams {
  limit?: number;
  offset?: number;
  location_id?: number;
  search?: string;
}

export interface AssetInfo {
  id: number;
  name: string;
  identifier: string;
}

export interface AssetHistoryItem {
  timestamp: string; // ISO 8601
  location_id: number | null;
  location_name: string | null;
  duration_seconds: number | null;
}

export interface AssetHistoryResponse {
  asset: AssetInfo;
  data: AssetHistoryItem[];
  count: number;
  offset: number;
  total_count: number;
}

export interface AssetHistoryParams {
  limit?: number;
  offset?: number;
  start_date?: string;
  end_date?: string;
}

export type FreshnessStatus = 'live' | 'today' | 'recent' | 'stale';
```

**Also update** `frontend/src/types/index.ts`:
```typescript
// Add export for reports types
export type {
  CurrentLocationItem,
  CurrentLocationsResponse,
  AssetInfo,
  AssetHistoryItem,
  AssetHistoryResponse,
  FreshnessStatus,
} from './reports';
```

**Validation**: `just frontend typecheck`

---

### Task 1.2: Create API Client

**File**: `frontend/src/lib/api/reports/index.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/api/locations/index.ts` (lines 31-49)

**Implementation**:
```typescript
/**
 * Reports API Client
 *
 * Type-safe wrapper around backend report endpoints.
 * Backend routes: backend/internal/handlers/reports/
 */

import { apiClient } from '../client';
import type {
  CurrentLocationsResponse,
  CurrentLocationsParams,
  AssetHistoryResponse,
  AssetHistoryParams,
} from '@/types/reports';

export const reportsApi = {
  /**
   * Get current locations for all assets
   * GET /api/v1/reports/current-locations
   */
  getCurrentLocations: (options: CurrentLocationsParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.location_id !== undefined) params.append('location_id', String(options.location_id));
    if (options.search) params.append('search', options.search);

    const queryString = params.toString();
    const url = queryString ? `/reports/current-locations?${queryString}` : '/reports/current-locations';
    return apiClient.get<CurrentLocationsResponse>(url);
  },

  /**
   * Get movement history for a specific asset
   * GET /api/v1/reports/assets/:id/history
   */
  getAssetHistory: (assetId: number, options: AssetHistoryParams = {}) => {
    const params = new URLSearchParams();
    if (options.limit !== undefined) params.append('limit', String(options.limit));
    if (options.offset !== undefined) params.append('offset', String(options.offset));
    if (options.start_date) params.append('start_date', options.start_date);
    if (options.end_date) params.append('end_date', options.end_date);

    const queryString = params.toString();
    const url = queryString
      ? `/reports/assets/${assetId}/history?${queryString}`
      : `/reports/assets/${assetId}/history`;
    return apiClient.get<AssetHistoryResponse>(url);
  },
};
```

**Validation**: `just frontend typecheck`

---

### Task 1.3: Create Utility Functions + Tests

**File**: `frontend/src/lib/reports/utils.ts`
**Action**: CREATE

**Implementation**:
```typescript
/**
 * Report Utility Functions
 */

import type { FreshnessStatus } from '@/types/reports';

/**
 * Determine freshness status based on last_seen timestamp
 * - live: < 15 minutes ago
 * - today: < 24 hours ago
 * - recent: < 7 days ago
 * - stale: >= 7 days ago
 */
export function getFreshnessStatus(lastSeen: string): FreshnessStatus {
  const diff = Date.now() - new Date(lastSeen).getTime();
  const minutes = diff / (1000 * 60);

  if (minutes < 15) return 'live';
  if (minutes < 24 * 60) return 'today';
  if (minutes < 7 * 24 * 60) return 'recent';
  return 'stale';
}

/**
 * Format duration in seconds to human-readable string
 * e.g., 3700 -> "1h 1m", 120 -> "2m", null -> "—"
 */
export function formatDuration(seconds: number | null): string {
  if (seconds === null) return '—';

  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);

  if (hours > 0) {
    return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
  }
  return `${mins}m`;
}

/**
 * Format ISO date to relative time string
 * e.g., "2 min ago", "3 hours ago", "Yesterday", "Jan 22"
 */
export function formatRelativeTime(isoDate: string): string {
  const date = new Date(isoDate);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins} min ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return `${diffDays} days ago`;

  // Format as "Jan 22" for older dates
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}
```

**File**: `frontend/src/lib/reports/utils.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/location/filters.test.ts`

**Implementation**:
```typescript
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { getFreshnessStatus, formatDuration, formatRelativeTime } from './utils';

describe('getFreshnessStatus', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "live" for < 15 minutes ago', () => {
    const tenMinAgo = new Date('2025-01-27T11:50:00Z').toISOString();
    expect(getFreshnessStatus(tenMinAgo)).toBe('live');
  });

  it('returns "today" for 15 min - 24 hours ago', () => {
    const twoHoursAgo = new Date('2025-01-27T10:00:00Z').toISOString();
    expect(getFreshnessStatus(twoHoursAgo)).toBe('today');
  });

  it('returns "recent" for 1-7 days ago', () => {
    const threeDaysAgo = new Date('2025-01-24T12:00:00Z').toISOString();
    expect(getFreshnessStatus(threeDaysAgo)).toBe('recent');
  });

  it('returns "stale" for > 7 days ago', () => {
    const twoWeeksAgo = new Date('2025-01-13T12:00:00Z').toISOString();
    expect(getFreshnessStatus(twoWeeksAgo)).toBe('stale');
  });
});

describe('formatDuration', () => {
  it('returns "—" for null', () => {
    expect(formatDuration(null)).toBe('—');
  });

  it('formats minutes only', () => {
    expect(formatDuration(120)).toBe('2m');
    expect(formatDuration(45 * 60)).toBe('45m');
  });

  it('formats hours and minutes', () => {
    expect(formatDuration(3700)).toBe('1h 1m');
    expect(formatDuration(2 * 3600 + 30 * 60)).toBe('2h 30m');
  });

  it('formats hours only when no remaining minutes', () => {
    expect(formatDuration(3600)).toBe('1h');
    expect(formatDuration(7200)).toBe('2h');
  });

  it('handles zero', () => {
    expect(formatDuration(0)).toBe('0m');
  });
});

describe('formatRelativeTime', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "Just now" for < 1 minute', () => {
    const now = new Date('2025-01-27T11:59:30Z').toISOString();
    expect(formatRelativeTime(now)).toBe('Just now');
  });

  it('returns minutes ago', () => {
    const fiveMinAgo = new Date('2025-01-27T11:55:00Z').toISOString();
    expect(formatRelativeTime(fiveMinAgo)).toBe('5 min ago');
  });

  it('returns hours ago', () => {
    const threeHoursAgo = new Date('2025-01-27T09:00:00Z').toISOString();
    expect(formatRelativeTime(threeHoursAgo)).toBe('3 hours ago');
  });

  it('returns "Yesterday" for 1 day ago', () => {
    const yesterday = new Date('2025-01-26T12:00:00Z').toISOString();
    expect(formatRelativeTime(yesterday)).toBe('Yesterday');
  });

  it('returns days ago for 2-6 days', () => {
    const threeDaysAgo = new Date('2025-01-24T12:00:00Z').toISOString();
    expect(formatRelativeTime(threeDaysAgo)).toBe('3 days ago');
  });

  it('returns formatted date for > 7 days', () => {
    const twoWeeksAgo = new Date('2025-01-13T12:00:00Z').toISOString();
    expect(formatRelativeTime(twoWeeksAgo)).toBe('Jan 13');
  });
});
```

**Validation**: `just frontend test -- src/lib/reports/utils.test.ts`

---

### Task 1.4: Create React Query Hooks + Tests

**File**: `frontend/src/hooks/useDebounce.ts`
**Action**: CREATE

```typescript
import { useState, useEffect } from 'react';

/**
 * Debounce a value by the specified delay
 * @param value - Value to debounce
 * @param delay - Delay in milliseconds (default: 300)
 */
export function useDebounce<T>(value: T, delay = 300): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedValue(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);

  return debouncedValue;
}
```

**File**: `frontend/src/hooks/reports/useCurrentLocations.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/hooks/locations/useLocations.ts`

```typescript
import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type { CurrentLocationsParams, CurrentLocationItem } from '@/types/reports';

export interface UseCurrentLocationsOptions extends CurrentLocationsParams {
  enabled?: boolean;
}

export function useCurrentLocations(options: UseCurrentLocationsOptions = {}) {
  const { enabled = true, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'current-locations', currentOrg?.id, params],
    queryFn: async () => {
      const response = await reportsApi.getCurrentLocations(params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id,
    staleTime: 30 * 1000, // 30 seconds - reports should refresh frequently
  });

  return {
    data: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    count: query.data?.count ?? 0,
    offset: query.data?.offset ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
```

**File**: `frontend/src/hooks/reports/useAssetHistory.ts`
**Action**: CREATE

```typescript
import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type { AssetHistoryParams, AssetInfo, AssetHistoryItem } from '@/types/reports';

export interface UseAssetHistoryOptions extends AssetHistoryParams {
  enabled?: boolean;
}

export function useAssetHistory(assetId: number | null, options: UseAssetHistoryOptions = {}) {
  const { enabled = true, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'asset-history', currentOrg?.id, assetId, params],
    queryFn: async () => {
      if (!assetId) throw new Error('Asset ID required');
      const response = await reportsApi.getAssetHistory(assetId, params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id && !!assetId,
    staleTime: 30 * 1000,
  });

  return {
    asset: query.data?.asset ?? null,
    data: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    count: query.data?.count ?? 0,
    offset: query.data?.offset ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
```

**File**: `frontend/src/hooks/reports/useCurrentLocations.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/hooks/locations/useLocations.test.ts`

```typescript
import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useCurrentLocations } from './useCurrentLocations';
import { reportsApi } from '@/lib/api/reports';
import type { CurrentLocationItem } from '@/types/reports';

vi.mock('@/lib/api/reports');

vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

const mockData: CurrentLocationItem[] = [
  {
    asset_id: 1,
    asset_name: 'Projector A1',
    asset_identifier: 'AST-001',
    location_id: 1,
    location_name: 'Room 101',
    last_seen: '2025-01-27T10:30:00Z',
  },
];

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useCurrentLocations', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch and return current locations', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockResolvedValue({
      data: { data: mockData, count: 1, offset: 0, total_count: 1 },
    } as any);

    const { result } = renderHook(() => useCurrentLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data).toEqual(mockData);
    expect(result.current.totalCount).toBe(1);
  });

  it('should pass params to API', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockResolvedValue({
      data: { data: [], count: 0, offset: 0, total_count: 0 },
    } as any);

    renderHook(() => useCurrentLocations({ search: 'test', limit: 10 }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(reportsApi.getCurrentLocations).toHaveBeenCalledWith({
        search: 'test',
        limit: 10,
      });
    });
  });

  it('should handle errors', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockRejectedValue(new Error('Failed'));

    const { result } = renderHook(() => useCurrentLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });

    expect(result.current.data).toEqual([]);
  });

  it('should respect enabled option', async () => {
    const { result } = renderHook(() => useCurrentLocations({ enabled: false }), {
      wrapper: createWrapper(),
    });

    await new Promise((r) => setTimeout(r, 100));
    expect(reportsApi.getCurrentLocations).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });
});
```

**File**: `frontend/src/hooks/reports/useAssetHistory.test.ts`
**Action**: CREATE

```typescript
import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetHistory } from './useAssetHistory';
import { reportsApi } from '@/lib/api/reports';

vi.mock('@/lib/api/reports');

vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

const mockResponse = {
  asset: { id: 1, name: 'Projector A1', identifier: 'AST-001' },
  data: [
    { timestamp: '2025-01-27T10:30:00Z', location_id: 1, location_name: 'Room 101', duration_seconds: null },
  ],
  count: 1,
  offset: 0,
  total_count: 1,
};

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useAssetHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch asset history', async () => {
    vi.mocked(reportsApi.getAssetHistory).mockResolvedValue({
      data: mockResponse,
    } as any);

    const { result } = renderHook(() => useAssetHistory(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.asset).toEqual(mockResponse.asset);
    expect(result.current.data).toEqual(mockResponse.data);
  });

  it('should not fetch when assetId is null', async () => {
    const { result } = renderHook(() => useAssetHistory(null), {
      wrapper: createWrapper(),
    });

    await new Promise((r) => setTimeout(r, 100));
    expect(reportsApi.getAssetHistory).not.toHaveBeenCalled();
    expect(result.current.asset).toBeNull();
  });

  it('should handle 404 errors', async () => {
    vi.mocked(reportsApi.getAssetHistory).mockRejectedValue(new Error('Not found'));

    const { result } = renderHook(() => useAssetHistory(999), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });
  });
});
```

**File**: `frontend/src/hooks/reports/index.ts`
**Action**: CREATE

```typescript
export { useCurrentLocations } from './useCurrentLocations';
export type { UseCurrentLocationsOptions } from './useCurrentLocations';
export { useAssetHistory } from './useAssetHistory';
export type { UseAssetHistoryOptions } from './useAssetHistory';
```

**Validation**:
```bash
just frontend typecheck
just frontend test -- src/hooks/reports/
just frontend test -- src/lib/reports/
```

---

## Phase 1 Validation Gate

After completing all Phase 1 tasks:

```bash
cd frontend
just typecheck   # Must pass
just lint        # Must pass
just test        # Must pass (including new tests)
```

**Phase 1 Complete when**: All 4 tasks done, all validation gates pass.

---

## PHASE 2: Routing + UI Components (Outline)

**Complexity**: 6/10 ⚠️
**Files**: 10 (8 create, 3 modify)
**Estimated**: 7 subtasks

### Task 2.1: Update Routing

**Files to modify**:
- `frontend/src/stores/uiStore.ts` - Add `'reports' | 'reports-history'` to TabType (line 7)
- `frontend/src/App.tsx`:
  - Add lazy imports (after line 30)
  - Add to VALID_TABS array (line 32)
  - Add to tabComponents mapping (line 188)
  - Add to loadingScreens (line 207)
- `frontend/src/components/TabNavigation.tsx`:
  - Import BarChart3 from lucide-react (line 5)
  - Add NavItem after Locations (after line 207)

### Task 2.2: Create FreshnessBadge Component

**File**: `frontend/src/components/reports/FreshnessBadge.tsx`

Badge with colors:
- `live` → Green (bg-green-100, text-green-800)
- `today` → Blue (bg-blue-100, text-blue-800)
- `recent` → Yellow (bg-yellow-100, text-yellow-800)
- `stale` → Gray (bg-gray-100, text-gray-600)

### Task 2.3: Create CurrentLocationsTable Component

**File**: `frontend/src/components/reports/CurrentLocationsTable.tsx`
**Pattern**: Reference `frontend/src/components/assets/AssetTable.tsx`

Uses DataTable with columns: Asset (name+id), Location, Last Seen, Status
Row click → navigate to `#reports-history?id={asset_id}`

### Task 2.4: Create AssetHistoryTable Component

**File**: `frontend/src/components/reports/AssetHistoryTable.tsx`

Uses DataTable with columns: Time, Location, Duration
First row (current location) shows "—" for duration

### Task 2.5: Create Mobile Card Components

**Files**:
- `frontend/src/components/reports/CurrentLocationCard.tsx`
- `frontend/src/components/reports/AssetHistoryCard.tsx`

**Pattern**: Reference `frontend/src/components/assets/AssetCard.tsx`

### Task 2.6: Create ReportsScreen

**File**: `frontend/src/components/ReportsScreen.tsx`
**Pattern**: Reference `frontend/src/components/AssetsScreen.tsx`

Structure: ProtectedRoute → Header → Search → Table (desktop) / Cards (mobile)
Uses useDebounce for search, pagination state

### Task 2.7: Create ReportsHistoryScreen

**File**: `frontend/src/components/ReportsHistoryScreen.tsx`

Structure: ProtectedRoute → Back button → Asset header → History table/cards
Get asset ID from URL hash params

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| API response shape mismatch | Types match backend Go structs exactly |
| Pagination offset issues | Follow existing useLocations pattern |
| Hash routing edge cases | Parse URL same way as reset-password screen in App.tsx |
| Timezone issues in freshness | Use UTC consistently, test with fake timers |

---

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are GATES that block progress.

After EVERY code change:
```bash
just frontend lint      # Gate 1: Style
just frontend typecheck # Gate 2: Types
just frontend test      # Gate 3: Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

---

## Plan Quality Assessment

**Complexity Score**: 9/10 split into:
- Phase 1: 3/10 (LOW) ✅
- Phase 2: 6/10 (MEDIUM-HIGH) ⚠️

**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec (backend contracts documented)
✅ Similar patterns found at `hooks/locations/`, `lib/api/locations/`
✅ All clarifying questions answered
✅ Existing test patterns at `hooks/locations/*.test.ts`
✅ No new dependencies required
⚠️ Multiple subsystem integration in Phase 2

**Assessment**: High confidence due to existing patterns for every component type.

**Estimated one-pass success probability**: 85%

**Reasoning**: All patterns exist in codebase, types match backend exactly, comprehensive test coverage planned. Main risk is integration complexity in Phase 2.
