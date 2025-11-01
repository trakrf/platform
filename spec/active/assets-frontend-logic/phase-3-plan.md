# Phase 3 Implementation Plan: Zustand Asset Store

## Overview

**File to Create**: `frontend/src/stores/assetStore.ts`
**Lines**: ~400-450
**Time Estimate**: 8-12 hours
**Complexity**: 6/10

---

## Task Breakdown

### Task 1: Setup Store Structure and Initial State
**Time**: 1 hour
**Action**: CREATE `frontend/src/stores/assetStore.ts`

#### Implementation

```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type {
  Asset,
  AssetCache,
  AssetFilters,
  AssetType,
  PaginationState,
  SortState,
} from '@/types/asset';
import {
  filterAssets,
  sortAssets,
  searchAssets,
  paginateAssets,
} from '@/lib/asset/filters';
import { serializeCache, deserializeCache } from '@/lib/asset/transforms';

/**
 * Asset store state interface
 */
interface AssetStore {
  // ============ Cache State ============
  cache: AssetCache;

  // ============ UI State ============
  selectedAssetId: number | null;
  filters: AssetFilters;
  pagination: PaginationState;
  sort: SortState;

  // ============ Bulk Upload State ============
  uploadJobId: string | null;
  pollingIntervalId: NodeJS.Timeout | null;

  // ============ Cache Actions ============
  addAssets: (assets: Asset[]) => void;
  addAsset: (asset: Asset) => void;
  updateCachedAsset: (id: number, updates: Partial<Asset>) => void;
  removeAsset: (id: number) => void;
  invalidateCache: () => void;

  // ============ Cache Queries ============
  getAssetById: (id: number) => Asset | undefined;
  getAssetByIdentifier: (identifier: string) => Asset | undefined;
  getAssetsByType: (type: AssetType) => Asset[];
  getActiveAssets: () => Asset[];
  getFilteredAssets: () => Asset[];
  getPaginatedAssets: () => Asset[];

  // ============ UI State Actions ============
  setFilters: (filters: Partial<AssetFilters>) => void;
  setPage: (page: number) => void;
  setPageSize: (size: number) => void;
  setSort: (field: keyof Asset, direction: 'asc' | 'desc') => void;
  setSearchTerm: (term: string) => void;
  resetPagination: () => void;
  selectAsset: (id: number | null) => void;
  getSelectedAsset: () => Asset | undefined;

  // ============ Bulk Upload Actions ============
  setUploadJobId: (jobId: string | null) => void;
  setPollingInterval: (intervalId: NodeJS.Timeout | null) => void;
  clearUploadState: () => void;
}

/**
 * Initial cache state
 */
const initialCache: AssetCache = {
  byId: new Map(),
  byIdentifier: new Map(),
  byType: new Map(),
  activeIds: new Set(),
  allIds: [],
  lastFetched: 0,
  ttl: 5 * 60 * 1000, // 5 minutes
};

/**
 * Initial filters state
 */
const initialFilters: AssetFilters = {
  type: 'all',
  is_active: 'all',
  searchTerm: '',
};

/**
 * Initial pagination state
 */
const initialPagination: PaginationState = {
  currentPage: 1,
  pageSize: 25,
  totalCount: 0,
  totalPages: 0,
};

/**
 * Initial sort state
 */
const initialSort: SortState = {
  field: 'created_at',
  direction: 'desc',
};

// Store implementation will go here
```

**Validation**:
```bash
cd frontend && pnpm typecheck
```

---

### Task 2: Implement Cache Actions
**Time**: 2-3 hours
**Action**: Add cache manipulation methods

#### Implementation

