# Phase 2: State Management - Locations Frontend

**Status**: Ready for Implementation
**Dependencies**: Phase 1 (Data Foundation) - ✅ Complete
**Estimated Time**: 2 days
**Branch**: `feature/locations-crud-frontend-phase2`

## Overview

Phase 2 implements the Zustand state management layer for locations, including:
- Hierarchical cache with optimized lookups
- LocalStorage persistence with TTL
- Cache operations maintaining index consistency
- Hierarchy query functions
- Comprehensive unit tests

This phase builds directly on Phase 1's pure functions and types.

## Goals

1. ✅ Create Zustand store with hierarchical cache structure
2. ✅ Implement all cache operations (add, update, delete, clear)
3. ✅ Implement hierarchy queries (ancestors, descendants, children, roots)
4. ✅ LocalStorage persistence with TTL enforcement
5. ✅ Maintain index consistency across all operations
6. ✅ Unit test all store actions and cache operations

## Architecture

### Store Structure

Following Assets store pattern at `frontend/src/stores/assets/assetStore.ts`:

```
frontend/src/stores/locations/
├── locationStore.ts          # Main Zustand store (state + actions)
├── locationActions.ts        # Store actions (cache operations)
└── locationPersistence.ts    # LocalStorage middleware
```

### Cache Design

**Optimized for Hierarchy Operations**:
- `byId`: O(1) lookup by ID
- `byIdentifier`: O(1) lookup by identifier
- `byParentId`: O(1) children lookup
- `rootIds`: O(1) root location access
- `activeIds`: O(1) active location filtering
- `allIds`: Ordered list for iteration
- `allIdentifiers`: Cached for dropdown performance

**Cache Invariants** (MUST maintain):
1. Every location in `byId` also in `byIdentifier`
2. Every child ID in `byParentId[parent]` exists in `byId`
3. Every root ID (parent_location_id = null) in `rootIds`
4. Every active location ID in `activeIds`
5. `allIds` contains every key from `byId`
6. `allIdentifiers` contains every key from `byIdentifier`

## Files to Create

### 1. `frontend/src/stores/locations/locationStore.ts`

**Purpose**: Main Zustand store combining state and actions

**State Shape**:
```typescript
interface LocationState {
  cache: LocationCache;
  selectedLocationId: number | null;
  filters: LocationFilters;
  pagination: PaginationState;
  sort: LocationSort;
  isLoading: boolean;
  error: string | null;

  addLocation: (location: Location) => void;
  updateLocation: (id: number, updates: Partial<Location>) => void;
  deleteLocation: (id: number) => void;
  setLocations: (locations: Location[]) => void;
  clearCache: () => void;

  getLocationById: (id: number) => Location | undefined;
  getLocationByIdentifier: (identifier: string) => Location | undefined;
  getChildren: (id: number) => Location[];
  getDescendants: (id: number) => Location[];
  getAncestors: (id: number) => Location[];
  getRootLocations: () => Location[];
  getActiveLocations: () => Location[];

  getFilteredLocations: () => Location[];
  getSortedLocations: (locations: Location[]) => Location[];
  getPaginatedLocations: (locations: Location[]) => Location[];

  setSelectedLocation: (id: number | null) => void;
  setFilters: (filters: Partial<LocationFilters>) => void;
  setSort: (sort: LocationSort) => void;
  setPagination: (pagination: Partial<PaginationState>) => void;
  resetFilters: () => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
}
```

**Implementation Pattern**:
```typescript
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import { locationPersistMiddleware } from './locationPersistence';
import { createCacheActions } from './locationActions';

const CACHE_TTL_MS = 5 * 60 * 1000;

const createEmptyCache = (): LocationCache => ({
  byId: new Map(),
  byIdentifier: new Map(),
  byParentId: new Map(),
  rootIds: new Set(),
  activeIds: new Set(),
  allIds: [],
  allIdentifiers: [],
  lastFetched: 0,
  ttl: CACHE_TTL_MS,
});

const createInitialState = () => ({
  cache: createEmptyCache(),
  selectedLocationId: null,
  filters: {
    search: '',
    identifier: '',
    is_active: 'all' as const,
  },
  pagination: {
    currentPage: 1,
    pageSize: 10,
    totalCount: 0,
    totalPages: 0,
  },
  sort: {
    field: 'identifier' as const,
    direction: 'asc' as const,
  },
  isLoading: false,
  error: null,
});

export const useLocationStore = create<LocationState>()(
  devtools(
    locationPersistMiddleware(
      (set, get) => ({
        ...createInitialState(),
        ...createCacheActions(set, get),
      })
    )
  )
);
```

