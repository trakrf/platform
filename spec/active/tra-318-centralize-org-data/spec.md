# Feature: Centralize Org-Scoped Data Management

## Origin
This specification emerged from debugging TRA-314 (inventory save flow) and discovering scattered org-scoping patterns across the codebase. User feedback: *"is there not one central location that is handling this org logic? it should be DRY. no reason for it to be scattered across tabs"*

## Linear Issue
- **TRA-318**: Locations not showing after logout → login → navigate to Locations tab
- **Blocking**: TRA-314 (inventory save flow)
- **Related**: TRA-320 (block deletion of last org - separate ticket)

## Outcome
A single, centralized mechanism for managing org-scoped data invalidation and refetch. When org context changes (login, logout, org switch), all org-scoped data is cleared and refetched from one place—not scattered across individual stores and hooks.

## User Story
As a developer maintaining the frontend,
I want org-scoped data management in one place,
So that I never have to hunt down cache invalidation bugs across multiple files.

## Context

### Discovery
The root cause of the "locations not showing after login" bug traces back to a race condition:

1. **Login Flow** (authStore.ts:44-77):
   - Login API returns token **without** `org_id` claim
   - `fetchProfile()` is called to get org data
   - `setCurrentOrg()` is called to get a token **with** `org_id` claim
   - Profile change triggers `orgStore.syncFromProfile()` → invalidates caches

2. **The Race**: React-query hooks may fire before the token refresh completes:
   - `useLocations` has queryKey `['locations', currentOrg?.id]`
   - If `currentOrg` is already set from previous session (localStorage), query fires immediately
   - API call uses old token without valid `org_id` → fails or returns empty
   - Later token refresh doesn't auto-refetch because query key didn't change

### Current State: Scattered Invalidation
Org-scoped cache invalidation happens in **4+ places**:

| Location | What It Invalidates |
|----------|---------------------|
| `orgStore.syncFromProfile()` | assetStore, locationStore |
| `orgStore.switchOrg()` | assetStore, locationStore |
| `useOrgSwitch.invalidateOrgCaches()` | assetStore, locationStore, tagStore, barcodeStore, react-query caches, localStorage |
| `tagStore` subscription | Self (enrichedOrgId tracking) |

**Problems**:
1. Different operations invalidate different stores
2. No single source of truth for "what is org-scoped"
3. React-query invalidation only in `useOrgSwitch` hook—not triggered on login
4. `enrichedOrgId` tracking in tagStore is a workaround for the scattered pattern

### Desired State
One function, one place, one contract:

```typescript
// Central function that EVERY org context change calls
function invalidateAllOrgScopedData(queryClient: QueryClient): void {
  // 1. Cancel in-flight queries
  // 2. Clear all org-scoped Zustand stores
  // 3. Clear react-query caches
  // 4. Clear localStorage
}
```

Called from:
- `authStore.login()` - after token refresh completes
- `authStore.logout()` - clear all caches
- `orgStore.switchOrg()` - on org switch
- `useOrgSwitch.createOrg()` - on new org creation

## Technical Requirements

### 1. Create Centralized Invalidation Service
Create `src/lib/cache/orgScopedCache.ts`:

```typescript
import type { QueryClient } from '@tanstack/react-query';

/**
 * Registry of all org-scoped stores.
 * Each store must implement invalidateCache() or clearFn().
 */
const ORG_SCOPED_STORES = [
  { name: 'assets', getStore: () => import('@/stores/assets/assetStore').then(m => m.useAssetStore) },
  { name: 'locations', getStore: () => import('@/stores/locations/locationStore').then(m => m.useLocationStore) },
  { name: 'tags', getStore: () => import('@/stores/tagStore').then(m => m.useTagStore), clearFn: 'clearTags' },
  { name: 'barcodes', getStore: () => import('@/stores/barcodeStore').then(m => m.useBarcodeStore), clearFn: 'clearBarcodes' },
] as const;

const ORG_SCOPED_LOCALSTORAGE_KEYS = ['asset-store'];

const ORG_SCOPED_QUERY_PREFIXES = ['assets', 'asset', 'locations', 'location', 'lookup'];

/**
 * Invalidates ALL org-scoped data across the application.
 * Call this when org context changes (login, logout, org switch).
 */
export async function invalidateAllOrgScopedData(queryClient: QueryClient): Promise<void> {
  // 1. Cancel in-flight queries AND mutations first
  // Mutations matter: user mid-save during org switch would corrupt data
  for (const prefix of ORG_SCOPED_QUERY_PREFIXES) {
    queryClient.cancelQueries({ queryKey: [prefix] });
    queryClient.cancelMutations({ mutationKey: [prefix] });
  }

  // 2. Clear Zustand stores
  for (const { getStore, clearFn } of ORG_SCOPED_STORES) {
    const store = await getStore();
    const fn = clearFn || 'invalidateCache';
    store.getState()[fn]?.();
  }

  // 3. Clear localStorage
  for (const key of ORG_SCOPED_LOCALSTORAGE_KEYS) {
    localStorage.removeItem(key);
  }

  // 4. Invalidate react-query caches
  queryClient.invalidateQueries({
    predicate: (query) => {
      const key = query.queryKey[0];
      return typeof key === 'string' && ORG_SCOPED_QUERY_PREFIXES.includes(key);
    },
  });
}
```

