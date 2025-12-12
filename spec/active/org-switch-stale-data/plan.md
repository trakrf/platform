# Implementation Plan: Org Switch Stale Data Fix

Generated: 2025-12-12
Specification: spec.md
Linear: TRA-189

## Understanding

When users switch organizations, stale data from the previous org persists in:
1. TanStack Query cache (query keys don't include org ID)
2. Zustand stores (asset, location, tag, barcode caches)
3. UI state (filters, pagination, sort)

The fix follows React best practices:
- Create `useOrgSwitch()` hook to orchestrate cache invalidation
- Add org ID to query keys for proper cache isolation
- Update `invalidateCache()` to reset full store state (cache + UI)

## Relevant Files

**Reference Patterns**:
- `frontend/src/hooks/assets/useAssetMutations.ts` (lines 1-7) - Pattern for using `useQueryClient()`
- `frontend/src/stores/assets/assetStore.ts` (lines 70-106) - Initial state defaults to restore

**Files to Create**:
- `frontend/src/hooks/orgs/useOrgSwitch.ts` - Orchestration hook for org switching
- `frontend/src/hooks/orgs/index.ts` - Barrel export

**Files to Modify**:
- `frontend/src/stores/assets/assetActions.ts` (lines 163-174) - Enhance `invalidateCache()` to reset all state
- `frontend/src/stores/locations/locationActions.ts` (lines 205-218) - Enhance `clearCache()` to reset all state
- `frontend/src/hooks/assets/useAssets.ts` (line 17) - Add org ID to queryKey
- `frontend/src/hooks/assets/useAsset.ts` (line 17) - Add org ID to queryKey
- `frontend/src/hooks/locations/useLocations.ts` (line 14) - Add org ID to queryKey
- `frontend/src/hooks/locations/useLocation.ts` (line 17) - Add org ID to queryKey
- `frontend/src/components/OrgSwitcher.tsx` (line 20) - Use `useOrgSwitch()` hook

## Architecture Impact

- **Subsystems affected**: Zustand stores, TanStack Query hooks, OrgSwitcher component
- **New dependencies**: None
- **Breaking changes**: None - `switchOrg()` still works, `useOrgSwitch()` is additive

## Task Breakdown

### Task 1: Enhance assetActions.invalidateCache()

**File**: `frontend/src/stores/assets/assetActions.ts`
**Action**: MODIFY
**Pattern**: Reference `assetStore.ts` lines 70-106 for initial state values

**Implementation**:
```typescript
// Update invalidateCache to reset all state (cache + filters + pagination + sort)
invalidateCache: () =>
  set({
    cache: {
      byId: new Map(),
      byIdentifier: new Map(),
      byType: new Map(),
      activeIds: new Set(),
      allIds: [],
      lastFetched: 0,
      ttl: 60 * 60 * 1000,
    },
    filters: {
      type: 'all',
      is_active: 'all',
      search: '',
      location_id: 'all',
    },
    pagination: {
      currentPage: 1,
      pageSize: 25,
      totalCount: 0,
      totalPages: 0,
    },
    sort: {
      field: 'created_at',
      direction: 'desc',
    },
    selectedAssetId: null,
  }),
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 2: Enhance locationActions.clearCache() → invalidateCache()

**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: MODIFY
**Pattern**: Reference `locationStore.ts` lines 60-76 for initial state values

**Implementation**:
```typescript
// Rename clearCache to invalidateCache and reset all state
invalidateCache: () =>
  set(() => ({
    cache: {
      byId: new Map(),
      byIdentifier: new Map(),
      byParentId: new Map(),
      rootIds: new Set(),
      activeIds: new Set(),
      allIds: [],
      allIdentifiers: [],
      lastFetched: 0,
      ttl: 0,
    },
    filters: {
      search: '',
      identifier: '',
      is_active: 'all',
    },
    pagination: {
      currentPage: 1,
      pageSize: 10,
      totalCount: 0,
      totalPages: 0,
    },
    sort: {
      field: 'identifier',
      direction: 'asc',
    },
    selectedLocationId: null,
  })),
```

**Also**: Update `locationStore.ts` interface to rename `clearCache` → `invalidateCache`

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 3: Create useOrgSwitch hook

**File**: `frontend/src/hooks/orgs/useOrgSwitch.ts`
**Action**: CREATE
**Pattern**: Reference `useAssetMutations.ts` lines 1-7 for useQueryClient pattern

**Implementation**:
```typescript
/**
 * useOrgSwitch - Orchestrates org switching with cache invalidation
 *
 * Handles:
 * - Calling orgStore.switchOrg() to update backend
 * - Invalidating all org-scoped TanStack Query caches
 * - Clearing all org-scoped Zustand stores
 */
import { useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, isLoading } = useOrgStore();

  const switchOrg = async (orgId: number) => {
    // 1. Call backend to switch org
    await storeSwitchOrg(orgId);

    // 2. Clear all org-scoped Zustand stores
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();

    // 3. Invalidate all org-scoped TanStack Query caches
    // Using predicate to exclude auth-related queries
    queryClient.invalidateQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        return key !== 'user' && key !== 'profile';
      },
    });
  };

  return {
    switchOrg,
    isLoading,
  };
}
```

**Also create**: `frontend/src/hooks/orgs/index.ts`
```typescript
export { useOrgSwitch } from './useOrgSwitch';
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 4: Update OrgSwitcher to use useOrgSwitch

