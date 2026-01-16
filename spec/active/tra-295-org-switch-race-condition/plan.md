# Implementation Plan: Fix Org Switch Race Condition

Generated: 2026-01-16
Specification: spec.md

## Understanding

When users switch organizations while on the Assets tab, in-flight API requests can complete after the org context has changed, writing stale data to the store. This causes wrong/empty asset lists until hard refresh.

The fix requires:
1. Capturing org ID at fetch time and validating before store updates
2. Cancelling in-flight queries when org switches
3. Scoping query invalidation to include org ID
4. Clearing persisted localStorage cache on org switch

## Relevant Files

**Reference Patterns**:
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (lines 27-42) - existing invalidation pattern
- `frontend/src/lib/api/client.ts` - axios client (already supports signal)

**Files to Modify**:
- `frontend/src/lib/api/assets/index.ts` (lines 29-42) - add signal support to list/get
- `frontend/src/hooks/assets/useAssets.ts` (lines 18-34) - validate org before store update
- `frontend/src/hooks/assets/useAsset.ts` (lines 18-29) - validate org before store update
- `frontend/src/hooks/assets/useAssetMutations.ts` (lines 14-47) - scope invalidation keys
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (lines 27-42) - cancel queries before invalidation
- `frontend/src/stores/assets/assetPersistence.ts` (line 18) - clear on org switch (via existing invalidateCache)

## Architecture Impact

- **Subsystems affected**: Frontend only (hooks, stores, API client)
- **New dependencies**: None
- **Breaking changes**: None - all changes are internal implementation

## Task Breakdown

### Task 1: Add Signal Support to Assets API

**File**: `frontend/src/lib/api/assets/index.ts`
**Action**: MODIFY
**Pattern**: Standard axios signal passing

**Implementation**:
```typescript
// Update ListAssetsOptions interface
export interface ListAssetsOptions {
  limit?: number;
  offset?: number;
  signal?: AbortSignal;  // Add this
}

// Update list method to pass signal
list: (options: ListAssetsOptions = {}) => {
  const { signal, ...params } = options;
  // ... existing param building ...
  return apiClient.get<ListAssetsResponse>(url, { signal });
},

// Update get method signature
get: (id: number, options?: { signal?: AbortSignal }) =>
  apiClient.get<AssetResponse>(`/assets/${id}`, { signal: options?.signal }),
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 2: Fix useAssets - Validate Org Before Store Update

**File**: `frontend/src/hooks/assets/useAssets.ts`
**Action**: MODIFY
**Pattern**: Capture org at fetch time, validate before side effects

**Implementation**:
```typescript
const query = useQuery({
  queryKey: ['assets', currentOrg?.id, pagination.currentPage, pagination.pageSize],
  queryFn: async ({ signal }) => {
    // Capture org ID at request time
    const orgIdAtFetch = currentOrg?.id;

    const offset = (pagination.currentPage - 1) * pagination.pageSize;
    const response = await assetsApi.list({
      limit: pagination.pageSize,
      offset,
      signal,
    });

    // Validate org hasn't changed before updating store
    const currentOrgId = useOrgStore.getState().currentOrg?.id;
    if (currentOrgId !== orgIdAtFetch) {
      console.warn('[useAssets] Discarding stale response - org changed during fetch');
      return response.data; // Return data but skip store update
    }

    useAssetStore.getState().addAssets(response.data.data);
    useTagStore.getState().refreshAssetEnrichment();
    return response.data;
  },
  enabled,
  refetchOnMount,
  staleTime: 60 * 60 * 1000,
});
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 3: Fix useAsset - Same Pattern as useAssets

**File**: `frontend/src/hooks/assets/useAsset.ts`
**Action**: MODIFY
**Pattern**: Same as Task 2

