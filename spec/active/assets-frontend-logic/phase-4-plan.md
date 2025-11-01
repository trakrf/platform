# Phase 4 Implementation Plan

**Feature**: Asset Management API Integration & React Hooks
**Phase**: 4 of 4 (Final Data Layer Phase)
**Branch**: `feature/assets-frontend-phase-4`
**Depends On**: Phase 1 (Types & API), Phase 2 (Business Logic), Phase 3 (Zustand Store)

---

## Overview

Phase 4 completes the data layer by creating React hooks that bridge the gap between UI components and the backend. This phase implements the integration patterns documented in `architecture.md`.

**Goal**: Provide production-ready React hooks for all asset operations with proper loading states, error handling, and cache synchronization.

---

## Implementation Tasks

### Task 1: Cache Integration Helpers

**File**: `frontend/src/lib/asset/cache-integration.ts`

**Purpose**: Reusable functions that handle API → Cache synchronization patterns.

**Functions to implement**:

```typescript
// ✅ Cache-First Pattern
export function isCacheStale(cache: AssetCache): boolean
export async function fetchAndCacheAssets(options?: ListAssetsOptions): Promise<Asset[]>
export async function fetchAndCacheSingle(id: number): Promise<Asset | null>

// ✅ Mutation + Cache Update Pattern
export async function createAndCache(data: CreateAssetRequest): Promise<Asset>
export async function updateAndCache(id: number, updates: UpdateAssetRequest): Promise<Asset>
export async function deleteAndRemoveFromCache(id: number): Promise<void>

// ✅ Bulk Upload Pattern
export async function handleBulkUploadComplete(jobId: string): Promise<void>
```

**Validation**:
- Unit tests for each helper (8 functions × 2-3 tests = ~20 tests)
- Mock `assetsApi` and `useAssetStore`
- Verify cache updates after API calls
- Test error scenarios (API failures should NOT update cache)

**Acceptance Criteria**:
- [ ] All 8 helper functions implemented
- [ ] 20+ unit tests passing
- [ ] TypeScript compiles with no errors
- [ ] All helpers handle errors correctly (no cache corruption)

**Estimated Time**: 2-3 hours

---

### Task 2: useAssets Hook (List/Query)

**File**: `frontend/src/hooks/assets/useAssets.ts`

**Purpose**: Fetch and display lists of assets with filters, sort, and pagination.

**API**:

```typescript
interface UseAssetsOptions {
  enabled?: boolean          // Auto-fetch on mount (default: true)
  refetchOnMount?: boolean   // Ignore cache on mount (default: false)
}

interface UseAssetsReturn {
  // Data
  assets: Asset[]            // Filtered, sorted, paginated
  totalCount: number         // Total matching filters

  // State
  isLoading: boolean         // Initial fetch
  isRefetching: boolean      // Background refresh
  error: Error | null        // Last error

  // Actions
  refetch: () => Promise<void>  // Manual refresh
}

export function useAssets(options?: UseAssetsOptions): UseAssetsReturn
```

**Implementation Pattern**:

```typescript
export function useAssets(options: UseAssetsOptions = {}) {
  const store = useAssetStore();
  const [isLoading, setIsLoading] = useState(false);
  const [isRefetching, setIsRefetching] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchAssets = useCallback(async (skipCache = false) => {
    // Check cache first (unless skipCache)
    if (!skipCache && !isCacheStale(store.cache)) {
      return;
    }

    // Fetch from API
    setIsRefetching(isLoading ? false : true);
    setIsLoading(!isRefetching);

    try {
      await fetchAndCacheAssets();
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setIsLoading(false);
      setIsRefetching(false);
    }
  }, [store]);

  useEffect(() => {
    if (options.enabled !== false) {
      fetchAssets(options.refetchOnMount);
    }
  }, []);

  return {
    assets: store.getPaginatedAssets(),
    totalCount: store.pagination.totalCount,
    isLoading,
    isRefetching,
    error,
    refetch: () => fetchAssets(true),
  };
}
```

**Validation**:
- Unit tests with React Testing Library
- Test cache-first behavior
- Test refetch behavior
- Test error handling
- Test loading states

**Acceptance Criteria**:
- [ ] Hook implemented with full TypeScript types
- [ ] 8+ unit tests passing
- [ ] Cache-first logic verified
- [ ] Integrates Phase 3 store queries (getFilteredAssets, getPaginatedAssets)
- [ ] No unnecessary re-renders (use useCallback, useMemo)

**Estimated Time**: 2-3 hours

---