### 2. Expose QueryClient at Module Level
The challenge: `authStore` and `orgStore` are Zustand stores without React context access, but `queryClient` lives in React context.

**Solution**: Create a singleton query client accessor:

```typescript
// src/lib/queryClient.ts
import { QueryClient } from '@tanstack/react-query';

let queryClientInstance: QueryClient | null = null;

export function setQueryClient(client: QueryClient): void {
  queryClientInstance = client;
}

export function getQueryClient(): QueryClient {
  if (!queryClientInstance) {
    throw new Error('QueryClient not initialized. Ensure setQueryClient is called in App.');
  }
  return queryClientInstance;
}
```

Register in App.tsx:
```typescript
const queryClient = useQueryClient();
useEffect(() => {
  setQueryClient(queryClient);
}, [queryClient]);
```

### 3. Update Auth Flow to Use Central Invalidation

**authStore.ts - login()** (lines 44-77):
```typescript
login: async (email, password) => {
  // ... existing login, set token/user ...

  await get().fetchProfile();

  const profile = get().profile;
  if (profile?.current_org?.id) {
    const orgResponse = await orgsApi.setCurrentOrg({ org_id: profile.current_org.id });
    set({ token: orgResponse.data.token });

    // INVALIDATE HERE: After setCurrentOrg() returns with org_id token
    // NOT before, NOT after fetchProfile() alone
    const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
    const { getQueryClient } = await import('@/lib/queryClient');
    await invalidateAllOrgScopedData(getQueryClient());
  }
}
```

**authStore.ts - logout()**:
```typescript
logout: () => {
  // Clear auth state
  set({ user: null, token: null, isAuthenticated: false, profile: null });

  // Clear all org-scoped data
  import('@/lib/cache/orgScopedCache').then(({ invalidateAllOrgScopedData }) => {
    import('@/lib/queryClient').then(({ getQueryClient }) => {
      invalidateAllOrgScopedData(getQueryClient());
    });
  });
}
```

### 4. Simplify orgStore
Remove duplicate invalidation from `orgStore.syncFromProfile()` and `orgStore.switchOrg()`. Instead, these should call the central function.

### 5. Simplify useOrgSwitch Hook
Replace `invalidateOrgCaches()` with call to central function.

### 6. Remove Scattered Subscriptions
- **Remove** `enrichedOrgId` field from tagStore state entirely
- **Remove** the `useOrgStore.subscribe()` block in tagStore (lines 523-569)
- The central `invalidateAllOrgScopedData()` now handles tag clearing via the registry
- No need for stores to independently track org changes

## Validation Criteria

### Unit Tests
- [ ] `invalidateAllOrgScopedData()` clears all registered stores
- [ ] `invalidateAllOrgScopedData()` cancels in-flight queries
- [ ] `invalidateAllOrgScopedData()` cancels pending mutations
- [ ] `invalidateAllOrgScopedData()` clears localStorage keys
- [ ] Adding a new store to registry causes it to be cleared
- [ ] tagStore no longer has `enrichedOrgId` field

### Integration Tests
- [ ] **TRA-318 repro**: Logout → Login → Navigate to Locations → Locations shown
- [ ] Org switch clears all caches and refetches with new org context
- [ ] Page reload after login fetches fresh data (no stale cache)

### Manual Verification
- [ ] Console shows single "[OrgCache] Invalidating all org-scoped data" log on org change
- [ ] No duplicate invalidation logs from multiple sources

## Files to Modify

| File | Changes |
|------|---------|
| `src/lib/cache/orgScopedCache.ts` | **NEW** - Central invalidation service |
| `src/lib/queryClient.ts` | **NEW** - QueryClient singleton accessor |
| `src/App.tsx` | Register queryClient singleton |
| `src/stores/authStore.ts` | Call central invalidation on login/logout |
| `src/stores/orgStore.ts` | Remove duplicate invalidation, use central |
| `src/hooks/orgs/useOrgSwitch.ts` | Remove `invalidateOrgCaches()`, use central |
| `src/stores/tagStore.ts` | Remove `enrichedOrgId` field and org subscription |

## Out of Scope
- Backend changes to include `org_id` in initial login token (would fix root cause but is a larger change)
- React-query global defaults for org-scoped queries
- Automatic refetch on org change (invalidation triggers refetch on next access)
- **TRA-320**: Block deletion of user's last org (separate ticket created)

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Dynamic import delays | Use top-level imports where possible; lazy imports only for code-splitting |
| Circular dependencies | `orgScopedCache.ts` imports stores dynamically to avoid cycles |
| Missing store in registry | Document registry update process; add ESLint rule if possible |

## References
- TRA-314 commit history: `88cf5a8`, `3f490f5`, `70ac416`
- User feedback: "it should be DRY. no reason for it to be scattered across tabs"
- Key files: `authStore.ts:44-77`, `orgStore.ts:38-47`, `useOrgSwitch.ts:27-49`