```typescript
export const useAssetStore = create<AssetStore>()((set, get) => ({
  // ============ Initial State ============
  cache: initialCache,
  selectedAssetId: null,
  filters: initialFilters,
  pagination: initialPagination,
  sort: initialSort,
  uploadJobId: null,
  pollingIntervalId: null,

  // ============ Cache Actions ============

  /**
   * Add multiple assets to cache (bulk operation)
   */
  addAssets: (assets) =>
    set((state) => {
      const newCache = { ...state.cache };

      // Clone Maps and Sets for immutability
      newCache.byId = new Map(state.cache.byId);
      newCache.byIdentifier = new Map(state.cache.byIdentifier);
      newCache.byType = new Map(state.cache.byType);
      newCache.activeIds = new Set(state.cache.activeIds);
      newCache.allIds = [...state.cache.allIds];

      assets.forEach((asset) => {
        // Update byId
        newCache.byId.set(asset.id, asset);

        // Update byIdentifier
        newCache.byIdentifier.set(asset.identifier, asset);

        // Update byType
        const typeSet = newCache.byType.get(asset.type) ?? new Set();
        const newTypeSet = new Set(typeSet);
        newTypeSet.add(asset.id);
        newCache.byType.set(asset.type, newTypeSet);

        // Update activeIds
        if (asset.is_active) {
          newCache.activeIds.add(asset.id);
        }

        // Update allIds (if not present)
        if (!newCache.allIds.includes(asset.id)) {
          newCache.allIds.push(asset.id);
        }
      });

      newCache.lastFetched = Date.now();

      return { cache: newCache };
    }),

  /**
   * Add single asset to cache
   */
  addAsset: (asset) => {
    get().addAssets([asset]);
  },

  /**
   * Update asset in cache
   * Handles type changes and active status changes
   */
  updateCachedAsset: (id, updates) =>
    set((state) => {
      const current = state.cache.byId.get(id);
      if (!current) {
        console.warn(`[AssetStore] Asset ${id} not found in cache`);
        return state;
      }

      const updated = { ...current, ...updates };
      const newCache = { ...state.cache };

      // Clone Maps and Sets
      newCache.byId = new Map(state.cache.byId);
      newCache.byIdentifier = new Map(state.cache.byIdentifier);
      newCache.byType = new Map(state.cache.byType);
      newCache.activeIds = new Set(state.cache.activeIds);
      newCache.allIds = [...state.cache.allIds];

      // Update byId
      newCache.byId.set(id, updated);

      // Handle identifier change
      if (updates.identifier && updates.identifier !== current.identifier) {
        newCache.byIdentifier.delete(current.identifier);
        newCache.byIdentifier.set(updates.identifier, updated);
      } else {
        newCache.byIdentifier.set(current.identifier, updated);
      }

      // Handle type change
      if (updates.type && updates.type !== current.type) {
        // Remove from old type
        const oldTypeSet = newCache.byType.get(current.type);
        if (oldTypeSet) {
          const newOldTypeSet = new Set(oldTypeSet);
          newOldTypeSet.delete(id);
          if (newOldTypeSet.size === 0) {
            newCache.byType.delete(current.type);
          } else {
            newCache.byType.set(current.type, newOldTypeSet);
          }
        }

        // Add to new type
        const newTypeSet = newCache.byType.get(updates.type) ?? new Set();
        const updatedNewTypeSet = new Set(newTypeSet);
        updatedNewTypeSet.add(id);
        newCache.byType.set(updates.type, updatedNewTypeSet);
      }

      // Handle active status change
      if (updates.is_active !== undefined) {
        if (updates.is_active) {
          newCache.activeIds.add(id);
        } else {
          newCache.activeIds.delete(id);
        }
      }

      return { cache: newCache };
    }),

  /**
   * Remove asset from all indexes
   */
  removeAsset: (id) =>
    set((state) => {
      const asset = state.cache.byId.get(id);
      if (!asset) {
        console.warn(`[AssetStore] Asset ${id} not found in cache`);
        return state;
      }

      const newCache = { ...state.cache };

      // Clone Maps and Sets
      newCache.byId = new Map(state.cache.byId);
      newCache.byIdentifier = new Map(state.cache.byIdentifier);
      newCache.byType = new Map(state.cache.byType);
      newCache.activeIds = new Set(state.cache.activeIds);

      // Remove from byId
      newCache.byId.delete(id);

      // Remove from byIdentifier
      newCache.byIdentifier.delete(asset.identifier);

      // Remove from byType
      const typeSet = newCache.byType.get(asset.type);
      if (typeSet) {
        const newTypeSet = new Set(typeSet);
        newTypeSet.delete(id);
        if (newTypeSet.size === 0) {
          newCache.byType.delete(asset.type);
        } else {
          newCache.byType.set(asset.type, newTypeSet);
        }
      }

      // Remove from activeIds
      newCache.activeIds.delete(id);

      // Remove from allIds
      newCache.allIds = state.cache.allIds.filter((aid) => aid !== id);

      return { cache: newCache };
    }),

  /**
   * Clear all cached data
   */
  invalidateCache: () =>
    set({
      cache: initialCache,
    }),

  // ... (other methods continue in next tasks)
}));
```

