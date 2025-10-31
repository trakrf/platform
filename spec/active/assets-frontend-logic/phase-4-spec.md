# Phase 4: Asset Management API Integration & React Hooks

## Metadata
**Phase**: 4 of 4
**Depends On**: Phase 1 (Types & API Client), Phase 2 (Business Logic), Phase 3 (Zustand Store)
**Complexity**: 7/10
**Estimated Time**: 8-12 hours

## Outcome
Complete API integration layer with custom React hooks, enabling React components to perform full CRUD operations, bulk uploads, and real-time data synchronization with automatic cache updates.

---

## Overview

**What We're Building**: React hooks that bridge the gap between UI components and the backend API:
- Connect Phase 1 API client with Phase 3 Zustand store
- Implement smart caching with automatic cache updates
- Create reusable React hooks for components
- Add error handling and loading states
- Support optimistic updates where appropriate

**Note**: This phase focuses on **DATA INTEGRATION** only. UI components are a separate phase.

---

## Files to Create

```
frontend/src/
├── hooks/
│   └── assets/
│       ├── useAssets.ts           # List/filter/paginate assets
│       ├── useAsset.ts            # Single asset by ID
│       ├── useAssetMutations.ts   # Create/update/delete
│       └── useBulkUpload.ts       # CSV upload with polling
└── lib/
    └── asset/
        └── cache-integration.ts   # API → Cache helpers
```

---

## Implementation Requirements

### 1. Cache Integration Helpers (`lib/asset/cache-integration.ts`)

**Purpose**: Helper functions that update the Zustand cache after API operations.

```typescript
import { useAssetStore } from '@/stores/assetStore';
import type { Asset } from '@/types/asset';

/**
 * Update cache after asset creation
 * Immediately adds to cache - no refetch needed
 */
export function cacheCreatedAsset(asset: Asset): void {
  useAssetStore.getState().addAsset(asset);
}

/**
 * Update cache after asset update
 * Uses server response as source of truth
 */
export function cacheUpdatedAsset(id: number, asset: Asset): void {
  useAssetStore.getState().updateCachedAsset(id, asset);
}

/**
 * Update cache after asset deletion
 * Immediately removes from all indexes
 */
export function cacheDeletedAsset(id: number): void {
  useAssetStore.getState().removeAsset(id);
}

/**
 * Populate cache after list fetch
 * Sets lastFetched timestamp automatically
 */
export function cacheAssetList(assets: Asset[]): void {
  useAssetStore.getState().addAssets(assets);
}

/**
 * Check if cache is stale (>1 hour old)
 */
export function isCacheStale(): boolean {
  const { cache } = useAssetStore.getState();
  const now = Date.now();
  return now - cache.lastFetched > cache.ttl;
}

/**
 * Invalidate cache after bulk upload
 * Forces refetch on next read
 */
export function invalidateAfterBulkUpload(): void {
  useAssetStore.getState().invalidateCache();
}
```

---

### 2. List/Filter Hook (`hooks/assets/useAssets.ts`)

**Purpose**: Fetch and filter asset lists with automatic caching.

```typescript
import { useEffect, useState } from 'react';
import { useAssetStore } from '@/stores/assetStore';
import { assetsApi } from '@/lib/api/assets';
import { cacheAssetList, isCacheStale } from '@/lib/asset/cache-integration';
import type { Asset, AssetFilters } from '@/types/asset';

interface UseAssetsOptions {
  filters?: Partial<AssetFilters>;
  autoFetch?: boolean; // Default: true
}

interface UseAssetsResult {
  assets: Asset[];
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
  totalCount: number;
}

/**
 * Hook for fetching and filtering asset lists
 *
 * Features:
 * - Automatic cache population
 * - Smart refetching (only if cache is stale)
 * - Client-side filtering from cache
 * - Loading and error states
 *
 * @example
 * const { assets, loading, error, refetch } = useAssets({
 *   filters: { type: 'device', is_active: true }
 * });
 */
export function useAssets(options: UseAssetsOptions = {}): UseAssetsResult {
  const { filters = {}, autoFetch = true } = options;

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Subscribe to filtered assets from cache
  const filteredAssets = useAssetStore((state) => state.getFilteredAssets());
  const totalCount = useAssetStore((state) => state.pagination.totalCount);

  // Apply filters to store
  useEffect(() => {
    if (Object.keys(filters).length > 0) {
      useAssetStore.getState().setFilters(filters);
    }
  }, [filters]);

  const fetchAssets = async () => {
    try {
      setLoading(true);
      setError(null);

      const response = await assetsApi.list();
      cacheAssetList(response.data);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  // Auto-fetch on mount if cache is empty or stale
  useEffect(() => {
    if (!autoFetch) return;

    const { cache } = useAssetStore.getState();
    if (cache.byId.size === 0 || isCacheStale()) {
      fetchAssets();
    }
  }, [autoFetch]);

  return {
    assets: filteredAssets,
    loading,
    error,
    refetch: fetchAssets,
    totalCount,
  };
}
```

