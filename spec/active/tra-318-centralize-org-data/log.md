# Build Log: Centralize Org-Scoped Data Management

## Session: 2026-01-23

### Completed Tasks

#### Task 1: Create queryClient singleton ✅
- Created `frontend/src/lib/queryClient.ts` with centralized QueryClient creation
- Configured with standard options (staleTime, gcTime, retry, refetchOnWindowFocus)

#### Task 2: Update main.tsx to use queryClient singleton ✅
- Updated `frontend/src/main.tsx` to import from `@/lib/queryClient`
- Removed inline QueryClient creation

#### Task 3: Create central org-scoped cache invalidation ✅
- Created `frontend/src/lib/cache/orgScopedCache.ts`
- Implemented registry-based invalidation with:
  - ORG_SCOPED_STORES: Dynamic imports for assetStore, locationStore, tagStore, barcodeStore
  - ORG_SCOPED_LOCALSTORAGE_KEYS: ['asset-store']
  - ORG_SCOPED_QUERY_PREFIXES: ['assets', 'asset', 'locations', 'location', 'lookup']
- Uses dynamic imports to avoid circular dependencies

#### Task 4: Update authStore login() ✅
- Added central invalidation call AFTER setCurrentOrg() returns with org_id token
- Uses dynamic imports for orgScopedCache and queryClient

#### Task 5: Update authStore logout() ✅
- Added central invalidation call via Promise.all with dynamic imports

#### Task 6: Update orgStore ✅
- switchOrg() now calls central invalidation after setCurrentOrg
- Removed duplicate invalidation from syncFromProfile()
- Removed unused imports (useAssetStore, useLocationStore)
- Fixed TypeScript error: removed unused `get` parameter

#### Task 7: Update useOrgSwitch hook ✅
- Simplified to delegate to orgStore.switchOrg()
- createOrg() still calls central invalidation directly (for new org creation flow)
- Removed invalidateOrgCaches() helper function

#### Task 8: Update tagStore ✅
- Removed `enrichedOrgId` from interface, state, and persistence
- Removed org subscription block (lines 522-569)
- Added module-level canary variable `lastEnrichmentOrgId`
- Added console.warn in _flushLookupQueue if stale data detected

#### Task 9: Create unit tests ✅
- Created `frontend/src/lib/cache/__tests__/orgScopedCache.unit.test.ts`
- 9 tests covering:
  - Logging behavior
  - Query cancellation for all prefixes
  - localStorage clearing
  - React-query cache invalidation with predicate
  - Registry completeness verification

#### Task 10: Create integration tests ✅
- Created `frontend/src/lib/cache/__tests__/orgScopedCache.integration.test.ts`
- Tests verify registry configuration without conflicting with unit test mocks
- Note: vitest runs in singleFork mode (for hardware tests), causing mock leakage

#### Task 11: Update existing tests ✅
- Updated `frontend/src/stores/authStore.test.ts`:
  - Added mocks for orgsApi, orgScopedCache, queryClient
  - Updated login/signup/persistence tests to mock the full flow
- Updated `frontend/src/hooks/orgs/useOrgSwitch.test.ts`:
  - Removed individual store spy expectations
  - Added mock for central invalidation function
  - Updated to expect orgStore.switchOrg delegation

### Test Results
- **902 tests passing, 26 skipped**
- Full validation passes (lint, typecheck, test, build)

### Files Changed
- `frontend/src/lib/queryClient.ts` (NEW)
- `frontend/src/lib/cache/orgScopedCache.ts` (NEW)
- `frontend/src/lib/cache/__tests__/orgScopedCache.unit.test.ts` (NEW)
- `frontend/src/lib/cache/__tests__/orgScopedCache.integration.test.ts` (NEW)
- `frontend/src/main.tsx` (MODIFIED)
- `frontend/src/stores/authStore.ts` (MODIFIED)
- `frontend/src/stores/authStore.test.ts` (MODIFIED)
- `frontend/src/stores/orgStore.ts` (MODIFIED)
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (MODIFIED)
- `frontend/src/hooks/orgs/useOrgSwitch.test.ts` (MODIFIED)
- `frontend/src/stores/tagStore.ts` (MODIFIED)

### Notes
- Dynamic imports used everywhere for consistency (per user preference)
- Module-level canary added to tagStore for stale data detection
- TRA-320 created for "block delete last org" as separate ticket