---

### 2. `frontend/src/stores/locations/locationActions.ts`

**Purpose**: Cache operations maintaining all index invariants

**Core Actions**:

```typescript
import type { Location, LocationCache } from '@/types/locations';
import { filterLocations, sortLocations, paginateLocations } from '@/lib/location/filters';

export function createCacheActions(set, get) {
  const ensureParentChildrenSet = (cache: LocationCache, parentId: number | null) => {
    if (!cache.byParentId.has(parentId)) {
      cache.byParentId.set(parentId, new Set());
    }
  };

  const updatePrimaryIndexes = (cache: LocationCache, location: Location) => {
    cache.byId.set(location.id, location);
    cache.byIdentifier.set(location.identifier, location);
  };

  const updateParentChildMapping = (cache: LocationCache, locationId: number, parentId: number | null) => {
    ensureParentChildrenSet(cache, parentId);
    cache.byParentId.get(parentId)!.add(locationId);
  };

  const updateRootSet = (cache: LocationCache, locationId: number, parentId: number | null) => {
    if (parentId === null) {
      cache.rootIds.add(locationId);
    }
  };

  const updateActiveSet = (cache: LocationCache, location: Location) => {
    if (location.is_active) {
      cache.activeIds.add(location.id);
    }
  };

  const rebuildOrderedLists = (cache: LocationCache) => {
    cache.allIds = Array.from(cache.byId.keys());
    cache.allIdentifiers = Array.from(cache.byIdentifier.keys()).sort();
  };

  const handleIdentifierChange = (
    cache: LocationCache,
    existing: Location,
    updated: Location,
    hasIdentifierChanged: boolean
  ) => {
    if (hasIdentifierChanged) {
      cache.byIdentifier.delete(existing.identifier);
      cache.byIdentifier.set(updated.identifier, updated);
      rebuildOrderedLists(cache);
    } else {
      cache.byIdentifier.set(updated.identifier, updated);
    }
  };

  const removeFromParentChildren = (cache: LocationCache, locationId: number, parentId: number | null) => {
    const parentChildren = cache.byParentId.get(parentId);
    if (parentChildren) {
      parentChildren.delete(locationId);
    }
  };

  const handleParentChange = (
    cache: LocationCache,
    locationId: number,
    oldParentId: number | null,
    newParentId: number | null
  ) => {
    removeFromParentChildren(cache, locationId, oldParentId);
    updateParentChildMapping(cache, locationId, newParentId);

    if (oldParentId === null) {
      cache.rootIds.delete(locationId);
    }
    if (newParentId === null) {
      cache.rootIds.add(locationId);
    }
  };

  const handleActiveStatusChange = (cache: LocationCache, location: Location, wasActive: boolean) => {
    if (location.is_active) {
      cache.activeIds.add(location.id);
    } else {
      cache.activeIds.delete(location.id);
    }
  };

  const removeFromAllIndexes = (cache: LocationCache, location: Location) => {
    cache.byId.delete(location.id);
    cache.byIdentifier.delete(location.identifier);
    removeFromParentChildren(cache, location.id, location.parent_location_id);

    if (location.parent_location_id === null) {
      cache.rootIds.delete(location.id);
    }

    if (location.is_active) {
      cache.activeIds.delete(location.id);
    }

    rebuildOrderedLists(cache);
  };

  return {
    addLocation: (location: Location) => {
      set((state) => {
        const cache = { ...state.cache };
        const parentId = location.parent_location_id;

        updatePrimaryIndexes(cache, location);
        updateParentChildMapping(cache, location.id, parentId);
        updateRootSet(cache, location.id, parentId);
        updateActiveSet(cache, location);
        rebuildOrderedLists(cache);

        return { cache };
      });
    },

    updateLocation: (id: number, updates: Partial<Location>) => {
      set((state) => {
        const cache = { ...state.cache };
        const existing = cache.byId.get(id);

        if (!existing) return state;

        const updated = { ...existing, ...updates };
        cache.byId.set(id, updated);

        const hasIdentifierChanged = updates.identifier && updates.identifier !== existing.identifier;
        handleIdentifierChange(cache, existing, updated, !!hasIdentifierChanged);

        const hasParentChanged =
          updates.parent_location_id !== undefined &&
          updates.parent_location_id !== existing.parent_location_id;

        if (hasParentChanged) {
          handleParentChange(cache, id, existing.parent_location_id, updates.parent_location_id!);
        }

        const hasActiveStatusChanged =
          updates.is_active !== undefined && updates.is_active !== existing.is_active;

        if (hasActiveStatusChanged) {
          handleActiveStatusChange(cache, updated, existing.is_active);
        }

        return { cache };
      });
    },

    deleteLocation: (id: number) => {
      set((state) => {
        const cache = { ...state.cache };
        const location = cache.byId.get(id);

        if (!location) return state;

        removeFromAllIndexes(cache, location);

        return { cache };
      });
    },

    setLocations: (locations: Location[]) => {
      set((state) => {
        const cache = {
          byId: new Map<number, Location>(),
          byIdentifier: new Map<string, Location>(),
          byParentId: new Map<number | null, Set<number>>(),
          rootIds: new Set<number>(),
          activeIds: new Set<number>(),
          allIds: [] as number[],
          allIdentifiers: [] as string[],
          lastFetched: Date.now(),
          ttl: state.cache.ttl,
        };

        for (const location of locations) {
          const parentId = location.parent_location_id;

          updatePrimaryIndexes(cache, location);
          updateParentChildMapping(cache, location.id, parentId);
          updateRootSet(cache, location.id, parentId);
          updateActiveSet(cache, location);
        }

        rebuildOrderedLists(cache);

        return { cache };
      });
    },

    clearCache: () => {
      set((state) => ({
        cache: {
          byId: new Map(),
          byIdentifier: new Map(),
          byParentId: new Map(),
          rootIds: new Set(),
          activeIds: new Set(),
          allIds: [],
          allIdentifiers: [],
          lastFetched: 0,
          ttl: state.cache.ttl,
        },
      }));
    },

    getLocationById: (id: number) => {
      return get().cache.byId.get(id);
    },

    getLocationByIdentifier: (identifier: string) => {
      return get().cache.byIdentifier.get(identifier);
    },

    getChildren: (id: number) => {
      const cache = get().cache;
      const childIds = cache.byParentId.get(id);

      if (!childIds) return [];

      return Array.from(childIds)
        .map((childId) => cache.byId.get(childId))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getDescendants: (id: number) => {
      const descendants: Location[] = [];
      const visited = new Set<number>();
      const { getChildren } = get();

      const collectDescendants = (parentId: number) => {
        if (visited.has(parentId)) return;
        visited.add(parentId);

        const children = getChildren(parentId);
        for (const child of children) {
          descendants.push(child);
          collectDescendants(child.id);
        }
      };

      collectDescendants(id);
      return descendants;
    },

    getAncestors: (id: number) => {
      const cache = get().cache;
      const ancestors: Location[] = [];
      const visited = new Set<number>([id]);

      let current = cache.byId.get(id);

      while (current && current.parent_location_id !== null) {
        if (visited.has(current.parent_location_id)) break;

        const parent = cache.byId.get(current.parent_location_id);
        if (!parent) break;

        ancestors.unshift(parent);
        visited.add(parent.id);
        current = parent;
      }

      return ancestors;
    },

    getRootLocations: () => {
      const cache = get().cache;
      return Array.from(cache.rootIds)
        .map((id) => cache.byId.get(id))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getActiveLocations: () => {
      const cache = get().cache;
      return Array.from(cache.activeIds)
        .map((id) => cache.byId.get(id))
        .filter((loc): loc is Location => loc !== undefined);
    },

    getFilteredLocations: () => {
      const { cache, filters } = get();
      const allLocations = Array.from(cache.byId.values());
      return filterLocations(allLocations, filters);
    },

    getSortedLocations: (locations: Location[]) => {
      const { sort } = get();
      return sortLocations(locations, sort);
    },

    getPaginatedLocations: (locations: Location[]) => {
      const { pagination } = get();
      return paginateLocations(locations, pagination);
    },

    setSelectedLocation: (id: number | null) => {
      set({ selectedLocationId: id });
    },

    setFilters: (filters: Partial<LocationFilters>) => {
      set((state) => ({
        filters: { ...state.filters, ...filters },
      }));
    },

    setSort: (sort: LocationSort) => {
      set({ sort });
    },

    setPagination: (pagination: Partial<PaginationState>) => {
      set((state) => ({
        pagination: { ...state.pagination, ...pagination },
      }));
    },

    resetFilters: () => {
      set({
        filters: {
          search: '',
          identifier: '',
          is_active: 'all',
        },
      });
    },

    setLoading: (isLoading: boolean) => {
      set({ isLoading });
    },

    setError: (error: string | null) => {
      set({ error });
    },
  };
}
```