### Task 3: useAsset Hook (Single Asset)

**File**: `frontend/src/hooks/assets/useAsset.ts`

**Purpose**: Fetch and display a single asset by ID.

**API**:

```typescript
interface UseAssetOptions {
  enabled?: boolean          // Auto-fetch on mount (default: true)
}

interface UseAssetReturn {
  // Data
  asset: Asset | null        // Single asset or null

  // State
  isLoading: boolean         // Fetch in progress
  error: Error | null        // Fetch error

  // Actions
  refetch: () => Promise<void>  // Reload asset
}

export function useAsset(
  id: number | null,
  options?: UseAssetOptions
): UseAssetReturn
```

**Implementation Pattern**:

```typescript
export function useAsset(id: number | null, options: UseAssetOptions = {}) {
  const store = useAssetStore();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const fetchAsset = useCallback(async () => {
    if (!id) return;

    // Check cache first
    const cached = store.getAssetById(id);
    if (cached) return;

    // Cache miss - fetch from API
    setIsLoading(true);
    try {
      await fetchAndCacheSingle(id);
      setError(null);
    } catch (err) {
      setError(err as Error);
    } finally {
      setIsLoading(false);
    }
  }, [id, store]);

  useEffect(() => {
    if (options.enabled !== false) {
      fetchAsset();
    }
  }, [id, fetchAsset, options.enabled]);

  return {
    asset: id ? store.getAssetById(id) ?? null : null,
    isLoading,
    error,
    refetch: fetchAsset,
  };
}
```

**Validation**:
- Unit tests for cache hit/miss
- Test null ID handling
- Test error scenarios
- Test refetch

**Acceptance Criteria**:
- [ ] Hook implemented with full TypeScript types
- [ ] 6+ unit tests passing
- [ ] Cache-first behavior verified
- [ ] Handles null ID gracefully
- [ ] No memory leaks (cleanup on unmount)

**Estimated Time**: 1-2 hours

---

### Task 4: useAssetMutations Hook (Create/Update/Delete)

**File**: `frontend/src/hooks/assets/useAssetMutations.ts`

**Purpose**: Perform CRUD operations with loading/error states.

**API**:

```typescript
interface UseAssetMutationsReturn {
  // Create
  createAsset: (data: CreateAssetRequest) => Promise<Asset>
  isCreating: boolean
  createError: Error | null

  // Update
  updateAsset: (id: number, updates: UpdateAssetRequest) => Promise<Asset>
  isUpdating: boolean
  updateError: Error | null

  // Delete
  deleteAsset: (id: number) => Promise<void>
  isDeleting: boolean
  deleteError: Error | null
}

export function useAssetMutations(): UseAssetMutationsReturn
```

**Implementation Pattern**:

```typescript
export function useAssetMutations() {
  const [isCreating, setIsCreating] = useState(false);
  const [createError, setCreateError] = useState<Error | null>(null);

  const [isUpdating, setIsUpdating] = useState(false);
  const [updateError, setUpdateError] = useState<Error | null>(null);

  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<Error | null>(null);

  const createAsset = useCallback(async (data: CreateAssetRequest) => {
    // Validate first (Phase 2)
    const validationError = validateAssetData(data);
    if (validationError) {
      throw new Error(validationError);
    }

    setIsCreating(true);
    setCreateError(null);

    try {
      const asset = await createAndCache(data);
      return asset;
    } catch (err) {
      setCreateError(err as Error);
      throw err;
    } finally {
      setIsCreating(false);
    }
  }, []);

  const updateAsset = useCallback(async (id: number, updates: UpdateAssetRequest) => {
    setIsUpdating(true);
    setUpdateError(null);

    try {
      const asset = await updateAndCache(id, updates);
      return asset;
    } catch (err) {
      setUpdateError(err as Error);
      throw err;
    } finally {
      setIsUpdating(false);
    }
  }, []);

  const deleteAsset = useCallback(async (id: number) => {
    setIsDeleting(true);
    setDeleteError(null);

    try {
      await deleteAndRemoveFromCache(id);
    } catch (err) {
      setDeleteError(err as Error);
      throw err;
    } finally {
      setIsDeleting(false);
    }
  }, []);

  return {
    createAsset,
    isCreating,
    createError,

    updateAsset,
    isUpdating,
    updateError,

    deleteAsset,
    isDeleting,
    deleteError,
  };
}
```

**Validation**:
- Unit tests for each mutation
- Test loading states
- Test error handling
- Test cache updates after success
- Test validation errors

