# Phase 4: Asset Management API Integration & React Hooks - ENHANCED SPECIFICATION

## Metadata
**Phase**: 4 of 4 (Final Data Layer Phase)
**Depends On**: Phase 1 (Types & API Client), Phase 2 (Business Logic), Phase 3 (Zustand Store)
**Complexity**: 7/10
**Estimated Time**: 15-20 hours
**Target Test Count**: 58+ tests (20 cache integration + 38 hooks)

---

## Executive Summary

Phase 4 completes the asset management data layer by creating React hooks that bridge React components with the backend API. This phase delivers production-ready data integration with:

- ✅ **Cache-first architecture** - Minimize redundant API calls
- ✅ **Automatic cache synchronization** - Server response updates client state
- ✅ **Comprehensive error handling** - Graceful failure with user feedback
- ✅ **Loading states per operation** - Fine-grained UX control
- ✅ **Polling for long-running jobs** - CSV bulk uploads
- ✅ **1-hour cache TTL** - Balance freshness vs performance

**Outcome**: React components can perform full CRUD + bulk operations without direct API knowledge.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    React Components                          │
│                  (Future Phase 5)                            │
└────────────────────┬────────────────────────────────────────┘
                     │
                     │ useAssets(), useAsset()
                     │ useAssetMutations(), useBulkUpload()
                     │
┌────────────────────▼────────────────────────────────────────┐
│              Phase 4: React Hooks Layer                      │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────┐    │
│  │ useAssets   │  │ useAsset    │  │useAssetMutations │    │
│  │ (List)      │  │ (Single)    │  │(Create/Edit/Del) │    │
│  └─────────────┘  └─────────────┘  └──────────────────┘    │
│                                                               │
│  ┌──────────────────┐                                        │
│  │ useBulkUpload    │                                        │
│  │ (CSV + Polling)  │                                        │
│  └──────────────────┘                                        │
│                                                               │
│  Uses: cache-integration.ts (helper functions)               │
└───────────────────┬──────────────┬──────────────────────────┘
                    │              │
         ┌──────────┘              └──────────┐
         │                                    │
         ▼                                    ▼
┌────────────────────┐              ┌────────────────────┐
│ Phase 3: Store     │              │ Phase 1: API       │
│ (useAssetStore)    │              │ (assetsApi)        │
│                    │              │                    │
│ - Cache reads      │              │ - HTTP calls       │
│ - Cache writes     │              │ - JWT auth         │
│ - Subscriptions    │              │ - Error handling   │
└────────────────────┘              └────────────────────┘
```

---

## Files to Implement

### Directory Structure

```
frontend/src/
├── hooks/
│   ├── assets/
│   │   ├── useAssets.ts              # 120 lines, 10 tests
│   │   ├── useAssets.test.ts
│   │   ├── useAsset.ts               # 80 lines, 8 tests
│   │   ├── useAsset.test.ts
│   │   ├── useAssetMutations.ts      # 150 lines, 12 tests
│   │   ├── useAssetMutations.test.ts
│   │   ├── useBulkUpload.ts          # 120 lines, 8 tests
│   │   ├── useBulkUpload.test.ts
│   │   └── __tests__/
│   │       └── integration.test.ts   # Integration tests (8 tests)
│   └── index.ts                      # Central exports
│
└── lib/
    └── asset/
        ├── cache-integration.ts      # 200 lines, 20 tests
        └── cache-integration.test.ts
```

**Total Production Code**: ~670 lines
**Total Test Code**: ~800 lines (58+ tests)

---

## Implementation Details

### File 1: `lib/asset/cache-integration.ts`

**Purpose**: Pure helper functions that synchronize API responses with Zustand cache.

**Design Principles**:
- ✅ **Server is source of truth** - Always use API response data
- ✅ **Immediate cache updates** - No waiting for refetch
- ✅ **Atomic operations** - Complete success or complete failure
- ✅ **No side effects** - Pure functions only

**Complete Implementation**:

```typescript
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, ListAssetsOptions } from '@/types/assets';

/**
 * Check if cache is stale (older than TTL)
 * TTL: 1 hour (assets change rarely)
 */
export function isCacheStale(): boolean {
  const { cache } = useAssetStore.getState();
  const now = Date.now();
  return now - cache.lastFetched > cache.ttl;
}

/**
 * Check if cache is empty
 */
export function isCacheEmpty(): boolean {
  const { cache } = useAssetStore.getState();
  return cache.byId.size === 0;
}

/**
 * Fetch all assets and populate cache
 * Used by: useAssets (initial fetch), useBulkUpload (after completion)
 *
 * Pattern: API call → Cache update
 *
 * @throws Error if API call fails (cache not updated)
 */
export async function fetchAndCacheAssets(options?: ListAssetsOptions): Promise<Asset[]> {
  const response = await assetsApi.list(options);
  useAssetStore.getState().addAssets(response.data);
  return response.data;
}

/**
 * Fetch single asset by ID and add to cache
 * Used by: useAsset (cache miss)
 *
 * Pattern: Check cache → API call (if miss) → Cache update
 *
 * @returns Asset from cache (either existing or newly fetched)
 * @throws Error if API call fails (cache not updated)
 */
export async function fetchAndCacheSingle(id: number): Promise<Asset> {
  // Check cache first
  const cached = useAssetStore.getState().getAssetById(id);
  if (cached) {
    return cached;
  }

  // Cache miss - fetch from API
  const response = await assetsApi.get(id);
  useAssetStore.getState().addAsset(response.data);
  return response.data;
}

/**
 * Create asset via API and immediately add to cache
 * Used by: useAssetMutations.createAsset
 *
 * Pattern: API call → Cache add
 *
 * @param data - Asset creation data
 * @returns Created asset from server
 * @throws Error if API call fails (cache not updated)
 */
export async function createAndCache(data: CreateAssetRequest): Promise<Asset> {
  const response = await assetsApi.create(data);

  // Server response is source of truth
  useAssetStore.getState().addAsset(response.data);

  return response.data;
}

/**
 * Update asset via API and immediately update cache
 * Used by: useAssetMutations.updateAsset
 *
 * Pattern: API call → Cache update (with server response)
 *
 * @param id - Asset ID to update
 * @param updates - Partial asset updates
 * @returns Updated asset from server
 * @throws Error if API call fails (cache rollback not needed - no optimistic update)
 */
export async function updateAndCache(
  id: number,
  updates: UpdateAssetRequest
): Promise<Asset> {
  const response = await assetsApi.update(id, updates);

  // Server response is source of truth
  useAssetStore.getState().updateCachedAsset(id, response.data);

  return response.data;
}

/**
 * Delete asset via API and immediately remove from cache
 * Used by: useAssetMutations.deleteAsset
 *
 * Pattern: API call → Cache remove
 *
 * @param id - Asset ID to delete
 * @throws Error if API call fails (cache not updated)
 */
