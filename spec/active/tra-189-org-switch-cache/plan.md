# Implementation Plan: Org Switch Cache Invalidation
Generated: 2026-01-13
Specification: spec.md

## Understanding
Two issues need fixing:
1. **Stale data on org switch**: `OrgSwitcher.tsx` bypasses the `useOrgSwitch` hook that handles cache invalidation
2. **New org access**: After creating an org, user doesn't have proper JWT token for the new org

Solution: Wire up `useOrgSwitch` hook consistently, and extend it to handle org creation by calling `setCurrentOrg` after creation to get a valid JWT.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/hooks/orgs/useOrgSwitch.ts` (lines 23-41) - existing cache invalidation pattern
- `frontend/src/hooks/assets/useAssetMutations.test.ts` (lines 1-35) - hook testing pattern with QueryClient wrapper
- `frontend/src/stores/orgStore.ts` (lines 53-81) - switchOrg pattern for token handling

**Files to Modify**:
- `frontend/src/components/OrgSwitcher.tsx` (line 24) - use `useOrgSwitch()` instead of `useOrgStore().switchOrg`
- `frontend/src/hooks/orgs/useOrgSwitch.ts` - add `createOrg` function with cache invalidation
- `frontend/src/components/useOrgModal.ts` (line 20) - use `useOrgSwitch().createOrg`

**Files to Create**:
- `frontend/src/hooks/orgs/useOrgSwitch.test.ts` - unit tests for the hook

## Architecture Impact
- **Subsystems affected**: Frontend hooks/components only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Fix OrgSwitcher to use useOrgSwitch hook
**File**: `frontend/src/components/OrgSwitcher.tsx`
**Action**: MODIFY
**Pattern**: Simple import swap

**Implementation**:
```typescript
// Line 8: Add import
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';

// Line 24: Change from
const { currentOrg, currentRole, orgs, isLoading, switchOrg } = useOrgStore();

// To:
const { currentOrg, currentRole, orgs, isLoading } = useOrgStore();
const { switchOrg } = useOrgSwitch();
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 2: Extend useOrgSwitch with createOrg function
**File**: `frontend/src/hooks/orgs/useOrgSwitch.ts`
**Action**: MODIFY
**Pattern**: Follow existing `switchOrg` pattern

**Implementation**:
```typescript
import { useOrgStore } from '@/stores/orgStore';
import { orgsApi } from '@/lib/api/orgs';
import { useAuthStore } from '@/stores/authStore';

export function useOrgSwitch() {
  const queryClient = useQueryClient();
  const { switchOrg: storeSwitchOrg, createOrg: storeCreateOrg, isLoading } = useOrgStore();

  // Helper to invalidate all org-scoped caches
  const invalidateOrgCaches = () => {
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();

    queryClient.invalidateQueries({
      predicate: (query) => {
        const key = query.queryKey[0];
        return key !== 'user' && key !== 'profile';
      },
    });
  };

  const switchOrg = async (orgId: number) => {
    await storeSwitchOrg(orgId);
    invalidateOrgCaches();
  };

  const createOrg = async (name: string) => {
    // 1. Create org via store
    const newOrg = await storeCreateOrg(name);

    // 2. Switch to new org to get valid JWT token
    const response = await orgsApi.setCurrentOrg({ org_id: newOrg.id });
    useAuthStore.setState({ token: response.data.token });

    // 3. Refetch profile with new token
    await useAuthStore.getState().fetchProfile();

    // 4. Clear all org-scoped caches
    invalidateOrgCaches();

    return newOrg;
  };

  return { switchOrg, createOrg, isLoading };
}
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 3: Update useOrgModal to use useOrgSwitch.createOrg
**File**: `frontend/src/components/useOrgModal.ts`
**Action**: MODIFY
**Pattern**: Simple import swap

**Implementation**:
```typescript
// Line 2: Add import
import { useOrgSwitch } from '@/hooks/orgs/useOrgSwitch';

// Line 20: Change from
const { currentOrg, currentRole, createOrg, isLoading: isOrgLoading } = useOrgStore();

