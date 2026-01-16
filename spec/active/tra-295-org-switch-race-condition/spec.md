# Feature: Fix Org Switch Race Condition in Asset Fetching

## Origin
This specification emerged from debugging TRA-295, a follow-on issue from TRA-189. While TRA-189 fixed cache clearing on org switch, TRA-295 reveals a race condition where asset fetches use stale org context.

## Outcome
Switching organizations while on the Assets tab will reliably show the correct org's assets without requiring a hard refresh.

## User Story
As a multi-org user
I want org switches to immediately show the correct organization's assets
So that I don't see confusing stale/empty data or need to refresh the page

## Context

**Discovery**: Investigation revealed multiple race conditions in how org ID flows through the asset fetching pipeline:
1. Query functions capture org ID in closures but don't enforce it
2. In-flight requests complete after org switch and write wrong data to store
3. Query invalidation keys are too broad (missing org ID)
4. localStorage cache has no org namespace

**Current State**:
- `useAssets` includes `currentOrg?.id` in query key but `queryFn` doesn't use it
- Mutations invalidate with `['assets']` instead of `['assets', orgId]`
- Asset store receives data without validating it matches current org
- Intermittent: works when navigating TO assets, fails when already there during switch

**Desired State**:
- All asset fetches are scoped to explicit org ID
- In-flight requests for wrong org are ignored/cancelled
- Query invalidation is org-scoped
- No stale data can leak between org contexts

## Technical Requirements

### 1. Enforce Org ID in Asset Query Functions (Critical)
**File**: `frontend/src/hooks/assets/useAssets.ts`

- Capture `currentOrg?.id` at query execution time, not just in key
- Validate response belongs to current org before updating store
- Consider using `AbortController` to cancel stale requests

```typescript
// Current (problematic)
queryFn: async () => {
  const response = await assetsApi.list({ limit, offset });
  useAssetStore.getState().addAssets(response.data.data); // No org check!
  return response.data;
}

// Fixed
queryFn: async ({ signal }) => {
  const orgIdAtFetch = currentOrg?.id;
  const response = await assetsApi.list({ limit, offset }, { signal });

  // Only update store if org hasn't changed
  if (useOrgStore.getState().currentOrg?.id === orgIdAtFetch) {
    useAssetStore.getState().addAssets(response.data.data);
  }
  return response.data;
}
```

### 2. Scope Query Invalidation to Org ID
**File**: `frontend/src/hooks/assets/useAssetMutations.ts`

- Include org ID in all `invalidateQueries` calls
- Ensures mutations only affect current org's cache

```typescript
// Current
queryClient.invalidateQueries({ queryKey: ['assets'] });

// Fixed
queryClient.invalidateQueries({ queryKey: ['assets', currentOrg?.id] });
```

### 3. Cancel In-Flight Queries on Org Switch
**File**: `frontend/src/hooks/orgs/useOrgSwitch.ts`

- Cancel pending asset queries before clearing cache
- Use React Query's `cancelQueries` before `invalidateQueries`

```typescript
const invalidateOrgCaches = () => {
  // Cancel first, then invalidate
  queryClient.cancelQueries({ queryKey: ['assets'] });
  queryClient.cancelQueries({ queryKey: ['asset'] });
  queryClient.invalidateQueries({ queryKey: ['assets'] });
  // ... rest of invalidation
};
```

### 4. Add Org to AssetsScreen Memoization Dependencies
**File**: `frontend/src/components/AssetsScreen.tsx`

- Include org context in `useMemo` dependency array
- Ensures filtered results update on org change

### 5. (Optional) Namespace localStorage Cache by Org
**File**: `frontend/src/stores/assets/assetPersistence.ts`

- Consider namespacing storage key: `asset-store-${orgId}`
- Or clear persisted cache on org switch
- Lower priority if other fixes resolve the issue

## Validation Criteria
- [ ] Switch orgs while on Assets tab → correct assets appear immediately
- [ ] Switch between orgs with different asset counts → counts match
- [ ] Rapid org switching (A→B→A) → no stale data flashes
- [ ] Hard refresh no longer required after org switch
- [ ] No console errors about cancelled/aborted requests
- [ ] Existing asset CRUD operations still work correctly

## Test Scenarios
1. **Already on Assets tab**: View Org A assets → switch to Org B → verify Org B assets
2. **Empty vs populated**: Switch from org with assets to org without → verify empty state
3. **Race condition**: Throttle network → switch org mid-request → verify correct result
4. **Mutation during switch**: Create asset → immediately switch org → verify asset in correct org

## Non-Goals
- Changing the API to accept explicit org ID (backend already uses JWT org context)
- Restructuring the entire state management approach
- Adding org ID to every single query in the app (focus on assets for now)

## Files to Modify
1. `frontend/src/hooks/assets/useAssets.ts` - Primary fix
2. `frontend/src/hooks/assets/useAsset.ts` - Same pattern
3. `frontend/src/hooks/assets/useAssetMutations.ts` - Invalidation keys
4. `frontend/src/hooks/orgs/useOrgSwitch.ts` - Cancel queries
5. `frontend/src/components/AssetsScreen.tsx` - Memoization deps

## Related Issues
- TRA-189: Original stale data issue (Done, but incomplete)
- TRA-295: This race condition issue