---

### 3. `frontend/src/stores/locations/locationPersistence.ts`

**Purpose**: LocalStorage middleware with TTL enforcement

```typescript
import { StateCreator, StoreMutatorFn } from 'zustand';
import { serializeCache, deserializeCache } from '@/lib/location/transforms';
import type { LocationState } from './locationStore';

const STORAGE_KEY = 'location-store';

const loadFromLocalStorage = () => {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (!stored) return null;

  try {
    return JSON.parse(stored);
  } catch (error) {
    console.error('[LocationStore] LocalStorage parse failed:', error);
    localStorage.removeItem(STORAGE_KEY);
    return null;
  }
};

const isCacheValid = (cache: any, now: number) => {
  if (!cache) return false;

  const age = now - cache.lastFetched;
  return age < cache.ttl;
};

const restoreCacheFromStorage = () => {
  const stored = loadFromLocalStorage();
  if (!stored?.cache) return {};

  const cache = deserializeCache(stored.cache);
  if (!cache) return {};

  const now = Date.now();
  if (!isCacheValid(cache, now)) return {};

  return { cache };
};

const persistToLocalStorage = (state: LocationState) => {
  try {
    const serialized = {
      cache: serializeCache(state.cache),
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(serialized));
  } catch (error) {
    console.error('[LocationStore] LocalStorage save failed:', error);
  }
};

export const locationPersistMiddleware: StoreMutatorFn = (config) => (set, get, api) => {
  const initialState = restoreCacheFromStorage();

  const wrappedSet = (args: any) => {
    set(args);
    persistToLocalStorage(get() as LocationState);
  };

  return {
    ...config(wrappedSet, get, api),
    ...initialState,
  };
};
```