export async function deleteAndRemoveFromCache(id: number): Promise<void> {
  await assetsApi.delete(id);

  // Remove from all indexes
  useAssetStore.getState().removeAsset(id);
}

/**
 * Invalidate cache and refetch all assets
 * Used by: useBulkUpload (after job completion)
 *
 * Pattern: Clear cache → API call → Cache repopulate
 *
 * Rationale: Bulk uploads can create many assets - individual cache
 * updates would be inefficient. Full invalidation + refetch is simpler.
 */
export async function handleBulkUploadComplete(jobId: string): Promise<void> {
  // Clear cache
  useAssetStore.getState().invalidateCache();

  // Refetch all assets
  await fetchAndCacheAssets();
}
```

**Test Requirements** (20 tests):

```typescript
// cache-integration.test.ts
describe('cache-integration helpers', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  describe('isCacheStale', () => {
    it('should return true when cache is older than 1 hour', () => {
      const store = useAssetStore.getState();
      store.cache.lastFetched = Date.now() - (61 * 60 * 1000); // 61 minutes ago
      expect(isCacheStale()).toBe(true);
    });

    it('should return false when cache is fresh', () => {
      const store = useAssetStore.getState();
      store.cache.lastFetched = Date.now() - (30 * 60 * 1000); // 30 minutes ago
      expect(isCacheStale()).toBe(false);
    });

    it('should return true when lastFetched is 0 (never fetched)', () => {
      expect(isCacheStale()).toBe(true);
    });
  });

  describe('isCacheEmpty', () => {
    it('should return true when cache has no assets', () => {
      expect(isCacheEmpty()).toBe(true);
    });

    it('should return false when cache has assets', () => {
      useAssetStore.getState().addAsset(mockAsset);
      expect(isCacheEmpty()).toBe(false);
    });
  });

  describe('fetchAndCacheAssets', () => {
    it('should fetch from API and populate cache', async () => {
      vi.mocked(assetsApi.list).mockResolvedValue({ data: [mockAsset1, mockAsset2] });

      const assets = await fetchAndCacheAssets();

      expect(assetsApi.list).toHaveBeenCalledWith(undefined);
      expect(assets).toHaveLength(2);
      expect(useAssetStore.getState().cache.byId.size).toBe(2);
    });

    it('should pass options to API', async () => {
      vi.mocked(assetsApi.list).mockResolvedValue({ data: [] });

      await fetchAndCacheAssets({ limit: 10, offset: 20 });

      expect(assetsApi.list).toHaveBeenCalledWith({ limit: 10, offset: 20 });
    });

    it('should not update cache on API error', async () => {
      vi.mocked(assetsApi.list).mockRejectedValue(new Error('Network error'));

      await expect(fetchAndCacheAssets()).rejects.toThrow('Network error');
      expect(useAssetStore.getState().cache.byId.size).toBe(0);
    });
  });

  describe('fetchAndCacheSingle', () => {
    it('should return cached asset without API call', async () => {
      useAssetStore.getState().addAsset(mockAsset);

      const asset = await fetchAndCacheSingle(mockAsset.id);

      expect(asset).toEqual(mockAsset);
      expect(assetsApi.get).not.toHaveBeenCalled();
    });

    it('should fetch from API on cache miss', async () => {
      vi.mocked(assetsApi.get).mockResolvedValue({ data: mockAsset });

      const asset = await fetchAndCacheSingle(mockAsset.id);

      expect(assetsApi.get).toHaveBeenCalledWith(mockAsset.id);
      expect(asset).toEqual(mockAsset);
      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toEqual(mockAsset);
    });

    it('should not update cache on API error', async () => {
      vi.mocked(assetsApi.get).mockRejectedValue(new Error('Not found'));

      await expect(fetchAndCacheSingle(999)).rejects.toThrow('Not found');
      expect(useAssetStore.getState().getAssetById(999)).toBeUndefined();
    });
  });

  describe('createAndCache', () => {
    it('should create via API and add to cache', async () => {
      const createData: CreateAssetRequest = {
        identifier: 'LAP-001',
        name: 'Test Laptop',
        type: 'device',
      };
      vi.mocked(assetsApi.create).mockResolvedValue({ data: mockAsset });

      const asset = await createAndCache(createData);

      expect(assetsApi.create).toHaveBeenCalledWith(createData);
      expect(asset).toEqual(mockAsset);
      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toEqual(mockAsset);
    });

    it('should not update cache on API error', async () => {
      const createData: CreateAssetRequest = {
        identifier: 'LAP-001',
        name: 'Test Laptop',
        type: 'device',
      };
      vi.mocked(assetsApi.create).mockRejectedValue(new Error('Validation error'));

      await expect(createAndCache(createData)).rejects.toThrow('Validation error');
      expect(useAssetStore.getState().cache.byId.size).toBe(0);
    });
  });

  describe('updateAndCache', () => {
    it('should update via API and update cache', async () => {
      useAssetStore.getState().addAsset(mockAsset);
      const updates = { name: 'Updated Name' };
      const updated = { ...mockAsset, ...updates };
      vi.mocked(assetsApi.update).mockResolvedValue({ data: updated });

      const asset = await updateAndCache(mockAsset.id, updates);

      expect(assetsApi.update).toHaveBeenCalledWith(mockAsset.id, updates);
      expect(asset).toEqual(updated);
      expect(useAssetStore.getState().getAssetById(mockAsset.id)?.name).toBe('Updated Name');
    });

    it('should not update cache on API error', async () => {
      useAssetStore.getState().addAsset(mockAsset);
      const originalName = mockAsset.name;
      vi.mocked(assetsApi.update).mockRejectedValue(new Error('Conflict'));

      await expect(updateAndCache(mockAsset.id, { name: 'New' })).rejects.toThrow('Conflict');
      expect(useAssetStore.getState().getAssetById(mockAsset.id)?.name).toBe(originalName);
    });
  });

  describe('deleteAndRemoveFromCache', () => {
    it('should delete via API and remove from cache', async () => {
      useAssetStore.getState().addAsset(mockAsset);
      vi.mocked(assetsApi.delete).mockResolvedValue({ data: { deleted: true } });

      await deleteAndRemoveFromCache(mockAsset.id);

      expect(assetsApi.delete).toHaveBeenCalledWith(mockAsset.id);
      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toBeUndefined();
    });

    it('should not remove from cache on API error', async () => {
      useAssetStore.getState().addAsset(mockAsset);
      vi.mocked(assetsApi.delete).mockRejectedValue(new Error('Server error'));

      await expect(deleteAndRemoveFromCache(mockAsset.id)).rejects.toThrow('Server error');
      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toBeDefined();
    });
  });

  describe('handleBulkUploadComplete', () => {
    it('should invalidate cache and refetch', async () => {
      useAssetStore.getState().addAsset(mockAsset);
      vi.mocked(assetsApi.list).mockResolvedValue({ data: [mockAsset1, mockAsset2] });

      await handleBulkUploadComplete('job123');

      expect(useAssetStore.getState().cache.byId.size).toBe(2);
      expect(assetsApi.list).toHaveBeenCalled();
    });

    it('should throw if refetch fails', async () => {
      vi.mocked(assetsApi.list).mockRejectedValue(new Error('Network error'));

      await expect(handleBulkUploadComplete('job123')).rejects.toThrow('Network error');
    });
  });
});
```

**Acceptance Criteria**:
- [ ] All 8 helper functions implemented
- [ ] 20+ unit tests passing
- [ ] No direct Zustand subscriptions (use getState())
- [ ] All functions handle errors correctly (no cache corruption)
- [ ] TypeScript compiles with 0 errors
- [ ] ESLint passes with 0 new warnings

---

### File 2: `hooks/assets/useAssets.ts`

**Purpose**: Fetch and display paginated asset lists with filters, sort, and search.

**Features**:
- ✅ Cache-first strategy (no API if fresh)
- ✅ Auto-fetch on mount (configurable)
- ✅ Apply filters/sort from store
- ✅ Manual refetch capability
- ✅ Loading and error states

**Complete Implementation**:

```typescript
import { useEffect, useState, useCallback } from 'react';
import { useAssetStore } from '@/stores/assets/assetStore';
import {
  fetchAndCacheAssets,
  isCacheStale,
  isCacheEmpty,
} from '@/lib/asset/cache-integration';
import type { Asset } from '@/types/assets';