---

### 3. Single Asset Hook (`hooks/assets/useAsset.ts`)

**Purpose**: Fetch a single asset by ID with cache-first strategy.

```typescript
import { useEffect, useState } from 'react';
import { useAssetStore } from '@/stores/assetStore';
import { assetsApi } from '@/lib/api/assets';
import { cacheCreatedAsset } from '@/lib/asset/cache-integration';
import type { Asset } from '@/types/asset';

interface UseAssetResult {
  asset: Asset | undefined;
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

/**
 * Hook for fetching a single asset by ID
 *
 * Features:
 * - Cache-first strategy (no API call if cached)
 * - Automatic cache population on fetch
 * - Loading and error states
 *
 * @example
 * const { asset, loading, error } = useAsset(123);
 */
export function useAsset(id: number | null): UseAssetResult {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Subscribe to asset from cache
  const asset = useAssetStore((state) =>
    id ? state.getAssetById(id) : undefined
  );

  const fetchAsset = async () => {
    if (!id) return;

    try {
      setLoading(true);
      setError(null);

      const response = await assetsApi.get(id);
      cacheCreatedAsset(response.data);
    } catch (err) {
      setError(err as Error);
    } finally {
      setLoading(false);
    }
  };

  // Fetch if not in cache
  useEffect(() => {
    if (id && !asset) {
      fetchAsset();
    }
  }, [id, asset]);

  return {
    asset,
    loading,
    error,
    refetch: fetchAsset,
  };
}
```

---

### 4. Mutation Hook (`hooks/assets/useAssetMutations.ts`)

**Purpose**: CRUD operations with automatic cache updates.

```typescript
import { useState } from 'react';
import { assetsApi } from '@/lib/api/assets';
import {
  cacheCreatedAsset,
  cacheUpdatedAsset,
  cacheDeletedAsset,
} from '@/lib/asset/cache-integration';
import type { CreateAssetRequest, UpdateAssetRequest, Asset } from '@/types/asset';

interface UseAssetMutationsResult {
  creating: boolean;
  updating: boolean;
  deleting: boolean;
  error: Error | null;
  createAsset: (data: CreateAssetRequest) => Promise<Asset>;
  updateAsset: (id: number, data: UpdateAssetRequest) => Promise<Asset>;
  deleteAsset: (id: number) => Promise<void>;
}

/**
 * Hook for asset CRUD mutations
 *
 * Features:
 * - Automatic cache updates on success
 * - Server response is source of truth
 * - Loading states per operation
 * - Error handling
 *
 * @example
 * const { createAsset, creating, error } = useAssetMutations();
 *
 * const handleCreate = async () => {
 *   const newAsset = await createAsset({
 *     identifier: 'LAP-001',
 *     name: 'Laptop',
 *     type: 'device',
 *     // ...
 *   });
 * };
 */
export function useAssetMutations(): UseAssetMutationsResult {
  const [creating, setCreating] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createAsset = async (data: CreateAssetRequest): Promise<Asset> => {
    try {
      setCreating(true);
      setError(null);

      const response = await assetsApi.create(data);

      // CRITICAL: Add to cache immediately
      cacheCreatedAsset(response.data);

      return response.data;
    } catch (err) {
      setError(err as Error);
      throw err;
    } finally {
      setCreating(false);
    }
  };

  const updateAsset = async (
    id: number,
    data: UpdateAssetRequest
  ): Promise<Asset> => {
    try {
      setUpdating(true);
      setError(null);

      const response = await assetsApi.update(id, data);

      // CRITICAL: Update cache with server response
      cacheUpdatedAsset(id, response.data);

      return response.data;
    } catch (err) {
      setError(err as Error);
      throw err;
    } finally {
      setUpdating(false);
    }
  };

  const deleteAsset = async (id: number): Promise<void> => {
    try {
      setDeleting(true);
      setError(null);

      await assetsApi.delete(id);

      // CRITICAL: Remove from cache immediately
      cacheDeletedAsset(id);
    } catch (err) {
      setError(err as Error);
      throw err;
    } finally {
      setDeleting(false);
    }
  };

  return {
    creating,
    updating,
    deleting,
    error,
    createAsset,
    updateAsset,
    deleteAsset,
  };
}
```