**Validation**:
```bash
cd frontend && pnpm typecheck
cd frontend && pnpm lint
```

---

### Task 3: Implement Cache Query Methods
**Time**: 1-2 hours
**Action**: Add cache accessor methods

#### Implementation

```typescript
  // ============ Cache Queries ============

  /**
   * Get asset by ID (O(1) lookup)
   */
  getAssetById: (id) => {
    return get().cache.byId.get(id);
  },

  /**
   * Get asset by identifier (O(1) lookup)
   */
  getAssetByIdentifier: (identifier) => {
    return get().cache.byIdentifier.get(identifier);
  },

  /**
   * Get all assets of a specific type
   */
  getAssetsByType: (type) => {
    const ids = get().cache.byType.get(type) ?? new Set();
    const { cache } = get();
    return Array.from(ids)
      .map((id) => cache.byId.get(id))
      .filter((asset): asset is Asset => asset !== undefined);
  },

  /**
   * Get all active assets
   */
  getActiveAssets: () => {
    const ids = get().cache.activeIds;
    const { cache } = get();
    return Array.from(ids)
      .map((id) => cache.byId.get(id))
      .filter((asset): asset is Asset => asset !== undefined);
  },

  /**
   * Get filtered and sorted assets
   * Applies filters, search, and sort from Phase 2 functions
   */
  getFilteredAssets: () => {
    const { cache, filters, sort } = get();
    let assets = Array.from(cache.byId.values());

    // Apply filters (type and is_active)
    assets = filterAssets(assets, filters);

    // Apply search
    if (filters.searchTerm) {
      assets = searchAssets(assets, filters.searchTerm);
    }

    // Apply sort
    assets = sortAssets(assets, sort);

    return assets;
  },

  /**
   * Get paginated assets
   * Applies pagination to filtered results
   */
  getPaginatedAssets: () => {
    const filtered = get().getFilteredAssets();
    const { pagination } = get();

    // Update total count
    const totalCount = filtered.length;
    const totalPages = Math.ceil(totalCount / pagination.pageSize);

    // Update pagination state if needed
    if (
      pagination.totalCount !== totalCount ||
      pagination.totalPages !== totalPages
    ) {
      set((state) => ({
        pagination: {
          ...state.pagination,
          totalCount,
          totalPages,
        },
      }));
    }

    return paginateAssets(filtered, {
      ...pagination,
      totalCount,
      totalPages,
    });
  },
```

**Validation**:
```bash
cd frontend && pnpm typecheck
```

---

### Task 4: Implement UI State Actions
**Time**: 1 hour
**Action**: Add filter, pagination, sort, and selection methods

#### Implementation

