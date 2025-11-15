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
  // ============ Cache ============
  cache: LocationCache;

  // ============ UI State ============
  selectedLocationId: number | null;
  filters: LocationFilters;
  pagination: PaginationState;
  sort: LocationSort;

  // ============ Loading State ============
  isLoading: boolean;
  error: string | null;

  // ============ Cache Actions ============
  addLocation: (location: Location) => void;
  updateLocation: (id: number, updates: Partial<Location>) => void;
  deleteLocation: (id: number) => void;
  setLocations: (locations: Location[]) => void;
  clearCache: () => void;

  // ============ Hierarchy Queries ============
  getLocationById: (id: number) => Location | undefined;
  getLocationByIdentifier: (identifier: string) => Location | undefined;
  getChildren: (id: number) => Location[];
  getDescendants: (id: number) => Location[];
  getAncestors: (id: number) => Location[];
  getRootLocations: () => Location[];
  getActiveLocations: () => Location[];

  // ============ Filtered Data ============
  getFilteredLocations: () => Location[];
  getSortedLocations: (locations: Location[]) => Location[];
  getPaginatedLocations: (locations: Location[]) => Location[];

  // ============ UI Actions ============
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

// Initial state
const initialState = {
  cache: {
    byId: new Map(),
    byIdentifier: new Map(),
    byParentId: new Map(),
    rootIds: new Set(),
    activeIds: new Set(),
    allIds: [],
    allIdentifiers: [],
    lastFetched: 0,
    ttl: 5 * 60 * 1000, // 5 minutes
  },
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
};

