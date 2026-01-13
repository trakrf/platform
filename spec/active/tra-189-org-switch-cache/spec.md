# Feature: Org Switch Cache Invalidation

## Origin
This specification addresses TRA-189: "Org switch shows stale asset data from previous org" and a related issue where new signups with new orgs don't have proper access.

## Outcome
Switching organizations will reliably clear all org-scoped cached data, and new organization creation will properly establish access for the user.

## User Stories

**As a user switching between organizations**
I want all cached data cleared when I switch orgs
So that I only see data belonging to my current organization

**As a new user creating their first organization**
I want immediate access to my newly created org
So that I can start using the platform without manual intervention

## Context

### Discovery
Code analysis revealed two root causes:

**Issue 1: Stale Data on Org Switch**
- `OrgSwitcher.tsx:24` uses `useOrgStore().switchOrg()` directly
- The `useOrgSwitch()` hook exists with proper cache invalidation but is NOT being used
- `useOrgSwitch()` properly clears: AssetStore, LocationStore, TagStore, BarcodeStore + TanStack Query caches

**Issue 2: New Org Access**
- `createOrg` in orgStore calls `fetchProfile()` to update the org list
- BUT unlike `switchOrg`, it does NOT update the JWT token with the new org_id claim
- No cache invalidation happens after org creation

### Current State
```
OrgSwitcher.tsx:24
  const { switchOrg } = useOrgStore();  // <-- Bypasses cache invalidation!

orgStore.switchOrg():
  - Calls backend API
  - Updates JWT token
  - Fetches profile
  - Does NOT clear caches (expects useOrgSwitch hook to handle this)

orgStore.createOrg():
  - Calls backend API
  - Fetches profile
  - Does NOT update JWT token
  - Does NOT clear caches
```

### Desired State
```
OrgSwitcher.tsx:
  const { switchOrg } = useOrgSwitch();  // <-- Uses hook with cache invalidation

All org transitions (switch AND create) should:
  1. Update JWT token with new org_id claim
  2. Clear all org-scoped Zustand stores
  3. Invalidate all org-scoped TanStack Query caches
  4. Refetch profile for updated state
```

## Technical Requirements

### 1. Fix OrgSwitcher to use useOrgSwitch hook
**File:** `frontend/src/components/OrgSwitcher.tsx`

Change from:
```typescript
const { currentOrg, currentRole, orgs, isLoading, switchOrg } = useOrgStore();
```

To:
```typescript
const { currentOrg, currentRole, orgs, isLoading } = useOrgStore();
const { switchOrg } = useOrgSwitch();
```

### 2. Add cache invalidation to org creation flow
**File:** `frontend/src/hooks/orgs/useOrgSwitch.ts`

Extend the hook to also handle org creation with cache invalidation:
```typescript
export function useOrgSwitch() {
  // ... existing code

  const createOrg = async (name: string) => {
    // 1. Create org via store (gets new token)
    const newOrg = await storeCreateOrg(name);

    // 2. Clear all org-scoped caches (same as switchOrg)
    invalidateAllOrgCaches();

    return newOrg;
  };

  return { switchOrg, createOrg, isLoading };
}
```

### 3. Ensure createOrg updates JWT token
**File:** `frontend/src/stores/orgStore.ts`

The `createOrg` function should update the token similar to `switchOrg`:
```typescript
createOrg: async (name: string) => {
  const response = await orgsApi.create({ name });
  const newOrg = response.data.data;

  // If backend returns new token, update it
  if (response.data.token) {
    useAuthStore.setState({ token: response.data.token });
  }

  await fetchProfile();
  // ...
}
```

### 4. Update useOrgModal to use the hook
**File:** `frontend/src/components/useOrgModal.ts`

Change from:
```typescript
const { createOrg } = useOrgStore();
```

To:
```typescript
const { createOrg } = useOrgSwitch();
```

### 5. Verify backend returns token on org creation
**File:** `backend/internal/handlers/orgs.go` (verify only)

Ensure the CreateOrg handler returns a new JWT with the created org_id, similar to SetCurrentOrg.

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/OrgSwitcher.tsx` | Use `useOrgSwitch()` instead of `useOrgStore().switchOrg` |
| `frontend/src/hooks/orgs/useOrgSwitch.ts` | Add `createOrg` function with cache invalidation |
| `frontend/src/stores/orgStore.ts` | Update `createOrg` to handle token from response |
| `frontend/src/components/useOrgModal.ts` | Use `useOrgSwitch().createOrg` |
| `backend/internal/handlers/orgs.go` | Verify CreateOrg returns JWT (check only) |

## Validation Criteria

- [ ] Switching orgs clears AssetStore cache (no stale assets displayed)
- [ ] Switching orgs clears LocationStore cache
- [ ] Switching orgs clears TagStore and BarcodeStore caches
- [ ] Switching orgs invalidates all TanStack Query caches (except auth)
- [ ] Creating new org immediately grants access (no 403/401 errors)
- [ ] Creating org as new user works without refresh
- [ ] LocalStorage `asset-store` key is cleared on org switch
- [ ] Page refresh after org switch shows correct org's data

## Test Scenarios

### Manual Testing
1. Log in, view assets in Org A
2. Switch to Org B via dropdown
3. Verify Org B's assets are displayed (not Org A's)
4. Refresh page - verify Org B's assets persist

### New User Flow
1. Sign up new account
2. Create first organization
3. Verify immediate access to org
4. Verify can view/create assets without error

## Edge Cases

- User with single org (no switch needed but cache should still be org-aware)
- User switching to org with no assets (should show empty state, not old data)
- Network failure during switch (should not leave partial state)
- Rapid org switching (debounce/queue switches)

## Related Issues

- Linear: TRA-189
- Previous PR: https://github.com/trakrf/platform/pull/77 (partial fix)

## Implementation Notes

The `useOrgSwitch` hook was created to centralize cache invalidation but wasn't wired up to the UI. This is a straightforward fix - just need to use the existing hook consistently.

For the new signup flow, we need to verify the backend is returning a proper JWT on org creation. If not, we may need a backend change.