```typescript
  // ============ UI State Actions ============

  /**
   * Update filters (partial update)
   */
  setFilters: (newFilters) =>
    set((state) => ({
      filters: { ...state.filters, ...newFilters },
      pagination: { ...state.pagination, currentPage: 1 }, // Reset to page 1
    })),

  /**
   * Set current page number
   */
  setPage: (page) =>
    set((state) => ({
      pagination: { ...state.pagination, currentPage: page },
    })),

  /**
   * Set page size (resets to page 1)
   */
  setPageSize: (size) =>
    set((state) => ({
      pagination: { ...state.pagination, pageSize: size, currentPage: 1 },
    })),

  /**
   * Update sort field and direction
   */
  setSort: (field, direction) =>
    set({
      sort: { field, direction },
    }),

  /**
   * Update search term in filters
   */
  setSearchTerm: (term) =>
    set((state) => ({
      filters: { ...state.filters, searchTerm: term },
      pagination: { ...state.pagination, currentPage: 1 }, // Reset to page 1
    })),

  /**
   * Reset pagination to page 1
   */
  resetPagination: () =>
    set((state) => ({
      pagination: { ...state.pagination, currentPage: 1 },
    })),

  /**
   * Select asset by ID
   */
  selectAsset: (id) =>
    set({
      selectedAssetId: id,
    }),

  /**
   * Get currently selected asset from cache
   */
  getSelectedAsset: () => {
    const { selectedAssetId, cache } = get();
    return selectedAssetId ? cache.byId.get(selectedAssetId) : undefined;
  },

  // ============ Bulk Upload Actions ============

  /**
   * Set bulk upload job ID
   */
  setUploadJobId: (jobId) =>
    set({
      uploadJobId: jobId,
    }),

  /**
   * Set polling interval ID for cleanup
   */
  setPollingInterval: (intervalId) =>
    set({
      pollingIntervalId: intervalId,
    }),

  /**
   * Clear bulk upload state
   */
  clearUploadState: () =>
    set({
      uploadJobId: null,
      pollingIntervalId: null,
    }),
```

**Validation**:
```bash
cd frontend && pnpm typecheck
cd frontend && pnpm lint
```

---

### Task 5: Add LocalStorage Persistence
**Time**: 2-3 hours
**Action**: Wrap store with persist middleware

#### Implementation

```typescript
/**
 * Asset management store with LocalStorage persistence
 *
 * Cache persists for 5 minutes (TTL)
 * UI state (filters, pagination, sort) persists across sessions
 */
export const useAssetStore = create<AssetStore>()(
  persist(
    (set, get) => ({
      // All state and actions from Tasks 2-4 go here
      // ... (see above implementations)
    }),
    {
      name: 'asset-store',

      // Only persist cache and UI state
      partialize: (state) => ({
        cache: state.cache,
        filters: state.filters,
        pagination: state.pagination,
        sort: state.sort,
      }),

      // Custom storage with Map/Set serialization
      storage: {
        getItem: (name) => {
          const str = localStorage.getItem(name);
          if (!str) return null;

          try {
            const parsed = JSON.parse(str);

            // Deserialize cache if present
            if (parsed.state?.cache) {
              // Serialize cache object to string for deserializeCache
              const cacheObj = parsed.state.cache;
              const cacheStr = JSON.stringify(cacheObj);
              const deserializedCache = deserializeCache(cacheStr);

              if (deserializedCache) {
                // Check TTL
                const now = Date.now();
                const age = now - deserializedCache.lastFetched;

                if (age < deserializedCache.ttl) {
                  // Cache is fresh
                  parsed.state.cache = deserializedCache;
                } else {
                  // Cache expired - use initial empty cache
                  console.info('[AssetStore] Cache expired, using empty cache');
                  parsed.state.cache = initialCache;
                }
              } else {
                // Deserialization failed - use empty cache
                console.warn('[AssetStore] Cache deserialization failed');
                parsed.state.cache = initialCache;
              }
            }

            return parsed;
          } catch (error) {
            console.error('[AssetStore] Failed to load from localStorage:', error);
            return null;
          }
        },

        setItem: (name, value) => {
          try {
            // Serialize cache using Phase 2 function
            const cacheStr = serializeCache(value.state.cache);
            const cacheObj = JSON.parse(cacheStr);

            const serialized = {
              ...value,
              state: {
                ...value.state,
                cache: cacheObj,
              },
            };

            localStorage.setItem(name, JSON.stringify(serialized));
          } catch (error) {
            console.error('[AssetStore] Failed to save to localStorage:', error);
          }
        },

        removeItem: (name) => {
          localStorage.removeItem(name);
        },
      },
    }
  )
);
```

