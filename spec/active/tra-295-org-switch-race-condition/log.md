# Build Log: Fix Org Switch Race Condition

## Session: 2026-01-16
Starting task: 1
Total tasks: 7

---

### Task 1: Add Signal Support to Assets API
Started: 2026-01-16
File: `frontend/src/lib/api/assets/index.ts`
Status: ✅ Complete
Validation: lint clean, typecheck passed

Changes:
- Added `signal?: AbortSignal` to `ListAssetsOptions` interface
- Updated `list()` method to pass signal to axios
- Updated `get()` method to accept options with signal

---

### Task 2: Fix useAssets - Validate Org Before Store Update
Started: 2026-01-16
File: `frontend/src/hooks/assets/useAssets.ts`
Status: ✅ Complete
Validation: lint clean (0 errors), typecheck passed

Changes:
- Added `{ signal }` destructuring from query context
- Capture org ID at request time
- Validate org hasn't changed before updating store
- Pass signal to assetsApi.list()

---

### Task 3: Fix useAsset - Same Pattern
Started: 2026-01-16
File: `frontend/src/hooks/assets/useAsset.ts`
Status: ✅ Complete
Validation: typecheck passed

Changes:
- Added `{ signal }` destructuring from query context
- Capture org ID at request time
- Validate org hasn't changed before updating store
- Pass signal to assetsApi.get()

---

### Task 4: Fix useAssetMutations - Scope Invalidation Keys
Started: 2026-01-16
File: `frontend/src/hooks/assets/useAssetMutations.ts`
Status: ✅ Complete
Validation: typecheck passed

Changes:
- Import useOrgStore
- Get currentOrg from store
- Scope all invalidateQueries calls to include `currentOrg?.id`

---

### Task 5: Fix useOrgSwitch - Cancel Queries Before Invalidation
Started: 2026-01-16
File: `frontend/src/hooks/orgs/useOrgSwitch.ts`
Status: ✅ Complete
Validation: typecheck passed

Changes:
- Added `queryClient.cancelQueries({ queryKey: ['assets'] })` before store invalidation
- Added `queryClient.cancelQueries({ queryKey: ['asset'] })` before store invalidation
- Added `localStorage.removeItem('asset-store')` to clear persisted cache

---

### Task 6: Clear Persisted Cache on Org Switch
Status: ✅ Complete (combined with Task 5)

Changes:
- Added localStorage.removeItem('asset-store') in invalidateOrgCaches()

---

### Task 7: Final Validation
Started: 2026-01-16
Status: ✅ Complete

Validation Results:
- Lint: 0 errors (298 warnings - pre-existing)
- Typecheck: passed
- Tests: 831 passed, 32 skipped
- Build: successful

Test Fixes Required:
- Updated `assets.test.ts` to expect signal parameter in API calls
- Updated `useAsset.test.ts` and `useAssets.test.ts` to mock `useOrgStore.getState()`

---

## Summary
Total tasks: 7
Completed: 7
Failed: 0
Duration: ~15 minutes

Ready for /check: YES

## Files Modified
1. `frontend/src/lib/api/assets/index.ts` - Signal support
2. `frontend/src/hooks/assets/useAssets.ts` - Org validation
3. `frontend/src/hooks/assets/useAsset.ts` - Org validation
4. `frontend/src/hooks/assets/useAssetMutations.ts` - Scoped invalidation
5. `frontend/src/hooks/orgs/useOrgSwitch.ts` - Cancel queries + clear cache
6. `frontend/src/lib/api/assets/assets.test.ts` - Test updates
7. `frontend/src/hooks/assets/useAsset.test.ts` - Test updates
8. `frontend/src/hooks/assets/useAssets.test.ts` - Test updates