// To:
const { currentOrg, currentRole, isLoading: isOrgLoading } = useOrgStore();
const { createOrg } = useOrgSwitch();
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 4: Add unit tests for useOrgSwitch
**File**: `frontend/src/hooks/orgs/useOrgSwitch.test.ts`
**Action**: CREATE
**Pattern**: Follow `useAssetMutations.test.ts` pattern

**Implementation**:
```typescript
import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useOrgSwitch } from './useOrgSwitch';
import { useOrgStore } from '@/stores/orgStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';

vi.mock('@/lib/api/orgs');

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useOrgSwitch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('switchOrg', () => {
    it('should invalidate all org-scoped caches after switching', async () => {
      const invalidateAssetCache = vi.spyOn(useAssetStore.getState(), 'invalidateCache');
      const invalidateLocationCache = vi.spyOn(useLocationStore.getState(), 'invalidateCache');
      const clearTags = vi.spyOn(useTagStore.getState(), 'clearTags');
      const clearBarcodes = vi.spyOn(useBarcodeStore.getState(), 'clearBarcodes');

      // Mock the store switchOrg
      vi.spyOn(useOrgStore.getState(), 'switchOrg').mockResolvedValue();

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      await result.current.switchOrg(123);

      expect(invalidateAssetCache).toHaveBeenCalled();
      expect(invalidateLocationCache).toHaveBeenCalled();
      expect(clearTags).toHaveBeenCalled();
      expect(clearBarcodes).toHaveBeenCalled();
    });
  });

  describe('createOrg', () => {
    it('should create org, switch to it, and invalidate caches', async () => {
      const mockOrg = { id: 456, name: 'New Org' };
      const mockToken = 'new-jwt-token';

      vi.spyOn(useOrgStore.getState(), 'createOrg').mockResolvedValue(mockOrg as any);
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { message: 'ok', token: mockToken },
      } as any);
      vi.spyOn(useAuthStore.getState(), 'fetchProfile').mockResolvedValue();

      const invalidateAssetCache = vi.spyOn(useAssetStore.getState(), 'invalidateCache');

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      const newOrg = await result.current.createOrg('New Org');

      expect(newOrg).toEqual(mockOrg);
      expect(orgsApi.setCurrentOrg).toHaveBeenCalledWith({ org_id: 456 });
      expect(invalidateAssetCache).toHaveBeenCalled();
    });
  });
});
```

**Validation**:
```bash
just frontend test
```

---

### Task 5: Run full validation
**Action**: VALIDATE

**Validation**:
```bash
just frontend validate
```

---

### Task 6: Manual smoke test
**Action**: MANUAL TEST

Test scenarios:
1. Log in with user that has multiple orgs
2. View assets in Org A
3. Switch to Org B - verify Org B's assets shown (not Org A's)
4. Refresh page - verify Org B's data persists
5. Create new org - verify immediate access without errors

## Risk Assessment
- **Risk**: Store mock setup in tests may not match actual store shape
  **Mitigation**: Use vi.spyOn on actual store methods rather than full mock replacement

- **Risk**: Race condition between createOrg and setCurrentOrg
  **Mitigation**: Sequential await calls ensure proper ordering

## Integration Points
- Store updates: `useOrgSwitch` coordinates between `orgStore`, `authStore`, and cache stores
- No route changes needed
- No config updates needed

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test`

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence
After each task: `just frontend lint && just frontend typecheck`
After Task 4: `just frontend test`
Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- Clear requirements from spec
- Existing `useOrgSwitch` hook already has the cache invalidation pattern
- Similar test patterns found in `useAssetMutations.test.ts`
- No new dependencies needed
- Single subsystem (frontend) affected
- Existing `setCurrentOrg` API returns JWT token

**Assessment**: Straightforward wiring fix using existing patterns. High confidence in one-pass success.

**Estimated one-pass success probability**: 90%

**Reasoning**: All the building blocks exist - we're just connecting them properly. The `useOrgSwitch` hook already works for switching; we're extending it to also handle creation and ensuring it's used consistently in the UI.
