# Implementation Plan: Centralize Org-Scoped Data Management

Generated: 2026-01-23
Specification: spec.md

## Understanding

This plan centralizes org-scoped cache invalidation into a single function. Currently, invalidation happens in 4+ scattered locations (orgStore, useOrgSwitch, tagStore subscriptions), causing race conditions where react-query fires before token refresh completes. The fix: one `invalidateAllOrgScopedData()` function called from all org context changes.

**Key insight from spec**: The invalidation must happen AFTER `setCurrentOrg()` returns with the org_id token, not just after `fetchProfile()`.

## Clarifying Questions - Answers

1. **Dynamic imports**: Use everywhere for consistency and cycle safety
2. **enrichedOrgId**: Remove entirely, add console.warn canary during enrichment
3. **QueryClient**: Create directly in `lib/queryClient.ts`, export instance (option b)
4. **Tests**: Both mock (verify wiring) and integration (verify behavior)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/main.tsx` (lines 31-40) - Current QueryClient creation pattern
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (lines 27-49) - Existing invalidation pattern to centralize
- `frontend/src/hooks/orgs/useOrgSwitch.test.ts` - Test pattern with mocked stores

**Files to Create**:
- `frontend/src/lib/queryClient.ts` - QueryClient singleton export
- `frontend/src/lib/cache/orgScopedCache.ts` - Central invalidation service
- `frontend/src/lib/cache/orgScopedCache.test.ts` - Unit tests (mock wiring)
- `frontend/src/lib/cache/orgScopedCache.integration.test.ts` - Integration tests (real stores)

**Files to Modify**:
- `frontend/src/main.tsx` - Import queryClient from new location
- `frontend/src/stores/authStore.ts` (lines 67-77, 168-179) - Call central invalidation
- `frontend/src/stores/orgStore.ts` (lines 44-47, 69-71) - Remove duplicate invalidation
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (lines 27-49) - Replace with central function
- `frontend/src/stores/tagStore.ts` (lines 59, 478, 522-569) - Remove enrichedOrgId and subscription

## Architecture Impact

- **Subsystems affected**: Zustand stores, React-Query cache
- **New dependencies**: None
- **Breaking changes**: None (internal refactor)

## Task Breakdown

### Task 1: Create QueryClient Module
**File**: `frontend/src/lib/queryClient.ts`
**Action**: CREATE

**Implementation**:
```typescript
import { QueryClient } from '@tanstack/react-query';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 minutes
      gcTime: 10 * 60 * 1000, // 10 minutes
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});
```

**Validation**: `just frontend typecheck`

---

### Task 2: Update main.tsx to Import QueryClient
**File**: `frontend/src/main.tsx`
**Action**: MODIFY

**Changes**:
- Remove QueryClient creation (lines 31-40)
- Import from `@/lib/queryClient`

**Implementation**:
```typescript
// Remove this:
// const queryClient = new QueryClient({...});

// Add this:
import { queryClient } from '@/lib/queryClient';
```

**Validation**: `just frontend typecheck && just frontend build`

---

### Task 3: Create Central Invalidation Service
**File**: `frontend/src/lib/cache/orgScopedCache.ts`
**Action**: CREATE
**Pattern**: Reference `useOrgSwitch.ts` lines 27-49

**Implementation**:
```typescript
import type { QueryClient } from '@tanstack/react-query';

/**
 * Registry of all org-scoped stores.
 * When adding a new org-scoped store, add it here.
 */
