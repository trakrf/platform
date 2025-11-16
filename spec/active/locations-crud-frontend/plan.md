# Implementation Plan: Phase 2 - Location State Management

Generated: 2025-11-15
Specification: phase2-spec.md

## Understanding

Phase 2 implements a Zustand state management store for hierarchical location data following the established asset store pattern. Key requirements:

**In-Memory Hierarchical Cache**:
- 6 optimized indexes for O(1) lookups: byId, byIdentifier, byParentId, rootIds, activeIds, allIds/allIdentifiers
- Cache invariants MUST be maintained across all operations
- Maps and Sets must be cloned for Zustand immutability

**Lightweight LocalStorage**:
- Only persist filter metadata (allIdentifiers array)
- NO full cache persistence (different from asset store)
- Save only after bulk setLocations() operation

**Strict Error Handling**:
- Throw errors for invalid operations (e.g., update non-existent location)
- No silent failures - catch bugs early

**3-File Pattern** (matching asset store):
- locationStore.ts - main store with types and initialization
- locationActions.ts - factory functions for cache/query/UI operations
- locationPersistence.ts - minimal LocalStorage for metadata only

## Relevant Files

**Reference Patterns** (existing code to follow):

- `frontend/src/stores/assets/assetStore.ts` (lines 1-120) - Store structure, type definitions, initialization pattern
- `frontend/src/stores/assets/assetActions.ts` (lines 1-100) - Factory functions pattern, Map/Set cloning for immutability
- `frontend/src/stores/assets/assetPersistence.ts` - Zustand persist() middleware with custom storage
- `frontend/src/stores/assets/assetStore.test.ts` (lines 1-150) - Test structure, beforeEach pattern, mock data factory

**Phase 1 Dependencies** (already complete):

- `frontend/src/types/locations/index.ts` - LocationCache, Location, LocationFilters, PaginationState, LocationSort types
- `frontend/src/lib/location/filters.ts` - filterLocations(), sortLocations(), paginateLocations() functions
- `frontend/src/lib/location/transforms.ts` - serializeCache(), deserializeCache() for Map/Set conversion (NOT used in lightweight approach)

**Files to Create**:

- `frontend/src/stores/locations/locationStore.ts` - Main store (types, state, initialization)
- `frontend/src/stores/locations/locationActions.ts` - Cache operations and hierarchy queries (~250 lines)
- `frontend/src/stores/locations/locationPersistence.ts` - Minimal metadata persistence (~60 lines)
- `frontend/src/stores/locations/locationStore.test.ts` - Comprehensive test suite (~400 lines, 30+ tests)

**Files to Modify**:

None - this is a new store isolated from existing code.

## Architecture Impact

- **Subsystems affected**: Frontend state management only
- **New dependencies**: None (zustand already exists)
- **Breaking changes**: None (new feature)

## Task Breakdown

### Task 1: Create locationStore.ts skeleton

**File**: `frontend/src/stores/locations/locationStore.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/stores/assets/assetStore.ts` lines 1-120

**Implementation**:

```typescript
import { create } from 'zustand';
import type {
  Location,
  LocationCache,
  LocationFilters,
  LocationSort,
  PaginationState,
} from '@/types/locations';
import {
  filterLocations,
  sortLocations,
  paginateLocations,
} from '@/lib/location/filters';
import { createCacheActions, createHierarchyQueries, createUIActions } from './locationActions';
import { createLocationPersistence } from './locationPersistence';

export interface LocationStore {
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

const initialCache: LocationCache = {
  byId: new Map(),
  byIdentifier: new Map(),
  byParentId: new Map(),
  rootIds: new Set(),
  activeIds: new Set(),
  allIds: [],
  allIdentifiers: [],
  lastFetched: 0,
  ttl: 0,
};

const initialFilters: LocationFilters = {
  search: '',
  identifier: '',
  is_active: 'all',
};

const initialPagination: PaginationState = {
  currentPage: 1,
  pageSize: 10,
  totalCount: 0,
  totalPages: 0,
};

const initialSort: LocationSort = {
  field: 'identifier',
  direction: 'asc',
};

export const useLocationStore = create<LocationStore>()(
  createLocationPersistence((set, get) => ({
    cache: initialCache,
    selectedLocationId: null,
    filters: initialFilters,
    pagination: initialPagination,
    sort: initialSort,
    isLoading: false,
    error: null,

    ...createCacheActions(set, get),
    ...createHierarchyQueries(set, get),
    ...createUIActions(set, get),
  }))
);
```

**Validation**:
```bash
cd frontend
just typecheck
just lint
```

**Success**: Zero type errors, zero lint errors.

---

### Task 2: Implement cache operations in locationActions.ts