**File**: `frontend/src/components/OrgSwitcher.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
// Change import
import { useOrgSwitch } from '@/hooks/orgs';
import { useOrgStore } from '@/stores';

// Update component
export function OrgSwitcher({ onCreateOrg }: OrgSwitcherProps) {
  const { currentOrg, currentRole, orgs } = useOrgStore();
  const { switchOrg, isLoading } = useOrgSwitch();

  const handleSwitchOrg = async (orgId: number) => {
    if (orgId === currentOrg?.id) return;
    try {
      await switchOrg(orgId);
    } catch (error) {
      console.error('Failed to switch org:', error);
    }
  };
  // ... rest unchanged
}
```

**Validation**: `just frontend typecheck && just frontend lint`

---

### Task 5: Add org ID to useAssets queryKey

**File**: `frontend/src/hooks/assets/useAssets.ts`
**Action**: MODIFY

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';

export function useAssets(options: UseAssetsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;

  const pagination = useAssetStore((state) => state.pagination);
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['assets', currentOrg?.id, pagination.currentPage, pagination.pageSize],
    // ... rest unchanged
  });
}
```

**Validation**: `just frontend typecheck && just frontend lint && just frontend test`

---

### Task 6: Add org ID to useAsset queryKey

**File**: `frontend/src/hooks/assets/useAsset.ts`
**Action**: MODIFY

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';

export function useAsset(id: number | null, options: UseAssetOptions = {}) {
  const { enabled = true } = options;

  const asset = useAssetStore((state) =>
    id ? state.getAssetById(id) ?? null : null
  );
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['asset', currentOrg?.id, id],
    // ... rest unchanged
  });
}
```

**Validation**: `just frontend typecheck && just frontend lint && just frontend test`

---

### Task 7: Add org ID to useLocations queryKey

**File**: `frontend/src/hooks/locations/useLocations.ts`
**Action**: MODIFY

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';

export function useLocations(options: UseLocationsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['locations', currentOrg?.id],
    // ... rest unchanged
  });
}
```

**Validation**: `just frontend typecheck && just frontend lint && just frontend test`

---

### Task 8: Add org ID to useLocation queryKey

**File**: `frontend/src/hooks/locations/useLocation.ts`
**Action**: MODIFY

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';

export function useLocation(id: number | null, options: UseLocationOptions = {}) {
  const { enabled = true } = options;

  const location = useLocationStore((state) =>
    id ? state.getLocationById(id) ?? null : null
  );
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['location', currentOrg?.id, id],
    // ... rest unchanged
  });
}
```

**Validation**: `just frontend typecheck && just frontend lint && just frontend test`

---

### Task 9: Update tests for new queryKey signatures

**Files**:
- `frontend/src/hooks/assets/useAssets.test.ts`
- `frontend/src/hooks/assets/useAsset.test.ts`
- `frontend/src/hooks/locations/useLocations.test.ts`
- `frontend/src/hooks/locations/useLocation.test.ts`

**Action**: MODIFY

**Implementation**: Mock `useOrgStore` to return a `currentOrg` with an `id` property.

```typescript
// Add to test setup
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));
```

**Validation**: `just frontend test`

---

## Risk Assessment

- **Risk**: Tests may fail due to queryKey changes
  **Mitigation**: Task 9 explicitly updates tests with mocked org store

- **Risk**: Circular import between stores
  **Mitigation**: useOrgSwitch hook uses `getState()` pattern, not hook subscriptions

- **Risk**: Race condition if user switches orgs rapidly
  **Mitigation**: `isLoading` state prevents concurrent switches (existing behavior)

## Integration Points

- **Store updates**: assetStore, locationStore get enhanced `invalidateCache()`
- **Hook addition**: New `useOrgSwitch()` hook in `hooks/orgs/`
- **Component update**: OrgSwitcher uses new hook
- **Query keys**: 4 hooks get org ID added to keys

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
just frontend lint       # Gate 1: Syntax & Style
just frontend typecheck  # Gate 2: Type Safety
just frontend test       # Gate 3: Unit Tests
```

After ALL tasks:
```bash
just frontend validate   # Full validation
just frontend build      # Final build check
```

## Validation Sequence

1. Tasks 1-2: Store changes → `just frontend typecheck && just frontend lint`
2. Task 3: New hook → `just frontend typecheck && just frontend lint`
3. Task 4: Component update → `just frontend typecheck && just frontend lint`
4. Tasks 5-8: Query key changes → `just frontend typecheck && just frontend lint`
5. Task 9: Test updates → `just frontend test`
6. Final: `just frontend validate && just frontend build`

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Similar patterns found: `useAssetMutations.ts` for useQueryClient pattern
- ✅ All clarifying questions answered
- ✅ Existing test patterns to follow in hook tests
- ✅ Initial state values clearly defined in store files
- ✅ No new dependencies required

**Assessment**: High confidence implementation. All patterns exist in codebase, changes are surgical and well-scoped.

**Estimated one-pass success probability**: 90%

**Reasoning**: The implementation follows existing patterns exactly. The main risk is test updates, but mocking patterns are established. No architectural changes, just enhancing existing code.