export interface UseAssetsOptions {
  enabled?: boolean;         // Auto-fetch on mount (default: true)
  refetchOnMount?: boolean;  // Ignore cache, force refetch (default: false)
}

export interface UseAssetsReturn {
  assets: Asset[];           // Filtered, sorted, paginated
  totalCount: number;        // Total matching filters (before pagination)
  isLoading: boolean;        // Initial fetch in progress
  isRefetching: boolean;     // Background refetch in progress
  error: Error | null;       // Last error
  refetch: () => Promise<void>;  // Manual refetch
}

/**
 * Hook for fetching and displaying asset lists
 *
 * Features:
 * - Cache-first: Only fetches if cache is empty or stale (>1 hour)
 * - Reactive: Automatically updates when store changes
 * - Filtering: Uses store filters (set via store.setFilters)
 * - Sorting: Uses store sort (set via store.setSort)
 * - Pagination: Uses store pagination (set via store.setPage)
 *
 * @example
 * // Basic usage
 * const { assets, isLoading, error } = useAssets();
 *
 * @example
 * // Force refetch on mount
 * const { assets, isLoading, refetch } = useAssets({ refetchOnMount: true });
 *
 * @example
 * // Manual control
 * const { assets, refetch } = useAssets({ enabled: false });
 * // Later: await refetch();
 */
export function useAssets(options: UseAssetsOptions = {}): UseAssetsReturn {
  const { enabled = true, refetchOnMount = false } = options;

  const [isLoading, setIsLoading] = useState(false);
  const [isRefetching, setIsRefetching] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Subscribe to filtered assets from store
  const assets = useAssetStore((state) => state.getPaginatedAssets());
  const totalCount = useAssetStore((state) => state.pagination.totalCount);

  /**
   * Fetch assets from API and update cache
   * Skips fetch if cache is fresh (unless forceRefetch)
   */
  const fetchAssets = useCallback(async (forceRefetch = false) => {
    // Check if fetch is needed
    if (!forceRefetch && !isCacheEmpty() && !isCacheStale()) {
      return; // Cache is fresh, no fetch needed
    }

    const isInitialLoad = isCacheEmpty();
    if (isInitialLoad) {
      setIsLoading(true);
    } else {
      setIsRefetching(true);
    }

    setError(null);

    try {
      await fetchAndCacheAssets();
    } catch (err) {
      setError(err as Error);
    } finally {
      setIsLoading(false);
      setIsRefetching(false);
    }
  }, []);

  /**
   * Auto-fetch on mount (if enabled)
   */
  useEffect(() => {
    if (enabled) {
      fetchAssets(refetchOnMount);
    }
  }, [enabled, refetchOnMount, fetchAssets]);

  /**
   * Manual refetch (always forces)
   */
  const refetch = useCallback(async () => {
    await fetchAssets(true);
  }, [fetchAssets]);

  return {
    assets,
    totalCount,
    isLoading,
    isRefetching,
    error,
    refetch,
  };
}
```

**Test Requirements** (10 tests):

```typescript
// useAssets.test.ts
import { renderHook, waitFor } from '@testing-library/react';
import { vi } from 'vitest';
import { useAssets } from './useAssets';
import { useAssetStore } from '@/stores/assets/assetStore';
import * as cacheIntegration from '@/lib/asset/cache-integration';

vi.mock('@/lib/asset/cache-integration');

describe('useAssets', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should return empty array initially', () => {
    const { result } = renderHook(() => useAssets({ enabled: false }));

    expect(result.current.assets).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('should auto-fetch on mount if cache is empty', async () => {
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(true);
    vi.mocked(cacheIntegration.isCacheStale).mockReturnValue(false);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset]);

    const { result } = renderHook(() => useAssets());

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheAssets).toHaveBeenCalled();
  });

  it('should auto-fetch on mount if cache is stale', async () => {
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(false);
    vi.mocked(cacheIntegration.isCacheStale).mockReturnValue(true);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset]);

    const { result } = renderHook(() => useAssets());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheAssets).toHaveBeenCalled();
  });

  it('should not fetch if cache is fresh', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(false);
    vi.mocked(cacheIntegration.isCacheStale).mockReturnValue(false);

    const { result } = renderHook(() => useAssets());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheAssets).not.toHaveBeenCalled();
    expect(result.current.assets).toHaveLength(1);
  });

  it('should force refetch when refetchOnMount is true', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(false);
    vi.mocked(cacheIntegration.isCacheStale).mockReturnValue(false);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset]);

    const { result } = renderHook(() => useAssets({ refetchOnMount: true }));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheAssets).toHaveBeenCalled();
  });

  it('should not fetch when enabled is false', async () => {
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(true);

    const { result } = renderHook(() => useAssets({ enabled: false }));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheAssets).not.toHaveBeenCalled();
  });

  it('should handle fetch errors', async () => {
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(true);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockRejectedValue(
      new Error('Network error')
    );

    const { result } = renderHook(() => useAssets());

    await waitFor(() => {
      expect(result.current.error).toEqual(new Error('Network error'));
    });

    expect(result.current.isLoading).toBe(false);
  });

  it('should allow manual refetch', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset]);

    const { result } = renderHook(() => useAssets({ enabled: false }));

    await result.current.refetch();

    await waitFor(() => {
      expect(cacheIntegration.fetchAndCacheAssets).toHaveBeenCalled();
    });
  });

  it('should show isRefetching on background refetch', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    vi.mocked(cacheIntegration.isCacheEmpty).mockReturnValue(false);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset]);

    const { result } = renderHook(() => useAssets({ enabled: false }));

    const refetchPromise = result.current.refetch();

    await waitFor(() => {
      expect(result.current.isRefetching).toBe(true);
    });

    await refetchPromise;

    expect(result.current.isRefetching).toBe(false);
  });

  it('should reactively update when store changes', () => {
    useAssetStore.getState().addAsset(mockAsset);

    const { result, rerender } = renderHook(() => useAssets({ enabled: false }));

    expect(result.current.assets).toHaveLength(1);

    // Add another asset
    useAssetStore.getState().addAsset(mockAsset2);
    rerender();

    expect(result.current.assets).toHaveLength(2);
  });
});
```

**Acceptance Criteria**:
- [ ] Hook implemented with TypeScript types
- [ ] 10+ unit tests passing
- [ ] Cache-first logic verified
- [ ] No unnecessary refetches
- [ ] Reactive to store updates
- [ ] Loading states accurate

---

### File 3: `hooks/assets/useAsset.ts`

**Purpose**: Fetch and display a single asset by ID with cache-first strategy.

**Complete Implementation**:

```typescript
import { useEffect, useState, useCallback } from 'react';
import { useAssetStore } from '@/stores/assets/assetStore';
import { fetchAndCacheSingle } from '@/lib/asset/cache-integration';
import type { Asset } from '@/types/assets';