export const useLocationStore = create<LocationState>()(
  devtools(
    locationPersistMiddleware(
      (set, get) => ({
        ...initialState,
        ...createCacheActions(set, get),
        // ... UI actions
      })
    )
  )
);
```

**Validation**:
- [ ] Store initializes with empty cache
- [ ] All actions available via hooks
- [ ] DevTools integration works
- [ ] No TypeScript errors

---

### 2. `frontend/src/stores/locations/locationActions.ts`

**Purpose**: Cache operations maintaining all index invariants

**Exports**:
```typescript
export function createCacheActions(
  set: SetState<LocationState>,
  get: GetState<LocationState>
) {
  return {
    addLocation,
    updateLocation,
    deleteLocation,
    setLocations,
    clearCache,
    getLocationById,
    getLocationByIdentifier,
    getChildren,
    getDescendants,
    getAncestors,
    getRootLocations,
    getActiveLocations,
    getFilteredLocations,
    getSortedLocations,
    getPaginatedLocations,
  };
}
```

**Critical Cache Operations**:

**`addLocation(location: Location)`**:
```typescript
function addLocation(location: Location) {
  set((state) => {
    const cache = { ...state.cache };

    // Add to primary indexes
    cache.byId.set(location.id, location);
    cache.byIdentifier.set(location.identifier, location);

    // Update parent-child mapping
    const parentId = location.parent_location_id;
    if (!cache.byParentId.has(parentId)) {
      cache.byParentId.set(parentId, new Set());
    }
    cache.byParentId.get(parentId)!.add(location.id);

    // Update root set
    if (parentId === null) {
      cache.rootIds.add(location.id);
    }

    // Update active set
    if (location.is_active) {
      cache.activeIds.add(location.id);
    }

    // Update ordered lists (MUST maintain consistency)
    cache.allIds = Array.from(cache.byId.keys());
    cache.allIdentifiers = Array.from(cache.byIdentifier.keys()).sort();

    return { cache };
  });
}
```

**`updateLocation(id: number, updates: Partial<Location>)`**:
```typescript
function updateLocation(id: number, updates: Partial<Location>) {
  set((state) => {
    const cache = { ...state.cache };
    const existing = cache.byId.get(id);

    if (!existing) return state; // No-op if not found

    const updated = { ...existing, ...updates };

    // Update primary indexes
    cache.byId.set(id, updated);

    // Handle identifier change
    if (updates.identifier && updates.identifier !== existing.identifier) {
      cache.byIdentifier.delete(existing.identifier);
      cache.byIdentifier.set(updated.identifier, updated);
      cache.allIdentifiers = Array.from(cache.byIdentifier.keys()).sort();
    } else {
      cache.byIdentifier.set(updated.identifier, updated);
    }

    // Handle parent change (re-parenting)
    if (updates.parent_location_id !== undefined && updates.parent_location_id !== existing.parent_location_id) {
      // Remove from old parent's children
      const oldParentChildren = cache.byParentId.get(existing.parent_location_id);
      if (oldParentChildren) {
        oldParentChildren.delete(id);
      }

      // Add to new parent's children
      const newParentId = updates.parent_location_id;
      if (!cache.byParentId.has(newParentId)) {
        cache.byParentId.set(newParentId, new Set());
      }
      cache.byParentId.get(newParentId)!.add(id);

      // Update root set
      if (existing.parent_location_id === null) {
        cache.rootIds.delete(id);
      }
      if (newParentId === null) {
        cache.rootIds.add(id);
      }
    }

    // Handle active status change
    if (updates.is_active !== undefined && updates.is_active !== existing.is_active) {
      if (updated.is_active) {
        cache.activeIds.add(id);
      } else {
        cache.activeIds.delete(id);
      }
    }

    return { cache };
  });
}
```

**`deleteLocation(id: number)`**:
```typescript
function deleteLocation(id: number) {
  set((state) => {
    const cache = { ...state.cache };
    const location = cache.byId.get(id);

    if (!location) return state;

    // Remove from primary indexes
    cache.byId.delete(id);
    cache.byIdentifier.delete(location.identifier);

    // Remove from parent's children
    const parentChildren = cache.byParentId.get(location.parent_location_id);
    if (parentChildren) {
      parentChildren.delete(id);
    }

    // Remove from root set if applicable
    if (location.parent_location_id === null) {
      cache.rootIds.delete(id);
    }

    // Remove from active set if applicable
    if (location.is_active) {
      cache.activeIds.delete(id);
    }

    // Update ordered lists
    cache.allIds = Array.from(cache.byId.keys());
    cache.allIdentifiers = Array.from(cache.byIdentifier.keys()).sort();

    // Note: Children are NOT automatically deleted (must be handled by caller)

    return { cache };
  });
}
```

**Hierarchy Query Functions**:

**`getChildren(id: number): Location[]`**:
```typescript
function getChildren(id: number): Location[] {
  const cache = get().cache;
  const childIds = cache.byParentId.get(id);

  if (!childIds) return [];

  return Array.from(childIds)
    .map(childId => cache.byId.get(childId))
    .filter((loc): loc is Location => loc !== undefined);
}
```

**`getDescendants(id: number): Location[]`** (recursive):
```typescript
function getDescendants(id: number): Location[] {
  const descendants: Location[] = [];
  const visited = new Set<number>();

  function collectDescendants(parentId: number) {
    if (visited.has(parentId)) return; // Prevent infinite loops
    visited.add(parentId);

    const children = getChildren(parentId);
    for (const child of children) {
      descendants.push(child);
      collectDescendants(child.id);
    }
  }

  collectDescendants(id);
  return descendants;
}
```

**`getAncestors(id: number): Location[]`**:
```typescript
function getAncestors(id: number): Location[] {
  const cache = get().cache;
  const ancestors: Location[] = [];
  const visited = new Set<number>([id]);

  let current = cache.byId.get(id);

  while (current && current.parent_location_id !== null) {
    if (visited.has(current.parent_location_id)) break; // Prevent cycles

    const parent = cache.byId.get(current.parent_location_id);
    if (!parent) break;

    ancestors.unshift(parent); // Add to front (root first)
    visited.add(parent.id);
    current = parent;
  }

  return ancestors;
}
```

**Filtered Data Functions**:

**`getFilteredLocations(): Location[]`**:
```typescript
function getFilteredLocations(): Location[] {
  const { cache, filters } = get();
  const allLocations = Array.from(cache.byId.values());

  return filterLocations(allLocations, filters); // Use Phase 1 filter
}
```

**Validation**:
- [ ] `addLocation` maintains all index invariants
- [ ] `updateLocation` handles identifier change correctly
- [ ] `updateLocation` handles re-parenting correctly
- [ ] `deleteLocation` cleans up all indexes
- [ ] `getChildren` returns immediate children only
- [ ] `getDescendants` returns full subtree
- [ ] `getAncestors` returns root-to-parent chain
- [ ] All hierarchy queries handle cycles gracefully

---

### 3. `frontend/src/stores/locations/locationPersistence.ts`

**Purpose**: LocalStorage middleware with TTL enforcement

**Implementation Pattern** (follow Assets):
```typescript
import { StateCreator, StoreMutatorFn } from 'zustand';
import { serializeCache, deserializeCache } from '@/lib/location/transforms';
import type { LocationState } from './locationStore';

const STORAGE_KEY = 'location-store';

