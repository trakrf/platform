# Phase 3: Asset Management Zustand Store

## Metadata
**Phase**: 3 of 4
**Depends On**: Phase 1 (Types & API Client), Phase 2 (Business Logic)
**Complexity**: 6/10
**Estimated Time**: 8-12 hours

## Outcome
A complete Zustand store for asset management with multi-index caching, O(1) lookups, LocalStorage persistence, and UI state management - ready for React components to consume.

---

## Overview

**What We're Building**: `stores/assetStore.ts` - A Zustand store that:
- Manages multi-index cache for O(1) lookups (byId, byIdentifier, byType)
- Tracks UI state (filters, pagination, sort, selection)
- Persists cache to LocalStorage with Map/Set serialization
- Handles bulk upload job tracking
- Provides pure cache operations (no API calls in this phase)

**Note**: This phase focuses on STATE MANAGEMENT only. API integration happens in Phase 4.

---

## File to Create

```
frontend/src/stores/
└── assetStore.ts           # Zustand store (~400 lines)
```

---

## Store Architecture

### 1. State Structure

```typescript
interface AssetStore {
  // ============ Cache State ============
  cache: AssetCache;                    // Multi-index cache from Phase 1 types

  // ============ UI State ============
  selectedAssetId: number | null;       // Currently selected asset
  filters: AssetFilters;                // Active filters
  pagination: PaginationState;          // Current page state
  sort: SortState;                      // Current sort state

  // ============ Bulk Upload State ============
  uploadJobId: string | null;           // Active CSV upload job
  pollingIntervalId: NodeJS.Timeout | null; // Polling cleanup

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
  getFilteredAssets: () => Asset[];      // Apply filters + sort + search
  getPaginatedAssets: () => Asset[];     // Apply pagination to filtered results

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
```

---

## Implementation Requirements

### Multi-Index Cache Operations

**Purpose**: Maintain consistency across all indexes when adding/updating/removing assets.

#### `addAssets(assets: Asset[])`
Bulk add assets to cache (used after API fetch).

```typescript
addAssets: (assets) => set((state) => {
  const newCache = { ...state.cache };

  assets.forEach((asset) => {
    // Update byId
    newCache.byId.set(asset.id, asset);

    // Update byIdentifier
    newCache.byIdentifier.set(asset.identifier, asset);

    // Update byType
    const typeSet = newCache.byType.get(asset.type) ?? new Set();
    typeSet.add(asset.id);
    newCache.byType.set(asset.type, typeSet);

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
```

#### `addAsset(asset: Asset)`
Add single asset (used after create).

**Same logic as addAssets but for one asset.**

#### `updateCachedAsset(id: number, updates: Partial<Asset>)`
Update asset in cache (used after update API).

**Critical**: Handle type changes and active status changes:
```typescript
updateCachedAsset: (id, updates) => set((state) => {
  const current = state.cache.byId.get(id);
  if (!current) return state;

  const updated = { ...current, ...updates };
  const newCache = { ...state.cache };

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
      oldTypeSet.delete(id);
    }

    // Add to new type
    const newTypeSet = newCache.byType.get(updates.type) ?? new Set();
    newTypeSet.add(id);
    newCache.byType.set(updates.type, newTypeSet);
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
```

#### `removeAsset(id: number)`
Remove asset from all indexes.

```typescript
removeAsset: (id) => set((state) => {
  const asset = state.cache.byId.get(id);
  if (!asset) return state;

  const newCache = { ...state.cache };

  // Remove from byId
  newCache.byId.delete(id);

  // Remove from byIdentifier
  newCache.byIdentifier.delete(asset.identifier);

  // Remove from byType
  const typeSet = newCache.byType.get(asset.type);
  if (typeSet) {
    typeSet.delete(id);
  }

  // Remove from activeIds
  newCache.activeIds.delete(id);

  // Remove from allIds
  newCache.allIds = newCache.allIds.filter((aid) => aid !== id);

  return { cache: newCache };
}),
```

#### `invalidateCache()`
Clear all cached data (used after bulk upload or on demand).

```typescript
invalidateCache: () => set({
  cache: {
    byId: new Map(),
    byIdentifier: new Map(),
    byType: new Map(),
    activeIds: new Set(),
    allIds: [],
    lastFetched: 0,
    ttl: 5 * 60 * 1000, // 5 minutes
  },
}),
```

---

### Cache Query Methods

**Purpose**: Provide convenient accessors for cached data.