**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/stores/assets/assetActions.ts` lines 12-120 for factory function pattern and Map/Set cloning

**Critical**: All cache operations MUST clone Maps and Sets for Zustand immutability. Follow asset store pattern exactly.

**Validation**:
```bash
cd frontend
just typecheck
just lint
```

**Success**: Zero type errors, Map/Set cloning verified.

---

### Task 3: Implement hierarchy queries in locationActions.ts

**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: MODIFY (add createHierarchyQueries factory)
**Pattern**: New pattern - hierarchy-specific queries

**Key considerations**:
- getDescendants MUST use visited Set to prevent infinite loops
- getAncestors MUST use visited Set to detect cycles
- All queries filter undefined values from results

**Validation**:
```bash
cd frontend
just typecheck
just lint
```

**Success**: Zero type errors, circular reference protection verified.

---

### Task 4: Implement UI actions in locationActions.ts

**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: MODIFY (add createUIActions factory)
**Pattern**: Reference `frontend/src/stores/assets/assetActions.ts` lines 180-250 for UI actions

**Key feature**: setFilters resets pagination to page 1 (same as asset store)

**Validation**:
```bash
cd frontend
just typecheck
just lint
```

**Success**: Zero type errors.

---

### Task 5: Create minimal persistence in locationPersistence.ts

**File**: `frontend/src/stores/locations/locationPersistence.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/stores/assets/assetPersistence.ts` but simplified (metadata only)

**Key differences from asset store**:
- NO zustand persist() middleware (different from assets)
- Manual LocalStorage load in initializer only
- Manual save triggered only in setLocations()
- Only stores allIdentifiers array (not full cache)

**Validation**:
```bash
cd frontend
just typecheck
just lint
```

**Success**: Zero type errors, LocalStorage access safe.

---

### Task 6: Create comprehensive test suite

**File**: `frontend/src/stores/locations/locationStore.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/stores/assets/assetStore.test.ts` for structure

**Test Coverage** (30+ tests):

**Cache Operations** (11 tests):
- Add location to all indexes
- Add child to byParentId index
- Update identifier removes old, adds new
- Re-parenting updates all indexes
- Active status change updates activeIds
- Throw error updating non-existent location
- Delete removes from all indexes
- Throw error deleting non-existent location
- setLocations rebuilds from scratch
- clearCache empties all indexes
- setLocations saves metadata to LocalStorage

**Hierarchy Queries** (10 tests):
- getLocationById returns correct location
- getLocationByIdentifier returns correct location
- getChildren returns immediate children only
- getChildren returns empty for leaf nodes
- getDescendants returns all descendants recursively
- getAncestors returns in root-first order
- getAncestors returns empty for roots
- getRootLocations returns only roots
- getActiveLocations returns only active
- getActiveLocations excludes deactivated

**UI State** (7 tests):
- setSelectedLocation updates state
- setFilters updates filters
- setFilters resets pagination to page 1
- setSort updates sort
- resetFilters clears all filters
- setLoading updates state
- setError updates state

**Integration** (3 tests):
- getFilteredLocations applies filters
- getSortedLocations sorts correctly
- getPaginatedLocations paginates correctly

**Validation**:
```bash
cd frontend
just test stores/locations/
```

**Success**: All 30+ tests passing.

---

### Task 7: Final validation

**Action**: Run complete frontend validation suite

**Commands**:
```bash
cd frontend
just typecheck
just lint
just test stores/locations/
just validate
```

**Success Criteria**:
- Zero type errors
- Zero lint errors
- All 30+ location store tests passing
- All existing frontend tests still passing

---

## Risk Assessment

**Risk**: Re-parenting logic creates invalid cache state
**Mitigation**: Comprehensive tests for parent changes, throw errors on invalid operations

**Risk**: Infinite loop in getDescendants due to circular references
**Mitigation**: Visited Set tracks processed nodes, breaks on cycles

**Risk**: Forgetting to clone Maps/Sets breaks Zustand immutability
**Mitigation**: Follow asset store pattern exactly, create new Map/Set in every mutation

**Risk**: LocalStorage quota exceeded
**Mitigation**: Only store minimal metadata (identifiers array), catch and log errors

**Risk**: Test pollution from shared store state
**Mitigation**: clearCache() in beforeEach, use isolated mock data

---

## Integration Points

**Store dependencies**:
- Types from Phase 1: Location, LocationCache, LocationFilters, etc.
- Functions from Phase 1: filterLocations, sortLocations, paginateLocations

**LocalStorage**:
- Key: 'location-metadata'
- Saves only allIdentifiers array after setLocations()
- Loads on store initialization

**No changes to**:
- API layer (not yet implemented)
- UI components (Phase 3+)
- Routes (Phase 3+)

---

## VALIDATION GATES (MANDATORY)

**After EVERY task**:
```bash
cd frontend
just typecheck
just lint
```

**After Task 6 (tests)**:
```bash
cd frontend
just test stores/locations/
```

**Final validation**:
```bash
cd frontend
just validate
```

**Enforcement**: If ANY gate fails → Fix immediately, re-run, repeat until pass.

---

## Validation Sequence

1. **Task 1**: typecheck + lint
2. **Task 2**: typecheck + lint
3. **Task 3**: typecheck + lint
4. **Task 4**: typecheck + lint
5. **Task 5**: typecheck + lint
6. **Task 6**: typecheck + lint + test stores/locations/
7. **Task 7**: full validate

---

## Plan Quality Assessment

**Complexity Score**: 4/10 (MEDIUM-LOW)

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in assetStore.ts, assetActions.ts, assetPersistence.ts
✅ All clarifying questions answered (separate files, metadata-only persistence, ~30 tests, throw errors)
✅ Existing test patterns to follow at assetStore.test.ts
✅ Phase 1 dependencies all complete and verified
✅ Well-defined cache invariants
⚠️ Hierarchy queries are new (not in asset store) - slight complexity increase

**Assessment**: High confidence implementation. Asset store provides excellent pattern to follow. Main risk is hierarchy-specific logic (re-parenting, recursive descendants), but comprehensive tests will catch issues.

**Estimated one-pass success probability**: 85%

**Reasoning**: Following proven asset store pattern reduces risk significantly. 3-file structure is familiar. Hierarchy queries add some complexity but are well-defined. Comprehensive test suite will catch cache invariant violations. Error throwing strategy will surface bugs immediately rather than silent failures.