export const locationPersistMiddleware: StoreMutatorFn = (config) => (set, get, api) => {
  // Load from LocalStorage on init
  const stored = localStorage.getItem(STORAGE_KEY);

  let initialState = {};

  if (stored) {
    try {
      const parsed = JSON.parse(stored);
      const cache = deserializeCache(parsed.cache);

      if (cache) {
        const now = Date.now();
        const age = now - cache.lastFetched;

        // Only restore if within TTL
        if (age < cache.ttl) {
          initialState = { cache };
        }
      }
    } catch (error) {
      console.error('[LocationStore] Failed to restore from LocalStorage:', error);
      localStorage.removeItem(STORAGE_KEY);
    }
  }

  // Create store with initial state
  const store = config(
    (args) => {
      set(args);

      // Persist to LocalStorage after state updates
      const state = get() as LocationState;

      try {
        const serialized = {
          cache: serializeCache(state.cache),
        };

        localStorage.setItem(STORAGE_KEY, JSON.stringify(serialized));
      } catch (error) {
        console.error('[LocationStore] Failed to persist to LocalStorage:', error);
      }
    },
    get,
    api
  );

  return {
    ...store,
    ...initialState,
  };
};
```

**Validation**:
- [ ] Cache persists to LocalStorage on updates
- [ ] Cache restores from LocalStorage on init
- [ ] Expired cache is ignored (TTL enforced)
- [ ] Corrupted cache is cleared
- [ ] No errors thrown during serialize/deserialize

---

## Testing Strategy

### Unit Tests

**File**: `frontend/src/stores/locations/locationStore.test.ts`

**Test Coverage** (minimum 30 tests):

**Cache Operations** (12 tests):
1. `addLocation` - adds location to all indexes
2. `addLocation` - handles root location (null parent)
3. `addLocation` - handles child location (with parent)
4. `addLocation` - adds to active set if is_active=true
5. `updateLocation` - updates location properties
6. `updateLocation` - handles identifier change
7. `updateLocation` - handles re-parenting
8. `updateLocation` - handles active status toggle
9. `deleteLocation` - removes from all indexes
10. `deleteLocation` - removes from parent's children
11. `setLocations` - replaces entire cache
12. `clearCache` - resets to empty state

**Hierarchy Queries** (9 tests):
13. `getChildren` - returns immediate children only
14. `getChildren` - returns empty array if no children
15. `getDescendants` - returns full subtree
16. `getDescendants` - handles deep hierarchies
17. `getDescendants` - handles cycles gracefully
18. `getAncestors` - returns root-to-parent chain
19. `getAncestors` - returns empty for root location
20. `getRootLocations` - returns all roots
21. `getActiveLocations` - returns only active locations

**Filtered Data** (5 tests):
22. `getFilteredLocations` - applies search filter
23. `getFilteredLocations` - applies identifier filter
24. `getFilteredLocations` - applies date range
25. `getSortedLocations` - sorts by field and direction
26. `getPaginatedLocations` - returns correct page

**LocalStorage** (4 tests):
27. Persists cache to LocalStorage on add
28. Restores cache from LocalStorage on init
29. Ignores expired cache (TTL)
30. Clears corrupted cache

**Test Pattern**:
```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { useLocationStore } from './locationStore';
import type { Location } from '@/types/locations';

describe('LocationStore', () => {
  beforeEach(() => {
    useLocationStore.getState().clearCache();
  });

  describe('addLocation()', () => {
    it('should add location to all indexes', () => {
      const location: Location = {
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
      };

      useLocationStore.getState().addLocation(location);

      const { cache } = useLocationStore.getState();

      expect(cache.byId.has(1)).toBe(true);
      expect(cache.byIdentifier.has('usa')).toBe(true);
      expect(cache.rootIds.has(1)).toBe(true);
      expect(cache.activeIds.has(1)).toBe(true);
      expect(cache.allIds).toContain(1);
      expect(cache.allIdentifiers).toContain('usa');
    });
  });

  // ... more tests
});
```

---

## Validation Gates

**After EVERY file**:
1. `just frontend typecheck` - No type errors
2. `just frontend lint` - Clean (ignore pre-existing)
3. `just frontend test stores/locations/` - All tests passing

**Final Validation**:
```bash
just frontend validate  # Runs all checks
```

**Success Criteria**:
- [ ] All 30+ store tests passing
- [ ] Zero type errors
- [ ] Cache operations maintain invariants
- [ ] Hierarchy queries correct
- [ ] LocalStorage persistence works
- [ ] TTL enforcement works
- [ ] No console errors

---

## Risk Assessment

**Low Risk**:
- Following proven Assets store pattern exactly
- Reusing Phase 1 serialize/deserialize functions
- Clear test coverage requirements

**Medium Risk**:
- Hierarchy queries complexity (cycles, deep trees)
- **Mitigation**: Comprehensive tests with cycle detection

**Potential Issues**:
- Re-parenting complexity
- **Mitigation**: Explicit test cases for re-parenting edge cases

---

## Success Metrics

- ✅ 30+ unit tests passing
- ✅ 100% TypeScript coverage
- ✅ Cache invariants maintained
- ✅ O(1) primary lookups
- ✅ LocalStorage persistence functional
- ✅ Ready for Phase 3 (React Hooks)

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

**Backend Reference**:
- Locations Handler: `backend/internal/handlers/locations/locations.go`