**Validation**:
```bash
cd frontend && pnpm typecheck
cd frontend && pnpm lint
```

---

### Task 6: Write Store Tests
**Time**: 3-4 hours
**Action**: CREATE `frontend/src/stores/assetStore.test.ts`

#### Test Structure

```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { useAssetStore } from './assetStore';
import type { Asset } from '@/types/asset';

describe('AssetStore - Cache Operations', () => {
  beforeEach(() => {
    useAssetStore.setState({
      cache: {
        byId: new Map(),
        byIdentifier: new Map(),
        byType: new Map(),
        activeIds: new Set(),
        allIds: [],
        lastFetched: 0,
        ttl: 5 * 60 * 1000,
      },
    });
  });

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    identifier: 'TEST-001',
    name: 'Test Asset',
    type: 'device',
    description: 'Test',
    valid_from: '2024-01-01',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  describe('addAsset()', () => {
    it('should add asset to all indexes', () => {
      useAssetStore.getState().addAsset(mockAsset);

      const state = useAssetStore.getState();
      expect(state.cache.byId.get(1)).toEqual(mockAsset);
      expect(state.cache.byIdentifier.get('TEST-001')).toEqual(mockAsset);
      expect(state.cache.byType.get('device')).toContain(1);
      expect(state.cache.activeIds.has(1)).toBe(true);
      expect(state.cache.allIds).toContain(1);
    });

    it('should update lastFetched timestamp', () => {
      const before = Date.now();
      useAssetStore.getState().addAsset(mockAsset);
      const after = Date.now();

      const lastFetched = useAssetStore.getState().cache.lastFetched;
      expect(lastFetched).toBeGreaterThanOrEqual(before);
      expect(lastFetched).toBeLessThanOrEqual(after);
    });
  });

  describe('addAssets()', () => {
    it('should add multiple assets in bulk', () => {
      const assets = [mockAsset, { ...mockAsset, id: 2, identifier: 'TEST-002' }];
      useAssetStore.getState().addAssets(assets);

      const state = useAssetStore.getState();
      expect(state.cache.byId.size).toBe(2);
      expect(state.cache.allIds).toHaveLength(2);
    });
  });

  describe('updateCachedAsset()', () => {
    beforeEach(() => {
      useAssetStore.getState().addAsset(mockAsset);
    });

    it('should update asset in cache', () => {
      useAssetStore.getState().updateCachedAsset(1, { name: 'Updated Name' });

      const updated = useAssetStore.getState().cache.byId.get(1);
      expect(updated?.name).toBe('Updated Name');
    });

    it('should handle type change', () => {
      useAssetStore.getState().updateCachedAsset(1, { type: 'person' });

      const state = useAssetStore.getState();
      expect(state.cache.byType.get('device')?.has(1)).toBe(false);
      expect(state.cache.byType.get('person')?.has(1)).toBe(true);
    });

    it('should handle active status change', () => {
      useAssetStore.getState().updateCachedAsset(1, { is_active: false });

      const state = useAssetStore.getState();
      expect(state.cache.activeIds.has(1)).toBe(false);
    });

    it('should handle identifier change', () => {
      useAssetStore.getState().updateCachedAsset(1, { identifier: 'NEW-001' });

      const state = useAssetStore.getState();
      expect(state.cache.byIdentifier.has('TEST-001')).toBe(false);
      expect(state.cache.byIdentifier.has('NEW-001')).toBe(true);
    });
  });

  describe('removeAsset()', () => {
    beforeEach(() => {
      useAssetStore.getState().addAsset(mockAsset);
    });

    it('should remove asset from all indexes', () => {
      useAssetStore.getState().removeAsset(1);

      const state = useAssetStore.getState();
      expect(state.cache.byId.has(1)).toBe(false);
      expect(state.cache.byIdentifier.has('TEST-001')).toBe(false);
      expect(state.cache.byType.get('device')?.has(1)).toBe(false);
      expect(state.cache.activeIds.has(1)).toBe(false);
      expect(state.cache.allIds).not.toContain(1);
    });
  });

  describe('invalidateCache()', () => {
    beforeEach(() => {
      useAssetStore.getState().addAsset(mockAsset);
    });

    it('should clear all cache data', () => {
      useAssetStore.getState().invalidateCache();

      const state = useAssetStore.getState();
      expect(state.cache.byId.size).toBe(0);
      expect(state.cache.byIdentifier.size).toBe(0);
      expect(state.cache.byType.size).toBe(0);
      expect(state.cache.activeIds.size).toBe(0);
      expect(state.cache.allIds).toHaveLength(0);
    });
  });
});

describe('AssetStore - Cache Queries', () => {
  beforeEach(() => {
    const mockAsset: Asset = {
      id: 1,
      org_id: 1,
      identifier: 'TEST-001',
      name: 'Test Asset',
      type: 'device',
      description: 'Test',
      valid_from: '2024-01-01',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    };
    useAssetStore.getState().addAsset(mockAsset);
  });

  describe('getAssetById()', () => {
    it('should return asset by ID', () => {
      const asset = useAssetStore.getState().getAssetById(1);
      expect(asset?.identifier).toBe('TEST-001');
    });

    it('should return undefined for non-existent ID', () => {
      const asset = useAssetStore.getState().getAssetById(999);
      expect(asset).toBeUndefined();
    });
  });

  describe('getAssetByIdentifier()', () => {
    it('should return asset by identifier', () => {
      const asset = useAssetStore.getState().getAssetByIdentifier('TEST-001');
      expect(asset?.id).toBe(1);
    });

    it('should return undefined for non-existent identifier', () => {
      const asset = useAssetStore.getState().getAssetByIdentifier('INVALID');
      expect(asset).toBeUndefined();
    });
  });

  describe('getAssetsByType()', () => {
    it('should return all assets of a type', () => {
      const devices = useAssetStore.getState().getAssetsByType('device');
      expect(devices).toHaveLength(1);
      expect(devices[0].type).toBe('device');
    });

    it('should return empty array for type with no assets', () => {
      const people = useAssetStore.getState().getAssetsByType('person');
      expect(people).toHaveLength(0);
    });
  });

  describe('getActiveAssets()', () => {
    it('should return only active assets', () => {
      const active = useAssetStore.getState().getActiveAssets();
      expect(active).toHaveLength(1);
      expect(active.every((a) => a.is_active)).toBe(true);
    });
  });
});

describe('AssetStore - UI State', () => {
  describe('setFilters()', () => {
    it('should update filters partially', () => {
      useAssetStore.getState().setFilters({ type: 'device' });
      expect(useAssetStore.getState().filters.type).toBe('device');
    });

    it('should reset pagination to page 1', () => {
      useAssetStore.setState({ pagination: { ...useAssetStore.getState().pagination, currentPage: 3 } });
      useAssetStore.getState().setFilters({ type: 'device' });
      expect(useAssetStore.getState().pagination.currentPage).toBe(1);
    });
  });

  describe('setPage()', () => {
    it('should update current page', () => {
      useAssetStore.getState().setPage(2);
      expect(useAssetStore.getState().pagination.currentPage).toBe(2);
    });
  });

  describe('setPageSize()', () => {
    it('should update page size and reset to page 1', () => {
      useAssetStore.setState({ pagination: { ...useAssetStore.getState().pagination, currentPage: 3 } });
      useAssetStore.getState().setPageSize(50);
      expect(useAssetStore.getState().pagination.pageSize).toBe(50);
      expect(useAssetStore.getState().pagination.currentPage).toBe(1);
    });
  });

  describe('selectAsset()', () => {
    it('should set selected asset ID', () => {
      useAssetStore.getState().selectAsset(1);
      expect(useAssetStore.getState().selectedAssetId).toBe(1);
    });

    it('should clear selection when null', () => {
      useAssetStore.getState().selectAsset(1);
      useAssetStore.getState().selectAsset(null);
      expect(useAssetStore.getState().selectedAssetId).toBeNull();
    });
  });
});

describe('AssetStore - LocalStorage Persistence', () => {
  it('should persist cache to localStorage', () => {
    // Test serialization (requires actual localStorage)
  });

  it('should load cache from localStorage', () => {
    // Test deserialization
  });

  it('should respect cache TTL', () => {
    // Test expired cache not loaded
  });
});
```