const ORG_SCOPED_STORES = [
  { name: 'assets', getStore: () => import('@/stores/assets/assetStore').then(m => m.useAssetStore), clearFn: 'invalidateCache' },
  { name: 'locations', getStore: () => import('@/stores/locations/locationStore').then(m => m.useLocationStore), clearFn: 'invalidateCache' },
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
  console.log('[OrgCache] Invalidating all org-scoped data');

  // 1. Cancel in-flight queries AND mutations first
  for (const prefix of ORG_SCOPED_QUERY_PREFIXES) {
    queryClient.cancelQueries({ queryKey: [prefix] });
    queryClient.cancelMutations({ mutationKey: [prefix] });
  }

  // 2. Clear Zustand stores
  for (const { name, getStore, clearFn } of ORG_SCOPED_STORES) {
    try {
      const store = await getStore();
      const fn = store.getState()[clearFn];
      if (typeof fn === 'function') {
        fn();
        console.log(`[OrgCache] Cleared ${name} store`);
      }
    } catch (e) {
      console.error(`[OrgCache] Failed to clear ${name} store:`, e);
    }
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

// Export for testing
export const _testExports = {
  ORG_SCOPED_STORES,
  ORG_SCOPED_LOCALSTORAGE_KEYS,
  ORG_SCOPED_QUERY_PREFIXES,
};
```

**Validation**: `just frontend typecheck`

---

### Task 4: Update authStore - Login
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY (lines 67-77)

**Changes**:
- After `setCurrentOrg()` returns, call central invalidation
- Use dynamic imports

**Implementation**:
```typescript
// In login(), after line 73 (set({ token: orgResponse.data.token })):
// INVALIDATE: After setCurrentOrg() returns with org_id token
const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
const { queryClient } = await import('@/lib/queryClient');
await invalidateAllOrgScopedData(queryClient);
```

Same change needed in signup() (lines 132-142).

**Validation**: `just frontend typecheck`

---

### Task 5: Update authStore - Logout
**File**: `frontend/src/stores/authStore.ts`
**Action**: MODIFY (lines 168-179)

**Changes**:
- After clearing auth state, call central invalidation

**Implementation**:
```typescript
logout: () => {
  // Clear Sentry user context
  Sentry.setUser(null);

  set({
    user: null,
    token: null,
    isAuthenticated: false,
    error: null,
    profile: null,
  });

  // Clear all org-scoped data
  Promise.all([
    import('@/lib/cache/orgScopedCache'),
    import('@/lib/queryClient'),
  ]).then(([{ invalidateAllOrgScopedData }, { queryClient }]) => {
    invalidateAllOrgScopedData(queryClient);
  });
},
```

**Validation**: `just frontend typecheck`

---

### Task 6: Update orgStore - Remove Duplicate Invalidation
**File**: `frontend/src/stores/orgStore.ts`
**Action**: MODIFY

**Changes**:
- Remove `invalidateCache()` calls in `syncFromProfile()` (lines 44-47)
- Remove `invalidateCache()` calls in `switchOrg()` (lines 69-71)
- In `switchOrg()`, call central invalidation after token update

**Implementation for syncFromProfile()**:
```typescript
syncFromProfile: () => {
  const profile = useAuthStore.getState().profile;
  // REMOVED: previousOrgId/newOrgId tracking and invalidateCache() calls
  // Central invalidation is now called by authStore after login/logout

  if (profile) {
    set({
      currentOrg: profile.current_org,
      currentRole: profile.current_org?.role ?? null,
      orgs: profile.orgs,
    });
  } else {
    set({
      currentOrg: null,
      currentRole: null,
      orgs: [],
    });
  }
},
```

**Implementation for switchOrg()**:
```typescript
switchOrg: async (orgId: number) => {
  set({ isLoading: true, error: null });
  try {
    const response = await orgsApi.setCurrentOrg({ org_id: orgId });
    // REMOVED: useAssetStore/useLocationStore invalidateCache() calls

    // Update the token with new org_id claim
    const authState = useAuthStore.getState();
    useAuthStore.setState({ ...authState, token: response.data.token });

    // Call central invalidation
    const { invalidateAllOrgScopedData } = await import('@/lib/cache/orgScopedCache');
    const { queryClient } = await import('@/lib/queryClient');
    await invalidateAllOrgScopedData(queryClient);

    // Refetch profile to get updated current_org
    await useAuthStore.getState().fetchProfile();
    // ... rest unchanged
  } catch (err: any) {
    // ... unchanged
  }
},
```

Also remove the now-unused imports of `useAssetStore` and `useLocationStore`.

**Validation**: `just frontend typecheck`

---

### Task 7: Update useOrgSwitch - Use Central Function
**File**: `frontend/src/hooks/orgs/useOrgSwitch.ts`
**Action**: MODIFY

**Changes**:
- Remove `invalidateOrgCaches()` helper function entirely (lines 27-49)
- Remove unused store imports (useAssetStore, useLocationStore, useTagStore, useBarcodeStore)
- In `switchOrg()` and `createOrg()`, call central invalidation

**Implementation**:
```typescript
import { useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';
import { invalidateAllOrgScopedData } from '@/lib/cache/orgScopedCache';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, createOrg: storeCreateOrg, isLoading } = useOrgStore();

  const switchOrg = async (orgId: number) => {
    await storeSwitchOrg(orgId);
    // storeSwitchOrg now calls invalidateAllOrgScopedData internally
  };

  const createOrg = async (name: string) => {
    const newOrg = await storeCreateOrg(name);

    // Switch to new org to get valid JWT token with org_id claim
    const response = await orgsApi.setCurrentOrg({ org_id: newOrg.id });
    useAuthStore.setState({ token: response.data.token });

    await useAuthStore.getState().fetchProfile();

    // Clear all org-scoped caches
    await invalidateAllOrgScopedData(queryClient);

    return newOrg;
  };

  return {
    switchOrg,
    createOrg,
    isLoading,
  };
}
```

**Validation**: `just frontend typecheck`

---

### Task 8: Update tagStore - Remove enrichedOrgId and Subscription
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY

**Changes**:
1. Remove `enrichedOrgId` field from interface (line 59)
2. Remove `enrichedOrgId` from initial state
3. Remove `enrichedOrgId` from persist partialize (line 478)
4. Remove the entire `useOrgStore.subscribe()` block (lines 522-569)
5. Keep the `useAuthStore.subscribe()` block (lines 488-518) for login/logout enrichment
6. Add canary warning in enrichment function

**For the canary warning**, find where enrichment happens (in `_flushLookupQueue` or similar) and add:
```typescript
// Canary: detect if central invalidation was bypassed
const currentOrgId = useOrgStore.getState().currentOrg?.id;
// If we have stale enrichment data for a different org, warn loudly
if (existingEnrichmentOrgId && existingEnrichmentOrgId !== currentOrgId) {
  console.warn('[tagStore] Stale enrichment detected - central invalidation may have been bypassed');
}
```

Note: Since we're removing `enrichedOrgId` from state, we need to track this differently. The canary should check if tags have enrichment data (assetId/locationId populated) but the current org doesn't match what was used to enrich them. However, without `enrichedOrgId` we can't know what org they were enriched for.

**Alternative approach**: Keep a module-level variable (not in state, not persisted) just for the canary:
```typescript
let lastEnrichmentOrgId: number | null = null;

// In enrichment function:
const currentOrgId = useOrgStore.getState().currentOrg?.id;
if (lastEnrichmentOrgId !== null && lastEnrichmentOrgId !== currentOrgId) {
  console.warn('[tagStore] Stale enrichment detected - central invalidation may have been bypassed');
}
lastEnrichmentOrgId = currentOrgId ?? null;
```

**Validation**: `just frontend typecheck`

---

### Task 9: Write Unit Tests (Mock Wiring)
**File**: `frontend/src/lib/cache/orgScopedCache.test.ts`
**Action**: CREATE

**Implementation**:
```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { invalidateAllOrgScopedData, _testExports } from './orgScopedCache';

// Mock all stores
vi.mock('@/stores/assets/assetStore', () => ({
  useAssetStore: { getState: () => ({ invalidateCache: vi.fn() }) },
}));
vi.mock('@/stores/locations/locationStore', () => ({
  useLocationStore: { getState: () => ({ invalidateCache: vi.fn() }) },
}));
vi.mock('@/stores/tagStore', () => ({
  useTagStore: { getState: () => ({ clearTags: vi.fn() }) },
}));
vi.mock('@/stores/barcodeStore', () => ({
  useBarcodeStore: { getState: () => ({ clearBarcodes: vi.fn() }) },
}));

describe('orgScopedCache', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.clearAllMocks();
    queryClient = new QueryClient();
    localStorage.clear();
  });

  describe('invalidateAllOrgScopedData', () => {
    it('should cancel in-flight queries for all org-scoped prefixes', async () => {
      const cancelQueriesSpy = vi.spyOn(queryClient, 'cancelQueries');

      await invalidateAllOrgScopedData(queryClient);

      for (const prefix of _testExports.ORG_SCOPED_QUERY_PREFIXES) {
        expect(cancelQueriesSpy).toHaveBeenCalledWith({ queryKey: [prefix] });
      }
    });

    it('should cancel pending mutations for all org-scoped prefixes', async () => {
      const cancelMutationsSpy = vi.spyOn(queryClient, 'cancelMutations');

      await invalidateAllOrgScopedData(queryClient);

      for (const prefix of _testExports.ORG_SCOPED_QUERY_PREFIXES) {
        expect(cancelMutationsSpy).toHaveBeenCalledWith({ mutationKey: [prefix] });
      }
    });

    it('should clear localStorage for all org-scoped keys', async () => {
      localStorage.setItem('asset-store', 'test-data');

      await invalidateAllOrgScopedData(queryClient);

      expect(localStorage.getItem('asset-store')).toBeNull();
    });

    it('should invalidate react-query caches', async () => {
      const invalidateQueriesSpy = vi.spyOn(queryClient, 'invalidateQueries');

      await invalidateAllOrgScopedData(queryClient);

      expect(invalidateQueriesSpy).toHaveBeenCalled();
    });
  });

  describe('registry completeness', () => {
    it('should have all expected stores in registry', () => {
      const storeNames = _testExports.ORG_SCOPED_STORES.map(s => s.name);
      expect(storeNames).toContain('assets');
      expect(storeNames).toContain('locations');
      expect(storeNames).toContain('tags');
      expect(storeNames).toContain('barcodes');
    });
  });
});
```

**Validation**: `just frontend test`

---

### Task 10: Write Integration Tests (Real Stores)
**File**: `frontend/src/lib/cache/orgScopedCache.integration.test.ts`
**Action**: CREATE

**Implementation**:
```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { invalidateAllOrgScopedData } from './orgScopedCache';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';