---

## Testing Strategy

### Unit Tests

**File**: `frontend/src/stores/locations/locationStore.test.ts`

**Test Coverage** (minimum 30 tests):

```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { useLocationStore } from './locationStore';
import type { Location } from '@/types/locations';

const createMockLocation = (overrides = {}): Location => ({
  id: 1,
  org_id: 1,
  identifier: 'usa',
  name: 'United States',
  description: '',
  parent_location_id: null,
  path: 'usa',
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('LocationStore', () => {
  beforeEach(() => {
    useLocationStore.getState().clearCache();
  });

  describe('Cache Operations', () => {
    it('adds location to all indexes', () => {
      const location = createMockLocation();
      useLocationStore.getState().addLocation(location);

      const { cache } = useLocationStore.getState();

      expect(cache.byId.has(1)).toBe(true);
      expect(cache.byIdentifier.has('usa')).toBe(true);
      expect(cache.rootIds.has(1)).toBe(true);
      expect(cache.activeIds.has(1)).toBe(true);
      expect(cache.allIds).toContain(1);
      expect(cache.allIdentifiers).toContain('usa');
    });

    it('updates location identifier correctly', () => {
      const location = createMockLocation();
      useLocationStore.getState().addLocation(location);

      useLocationStore.getState().updateLocation(1, { identifier: 'united_states' });

      const { cache } = useLocationStore.getState();

      expect(cache.byIdentifier.has('usa')).toBe(false);
      expect(cache.byIdentifier.has('united_states')).toBe(true);
      expect(cache.allIdentifiers).toContain('united_states');
    });

    it('handles re-parenting correctly', () => {
      const root = createMockLocation({ id: 1, identifier: 'root' });
      const child = createMockLocation({ id: 2, identifier: 'child', parent_location_id: 1 });

      useLocationStore.getState().addLocation(root);
      useLocationStore.getState().addLocation(child);

      useLocationStore.getState().updateLocation(2, { parent_location_id: null });

      const { cache } = useLocationStore.getState();

      expect(cache.rootIds.has(2)).toBe(true);
      expect(cache.byParentId.get(1)?.has(2)).toBe(false);
      expect(cache.byParentId.get(null)?.has(2)).toBe(true);
    });

    it('removes location from all indexes', () => {
      const location = createMockLocation();
      useLocationStore.getState().addLocation(location);
      useLocationStore.getState().deleteLocation(1);

      const { cache } = useLocationStore.getState();

      expect(cache.byId.has(1)).toBe(false);
      expect(cache.byIdentifier.has('usa')).toBe(false);
      expect(cache.rootIds.has(1)).toBe(false);
      expect(cache.activeIds.has(1)).toBe(false);
      expect(cache.allIds).not.toContain(1);
    });
  });

  describe('Hierarchy Queries', () => {
    beforeEach(() => {
      const root = createMockLocation({ id: 1, identifier: 'root' });
      const child1 = createMockLocation({ id: 2, identifier: 'child1', parent_location_id: 1 });
      const child2 = createMockLocation({ id: 3, identifier: 'child2', parent_location_id: 1 });
      const grandchild = createMockLocation({ id: 4, identifier: 'grandchild', parent_location_id: 2 });

      useLocationStore.getState().setLocations([root, child1, child2, grandchild]);
    });

    it('returns immediate children only', () => {
      const children = useLocationStore.getState().getChildren(1);

      expect(children).toHaveLength(2);
      expect(children.map((c) => c.id)).toEqual([2, 3]);
    });

    it('returns all descendants', () => {
      const descendants = useLocationStore.getState().getDescendants(1);

      expect(descendants).toHaveLength(3);
      expect(descendants.map((d) => d.id)).toContain(2);
      expect(descendants.map((d) => d.id)).toContain(3);
      expect(descendants.map((d) => d.id)).toContain(4);
    });

    it('returns ancestors in root-first order', () => {
      const ancestors = useLocationStore.getState().getAncestors(4);

      expect(ancestors).toHaveLength(2);
      expect(ancestors[0].id).toBe(1);
      expect(ancestors[1].id).toBe(2);
    });

    it('returns empty array for root ancestors', () => {
      const ancestors = useLocationStore.getState().getAncestors(1);
      expect(ancestors).toHaveLength(0);
    });
  });
});
```

---

## Validation Gates

**After EVERY file**:
```bash
just frontend typecheck
just frontend lint
just frontend test stores/locations/
```

**Final Validation**:
```bash
just frontend validate
```

**Success Criteria**:
- [ ] All 30+ store tests passing
- [ ] Zero type errors
- [ ] Cache operations maintain invariants
- [ ] Hierarchy queries correct
- [ ] LocalStorage persistence works
- [ ] TTL enforcement works

---

## References

**Patterns to Follow**:
- Assets Store: `frontend/src/stores/assets/assetStore.ts`
- Assets Actions: `frontend/src/stores/assets/assetActions.ts`
- Assets Persistence: `frontend/src/stores/assets/assetPersistence.ts`

**Phase 1 Dependencies**:
- Types: `frontend/src/types/locations/index.ts`
- Transforms: `frontend/src/lib/location/transforms.ts`
- Filters: `frontend/src/lib/location/filters.ts`