export interface UseAssetOptions {
  enabled?: boolean;  // Auto-fetch on mount (default: true)
}

export interface UseAssetReturn {
  asset: Asset | null;       // Single asset or null if not found
  isLoading: boolean;        // Fetch in progress
  error: Error | null;       // Last error
  refetch: () => Promise<void>;  // Manual refetch
}

/**
 * Hook for fetching a single asset by ID
 *
 * Features:
 * - Cache-first: Returns cached asset instantly, no API call
 * - Lazy fetch: Only fetches if not in cache
 * - Reactive: Updates when store changes
 * - Null-safe: Handles null/undefined IDs gracefully
 *
 * @example
 * const { asset, isLoading, error } = useAsset(123);
 *
 * @example
 * // Conditional fetching
 * const { asset } = useAsset(selectedId); // selectedId can be null
 *
 * @example
 * // Manual control
 * const { asset, refetch } = useAsset(123, { enabled: false });
 * // Later: await refetch();
 */
export function useAsset(
  id: number | null,
  options: UseAssetOptions = {}
): UseAssetReturn {
  const { enabled = true } = options;

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Subscribe to asset from cache
  const asset = useAssetStore((state) =>
    id ? state.getAssetById(id) ?? null : null
  );

  /**
   * Fetch asset from API (only if not cached)
   */
  const fetchAsset = useCallback(async () => {
    if (!id) return;

    // Check cache first
    const cached = useAssetStore.getState().getAssetById(id);
    if (cached) return; // Already cached

    setIsLoading(true);
    setError(null);

    try {
      await fetchAndCacheSingle(id);
    } catch (err) {
      setError(err as Error);
    } finally {
      setIsLoading(false);
    }
  }, [id]);

  /**
   * Auto-fetch on mount or when ID changes
   */
  useEffect(() => {
    if (enabled && id) {
      fetchAsset();
    }
  }, [id, enabled, fetchAsset]);

  return {
    asset,
    isLoading,
    error,
    refetch: fetchAsset,
  };
}
```

**Test Requirements** (8 tests):

```typescript
// useAsset.test.ts
describe('useAsset', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should return null for null ID', () => {
    const { result } = renderHook(() => useAsset(null));

    expect(result.current.asset).toBeNull();
    expect(result.current.isLoading).toBe(false);
  });

  it('should return cached asset without API call', () => {
    useAssetStore.getState().addAsset(mockAsset);

    const { result } = renderHook(() => useAsset(mockAsset.id));

    expect(result.current.asset).toEqual(mockAsset);
    expect(cacheIntegration.fetchAndCacheSingle).not.toHaveBeenCalled();
  });

  it('should fetch from API when not cached', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheSingle).mockResolvedValue(mockAsset);

    const { result } = renderHook(() => useAsset(mockAsset.id));

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(cacheIntegration.fetchAndCacheSingle).toHaveBeenCalledWith(mockAsset.id);
  });

  it('should handle fetch errors', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheSingle).mockRejectedValue(
      new Error('Not found')
    );

    const { result } = renderHook(() => useAsset(999));

    await waitFor(() => {
      expect(result.current.error).toEqual(new Error('Not found'));
    });

    expect(result.current.asset).toBeNull();
  });

  it('should not fetch when enabled is false', () => {
    const { result } = renderHook(() => useAsset(123, { enabled: false }));

    expect(cacheIntegration.fetchAndCacheSingle).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });

  it('should refetch when ID changes', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheSingle).mockResolvedValue(mockAsset);

    const { rerender } = renderHook(
      ({ id }) => useAsset(id),
      { initialProps: { id: 1 } }
    );

    await waitFor(() => {
      expect(cacheIntegration.fetchAndCacheSingle).toHaveBeenCalledWith(1);
    });

    vi.clearAllMocks();
    rerender({ id: 2 });

    await waitFor(() => {
      expect(cacheIntegration.fetchAndCacheSingle).toHaveBeenCalledWith(2);
    });
  });

  it('should allow manual refetch', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheSingle).mockResolvedValue(mockAsset);

    const { result } = renderHook(() => useAsset(123, { enabled: false }));

    await result.current.refetch();

    await waitFor(() => {
      expect(cacheIntegration.fetchAndCacheSingle).toHaveBeenCalled();
    });
  });

  it('should reactively update when cache changes', () => {
    const { result, rerender } = renderHook(() => useAsset(mockAsset.id, { enabled: false }));

    expect(result.current.asset).toBeNull();

    useAssetStore.getState().addAsset(mockAsset);
    rerender();

    expect(result.current.asset).toEqual(mockAsset);
  });
});
```

**Acceptance Criteria**:
- [ ] Hook implemented with TypeScript types
- [ ] 8+ unit tests passing
- [ ] Cache-first behavior verified
- [ ] Null ID handling tested
- [ ] Reactive to store updates
- [ ] No memory leaks (useEffect cleanup)

---

### File 4: `hooks/assets/useAssetMutations.ts`

**Purpose**: CRUD operations (Create/Update/Delete) with loading states and error handling.

**Complete Implementation**:

```typescript
import { useState, useCallback } from 'react';
import {
  createAndCache,
  updateAndCache,
  deleteAndRemoveFromCache,
} from '@/lib/asset/cache-integration';
import { validateAssetData } from '@/lib/asset/validators';
import type { Asset, CreateAssetRequest, UpdateAssetRequest } from '@/types/assets';