**Acceptance Criteria**:
- [ ] Hook implemented with full TypeScript types
- [ ] 12+ unit tests passing (4 per mutation)
- [ ] Phase 2 validation integrated (createAsset)
- [ ] Proper error state management
- [ ] Cache updates verified after each operation

**Estimated Time**: 3-4 hours

---

### Task 5: useBulkUpload Hook (CSV Upload with Polling)

**File**: `frontend/src/hooks/assets/useBulkUpload.ts`

**Purpose**: Handle CSV uploads with polling for job completion.

**API**:

```typescript
interface UseBulkUploadReturn {
  // Upload
  uploadCSV: (file: File) => Promise<void>

  // Status
  jobStatus: JobStatusResponse | null

  // State
  isUploading: boolean      // File upload in progress
  isPolling: boolean        // Polling for job status
  error: Error | null       // Upload or polling error

  // Actions
  cancelPolling: () => void // Stop polling manually
}

export function useBulkUpload(): UseBulkUploadReturn
```

**Implementation Pattern**:

```typescript
export function useBulkUpload() {
  const store = useAssetStore();
  const [isUploading, setIsUploading] = useState(false);
  const [isPolling, setIsPolling] = useState(false);
  const [jobStatus, setJobStatus] = useState<JobStatusResponse | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);

  const cancelPolling = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
      store.clearUploadState();
      setIsPolling(false);
    }
  }, [store]);

  const uploadCSV = useCallback(async (file: File) => {
    // Validate file
    if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
      throw new Error(`File too large. Max size: ${CSV_VALIDATION.MAX_FILE_SIZE / 1024 / 1024}MB`);
    }

    setIsUploading(true);
    setError(null);

    try {
      // Start upload
      const response = await assetsApi.uploadCSV(file);
      store.setUploadJobId(response.job_id);
      setJobStatus(response);
      setIsUploading(false);

      // Start polling
      setIsPolling(true);
      intervalRef.current = setInterval(async () => {
        try {
          const status = await assetsApi.getJobStatus(response.job_id);
          setJobStatus(status);

          if (status.status === 'completed') {
            // Success - invalidate cache and refetch
            cancelPolling();
            await handleBulkUploadComplete(response.job_id);
          } else if (status.status === 'failed') {
            // Failed - stop polling
            cancelPolling();
            setError(new Error('Bulk upload failed'));
          }
        } catch (err) {
          cancelPolling();
          setError(err as Error);
        }
      }, 2000); // Poll every 2 seconds

      store.setPollingInterval(intervalRef.current);
    } catch (err) {
      setError(err as Error);
      setIsUploading(false);
      throw err;
    }
  }, [store, cancelPolling]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);

  return {
    uploadCSV,
    jobStatus,
    isUploading,
    isPolling,
    error,
    cancelPolling,
  };
}
```

**Validation**:
- Unit tests with fake timers (vi.useFakeTimers)
- Test upload flow
- Test polling behavior
- Test completion handling
- Test error scenarios
- Test cleanup on unmount

**Acceptance Criteria**:
- [ ] Hook implemented with full TypeScript types
- [ ] 10+ unit tests passing
- [ ] Polling logic verified (setInterval)
- [ ] Cleanup verified (clearInterval on unmount)
- [ ] File validation integrated (Phase 2)
- [ ] Cache invalidation on completion

**Estimated Time**: 3-4 hours

---

### Task 6: Integration Tests

**File**: `frontend/src/hooks/assets/__tests__/integration.test.ts`

**Purpose**: Verify all hooks work together correctly.

**Test Scenarios**:

1. **Full CRUD Flow**:
   - Create asset → Verify in useAssets list
   - Update asset → Verify changes in useAsset
   - Delete asset → Verify removed from useAssets list

2. **Cache Behavior**:
   - useAssets fetches data
   - useAsset(id) reads from cache (no API call)
   - useAssetMutations.createAsset → useAssets reflects new asset

3. **Error Handling**:
   - API failure in useAssets → error state set
   - API failure in createAsset → cache unchanged

4. **Bulk Upload**:
   - useBulkUpload starts job
   - Polling completes
   - useAssets reflects new assets

**Acceptance Criteria**:
- [ ] 8+ integration tests passing
- [ ] All hooks tested together
- [ ] Cache synchronization verified
- [ ] Real API client mocked consistently

**Estimated Time**: 2-3 hours

---

### Task 7: Update stores/index.ts and hooks/index.ts