```typescript
// O(1) lookup by ID
getAssetById: (id) => get().cache.byId.get(id),

// O(1) lookup by identifier
getAssetByIdentifier: (identifier) =>
  get().cache.byIdentifier.get(identifier),

// Get all assets of a specific type
getAssetsByType: (type) => {
  const ids = get().cache.byType.get(type) ?? new Set();
  return Array.from(ids)
    .map((id) => get().cache.byId.get(id))
    .filter((asset): asset is Asset => asset !== undefined);
},

// Get all active assets
getActiveAssets: () => {
  const ids = get().cache.activeIds;
  return Array.from(ids)
    .map((id) => get().cache.byId.get(id))
    .filter((asset): asset is Asset => asset !== undefined);
},

// Get filtered assets (apply filters + sort + search)
getFilteredAssets: () => {
  const { cache, filters, sort } = get();
  let assets = Array.from(cache.byId.values());

  // Apply filters (from Phase 2)
  assets = filterAssets(assets, filters);

  // Apply sort (from Phase 2)
  assets = sortAssets(assets, sort);

  return assets;
},

// Get paginated assets
getPaginatedAssets: () => {
  const filtered = get().getFilteredAssets();
  const { pagination } = get();

  // Apply pagination (from Phase 2)
  return paginateAssets(filtered, pagination);
},
```

---

### UI State Actions

**Purpose**: Manage UI state for filters, pagination, sort, and selection.

```typescript
// Update filters (partial update)
setFilters: (newFilters) => set((state) => ({
  filters: { ...state.filters, ...newFilters },
})),

// Set current page
setPage: (page) => set((state) => ({
  pagination: { ...state.pagination, currentPage: page },
})),

// Set page size
setPageSize: (size) => set((state) => ({
  pagination: { ...state.pagination, pageSize: size, currentPage: 1 },
})),

// Update sort
setSort: (field, direction) => set({
  sort: { field, direction },
}),

// Update search term
setSearchTerm: (term) => set((state) => ({
  filters: { ...state.filters, searchTerm: term },
})),

// Reset to page 1
resetPagination: () => set((state) => ({
  pagination: { ...state.pagination, currentPage: 1 },
})),

// Select asset
selectAsset: (id) => set({ selectedAssetId: id }),

// Get selected asset from cache
getSelectedAsset: () => {
  const { selectedAssetId, cache } = get();
  return selectedAssetId ? cache.byId.get(selectedAssetId) : undefined;
},
```

---

### Bulk Upload State

**Purpose**: Track CSV upload job status for polling (Phase 4).

```typescript
setUploadJobId: (jobId) => set({ uploadJobId: jobId }),

setPollingInterval: (intervalId) => set({ pollingIntervalId: intervalId }),

clearUploadState: () => set({
  uploadJobId: null,
  pollingIntervalId: null,
}),
```

---

### LocalStorage Persistence

**Use Zustand's persist middleware with custom serialization for Maps/Sets.**

```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { serializeCache, deserializeCache } from '@/lib/asset/transforms';

export const useAssetStore = create<AssetStore>()(
  persist(
    (set, get) => ({
      // ... all state and actions above
    }),
    {
      name: 'asset-store',
      partialize: (state) => ({
        cache: state.cache,
        filters: state.filters,
        pagination: state.pagination,
        sort: state.sort,
      }),
      storage: {
        getItem: (name) => {
          const str = localStorage.getItem(name);
          if (!str) return null;

          try {
            const parsed = JSON.parse(str);

            // Deserialize cache
            if (parsed.state?.cache) {
              const cacheStr = JSON.stringify(parsed.state.cache);
              const deserializedCache = deserializeCache(cacheStr);

              if (deserializedCache) {
                // Check TTL
                const now = Date.now();
                if (now - deserializedCache.lastFetched < deserializedCache.ttl) {
                  parsed.state.cache = deserializedCache;
                } else {
                  // Expired - return empty cache
                  parsed.state.cache = {
                    byId: new Map(),
                    byIdentifier: new Map(),
                    byType: new Map(),
                    activeIds: new Set(),
                    allIds: [],
                    lastFetched: 0,
                    ttl: 5 * 60 * 1000,
                  };
                }
              }
            }

            return parsed;
          } catch {
            return null;
          }
        },
        setItem: (name, value) => {
          const serialized = {
            ...value,
            state: {
              ...value.state,
              cache: JSON.parse(serializeCache(value.state.cache)),
            },
          };
          localStorage.setItem(name, JSON.stringify(serialized));
        },
        removeItem: (name) => localStorage.removeItem(name),
      },
    }
  )
);
```