export interface UseAssetMutationsReturn {
  // Create
  createAsset: (data: CreateAssetRequest) => Promise<Asset>;
  isCreating: boolean;
  createError: Error | null;

  // Update
  updateAsset: (id: number, updates: UpdateAssetRequest) => Promise<Asset>;
  isUpdating: boolean;
  updateError: Error | null;

  // Delete
  deleteAsset: (id: number) => Promise<void>;
  isDeleting: boolean;
  deleteError: Error | null;
}

/**
 * Hook for asset CRUD operations
 *
 * Features:
 * - Automatic cache updates (server response is source of truth)
 * - Validation before create (Phase 2 validators)
 * - Loading states per operation
 * - Error states per operation
 * - No optimistic updates (safer pattern)
 *
 * @example
 * const { createAsset, isCreating, createError } = useAssetMutations();
 *
 * const handleCreate = async () => {
 *   try {
 *     const asset = await createAsset({
 *       identifier: 'LAP-001',
 *       name: 'Laptop',
 *       type: 'device'
 *     });
 *     console.log('Created:', asset);
 *   } catch (err) {
 *     console.error('Error:', createError);
 *   }
 * };
 */
export function useAssetMutations(): UseAssetMutationsReturn {
  // Create state
  const [isCreating, setIsCreating] = useState(false);
  const [createError, setCreateError] = useState<Error | null>(null);

  // Update state
  const [isUpdating, setIsUpdating] = useState(false);
  const [updateError, setUpdateError] = useState<Error | null>(null);

  // Delete state
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<Error | null>(null);

  /**
   * Create new asset
   * Pattern: Validate → API call → Cache update
   */
  const createAsset = useCallback(async (data: CreateAssetRequest): Promise<Asset> => {
    // Validate first (Phase 2 validators)
    const validationError = validateAssetData(data);
    if (validationError) {
      const error = new Error(`Validation error: ${validationError}`);
      setCreateError(error);
      throw error;
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

  /**
   * Update existing asset
   * Pattern: API call → Cache update (with server response)
   */
  const updateAsset = useCallback(
    async (id: number, updates: UpdateAssetRequest): Promise<Asset> => {
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
    },
    []
  );

  /**
   * Delete asset
   * Pattern: API call → Cache remove
   */
  const deleteAsset = useCallback(async (id: number): Promise<void> => {
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

**Test Requirements** (12 tests):

```typescript
// useAssetMutations.test.ts
describe('useAssetMutations', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  describe('createAsset', () => {
    it('should create asset and update cache', async () => {
      vi.mocked(cacheIntegration.createAndCache).mockResolvedValue(mockAsset);

      const { result } = renderHook(() => useAssetMutations());

      const asset = await result.current.createAsset({
        identifier: 'LAP-001',
        name: 'Laptop',
        type: 'device',
      });

      expect(asset).toEqual(mockAsset);
      expect(result.current.createError).toBeNull();
    });

    it('should validate before creating', async () => {
      const { result } = renderHook(() => useAssetMutations());

      await expect(
        result.current.createAsset({
          identifier: '',  // Invalid
          name: 'Laptop',
          type: 'device',
        })
      ).rejects.toThrow('Validation error');

      expect(result.current.createError).toBeTruthy();
      expect(cacheIntegration.createAndCache).not.toHaveBeenCalled();
    });

    it('should handle API errors', async () => {
      vi.mocked(cacheIntegration.createAndCache).mockRejectedValue(
        new Error('Duplicate identifier')
      );

      const { result } = renderHook(() => useAssetMutations());

      await expect(
        result.current.createAsset({
          identifier: 'LAP-001',
          name: 'Laptop',
          type: 'device',
        })
      ).rejects.toThrow('Duplicate identifier');

      expect(result.current.createError?.message).toBe('Duplicate identifier');
    });

    it('should track isCreating state', async () => {
      vi.mocked(cacheIntegration.createAndCache).mockImplementation(
        () => new Promise((resolve) => setTimeout(() => resolve(mockAsset), 100))
      );

      const { result } = renderHook(() => useAssetMutations());

      const createPromise = result.current.createAsset({
        identifier: 'LAP-001',
        name: 'Laptop',
        type: 'device',
      });

      await waitFor(() => {
        expect(result.current.isCreating).toBe(true);
      });

      await createPromise;

      expect(result.current.isCreating).toBe(false);
    });
  });

  describe('updateAsset', () => {
    it('should update asset and update cache', async () => {
      const updated = { ...mockAsset, name: 'Updated Name' };
      vi.mocked(cacheIntegration.updateAndCache).mockResolvedValue(updated);

      const { result } = renderHook(() => useAssetMutations());

      const asset = await result.current.updateAsset(mockAsset.id, { name: 'Updated Name' });

      expect(asset).toEqual(updated);
      expect(result.current.updateError).toBeNull();
    });

    it('should handle API errors', async () => {
      vi.mocked(cacheIntegration.updateAndCache).mockRejectedValue(
        new Error('Not found')
      );

      const { result } = renderHook(() => useAssetMutations());

      await expect(
        result.current.updateAsset(999, { name: 'New' })
      ).rejects.toThrow('Not found');

      expect(result.current.updateError?.message).toBe('Not found');
    });

    it('should track isUpdating state', async () => {
      vi.mocked(cacheIntegration.updateAndCache).mockImplementation(
        () => new Promise((resolve) => setTimeout(() => resolve(mockAsset), 100))
      );

      const { result } = renderHook(() => useAssetMutations());

      const updatePromise = result.current.updateAsset(1, { name: 'New' });

      await waitFor(() => {
        expect(result.current.isUpdating).toBe(true);
      });

      await updatePromise;

      expect(result.current.isUpdating).toBe(false);
    });
  });

  describe('deleteAsset', () => {
    it('should delete asset and remove from cache', async () => {
      vi.mocked(cacheIntegration.deleteAndRemoveFromCache).mockResolvedValue();

      const { result } = renderHook(() => useAssetMutations());

      await result.current.deleteAsset(mockAsset.id);

      expect(cacheIntegration.deleteAndRemoveFromCache).toHaveBeenCalledWith(mockAsset.id);
      expect(result.current.deleteError).toBeNull();
    });

    it('should handle API errors', async () => {
      vi.mocked(cacheIntegration.deleteAndRemoveFromCache).mockRejectedValue(
        new Error('Cannot delete')
      );

      const { result } = renderHook(() => useAssetMutations());

      await expect(
        result.current.deleteAsset(123)
      ).rejects.toThrow('Cannot delete');

      expect(result.current.deleteError?.message).toBe('Cannot delete');
    });

    it('should track isDeleting state', async () => {
      vi.mocked(cacheIntegration.deleteAndRemoveFromCache).mockImplementation(
        () => new Promise((resolve) => setTimeout(() => resolve(), 100))
      );

      const { result } = renderHook(() => useAssetMutations());

      const deletePromise = result.current.deleteAsset(1);

      await waitFor(() => {
        expect(result.current.isDeleting).toBe(true);
      });

      await deletePromise;

      expect(result.current.isDeleting).toBe(false);
    });
  });

  it('should maintain separate error states', async () => {
    vi.mocked(cacheIntegration.createAndCache).mockRejectedValue(new Error('Create error'));
    vi.mocked(cacheIntegration.updateAndCache).mockRejectedValue(new Error('Update error'));

    const { result } = renderHook(() => useAssetMutations());

    await expect(result.current.createAsset({ identifier: 'A', name: 'B', type: 'device' }))
      .rejects.toThrow('Create error');

    expect(result.current.createError?.message).toBe('Create error');
    expect(result.current.updateError).toBeNull();
    expect(result.current.deleteError).toBeNull();
  });
});
```

**Acceptance Criteria**:
- [ ] Hook implemented with TypeScript types
- [ ] 12+ unit tests passing
- [ ] Phase 2 validation integrated
- [ ] Separate loading/error states per operation
- [ ] No optimistic updates (server-first pattern)
- [ ] Cache updates verified

---

### File 5: `hooks/assets/useBulkUpload.ts`

**Purpose**: CSV upload with automatic job status polling and cleanup.

**Complete Implementation**:

```typescript
import { useState, useRef, useEffect, useCallback } from 'react';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import { handleBulkUploadComplete } from '@/lib/asset/cache-integration';
import { CSV_VALIDATION } from '@/types/assets';
import type { JobStatusResponse } from '@/types/assets';

export interface UseBulkUploadReturn {
  uploadCSV: (file: File) => Promise<void>;
  jobStatus: JobStatusResponse | null;
  isUploading: boolean;     // File upload in progress
  isPolling: boolean;       // Job status polling in progress
  error: Error | null;
  cancelPolling: () => void;
}

/**
 * Hook for CSV bulk upload with automatic job status polling
 *
 * Features:
 * - File validation (size, type)
 * - Automatic polling every 2 seconds
 * - Cache invalidation on success
 * - Cleanup on unmount
 * - Manual cancellation
 *
 * @example
 * const { uploadCSV, jobStatus, isUploading, isPolling, error } = useBulkUpload();
 *
 * const handleFileSelect = async (file: File) => {
 *   try {
 *     await uploadCSV(file);
 *     // Polling starts automatically
 *     // When complete, cache is invalidated and refetched
 *   } catch (err) {
 *     console.error('Upload failed:', err);
 *   }
 * };
 */
export function useBulkUpload(): UseBulkUploadReturn {
  const [isUploading, setIsUploading] = useState(false);
  const [isPolling, setIsPolling] = useState(false);
  const [jobStatus, setJobStatus] = useState<JobStatusResponse | null>(null);
  const [error, setError] = useState<Error | null>(null);

  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const { setUploadJobId, setPollingInterval, clearUploadState } = useAssetStore();

  /**
   * Stop polling and cleanup
   */
  const cancelPolling = useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
      setPollingInterval(null);
    }
    setIsPolling(false);
    clearUploadState();
  }, [setPollingInterval, clearUploadState]);

  /**
   * Poll job status
   */
  const pollJobStatus = useCallback(
    async (jobId: string) => {
      try {
        const status = await assetsApi.getJobStatus(jobId);
        setJobStatus(status);

        // Check if complete
        if (status.status === 'completed') {
          cancelPolling();

          // Invalidate cache and refetch
          await handleBulkUploadComplete(jobId);
        } else if (status.status === 'failed') {
          cancelPolling();
          setError(new Error('Bulk upload job failed'));
        }
        // Continue polling if status === 'processing'
      } catch (err) {
        setError(err as Error);
        cancelPolling();
      }
    },
    [cancelPolling]
  );

  /**
   * Upload CSV file
   */
  const uploadCSV = useCallback(
    async (file: File): Promise<void> => {
      // Validate file size
      if (file.size > CSV_VALIDATION.MAX_FILE_SIZE) {
        const error = new Error(
          `File too large. Maximum size: ${CSV_VALIDATION.MAX_FILE_SIZE / 1024 / 1024}MB`
        );
        setError(error);
        throw error;
      }

      // Validate file type
      if (!CSV_VALIDATION.ACCEPTED_TYPES.includes(file.type) &&
          !file.name.endsWith('.csv')) {
        const error = new Error('Invalid file type. Only CSV files are allowed.');
        setError(error);
        throw error;
      }

      setIsUploading(true);
      setError(null);
      setJobStatus(null);

      try {
        // Upload file
        const response = await assetsApi.uploadCSV(file);
        setUploadJobId(response.job_id);
        setJobStatus(response);

        // Start polling
        setIsPolling(true);
        const intervalId = setInterval(() => {
          pollJobStatus(response.job_id);
        }, 2000); // Poll every 2 seconds

        intervalRef.current = intervalId;
        setPollingInterval(intervalId);
      } catch (err) {
        setError(err as Error);
        throw err;
      } finally {
        setIsUploading(false);
      }
    },
    [setUploadJobId, setPollingInterval, pollJobStatus]
  );

  /**
   * Cleanup on unmount
   */
  useEffect(() => {
    return () => {
      cancelPolling();
    };
  }, [cancelPolling]);

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

**Test Requirements** (8 tests):

```typescript
// useBulkUpload.test.ts
import { renderHook, waitFor, act } from '@testing-library/react';
import { vi } from 'vitest';
import { useBulkUpload } from './useBulkUpload';
import { assetsApi } from '@/lib/api/assets';
import * as cacheIntegration from '@/lib/asset/cache-integration';

vi.mock('@/lib/api/assets');
vi.mock('@/lib/asset/cache-integration');

describe('useBulkUpload', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('should validate file size', async () => {
    const largeFile = new File(['x'.repeat(6 * 1024 * 1024)], 'large.csv', { type: 'text/csv' });

    const { result } = renderHook(() => useBulkUpload());

    await expect(result.current.uploadCSV(largeFile)).rejects.toThrow('File too large');
    expect(result.current.error?.message).toContain('File too large');
  });

  it('should validate file type', async () => {
    const invalidFile = new File(['data'], 'file.txt', { type: 'text/plain' });

    const { result } = renderHook(() => useBulkUpload());

    await expect(result.current.uploadCSV(invalidFile)).rejects.toThrow('Invalid file type');
    expect(result.current.error?.message).toContain('Invalid file type');
  });

  it('should upload CSV and start polling', async () => {
    const file = new File(['col1,col2\nval1,val2'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };
    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);

    const { result } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    expect(assetsApi.uploadCSV).toHaveBeenCalledWith(file);
    expect(result.current.jobStatus).toEqual(uploadResponse);
    expect(result.current.isPolling).toBe(true);
  });

  it('should poll job status every 2 seconds', async () => {
    const file = new File(['data'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };
    const processingStatus = { job_id: 'job123', status: 'processing', created_count: 0, error_count: 0, errors: null };

    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);
    vi.mocked(assetsApi.getJobStatus).mockResolvedValue(processingStatus);

    const { result } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    // Advance timer by 2 seconds
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    await waitFor(() => {
      expect(assetsApi.getJobStatus).toHaveBeenCalledWith('job123');
    });

    // Advance another 2 seconds
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    await waitFor(() => {
      expect(assetsApi.getJobStatus).toHaveBeenCalledTimes(2);
    });
  });

  it('should stop polling when job completes', async () => {
    const file = new File(['data'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };
    const completedStatus = {
      job_id: 'job123',
      status: 'completed',
      created_count: 10,
      error_count: 0,
      errors: null
    };

    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);
    vi.mocked(assetsApi.getJobStatus).mockResolvedValue(completedStatus);
    vi.mocked(cacheIntegration.handleBulkUploadComplete).mockResolvedValue();

    const { result } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    await waitFor(() => {
      expect(result.current.isPolling).toBe(false);
    });

    expect(cacheIntegration.handleBulkUploadComplete).toHaveBeenCalledWith('job123');
  });

  it('should stop polling when job fails', async () => {
    const file = new File(['data'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };
    const failedStatus = {
      job_id: 'job123',
      status: 'failed',
      created_count: 0,
      error_count: 10,
      errors: [{ row: 1, error: 'Invalid data' }]
    };

    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);
    vi.mocked(assetsApi.getJobStatus).mockResolvedValue(failedStatus);

    const { result } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    await waitFor(() => {
      expect(result.current.isPolling).toBe(false);
    });

    expect(result.current.error?.message).toBe('Bulk upload job failed');
  });

  it('should cleanup polling on unmount', async () => {
    const file = new File(['data'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };

    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);

    const { result, unmount } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    expect(result.current.isPolling).toBe(true);

    unmount();

    // Verify interval was cleared
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(assetsApi.getJobStatus).not.toHaveBeenCalled();
  });

  it('should allow manual cancellation', async () => {
    const file = new File(['data'], 'test.csv', { type: 'text/csv' });
    const uploadResponse = { job_id: 'job123', status: 'processing', message: 'Processing' };

    vi.mocked(assetsApi.uploadCSV).mockResolvedValue(uploadResponse);

    const { result } = renderHook(() => useBulkUpload());

    await act(async () => {
      await result.current.uploadCSV(file);
    });

    expect(result.current.isPolling).toBe(true);

    act(() => {
      result.current.cancelPolling();
    });

    expect(result.current.isPolling).toBe(false);

    // Verify no more polling
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(assetsApi.getJobStatus).not.toHaveBeenCalled();
  });
});
```

**Acceptance Criteria**:
- [ ] Hook implemented with TypeScript types
- [ ] 8+ unit tests passing
- [ ] File validation (size, type)
- [ ] Polling every 2 seconds
- [ ] Cleanup on unmount verified
- [ ] Manual cancellation works
- [ ] Cache invalidation on completion

---

### File 6: `hooks/index.ts`

**Purpose**: Central export point for clean imports.

```typescript
/**
 * Asset Management Hooks
 *
 * Phase 4 React hooks for data integration
 */

// List hooks
export { useAssets } from './assets/useAssets';
export type { UseAssetsOptions, UseAssetsReturn } from './assets/useAssets';

// Single asset hooks
export { useAsset } from './assets/useAsset';
export type { UseAssetOptions, UseAssetReturn } from './assets/useAsset';

// Mutation hooks
export { useAssetMutations } from './assets/useAssetMutations';
export type { UseAssetMutationsReturn } from './assets/useAssetMutations';

// Bulk upload hooks
export { useBulkUpload } from './assets/useBulkUpload';
export type { UseBulkUploadReturn } from './assets/useBulkUpload';
```

---

## Integration Tests

**File**: `hooks/assets/__tests__/integration.test.ts`

**Purpose**: Verify all hooks work together correctly in realistic scenarios.

```typescript
import { renderHook, waitFor, act } from '@testing-library/react';
import { vi } from 'vitest';
import { useAssets } from '../useAssets';
import { useAsset } from '../useAsset';
import { useAssetMutations } from '../useAssetMutations';
import { useAssetStore } from '@/stores/assets/assetStore';
import * as cacheIntegration from '@/lib/asset/cache-integration';

describe('Integration Tests', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should create asset and reflect in useAssets list', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([]);
    vi.mocked(cacheIntegration.createAndCache).mockResolvedValue(mockAsset);

    // Start with useAssets
    const { result: assetsResult } = renderHook(() => useAssets());

    await waitFor(() => {
      expect(assetsResult.current.isLoading).toBe(false);
    });

    expect(assetsResult.current.assets).toHaveLength(0);

    // Create asset
    const { result: mutationsResult } = renderHook(() => useAssetMutations());

    await act(async () => {
      await mutationsResult.current.createAsset({
        identifier: 'LAP-001',
        name: 'Laptop',
        type: 'device',
      });
    });

    // Verify shows up in list
    expect(assetsResult.current.assets).toHaveLength(1);
    expect(assetsResult.current.assets[0]).toEqual(mockAsset);
  });

  it('should update asset and reflect in useAsset', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    const updated = { ...mockAsset, name: 'Updated Name' };
    vi.mocked(cacheIntegration.updateAndCache).mockResolvedValue(updated);

    // Start with useAsset
    const { result: assetResult } = renderHook(() => useAsset(mockAsset.id, { enabled: false }));

    expect(assetResult.current.asset?.name).toBe('Test Laptop');

    // Update asset
    const { result: mutationsResult } = renderHook(() => useAssetMutations());

    await act(async () => {
      await mutationsResult.current.updateAsset(mockAsset.id, { name: 'Updated Name' });
    });

    // Verify change reflected
    expect(assetResult.current.asset?.name).toBe('Updated Name');
  });

  it('should delete asset and remove from useAssets list', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    vi.mocked(cacheIntegration.deleteAndRemoveFromCache).mockResolvedValue();

    // Start with useAssets
    const { result: assetsResult } = renderHook(() => useAssets({ enabled: false }));

    expect(assetsResult.current.assets).toHaveLength(1);

    // Delete asset
    const { result: mutationsResult } = renderHook(() => useAssetMutations());

    await act(async () => {
      await mutationsResult.current.deleteAsset(mockAsset.id);
    });

    // Verify removed from list
    expect(assetsResult.current.assets).toHaveLength(0);
  });

  it('should handle useAssets fetch error gracefully', async () => {
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockRejectedValue(
      new Error('Network error')
    );

    const { result } = renderHook(() => useAssets());

    await waitFor(() => {
      expect(result.current.error?.message).toBe('Network error');
    });

    expect(result.current.assets).toHaveLength(0);
    expect(result.current.isLoading).toBe(false);
  });

  it('should maintain cache consistency across hooks', async () => {
    useAssetStore.getState().addAsset(mockAsset);

    const { result: assetsResult } = renderHook(() => useAssets({ enabled: false }));
    const { result: assetResult } = renderHook(() => useAsset(mockAsset.id, { enabled: false }));

    expect(assetsResult.current.assets).toHaveLength(1);
    expect(assetResult.current.asset).toEqual(mockAsset);

    // Verify both read from same cache
    expect(assetsResult.current.assets[0]).toBe(assetResult.current.asset);
  });

  it('should handle concurrent mutations correctly', async () => {
    vi.mocked(cacheIntegration.createAndCache).mockImplementation((data) =>
      Promise.resolve({ ...mockAsset, identifier: data.identifier, name: data.name })
    );

    const { result } = renderHook(() => useAssetMutations());

    // Create multiple assets concurrently
    await act(async () => {
      await Promise.all([
        result.current.createAsset({ identifier: 'A', name: 'Asset A', type: 'device' }),
        result.current.createAsset({ identifier: 'B', name: 'Asset B', type: 'person' }),
        result.current.createAsset({ identifier: 'C', name: 'Asset C', type: 'location' }),
      ]);
    });

    expect(useAssetStore.getState().cache.byId.size).toBe(3);
  });

  it('should not cache on API error', async () => {
    vi.mocked(cacheIntegration.createAndCache).mockRejectedValue(new Error('Duplicate'));

    const { result } = renderHook(() => useAssetMutations());

    await expect(
      result.current.createAsset({ identifier: 'LAP-001', name: 'Laptop', type: 'device' })
    ).rejects.toThrow('Duplicate');

    // Verify cache unchanged
    expect(useAssetStore.getState().cache.byId.size).toBe(0);
  });

  it('should refetch after cache invalidation', async () => {
    useAssetStore.getState().addAsset(mockAsset);
    vi.mocked(cacheIntegration.fetchAndCacheAssets).mockResolvedValue([mockAsset, mockAsset2]);

    const { result } = renderHook(() => useAssets({ enabled: false }));

    expect(result.current.assets).toHaveLength(1);

    // Invalidate cache
    act(() => {
      useAssetStore.getState().invalidateCache();
    });

    // Refetch
    await act(async () => {
      await result.current.refetch();
    });

    expect(result.current.assets).toHaveLength(2);
  });
});
```

**Acceptance Criteria**:
- [ ] 8+ integration tests passing
- [ ] All hooks tested together
- [ ] Cache consistency verified
- [ ] Error scenarios covered

---

## Validation Steps

### Step 1: TypeScript Compilation

```bash
cd frontend
pnpm typecheck
```

**Expected**: 0 errors

---

### Step 2: Linting

```bash
pnpm lint
```

**Expected**: 0 new warnings/errors

---

### Step 3: Unit Tests

```bash
# Cache integration helpers
pnpm vitest run src/lib/asset/cache-integration.test.ts

