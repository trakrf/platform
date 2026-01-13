# Implementation Plan: Update Inventory Asset Match to Use identifiers.value

Generated: 2026-01-12
Specification: spec.md

## Understanding

When RFID tags are scanned, the frontend currently tries to match them to assets by looking up the EPC in `assetStore.cache.byIdentifier`, which is keyed by `assets.identifier` (the customer's business ID). This is wrong - EPCs are stored in the `identifiers` table with `type='rfid'`.

**Solution**:
1. Add a batch lookup endpoint to the backend (`POST /api/v1/lookup/tags`)
2. Create a frontend API client for lookup
3. Update `tagStore` to batch unenriched tags and call the API with 500ms debounce
4. Store results in tagStore (acts as lookup cache)

**Key Design Decisions**:
- API-only approach (offline support deferred)
- Batch lookup with 500ms interval
- TagStore acts as cache - once enriched, no re-lookup needed
- Deduplication happens at tagStore level before queuing for lookup

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/lookup/lookup.go` (lines 37-77) - Single lookup handler pattern
- `backend/internal/storage/identifiers.go` (lines 275-308) - `LookupByTagValue` SQL pattern
- `frontend/src/lib/api/assets/index.ts` - API client object pattern
- `frontend/src/stores/tagStore.ts` (lines 233-289) - `addTag()` with enrichment pattern

**Files to Create**:
- `frontend/src/lib/api/lookup/index.ts` - Lookup API client
- `frontend/src/lib/api/lookup/index.test.ts` - API client tests

**Files to Modify**:
- `backend/internal/storage/identifiers.go` - Add `LookupByTagValues()` batch method
- `backend/internal/handlers/lookup/lookup.go` - Add `LookupByTags()` batch handler
- `frontend/src/stores/tagStore.ts` - Replace `getAssetByIdentifier()` with batched API lookup

## Architecture Impact

- **Subsystems affected**: Frontend (tagStore, API client), Backend (lookup handler, storage)
- **New dependencies**: None
- **Breaking changes**: None - existing single lookup remains, batch is additive

## Task Breakdown

### Task 1: Add Batch Lookup Storage Method

**File**: `backend/internal/storage/identifiers.go`
**Action**: MODIFY
**Pattern**: Reference `LookupByTagValue()` at lines 275-308

**Implementation**:
```go
// LookupByTagValues finds assets/locations for multiple tag values (batch)
// Returns a map of value -> LookupResult (nil if not found)
func (s *Storage) LookupByTagValues(ctx context.Context, orgID int, tagType string, values []string) (map[string]*LookupResult, error) {
    // Query: SELECT value, asset_id, location_id FROM identifiers
    //        WHERE org_id = $1 AND type = $2 AND value = ANY($3)
    // For each result, fetch asset/location and build result map
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 2: Add Batch Lookup Handler

**File**: `backend/internal/handlers/lookup/lookup.go`
**Action**: MODIFY
**Pattern**: Reference `LookupByTag()` at lines 37-77

**Implementation**:
```go
// Request body for batch lookup
type BatchLookupRequest struct {
    Type   string   `json:"type"`   // e.g., "rfid"
    Values []string `json:"values"` // EPCs to lookup
}

// @Summary Batch lookup entities by tags
// @Router /api/v1/lookup/tags [post]
func (h *Handler) LookupByTags(w http.ResponseWriter, r *http.Request) {
    // Parse JSON body
    // Call storage.LookupByTagValues()
    // Return map[string]LookupResult
}

func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Get("/api/v1/lookup/tag", h.LookupByTag)
    r.Post("/api/v1/lookup/tags", h.LookupByTags)  // Add this
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 3: Create Frontend Lookup API Client

**File**: `frontend/src/lib/api/lookup/index.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/api/assets/index.ts`

**Implementation**:
```typescript
import { apiClient } from '../client';
import type { Asset } from '@/types/assets';
import type { Location } from '@/types/locations';

export interface LookupResult {
  entity_type: 'asset' | 'location';
  entity_id: number;
  asset?: Asset;
  location?: Location;
}

export interface BatchLookupRequest {
  type: string;
  values: string[];
}

export type BatchLookupResponse = {
  data: Record<string, LookupResult | null>;
};

export const lookupApi = {
  /** Single tag lookup: GET /api/v1/lookup/tag?type=rfid&value={epc} */
  byTag: (type: string, value: string) =>
    apiClient.get<{ data: LookupResult }>(`/lookup/tag`, { params: { type, value } }),

  /** Batch tag lookup: POST /api/v1/lookup/tags */
  byTags: (request: BatchLookupRequest) =>
    apiClient.post<BatchLookupResponse>('/lookup/tags', request),
};
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 4: Update TagStore - Add Lookup Queue and Batch Mechanism

**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY
**Pattern**: Existing `addTag()` enrichment at lines 239-250

**Implementation**:

Add to state interface:
```typescript
// Internal queue for tags needing lookup (not persisted)
_lookupQueue: Set<string>;
_lookupTimer: ReturnType<typeof setTimeout> | null;
_isLookupInProgress: boolean;

// Actions
_queueForLookup: (epc: string) => void;
_flushLookupQueue: () => Promise<void>;
```

Modify `addTag()`:
```typescript
addTag: (tag) => set((state) => {
  // ... existing dedup logic ...

  // If this is a NEW tag (not existing), queue for lookup
  if (existingIndex < 0) {
    // Queue EPC for batch lookup (don't block)
    get()._queueForLookup(tag.epc || '');
  }

  // Return new tag WITHOUT asset data initially
  // Asset data added when batch lookup completes
});
```

Add batch lookup mechanism:
```typescript
_queueForLookup: (epc) => {
  const state = get();
  state._lookupQueue.add(epc);

  // Debounce: flush after 500ms of no new additions
  if (state._lookupTimer) clearTimeout(state._lookupTimer);
  set({ _lookupTimer: setTimeout(() => get()._flushLookupQueue(), 500) });
},

_flushLookupQueue: async () => {
  const state = get();
  if (state._isLookupInProgress || state._lookupQueue.size === 0) return;

  const epcs = Array.from(state._lookupQueue);
  set({ _lookupQueue: new Set(), _isLookupInProgress: true });

  try {
    const response = await lookupApi.byTags({ type: 'rfid', values: epcs });
    const results = response.data.data;

    // Update tags with asset info
    set((state) => ({
      tags: state.tags.map(tag => {
        const result = results[tag.epc];
        if (result?.asset) {
          return {
            ...tag,
            assetId: result.asset.id,
            assetName: result.asset.name,
            assetIdentifier: result.asset.identifier,
          };
        }
        return tag;
      })
    }));
  } finally {
    set({ _isLookupInProgress: false });
  }
},
```

Update `refreshAssetEnrichment()`:
```typescript
refreshAssetEnrichment: async () => {
  const state = get();
  // Get all EPCs that don't have assetId yet
  const unenriched = state.tags
    .filter(t => t.assetId === undefined)
    .map(t => t.epc);

  if (unenriched.length === 0) return;

  // Queue them all for lookup
  unenriched.forEach(epc => state._lookupQueue.add(epc));
  await get()._flushLookupQueue();
},
```

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 5: Remove Old getAssetByIdentifier Calls

**File**: `frontend/src/stores/tagStore.ts`
**Action**: MODIFY

**Implementation**:
Remove these lines from `addTag()` (lines 239-250):
```typescript
// DELETE THIS:
const assetStore = useAssetStore.getState();
let asset = assetStore.getAssetByIdentifier(tag.epc || '');
if (!asset) {
  asset = assetStore.getAssetByIdentifier(displayEpc);
}
const assetData = asset ? {...} : {};
```

The enrichment now happens asynchronously via `_flushLookupQueue()`.

**Validation**:
```bash
cd frontend && just lint && just typecheck && just test
```

---

### Task 6: Add Tests for Batch Lookup

**Files**:
- `frontend/src/lib/api/lookup/index.test.ts` (CREATE)
- Update `frontend/src/stores/tagStore.ts` tests if needed

**Action**: CREATE/MODIFY

**Implementation**:
```typescript
// index.test.ts
import { lookupApi } from './index';
import { apiClient } from '../client';
import { vi, describe, it, expect } from 'vitest';

vi.mock('../client');

describe('lookupApi', () => {
  it('byTags sends POST with type and values', async () => {
    const mockResponse = { data: { data: { 'EPC123': { entity_type: 'asset', ... } } } };
    vi.mocked(apiClient.post).mockResolvedValue(mockResponse);

    await lookupApi.byTags({ type: 'rfid', values: ['EPC123', 'EPC456'] });

    expect(apiClient.post).toHaveBeenCalledWith('/lookup/tags', {
      type: 'rfid',
      values: ['EPC123', 'EPC456'],
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

### Task 7: Final Validation

**Action**: VALIDATE

Run full validation suite:
```bash
just validate
```

Ensure:
- All backend tests pass
- All frontend tests pass
- Lint is clean
- TypeScript compiles
- Build succeeds

## Risk Assessment

- **Risk**: Batch lookup may timeout with very large batches (1000+ EPCs)
  **Mitigation**: Limit batch size to 100 EPCs max, chunk if needed

- **Risk**: Race condition between addTag and flush timer
  **Mitigation**: Use `_isLookupInProgress` flag, queue additions during lookup are processed in next batch

- **Risk**: Network failures leave tags unenriched
  **Mitigation**: `refreshAssetEnrichment()` can be called manually to retry; consider retry logic in `_flushLookupQueue`

## Integration Points

- **Store updates**: `tagStore` gains new internal state (`_lookupQueue`, `_lookupTimer`, `_isLookupInProgress`)
- **API client**: New `lookupApi` module in `frontend/src/lib/api/lookup/`
- **Route changes**: New `POST /api/v1/lookup/tags` endpoint
- **Config updates**: None required

## VALIDATION GATES (MANDATORY)

After EVERY code change, run from appropriate directory:

**Backend changes**:
```bash
cd backend && just lint && just test
```

**Frontend changes**:
```bash
cd frontend && just lint && just typecheck && just test
```

**Final validation**:
```bash
just validate
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: Run lint, typecheck, and test commands

Final validation:
```bash
just validate
just build
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and user clarifications
✅ Similar patterns found: `LookupByTagValue()` in identifiers.go, API client in assets/index.ts
✅ All clarifying questions answered (API-only, batch, 500ms, tagStore as cache)
✅ Existing test patterns to follow in assets_integration_test.go
✅ Backend lookup logic already correct, just adding batch variant

**Assessment**: Straightforward feature with clear patterns to follow. Main complexity is the batching/debounce logic in tagStore, but the approach is well-defined.

**Estimated one-pass success probability**: 85%

**Reasoning**: High confidence due to clear existing patterns, user-clarified requirements, and limited scope. The 15% risk comes from potential edge cases in the debounce timing and async state updates in the tagStore.
