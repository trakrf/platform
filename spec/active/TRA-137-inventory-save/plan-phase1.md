# Implementation Plan: TRA-312 - Tag Classification Infrastructure

Generated: 2026-01-23
Specification: spec.md (Phase 1 of TRA-137)
Linear Issue: [TRA-312](https://linear.app/trakrf/issue/TRA-312)

## Understanding

Add tag classification to detect location tags from scanned RFID EPCs. When a tag is scanned:
1. Check asset cache (existing behavior via API batch lookup)
2. Check location cache synchronously for EPC match
3. Set `type: 'asset' | 'location' | 'unknown'` based on results
4. On login, re-enrich existing tags to catch location matches

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/stores/tagStore.ts` (lines 247-300) - `addTag()` implementation
- `frontend/src/stores/tagStore.ts` (lines 422-428) - Auth subscription pattern
- `frontend/src/stores/locations/locationActions.ts` (lines 232-269) - `setLocations()` cache building
- `frontend/src/stores/locations/locationActions.ts` (lines 317-318) - `getLocationByIdentifier()` O(1) lookup

**Files to Create**:
- None (all changes are modifications)

**Files to Modify**:
- `frontend/src/stores/tagStore.ts` - Add type field, location lookup, re-enrichment
- `frontend/src/stores/locations/locationStore.ts` - Add `byTagEpc` index and `getLocationByTagEpc()` method
- `frontend/src/stores/locations/locationActions.ts` - Build EPC index in `setLocations()`
- `frontend/src/types/locations/index.ts` - Add `byTagEpc` to `LocationCache` type
- `frontend/src/stores/tagStore.test.ts` - Add tests for tag classification
- `frontend/src/stores/locations/locationStore.test.ts` - Add tests for EPC lookup

## Architecture Impact
- **Subsystems affected**: Frontend stores only (tagStore, locationStore)
- **New dependencies**: None
- **Breaking changes**: None - `type` field defaults to `'unknown'`, existing behavior preserved

## Task Breakdown

### Task 1: Extend LocationCache type with EPC index
**File**: `frontend/src/types/locations/index.ts`
**Action**: MODIFY
**Pattern**: Reference existing `byIdentifier` type pattern

**Implementation**:
```typescript
export interface LocationCache {
  byId: Map<number, Location>;
  byIdentifier: Map<string, Location>;
  byTagEpc: Map<string, Location>;  // NEW: EPC value → Location
  byParentId: Map<number | null, Set<number>>;
  // ... rest unchanged
}
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 2: Initialize byTagEpc in locationStore
**File**: `frontend/src/stores/locations/locationStore.ts`
**Action**: MODIFY
**Pattern**: Reference `initialCache` structure at lines 49-59

**Implementation**:
Add `byTagEpc: new Map()` to `initialCache`:
```typescript
const initialCache: LocationCache = {
  byId: new Map(),
  byIdentifier: new Map(),
  byTagEpc: new Map(),  // NEW
  byParentId: new Map(),
  // ... rest unchanged
};
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 3: Build EPC index in setLocations
**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: MODIFY
**Pattern**: Reference `setLocations` at lines 232-269

**Implementation**:
In `setLocations()`, after building other indexes, build EPC index from `location.identifiers`:
```typescript
setLocations: (locations: Location[]) =>
  set(() => {
    const cache: LocationCache = {
      byId: new Map(),
      byIdentifier: new Map(),
      byTagEpc: new Map(),  // NEW
      byParentId: new Map(),
      rootIds: new Set(),
      activeIds: new Set(),
      allIds: [],
      allIdentifiers: [],
      lastFetched: Date.now(),
      ttl: 0,
    };

    for (const location of locations) {
      const parentId = location.parent_location_id;
      updatePrimaryIndexes(cache, location);
      updateParentChildMapping(cache, location.id, parentId);
      updateRootSet(cache, location.id, parentId);
      updateActiveSet(cache, location);

      // NEW: Build EPC index from location's tag identifiers
      if (location.identifiers) {
        for (const identifier of location.identifiers) {
          if (identifier.is_active && identifier.type === 'rfid') {
            cache.byTagEpc.set(identifier.value, location);
          }
        }
      }
    }

    rebuildOrderedLists(cache);
    // ... rest unchanged (localStorage save)
    return { cache };
  }),
```

Also update `invalidateCache()` to clear `byTagEpc`:
```typescript
invalidateCache: () =>
  set(() => ({
    cache: {
      byId: new Map(),
      byIdentifier: new Map(),
      byTagEpc: new Map(),  // NEW
      byParentId: new Map(),
      // ... rest unchanged
    },
    // ... rest unchanged
  })),
```

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 4: Add getLocationByTagEpc query method
**File**: `frontend/src/stores/locations/locationActions.ts`
**Action**: MODIFY
**Pattern**: Reference `getLocationByIdentifier` at lines 317-319

**Implementation**:
Add to `createHierarchyQueries` return object:
```typescript
getLocationByTagEpc: (epc: string) => {
  return (get() as any).cache.byTagEpc.get(epc);
},
```

Also add to `LocationStore` interface in `locationStore.ts`:
```typescript
getLocationByTagEpc: (epc: string) => Location | undefined;
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 5: Add TagType and extend TagInfo interface
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference existing `TagInfo` interface at lines 14-37

**Implementation**:
```typescript
// Add type union above TagInfo
export type TagType = 'asset' | 'location' | 'unknown';

export interface TagInfo {
  epc: string;
  displayEpc?: string;
  // ... existing fields ...

  // Tag classification
  type: TagType;  // NEW

  // Asset fields (existing)
  assetId?: number;
  assetName?: string;
  assetIdentifier?: string;

  // Location fields (NEW)
  locationId?: number;
  locationName?: string;
}
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 6: Add location lookup to addTag
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference `addTag` at lines 247-300

**Implementation**:
Import locationStore at top:
```typescript
import { useLocationStore } from './locations/locationStore';
```

Modify `addTag` to check location cache synchronously:
```typescript
addTag: (tag) => {
  const epc = tag.epc || '';
  const state = get();
  const existingIndex = state.tags.findIndex(t => t.epc === epc);
  const isNewTag = existingIndex < 0;

  // NEW: Check location cache synchronously for new tags
  let tagType: TagType = 'unknown';
  let locationId: number | undefined;
  let locationName: string | undefined;

  if (isNewTag) {
    const location = useLocationStore.getState().getLocationByTagEpc(epc);
    if (location) {
      tagType = 'location';
      locationId = location.id;
      locationName = location.name;
    }
  }

  set((state) => {
    const now = Date.now();
    const displayEpc = removeLeadingZeros(epc);

    let newTags;
    if (existingIndex >= 0) {
      // Update existing tag (preserve existing type/enrichment)
      newTags = [...state.tags];
      newTags[existingIndex] = {
        ...newTags[existingIndex],
        ...tag,
        displayEpc,
        lastSeenTime: now,
        readCount: (newTags[existingIndex].readCount || 0) + 1,
        count: (newTags[existingIndex].count || 0) + 1,
        timestamp: now
      };
    } else {
      // Create new tag with classification
      const newTag: TagInfo = {
        epc,
        displayEpc,
        count: 1,
        source: 'rfid',
        type: tagType,  // NEW: Set type
        locationId,     // NEW: Set location fields
        locationName,
        firstSeenTime: now,
        lastSeenTime: now,
        readCount: 1,
        timestamp: now,
        ...tag,
      };
      newTags = [...state.tags, newTag];
    }

    const totalPages = Math.max(1, Math.ceil(newTags.length / state.pageSize));
    const validCurrentPage = state.currentPage > totalPages ? totalPages : state.currentPage;

    return {
      tags: newTags,
      totalPages,
      currentPage: validCurrentPage
    };
  });

  // Queue new tags for batch asset lookup (existing behavior)
  // Only queue if not already classified as location
  if (isNewTag && epc && tagType !== 'location') {
    get()._queueForLookup(epc);
  }
},
```

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 7: Update _flushLookupQueue to set type
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference `_flushLookupQueue` at lines 359-407

**Implementation**:
Update the tag enrichment in `_flushLookupQueue` to set `type: 'asset'`:
```typescript
// Update tags with asset info from lookup results
set((state) => ({
  tags: state.tags.map(tag => {
    const result = results[tag.epc];
    if (result?.asset) {
      return {
        ...tag,
        type: 'asset' as TagType,  // NEW: Set type
        assetId: result.asset.id,
        assetName: result.asset.name,
        assetIdentifier: result.asset.identifier,
        description: result.asset.description || undefined,
      };
    }
    return tag;
  })
}));
```

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 8: Add location re-enrichment on login
**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Reference auth subscription at lines 422-428

**Implementation**:
Extend existing auth subscription to re-enrich tags with location data:
```typescript
// Flush lookup queue when user logs in (for tags scanned while anonymous)
useAuthStore.subscribe((state, prevState) => {
  // Only react to login (false -> true transition)
  if (state.isAuthenticated && !prevState.isAuthenticated) {
    // User just logged in - flush any queued EPCs for asset enrichment
    useTagStore.getState()._flushLookupQueue();

    // NEW: Re-enrich tags with location data now that cache is populated
    // Small delay to ensure locationStore has been initialized by useLocations hook
    setTimeout(() => {
      useTagStore.getState()._enrichTagsWithLocations();
    }, 100);
  }
});
```

Add the `_enrichTagsWithLocations` internal action to TagState interface and implementation:
```typescript
// In TagState interface:
_enrichTagsWithLocations: () => void;

// In store implementation:
_enrichTagsWithLocations: () => {
  set((state) => ({
    tags: state.tags.map(tag => {
      // Only process unknown tags (not already classified)
      if (tag.type !== 'unknown') {
        return tag;
      }

      const location = useLocationStore.getState().getLocationByTagEpc(tag.epc);
      if (location) {
        return {
          ...tag,
          type: 'location' as TagType,
          locationId: location.id,
          locationName: location.name,
        };
      }
      return tag;
    })
  }));
},
```

**Validation**:
```bash
cd frontend && just typecheck && just test
```

---

### Task 9: Add unit tests for tag classification
**File**: `frontend/src/stores/tagStore.test.ts`
**Action**: MODIFY
**Pattern**: Reference existing test structure

**Implementation**:
Add test cases:
```typescript
describe('tag classification', () => {
  it('should set type to unknown for unrecognized tags', () => {
    useTagStore.getState().addTag({ epc: 'UNKNOWN123' });
    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('unknown');
  });

  it('should set type to location when EPC matches location tag', () => {
    // Setup: populate location cache with a location that has tag identifier
    useLocationStore.getState().setLocations([{
      id: 1,
      identifier: 'WH-A-R12',
      name: 'Warehouse A - Rack 12',
      identifiers: [{ id: 1, type: 'rfid', value: 'LOCATION123', is_active: true }],
      // ... other required fields
    }]);

    useTagStore.getState().addTag({ epc: 'LOCATION123' });
    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('location');
    expect(tag.locationId).toBe(1);
    expect(tag.locationName).toBe('Warehouse A - Rack 12');
  });

  it('should not queue location tags for asset lookup', () => {
    // Setup location cache
    useLocationStore.getState().setLocations([{
      id: 1,
      identifier: 'WH-A',
      name: 'Warehouse A',
      identifiers: [{ id: 1, type: 'rfid', value: 'LOCATIONEPC', is_active: true }],
    }]);

    useTagStore.getState().addTag({ epc: 'LOCATIONEPC' });

    // Verify tag was NOT queued for lookup
    expect(useTagStore.getState()._lookupQueue.size).toBe(0);
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

### Task 10: Add unit tests for location EPC lookup
**File**: `frontend/src/stores/locations/locationStore.test.ts`
**Action**: MODIFY
**Pattern**: Reference existing test at lines 177-185

**Implementation**:
Add test cases:
```typescript
describe('getLocationByTagEpc', () => {
  it('should return location by RFID tag EPC', () => {
    useLocationStore.getState().setLocations([{
      ...createTestLocation(1, 'WH-A', 'Warehouse A'),
      identifiers: [{ id: 1, type: 'rfid', value: '300833B2DDD9014000000001', is_active: true }],
    }]);

    const location = useLocationStore.getState().getLocationByTagEpc('300833B2DDD9014000000001');
    expect(location?.id).toBe(1);
  });

  it('should return undefined for non-existent EPC', () => {
    const location = useLocationStore.getState().getLocationByTagEpc('NONEXISTENT');
    expect(location).toBeUndefined();
  });

  it('should not index inactive tag identifiers', () => {
    useLocationStore.getState().setLocations([{
      ...createTestLocation(1, 'WH-A', 'Warehouse A'),
      identifiers: [{ id: 1, type: 'rfid', value: 'INACTIVE123', is_active: false }],
    }]);

    const location = useLocationStore.getState().getLocationByTagEpc('INACTIVE123');
    expect(location).toBeUndefined();
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

## Risk Assessment

- **Risk**: Location cache not populated when addTag is called (anonymous user)
  **Mitigation**: `_enrichTagsWithLocations()` runs on login to catch any missed matches

- **Risk**: Race condition between location fetch and tag enrichment
  **Mitigation**: 100ms delay before enrichment gives useLocations hook time to populate cache

- **Risk**: EPC format mismatch (leading zeros, case sensitivity)
  **Mitigation**: Store EPCs as-is; consider normalizing in future if needed

## Integration Points
- **Store updates**: tagStore gains `type` field, locationStore gains `byTagEpc` index
- **No route changes**
- **No config updates**

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
cd frontend && just lint      # Gate 1: Syntax & Style
cd frontend && just typecheck # Gate 2: Type Safety
cd frontend && just test      # Gate 3: Unit Tests
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task:
```bash
cd frontend && just validate
```

Final validation:
```bash
just validate  # Full stack validation
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- Similar patterns found in codebase (auth subscription, cache indexing)
- Clear requirements from spec and clarifying questions
- Existing test patterns to follow
- No new dependencies
- Single subsystem (frontend stores)

**Assessment**: Straightforward extension of existing patterns with clear precedent.

**Estimated one-pass success probability**: 85%

**Reasoning**: All patterns exist in codebase. Main risk is edge cases around EPC format matching and race conditions, both mitigated in design.