# Individual hooks
pnpm vitest run src/hooks/assets/useAssets.test.ts
pnpm vitest run src/hooks/assets/useAsset.test.ts
pnpm vitest run src/hooks/assets/useAssetMutations.test.ts
pnpm vitest run src/hooks/assets/useBulkUpload.test.ts

# All hooks tests
pnpm vitest run src/hooks/assets/
```

**Expected**: 58+ tests passing

---

### Step 4: Integration Tests

```bash
pnpm vitest run src/hooks/assets/__tests__/integration.test.ts
```

**Expected**: 8+ tests passing

---

### Step 5: Full Validation

```bash
pnpm validate
```

**Expected**: All checks pass

---

## Success Criteria Checklist

### Code Quality
- [ ] All TypeScript types defined with 0 errors
- [ ] ESLint passes with 0 new warnings
- [ ] All functions documented with JSDoc
- [ ] Consistent naming conventions
- [ ] No console.log statements (use proper error handling)

### Functionality
- [ ] Cache-first strategy works (no unnecessary API calls)
- [ ] Stale cache detection (>1 hour) accurate
- [ ] All CRUD operations update cache immediately
- [ ] Bulk upload polling works (every 2 seconds)
- [ ] Polling cleanup on unmount verified
- [ ] Error states don't corrupt cache
- [ ] Loading states accurate per operation

### Tests
- [ ] 20+ tests: cache-integration.test.ts
- [ ] 10+ tests: useAssets.test.ts
- [ ] 8+ tests: useAsset.test.ts
- [ ] 12+ tests: useAssetMutations.test.ts
- [ ] 8+ tests: useBulkUpload.test.ts
- [ ] 8+ tests: integration.test.ts
- [ ] Total: 66+ tests passing
- [ ] Test coverage >80% on new code

### Integration
- [ ] Phase 1 API client integration verified
- [ ] Phase 2 business logic integration verified
- [ ] Phase 3 store integration verified
- [ ] All hooks work together correctly
- [ ] No circular dependencies
- [ ] Clean central exports (hooks/index.ts)

### Performance
- [ ] No unnecessary re-renders (use Zustand selectors)
- [ ] useCallback/useMemo used appropriately
- [ ] Polling doesn't cause memory leaks
- [ ] Large datasets (1000+ assets) perform well

---

## Implementation Timeline

**Day 1 (6-8 hours)**:
- Task 1: cache-integration.ts + tests (3 hours)
- Task 2: useAssets.ts + tests (3 hours)

**Day 2 (6-8 hours)**:
- Task 3: useAsset.ts + tests (2 hours)
- Task 4: useAssetMutations.ts + tests (4 hours)

**Day 3 (5-7 hours)**:
- Task 5: useBulkUpload.ts + tests (4 hours)
- Task 6: Integration tests (2 hours)
- Task 7: Central exports + validation (1 hour)

**Total: 17-23 hours** (2-3 focused days)

---

## References

- **Phase 1**: Types & API Client
  - `types/assets/index.ts`
  - `lib/api/assets/index.ts`

- **Phase 2**: Business Logic
  - `lib/asset/validators.ts`
  - `lib/asset/filters.ts`

- **Phase 3**: Zustand Store
  - `stores/assets/assetStore.ts`
  - `stores/assets/assetActions.ts`

- **Architecture**:
  - `spec/active/assets-frontend-logic/architecture.md`
  - `spec/active/assets-frontend-logic/caching-strategy.md`

- **External Docs**:
  - React Hooks: https://react.dev/reference/react/hooks
  - Zustand: https://zustand-demo.pmnd.rs/
  - React Testing Library: https://testing-library.com/react

---

**Status**: Ready for Implementation
**Branch**: `feature/assets-frontend-phase-4`
**Next Phase**: Phase 5 - UI Components (future)