describe('orgScopedCache integration', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient();
    localStorage.clear();

    // Reset all stores to clean state
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();
  });

  it('should clear assetStore data', async () => {
    // Populate store with test data
    useAssetStore.setState({
      assets: [{ id: 1, identifier: 'TEST-001', name: 'Test Asset' }] as any,
      byId: { 1: { id: 1, identifier: 'TEST-001', name: 'Test Asset' } as any },
    });
    expect(useAssetStore.getState().assets.length).toBe(1);

    await invalidateAllOrgScopedData(queryClient);

    expect(useAssetStore.getState().assets.length).toBe(0);
  });

  it('should clear locationStore data', async () => {
    // Populate store with test data
    useLocationStore.setState({
      locations: [{ id: 1, identifier: 'LOC-001', name: 'Test Location' }] as any,
      byId: { 1: { id: 1, identifier: 'LOC-001', name: 'Test Location' } as any },
    });
    expect(useLocationStore.getState().locations.length).toBe(1);

    await invalidateAllOrgScopedData(queryClient);

    expect(useLocationStore.getState().locations.length).toBe(0);
  });

  it('should clear tagStore data', async () => {
    // Populate store with test data
    useTagStore.setState({
      tags: [{ epc: 'E200001', count: 1, source: 'scan', type: 'unknown' }] as any,
    });
    expect(useTagStore.getState().tags.length).toBe(1);

    await invalidateAllOrgScopedData(queryClient);

    expect(useTagStore.getState().tags.length).toBe(0);
  });

  it('should clear barcodeStore data', async () => {
    // Populate store with test data
    useBarcodeStore.setState({
      barcodes: [{ code: '123456', timestamp: Date.now() }] as any,
    });
    expect(useBarcodeStore.getState().barcodes.length).toBe(1);

    await invalidateAllOrgScopedData(queryClient);

    expect(useBarcodeStore.getState().barcodes.length).toBe(0);
  });
});
```

**Validation**: `just frontend test`

---

### Task 11: Update Existing Tests
**File**: `frontend/src/hooks/orgs/useOrgSwitch.test.ts`
**Action**: MODIFY

**Changes**:
- Update tests to expect central invalidation instead of individual store calls
- Mock `invalidateAllOrgScopedData` instead of individual stores

**Validation**: `just frontend test`

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Circular dependency with queryClient | queryClient.ts has no store imports; stores use dynamic imports |
| Tests break due to module mocking | Update tests incrementally; run after each task |
| Missed invalidation call site | Search for `invalidateCache`, `clearTags`, `clearBarcodes` to verify all sites updated |
| tagStore enrichment race condition | Canary warning will alert if issue occurs |

## Integration Points

- **Store updates**: assetStore, locationStore, tagStore, barcodeStore all cleared via registry
- **QueryClient**: Created in lib/queryClient.ts, imported everywhere
- **Auth flow**: login() and logout() call invalidation
- **Org flow**: switchOrg() calls invalidation

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
just frontend lint      # Gate 1: Syntax & Style
just frontend typecheck # Gate 2: Type Safety
just frontend test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: `just frontend typecheck`
After Tasks 9-11: `just frontend test`
Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 6/10 (MEDIUM)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec with explicit code examples
- ✅ Similar patterns found: useOrgSwitch.ts lines 27-49
- ✅ All clarifying questions answered with specific guidance
- ✅ Existing test patterns to follow: useOrgSwitch.test.ts
- ✅ main.tsx already creates QueryClient at module level (easy refactor)
- ⚠️ Canary warning logic needs careful placement in enrichment flow

**Assessment**: High confidence due to well-defined spec, existing patterns, and clear test strategy.

**Estimated one-pass success probability**: 85%

**Reasoning**: All the pieces are well-documented in the spec, existing code patterns are clear, and the changes are mostly consolidation of existing logic rather than new functionality. Main risk is missing a call site or test needing adjustment.