**Implementation**:
```typescript
const query = useQuery({
  queryKey: ['asset', currentOrg?.id, id],
  queryFn: async ({ signal }) => {
    if (!id) return null;

    // Capture org ID at request time
    const orgIdAtFetch = currentOrg?.id;

    const response = await assetsApi.get(id, { signal });
    const asset = response.data.data;

    // Validate org hasn't changed before updating store
    const currentOrgId = useOrgStore.getState().currentOrg?.id;
    if (currentOrgId !== orgIdAtFetch) {
      console.warn('[useAsset] Discarding stale response - org changed during fetch');
      return asset; // Return data but skip store update
    }

    useAssetStore.getState().addAsset(asset);
    return asset;
  },
  enabled: enabled && !!id && !asset,
  staleTime: 60 * 60 * 1000,
});
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 4: Fix useAssetMutations - Scope Invalidation Keys

**File**: `frontend/src/hooks/assets/useAssetMutations.ts`
**Action**: MODIFY
**Pattern**: Include org ID in invalidation query keys

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';

export function useAssetMutations() {
  const queryClient = useQueryClient();
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const createMutation = useMutation({
    // ... mutationFn unchanged ...
    onSuccess: (asset) => {
      useAssetStore.getState().addAsset(asset);
      queryClient.invalidateQueries({ queryKey: ['assets', currentOrg?.id] });
    },
  });

  const updateMutation = useMutation({
    // ... mutationFn unchanged ...
    onSuccess: (asset) => {
      useAssetStore.getState().updateCachedAsset(asset.id, asset);
      queryClient.invalidateQueries({ queryKey: ['assets', currentOrg?.id] });
      queryClient.invalidateQueries({ queryKey: ['asset', currentOrg?.id, asset.id] });
    },
  });

  const deleteMutation = useMutation({
    // ... mutationFn unchanged ...
    onSuccess: (id) => {
      useAssetStore.getState().removeAsset(id);
      queryClient.invalidateQueries({ queryKey: ['assets', currentOrg?.id] });
      queryClient.invalidateQueries({ queryKey: ['asset', currentOrg?.id, id] });
    },
  });

  // ... rest unchanged ...
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 5: Fix useOrgSwitch - Cancel Queries Before Invalidation

**File**: `frontend/src/hooks/orgs/useOrgSwitch.ts`
**Action**: MODIFY
**Pattern**: Cancel in-flight queries before clearing caches

**Implementation**:
```typescript
// Helper to invalidate all org-scoped caches
const invalidateOrgCaches = () => {
  // 1. Cancel all in-flight asset queries FIRST
  queryClient.cancelQueries({ queryKey: ['assets'] });
  queryClient.cancelQueries({ queryKey: ['asset'] });

  // 2. Clear all org-scoped Zustand stores
  useAssetStore.getState().invalidateCache();
  useLocationStore.getState().invalidateCache();
  useTagStore.getState().clearTags();
  useBarcodeStore.getState().clearBarcodes();

  // 3. Invalidate all org-scoped TanStack Query caches
  queryClient.invalidateQueries({
    predicate: (query) => {
      const key = query.queryKey[0];
      return key !== 'user' && key !== 'profile';
    },
  });
};
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 6: Clear Persisted Cache on Org Switch

**File**: `frontend/src/stores/assets/assetPersistence.ts`
**Action**: VERIFY (may already be handled)

The `invalidateCache()` method in assetStore resets the cache to initial state. When this runs, the persistence middleware should write the empty cache to localStorage.

**Verify** by checking `assetActions.ts` for `invalidateCache` implementation:
- If it sets `lastFetched: 0`, the TTL check in `assetPersistence.ts` will expire the cache on next load
- If needed, explicitly call `localStorage.removeItem('asset-store')` in `invalidateOrgCaches()`

**Implementation** (if needed):
```typescript
// In useOrgSwitch.ts invalidateOrgCaches()
// After useAssetStore.getState().invalidateCache();
localStorage.removeItem('asset-store');
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 7: Final Integration Test

**Action**: Manual testing

**Test Scenarios**:
1. Load Assets tab in Org A → switch to Org B → verify Org B assets appear
2. Switch from org with assets to org without → verify empty state
3. Rapidly switch A→B→A → verify no stale data flashes
4. Create asset → switch org → verify asset in correct org
5. Throttle network (DevTools) → switch org mid-request → verify correct result

**Validation**:
```bash
just frontend validate
just validate
```

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| AbortController not supported in old browsers | Axios handles gracefully, signal is optional |
| Race between cancel and response | Response validation catches any leaks |
| Tests don't cover race conditions | Manual testing required for timing issues |

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend
just lint       # Gate 1: Syntax & Style
just typecheck  # Gate 2: Type Safety
just test       # Gate 3: Unit Tests
```

Final validation:
```bash
just validate   # Full stack validation
```

**Do not proceed to next task until current task passes all gates.**

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and investigation
✅ All patterns exist in codebase (React Query, Zustand, axios)
✅ All clarifying questions answered
✅ No new dependencies required
✅ Small, focused changes to 5 files
⚠️ Race condition testing requires manual verification

**Assessment**: High confidence fix with well-understood patterns. The main uncertainty is edge case timing that requires manual testing.

**Estimated one-pass success probability**: 90%

**Reasoning**: All changes follow existing patterns, no new architecture, focused scope. Only risk is unforeseen race conditions that tests can't catch.