---

## Initial State

```typescript
// Initial cache state
cache: {
  byId: new Map(),
  byIdentifier: new Map(),
  byType: new Map(),
  activeIds: new Set(),
  allIds: [],
  lastFetched: 0,
  ttl: 5 * 60 * 1000, // 5 minutes
},

// Initial UI state
selectedAssetId: null,
filters: {
  type: 'all',
  is_active: 'all',
  searchTerm: '',
},
pagination: {
  currentPage: 1,
  pageSize: 25,
  totalCount: 0,
  totalPages: 0,
},
sort: {
  field: 'created_at',
  direction: 'desc',
},

// Initial bulk upload state
uploadJobId: null,
pollingIntervalId: null,
```

---

## Testing Requirements

### Cache Operations Tests

```typescript
describe('AssetStore - Cache Operations', () => {
  it('should add single asset to cache', () => {
    // Test addAsset updates all indexes
  });

  it('should add multiple assets to cache', () => {
    // Test addAssets bulk operation
  });

  it('should update asset in cache', () => {
    // Test updateCachedAsset maintains consistency
  });

  it('should handle type change when updating', () => {
    // Test byType index updated correctly
  });

  it('should handle active status change when updating', () => {
    // Test activeIds set updated correctly
  });

  it('should remove asset from all indexes', () => {
    // Test removeAsset cleans up completely
  });

  it('should invalidate cache completely', () => {
    // Test invalidateCache clears everything
  });
});
```

### Cache Query Tests

```typescript
describe('AssetStore - Cache Queries', () => {
  it('should get asset by ID', () => {
    // Test O(1) lookup
  });

  it('should get asset by identifier', () => {
    // Test O(1) lookup
  });

  it('should get assets by type', () => {
    // Test byType index usage
  });

  it('should get active assets only', () => {
    // Test activeIds set usage
  });

  it('should get filtered assets', () => {
    // Test integration with Phase 2 filterAssets()
  });

  it('should get paginated assets', () => {
    // Test integration with Phase 2 paginateAssets()
  });
});
```

### UI State Tests

```typescript
describe('AssetStore - UI State', () => {
  it('should update filters partially', () => {});
  it('should set page number', () => {});
  it('should set page size and reset to page 1', () => {});
  it('should update sort field and direction', () => {});
  it('should select asset', () => {});
  it('should get selected asset from cache', () => {});
});
```

### LocalStorage Persistence Tests

```typescript
describe('AssetStore - Persistence', () => {
  it('should serialize cache to LocalStorage', () => {
    // Test Maps/Sets converted to arrays
  });

  it('should deserialize cache from LocalStorage', () => {
    // Test arrays converted back to Maps/Sets
  });

  it('should respect cache TTL on load', () => {
    // Test expired cache not loaded
  });

  it('should persist filters, pagination, sort', () => {});
});
```

---

## Success Criteria

- [ ] All cache operations maintain index consistency
- [ ] Cache lookups are O(1) for byId and byIdentifier
- [ ] Type changes update byType index correctly
- [ ] Active status changes update activeIds set correctly
- [ ] Asset removal cleans up all indexes
- [ ] LocalStorage persistence works with Maps/Sets
- [ ] Cache TTL respected (expired cache not loaded)
- [ ] UI state (filters, pagination, sort) persisted correctly
- [ ] All tests passing (target: 20+ tests)
- [ ] TypeScript: 0 errors
- [ ] Lint: 0 issues

---

## Dependencies

**Phase 1 Types**:
```typescript
import type {
  Asset,
  AssetCache,
  AssetFilters,
  PaginationState,
  SortState
} from '@/types/asset';
```

**Phase 2 Functions**:
```typescript
import { filterAssets, sortAssets, paginateAssets } from '@/lib/asset/filters';
import { serializeCache, deserializeCache } from '@/lib/asset/transforms';
```

**Zustand**:
```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
```

---

## Notes

- **No API calls in this phase** - Pure state management
- API integration happens in Phase 4
- Follow pattern from `authStore.ts`
- Use JSDoc comments for all exported functions
- Named exports only (no default export)

---

## References

- Phase 1: `types/asset.ts`, `lib/api/assets.ts`
- Phase 2: `lib/asset/validators.ts`, `lib/asset/transforms.ts`, `lib/asset/filters.ts`
- Pattern: `stores/authStore.ts`
- Main Spec: `spec/active/assets-frontend-logic/spec.md`
