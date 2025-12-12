# Feature: Org Switch Stale Data Fix

**Linear**: [TRA-189](https://linear.app/trakrf/issue/TRA-189/org-switch-shows-stale-asset-data-from-previous-org)
**Priority**: High
**Branch**: `miks2u/tra-189-org-switch-shows-stale-asset-data-from-previous-org`

## Origin

This specification addresses a data isolation bug where switching organizations shows stale asset data from the previous org, even after page refresh.

## Outcome

After switching organizations, all org-scoped data (assets, locations) is immediately cleared and fresh data is fetched for the newly selected organization.

## User Story

As a **multi-org user**
I want **data to refresh when I switch organizations**
So that **I see accurate data for my current organization, not stale data from another org**

## Context

**Discovery**: Codebase exploration revealed three root causes:

1. **TanStack Query keys lack org ID** - Query keys like `['assets', page, size]` don't include `currentOrg.id`, so the cache returns old data when org changes
2. **Zustand persistence isn't org-scoped** - The `'asset-store'` localStorage key stores data without org context
3. **No cache invalidation on org switch** - Unlike MembersScreen/OrgSettingsScreen which watch `currentOrg`, AssetsScreen and LocationsScreen have no org-change listeners

**Current State**:
- `switchOrg()` correctly updates backend and fetches new profile
- Query `staleTime: 60 * 60 * 1000` (1 hour) prevents automatic refetch
- Assets/locations persist in localStorage under non-org-scoped keys
- No cleanup triggered when `currentOrg` changes

**Desired State**:
- All org-scoped queries invalidated on org switch
- Query keys include org ID for proper cache isolation
- Zustand stores cleared on org switch
- Optional: org-scoped localStorage keys for cross-session persistence

## Guiding Principle

**All UI state is org-scoped EXCEPT:**
- User details (auth store - token, user profile)
- Active BLE connection state (live hardware connections to physical devices)

**Org-scoped (reset on switch):**
- Assets, locations, tags, barcodes
- Device settings/configuration (devices table has org_id)
- Filters, pagination, sort state

**Note on devices**: The `devices` table is org-scoped, so device settings data should reset. However, an active BLE connection to a physical reader should persist - the user may want to stay connected to their hardware even when viewing a different org's data.

**Schema confirmation**: Backend migrations confirm `org_id` exists on nearly every table:
- `assets`, `locations`, `identifiers`, `scan_devices`, `scan_points`, `asset_scans`, `bulk_import_jobs`, `org_users`, `org_invitations`
- All use RLS policy: `org_id = current_setting('app.current_org_id')::INT`

## Technical Requirements

### 1. Add Org ID to Query Keys

Update TanStack Query keys to include current org ID:

```typescript
// Before
queryKey: ['assets', pagination.currentPage, pagination.pageSize]

// After
queryKey: ['assets', currentOrg?.id, pagination.currentPage, pagination.pageSize]
```

**Files to modify:**
- `frontend/src/hooks/assets/useAssets.ts` (line 17)
- `frontend/src/hooks/assets/useAsset.ts` (line 17)
- `frontend/src/hooks/locations/useLocations.ts`
- `frontend/src/hooks/locations/useLocation.ts`

### 2. Invalidate Caches on Org Switch

Add cache invalidation to `switchOrg()` action:

```typescript
// In orgStore.ts switchOrg()
import { useAssetStore } from '../assets/assetStore';
import { useTagStore } from '../tags/tagStore';
import { useBarcodeStore } from '../barcodes/barcodeStore';
import { useLocationStore } from '../locations/locationStore';
import { queryClient } from '@/lib/queryClient';

switchOrg: async (orgId: string) => {
  set({ isLoading: true, error: null });
  try {
    await orgsApi.setCurrentOrg({ org_id: orgId });

    // Clear ALL org-scoped Zustand stores
    // (Everything except auth and BLE connections)
    useAssetStore.getState().invalidateCache();
    useTagStore.getState().invalidateCache();
    useBarcodeStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();

    // Clear ALL org-scoped TanStack Query caches
    // More aggressive: clear everything except user/auth queries
    queryClient.invalidateQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        // Keep only auth-related queries
        return key !== 'user' && key !== 'profile';
      }
    });

    await useAuthStore.getState().fetchProfile();
  } catch (error) {
    // error handling
  }
}
```

**File to modify:**
- `frontend/src/stores/orgStore.ts` (lines 53-78)

### 3. Clear Zustand Stores on Org Switch

Ensure `invalidateCache()` action properly clears all cached state:

```typescript
// In assetStore.ts or assetActions.ts
invalidateCache: () => {
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
    filters: defaultFilters,
    pagination: defaultPagination,
  });
}
```

**Files to verify/modify:**
- `frontend/src/stores/assets/assetActions.ts` (invalidateCache)
- `frontend/src/stores/locations/locationStore.ts` (add similar invalidation)
- `frontend/src/stores/tags/tagStore.ts` (add similar invalidation)
- `frontend/src/stores/barcodes/barcodeStore.ts` (add similar invalidation)

### 4. Reset Filters and Pagination

Clear filters/pagination state on org switch since they may not be relevant to the new org:

```typescript
// Reset to defaults when switching org
filters: { ...defaultFilters },
pagination: { currentPage: 1, pageSize: 25, total: 0 },
```

## Out of Scope

- **Org-scoped localStorage keys** (e.g., `asset-store-${orgId}`) - would preserve per-org caches across sessions but adds complexity
- **E2E Playwright coverage** - deferred to TRA-175 (E2E test infrastructure)

## Validation Criteria

- [ ] Switching from Org A to Org B shows Org B's assets immediately
- [ ] Page refresh after org switch shows correct org data
- [ ] Filter state resets when switching orgs
- [ ] No console errors during org switch
- [ ] MembersScreen and OrgSettingsScreen continue to work correctly
- [ ] Performance: no unnecessary API calls when staying in same org

## Test Cases

1. **Basic switch**: View assets in Org A → switch to Org B → verify Org B assets displayed
2. **With pagination**: Navigate to page 2 in Org A → switch to Org B → verify page 1 of Org B
3. **With filters**: Apply filter in Org A → switch to Org B → verify filters reset
4. **Refresh persistence**: Switch to Org B → refresh page → verify Org B data persists
5. **Rapid switching**: Switch A → B → A quickly → verify correct data each time

## Files Summary

| File | Change |
|------|--------|
| `stores/orgStore.ts` | Add comprehensive cache invalidation to `switchOrg()` |
| `hooks/assets/useAssets.ts` | Add `currentOrg?.id` to queryKey |
| `hooks/assets/useAsset.ts` | Add `currentOrg?.id` to queryKey |
| `hooks/locations/useLocations.ts` | Add `currentOrg?.id` to queryKey |
| `hooks/locations/useLocation.ts` | Add `currentOrg?.id` to queryKey |
| `stores/assets/assetActions.ts` | Verify/add invalidateCache resets all state |
| `stores/locations/locationStore.ts` | Verify/add invalidateCache action |
| `stores/tags/tagStore.ts` | Verify/add invalidateCache action |
| `stores/barcodes/barcodeStore.ts` | Verify/add invalidateCache action |

**Stores to preserve (NOT reset on org switch):**
- `authStore` - user token/profile
- `bleStore` - active hardware BLE connections only (not device settings/config)

## Implementation Notes

- The fix is primarily frontend - no backend changes required
- Pattern already exists in MembersScreen (line 135 useEffect with currentOrg dependency)
- Consider creating a `useOrgScopedQuery()` hook for future org-scoped data to enforce this pattern