---

### 5. Bulk Upload Hook (`hooks/assets/useBulkUpload.ts`)

**Purpose**: CSV upload with job status polling and cache invalidation.

```typescript
import { useState, useEffect, useRef } from 'react';
import { assetsApi } from '@/lib/api/assets';
import { invalidateAfterBulkUpload } from '@/lib/asset/cache-integration';
import { useAssetStore } from '@/stores/assetStore';
import type { JobStatusResponse } from '@/types/asset';

interface UseBulkUploadResult {
  uploading: boolean;
  polling: boolean;
  jobStatus: JobStatusResponse | null;
  error: Error | null;
  uploadCSV: (file: File) => Promise<void>;
  cancelPolling: () => void;
}

/**
 * Hook for CSV bulk upload with automatic job status polling
 *
 * Features:
 * - CSV upload with validation
 * - Automatic job status polling (every 2 seconds)
 * - Cache invalidation on completion
 * - Cleanup on unmount
 *
 * @example
 * const { uploadCSV, uploading, jobStatus, error } = useBulkUpload();
 *
 * const handleUpload = async (file: File) => {
 *   await uploadCSV(file);
 *   // Polling starts automatically
 * };
 */
export function useBulkUpload(): UseBulkUploadResult {
  const [uploading, setUploading] = useState(false);
  const [polling, setPolling] = useState(false);
  const [jobStatus, setJobStatus] = useState<JobStatusResponse | null>(null);
  const [error, setError] = useState<Error | null>(null);

  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);
  const { setUploadJobId, setPollingInterval, clearUploadState } =
    useAssetStore();

  const cancelPolling = () => {
    if (pollingIntervalRef.current) {
      clearInterval(pollingIntervalRef.current);
      pollingIntervalRef.current = null;
      setPollingInterval(null);
    }
    setPolling(false);
  };

  const pollJobStatus = async (jobId: string) => {
    try {
      const status = await assetsApi.getJobStatus(jobId);
      setJobStatus(status);

      // Stop polling on completion or failure
      if (status.status === 'completed' || status.status === 'failed') {
        cancelPolling();
        setUploadJobId(null);

        // Invalidate cache on successful completion
        if (status.status === 'completed') {
          invalidateAfterBulkUpload();
        }
      }
    } catch (err) {
      setError(err as Error);
      cancelPolling();
    }
  };

  const uploadCSV = async (file: File): Promise<void> => {
    try {
      setUploading(true);
      setError(null);

      const response = await assetsApi.uploadCSV(file);
      setUploadJobId(response.job_id);

      // Start polling
      setPolling(true);
      const intervalId = setInterval(
        () => pollJobStatus(response.job_id),
        2000 // Poll every 2 seconds
      );
      pollingIntervalRef.current = intervalId;
      setPollingInterval(intervalId);
    } catch (err) {
      setError(err as Error);
      throw err;
    } finally {
      setUploading(false);
    }
  };

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cancelPolling();
      clearUploadState();
    };
  }, []);

  return {
    uploading,
    polling,
    jobStatus,
    error,
    uploadCSV,
    cancelPolling,
  };
}
```