**Files**:
- `frontend/src/stores/index.ts` (already updated)
- `frontend/src/hooks/index.ts` (create if doesn't exist)

**Purpose**: Central exports for clean imports.

```typescript
// hooks/index.ts
export { useAssets } from './assets/useAssets';
export { useAsset } from './assets/useAsset';
export { useAssetMutations } from './assets/useAssetMutations';
export { useBulkUpload } from './assets/useBulkUpload';

export type { UseAssetsOptions, UseAssetsReturn } from './assets/useAssets';
export type { UseAssetOptions, UseAssetReturn } from './assets/useAsset';
export type { UseAssetMutationsReturn } from './assets/useAssetMutations';
export type { UseBulkUploadReturn } from './assets/useBulkUpload';
```

**Acceptance Criteria**:
- [ ] Central export file created
- [ ] All hooks exported
- [ ] All types exported
- [ ] TypeScript compiles

**Estimated Time**: 15 minutes

---

### Task 8: Documentation & Validation

**Files**:
- `spec/active/assets-frontend-logic/phase-4-log.md` (create)
- Update `architecture.md` with Phase 4 completion

**Purpose**: Document implementation and validate completion.

**Phase 4 Log Template**:

```markdown
# Phase 4 Implementation Log

## Summary
- **Start Date**: YYYY-MM-DD
- **End Date**: YYYY-MM-DD
- **Status**: Complete ✅
- **Tests**: X passing

## Files Created
- [ ] lib/asset/cache-integration.ts (X lines)
- [ ] hooks/assets/useAssets.ts (X lines)
- [ ] hooks/assets/useAsset.ts (X lines)
- [ ] hooks/assets/useAssetMutations.ts (X lines)
- [ ] hooks/assets/useBulkUpload.ts (X lines)
- [ ] hooks/index.ts (X lines)

## Tests Created
- [ ] cache-integration.test.ts (X tests)
- [ ] useAssets.test.ts (X tests)
- [ ] useAsset.test.ts (X tests)
- [ ] useAssetMutations.test.ts (X tests)
- [ ] useBulkUpload.test.ts (X tests)
- [ ] integration.test.ts (X tests)

## Validation Checklist
- [ ] All TypeScript types defined
- [ ] All unit tests passing (50+ total)
- [ ] Integration tests passing (8+ total)
- [ ] `pnpm typecheck` passes
- [ ] `pnpm lint` passes
- [ ] `pnpm test hooks/assets` passes
- [ ] `pnpm validate` passes
- [ ] Architecture diagram updated
```

**Acceptance Criteria**:
- [ ] Phase 4 log created with all checks
- [ ] architecture.md updated
- [ ] All validation commands pass

**Estimated Time**: 1 hour

---

## Task Execution Order

```
Task 1: Cache Integration Helpers (2-3h)
   ↓
Task 2: useAssets Hook (2-3h)
   ↓
Task 3: useAsset Hook (1-2h)
   ↓
Task 4: useAssetMutations Hook (3-4h)
   ↓
Task 5: useBulkUpload Hook (3-4h)
   ↓
Task 6: Integration Tests (2-3h)
   ↓
Task 7: Central Exports (15m)
   ↓
Task 8: Documentation (1h)
```

**Total Estimated Time**: 15-20 hours

---

## Validation Commands

```bash
# TypeScript compilation
pnpm typecheck

# Linting
pnpm lint

# Unit tests (cache-integration + hooks)
pnpm vitest run src/lib/asset/cache-integration.test.ts
pnpm vitest run src/hooks/assets/

# Integration tests
pnpm vitest run src/hooks/assets/__tests__/integration.test.ts

# Full validation
pnpm validate
```

---

## Success Criteria

Phase 4 is complete when:

- ✅ All 8 tasks completed
- ✅ 50+ unit tests passing
- ✅ 8+ integration tests passing
- ✅ TypeScript compiles with no errors
- ✅ All validation commands pass
- ✅ Architecture diagram updated
- ✅ Phase 4 log created

**Deliverables**:
- ~600 lines of production code (hooks + cache integration)
- ~50+ unit tests
- ~8+ integration tests
- Complete documentation

---

## Next Steps After Phase 4

**Phase 5 (Future)**: UI Components
- `components/assets/AssetList.tsx`
- `components/assets/AssetCard.tsx`
- `components/assets/AssetForm.tsx`
- `components/assets/BulkUploadModal.tsx`

Phase 5 will consume the hooks from Phase 4 to build the complete UI.

---

**Created**: 2025-10-31
**Branch**: `feature/assets-frontend-phase-4`
**Estimated Completion**: 2-3 days of focused work
