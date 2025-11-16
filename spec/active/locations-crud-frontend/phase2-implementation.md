# Phase 2 Implementation Guide: Location Store

**Single File**: `frontend/src/stores/locations/locationStore.ts`
**Pattern**: In-memory Zustand store with lightweight LocalStorage for filter metadata only

---

## Implementation Checklist

- [ ] Create `frontend/src/stores/locations/` directory
- [ ] Implement `locationStore.ts` with all actions
- [ ] Create `locationStore.test.ts` with 30+ tests
- [ ] Verify all tests pass
- [ ] Validate with typecheck and lint

---

## Store Structure

```typescript
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { Location, LocationCache, LocationFilters, LocationSort, PaginationState } from '@/types/locations';
import { filterLocations, sortLocations, paginateLocations } from '@/lib/location/filters';

const STORAGE_KEY = 'location-metadata';

interface LocationMetadata {
  allIdentifiers: string[];
  lastFetched: number;
}

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

---

## Lightweight Persistence Helpers

```typescript
const loadMetadata = (): Partial<{ allIdentifiers: string[] }> => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return {};
    const metadata: LocationMetadata = JSON.parse(stored);
    return { allIdentifiers: metadata.allIdentifiers };
  } catch {
    return {};
  }
};

const saveMetadata = (allIdentifiers: string[]) => {
  try {
    const metadata: LocationMetadata = {
      allIdentifiers,
      lastFetched: Date.now(),
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(metadata));
  } catch (error) {
    console.error('[LocationStore] Failed to save metadata:', error);
  }
};

const createEmptyCache = (): LocationCache => ({
  byId: new Map(),
  byIdentifier: new Map(),
  byParentId: new Map(),
  rootIds: new Set(),
  activeIds: new Set(),
  allIds: [],
  allIdentifiers: [],
  lastFetched: 0,
  ttl: 0,
});
```

---

## Cache Operation Helpers

```typescript
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

const removeFromParentChildren = (cache: LocationCache, locationId: number, parentId: number | null) => {
  const parentChildren = cache.byParentId.get(parentId);
  if (parentChildren) {
    parentChildren.delete(locationId);
  }
};
```

---

## Critical Actions to Implement

### 1. setLocations (bulk replace)
- Clear existing cache
- Rebuild all indexes from scratch
- Call `saveMetadata(cache.allIdentifiers)` at end

### 2. addLocation
- Update byId, byIdentifier
- Update byParentId
- Update rootIds (if parent is null)
- Update activeIds (if is_active)
- Rebuild allIds and allIdentifiers

### 3. updateLocation
- Handle identifier changes (remove old, add new)
- Handle parent changes (re-parent logic)
- Handle active status changes
- Maintain all index consistency

### 4. deleteLocation
- Remove from all indexes
- Rebuild ordered lists

### 5. Hierarchy Queries
- `getChildren`: O(1) lookup in byParentId
- `getDescendants`: Recursive traversal
- `getAncestors`: Walk up parent chain
- `getRootLocations`: Iterate rootIds
- `getActiveLocations`: Iterate activeIds

---

## Test Requirements

**File**: `frontend/src/stores/locations/locationStore.test.ts`

**Minimum 30 tests covering**:
- [ ] addLocation updates all indexes correctly
- [ ] updateLocation handles identifier changes
- [ ] updateLocation handles re-parenting
- [ ] updateLocation handles active status changes
- [ ] deleteLocation removes from all indexes
- [ ] setLocations rebuilds cache from scratch
- [ ] clearCache empties all indexes
- [ ] getChildren returns immediate children only
- [ ] getDescendants returns all descendants
- [ ] getAncestors returns parents in root-first order
- [ ] getRootLocations returns only roots
- [ ] getActiveLocations returns only active
- [ ] getFilteredLocations applies filters correctly
- [ ] Filtered/sorted/paginated integration

**Test Pattern**:
```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { useLocationStore } from './locationStore';

const createMockLocation = (id: number, overrides = {}) => ({
  id,
  org_id: 1,
  identifier: `loc_${id}`,
  name: `Location ${id}`,
  description: '',
  parent_location_id: null,
  path: `loc_${id}`,
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

  it('adds location to all indexes', () => {
    const location = createMockLocation(1);
    useLocationStore.getState().addLocation(location);
    const { cache } = useLocationStore.getState();

    expect(cache.byId.has(1)).toBe(true);
    expect(cache.byIdentifier.has('loc_1')).toBe(true);
    expect(cache.rootIds.has(1)).toBe(true);
    expect(cache.activeIds.has(1)).toBe(true);
    expect(cache.allIds).toContain(1);
    expect(cache.allIdentifiers).toContain('loc_1');
  });
});
```

---

## Validation Commands

```bash
just frontend typecheck
just frontend lint
just frontend test stores/locations/
```

**Success Criteria**:
- All 30+ tests passing
- Zero type errors
- Zero lint errors
- Cache invariants maintained across all operations