---

## Testing Requirements

### Hook Tests

```typescript
describe('useAssets', () => {
  it('should fetch assets on mount if cache is empty', () => {});
  it('should not fetch if cache is fresh', () => {});
  it('should refetch when refetch() is called', () => {});
  it('should apply filters to cached assets', () => {});
  it('should handle API errors gracefully', () => {});
});

describe('useAsset', () => {
  it('should return cached asset if available', () => {});
  it('should fetch asset if not in cache', () => {});
  it('should handle invalid IDs', () => {});
  it('should update when cache changes', () => {});
});

describe('useAssetMutations', () => {
  it('should create asset and update cache', () => {});
  it('should update asset and update cache', () => {});
  it('should delete asset and remove from cache', () => {});
  it('should not update cache on API errors', () => {});
  it('should track loading states correctly', () => {});
});

describe('useBulkUpload', () => {
  it('should upload CSV and start polling', () => {});
  it('should stop polling on completion', () => {});
  it('should invalidate cache on success', () => {});
  it('should cleanup polling on unmount', () => {});
  it('should handle upload errors', () => {});
});

describe('cache-integration', () => {
  it('should detect stale cache (>1 hour)', () => {});
  it('should not detect fresh cache as stale', () => {});
  it('should update all indexes on create', () => {});
  it('should update all indexes on update', () => {});
  it('should remove from all indexes on delete', () => {});
});
```

---

## Success Criteria

- [ ] All hooks implemented and tested
- [ ] Cache updates after CRUD operations
- [ ] No unnecessary API calls (cache-first strategy)
- [ ] Stale cache detection (>1 hour)
- [ ] Bulk upload polling works correctly
- [ ] Cache invalidation after bulk upload
- [ ] Error states handled gracefully
- [ ] Loading states accurate
- [ ] All tests passing (target: 25+ tests)
- [ ] TypeScript: 0 errors
- [ ] Lint: 0 new issues
- [ ] Components can consume hooks without direct API calls

---

## Implementation Checklist

- [ ] Create `cache-integration.ts` with helper functions
- [ ] Implement `useAssets` hook with cache-first strategy
- [ ] Implement `useAsset` hook with single-asset fetch
- [ ] Implement `useAssetMutations` hook with CRUD operations
- [ ] Implement `useBulkUpload` hook with polling
- [ ] Write tests for all hooks
- [ ] Write tests for cache integration helpers
- [ ] Validate stale cache detection (>1 hour)
- [ ] Test bulk upload polling (mock JobStatusResponse)
- [ ] Verify cache updates after create/update/delete
- [ ] Run full validation (typecheck, lint, test)

---

## Dependencies

**Phase 1 (Types & API)**:
```typescript
import { assetsApi } from '@/lib/api/assets';
import type { Asset, CreateAssetRequest, UpdateAssetRequest } from '@/types/asset';
```

**Phase 3 (Store)**:
```typescript
import { useAssetStore } from '@/stores/assetStore';
```

**React**:
```typescript
import { useState, useEffect, useRef } from 'react';
```

---

## Notes

- **No UI components** - This phase only provides data hooks
- **Cache is source of truth** - Hooks read from cache, API updates cache
- **Server response trumps optimistic updates** - Always use API response
- **Polling cleanup critical** - Use refs and cleanup in useEffect
- **Error boundaries** - Don't cache on error
- **Loading states per operation** - creating/updating/deleting separate
- Follow React hooks best practices (dependencies, cleanup)
- Use Zustand selectors to prevent unnecessary re-renders

---

## References

- Phase 1: `lib/api/assets.ts`, `types/asset.ts`
- Phase 2: `lib/asset/validators.ts`, `lib/asset/filters.ts`
- Phase 3: `stores/assetStore.ts`
- Caching Strategy: `spec/active/assets-frontend-logic/caching-strategy.md`
- React Hooks: https://react.dev/reference/react/hooks
- Zustand: https://zustand-demo.pmnd.rs/