**Validation**:
```bash
cd frontend && pnpm test src/stores/assetStore.test.ts
```

**Expected**: 20+ tests passing

---

### Task 7: Final Validation and Documentation
**Time**: 1 hour
**Action**: Run full validation suite and add JSDoc

#### Validation Checklist

```bash
# TypeScript validation
cd frontend && pnpm typecheck

# Lint validation
cd frontend && pnpm lint

# Run all store tests
cd frontend && pnpm test src/stores/assetStore.test.ts

# Run full test suite
cd frontend && pnpm test
```

#### Documentation

Add JSDoc comments to all exported functions:

```typescript
/**
 * Asset management store
 *
 * Provides:
 * - Multi-index cache with O(1) lookups (byId, byIdentifier, byType)
 * - UI state management (filters, pagination, sort, selection)
 * - LocalStorage persistence with 5-minute TTL
 * - Bulk upload job tracking
 *
 * @example
 * ```typescript
 * // Get asset by ID
 * const asset = useAssetStore.getState().getAssetById(1);
 *
 * // Add assets to cache
 * useAssetStore.getState().addAssets(assets);
 *
 * // Get filtered and paginated assets
 * const filtered = useAssetStore.getState().getFilteredAssets();
 * const paginated = useAssetStore.getState().getPaginatedAssets();
 * ```
 */
export const useAssetStore = create<AssetStore>()(/* ... */);
```

---

## Success Criteria

- [ ] TypeScript: 0 errors
- [ ] Lint: 0 issues
- [ ] Tests: 20+ tests passing
- [ ] Cache operations maintain index consistency
- [ ] O(1) lookups for byId and byIdentifier
- [ ] Type changes update byType index correctly
- [ ] Active status changes update activeIds correctly
- [ ] LocalStorage persistence works
- [ ] Cache TTL respected
- [ ] All functions have JSDoc comments

---

## Final File Structure

```
frontend/src/
├── stores/
│   ├── assetStore.ts          # ~400 lines
│   └── assetStore.test.ts     # ~350 lines
```

---

## Dependencies

**From Phase 1**:
- `types/asset.ts` - All type definitions

**From Phase 2**:
- `lib/asset/filters.ts` - filterAssets, sortAssets, searchAssets, paginateAssets
- `lib/asset/transforms.ts` - serializeCache, deserializeCache

**External**:
- `zustand` - State management
- `zustand/middleware` - Persist middleware

---

## Notes

- No API calls in this phase (pure state management)
- API integration happens in Phase 4
- Follow pattern from `stores/authStore.ts`
- Use proper TypeScript types throughout
- Maintain immutability in all state updates
