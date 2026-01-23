# Implementation Plan: TRA-314 - Inventory Save Flow and Persistence

Generated: 2026-01-23
Specification: spec.md

## Understanding

Implement the backend API endpoint and frontend integration layer for persisting scanned RFID inventory to the `asset_scans` hypertable. This completes the save workflow: user selects location → clicks Save → assets persisted → toast confirmation → clear button pulses.

**Key decisions from planning:**
- Skip `identifier_scan_ids` - natural key (timestamp + FKs) sufficient for analytics table
- Use 403 Forbidden for org ownership failures (not 400)
- State-driven animation in parent component with `onAnimationEnd` cleanup
- Save logic lives in page component, InventoryHeader stays presentational

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/locations/locations.go` (lines 71-112) - Handler pattern with claims, validation, storage call
- `backend/internal/storage/locations.go` (lines 14-41) - Storage method with transaction pattern
- `frontend/src/hooks/locations/useLocationMutations.ts` (lines 9-85) - React Query mutation hook pattern
- `frontend/src/lib/api/locations/index.ts` (lines 31-160) - API client pattern with apiClient wrapper

**Files to Create**:
- `backend/internal/handlers/inventory/save.go` - Save endpoint handler
- `backend/internal/storage/inventory.go` - Storage methods for asset_scans
- `frontend/src/lib/api/inventory.ts` - API client for inventory endpoints
- `frontend/src/hooks/inventory/useInventorySave.ts` - Mutation hook

**Files to Modify**:
- `backend/main.go` (lines 95-151) - Register inventory handler routes
- `frontend/src/pages/Inventory.tsx` - Wire save handler with animation state

## Architecture Impact

- **Subsystems affected**: Backend API, Frontend hooks/state
- **New dependencies**: None (using existing patterns)
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create inventory storage layer

**File**: `backend/internal/storage/inventory.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/storage/locations.go` lines 14-41

**Implementation**:
```go
package storage

// SaveInventoryRequest represents the request to save inventory scans
type SaveInventoryRequest struct {
    LocationID int
    AssetIDs   []int
}

// SaveInventoryResult represents the result of saving inventory scans
type SaveInventoryResult struct {
    Count        int       `json:"count"`
    LocationID   int       `json:"location_id"`
    LocationName string    `json:"location_name"`
    Timestamp    time.Time `json:"timestamp"`
}

func (s *Storage) SaveInventoryScans(ctx context.Context, orgID int, req SaveInventoryRequest) (*SaveInventoryResult, error) {
    // 1. Validate location belongs to org
    // 2. Validate all assets belong to org (batch query)
    // 3. Begin transaction
    // 4. Batch INSERT into asset_scans
    // 5. Return result with count and location name
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 2: Create inventory handler

**File**: `backend/internal/handlers/inventory/save.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/handlers/locations/locations.go` lines 71-112

**Implementation**:
```go
package inventory

type Handler struct {
    storage *storage.Storage
}

type SaveRequest struct {
    LocationID int   `json:"location_id" validate:"required"`
    AssetIDs   []int `json:"asset_ids" validate:"required,min=1"`
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
    // 1. Get org from claims (401 if missing)
    // 2. Decode and validate request (400 if invalid)
    // 3. Call storage.SaveInventoryScans
    // 4. Return 403 if ownership validation fails
    // 5. Return 201 with result on success
}

func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Post("/api/v1/inventory/save", h.Save)
}
```

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 3: Register inventory routes in main.go

**File**: `backend/main.go`
**Action**: MODIFY
**Pattern**: Reference lines 23-31 (imports) and 133-138 (route registration)

**Implementation**:
1. Add import: `inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"`
2. Add to setupRouter params: `inventoryHandler *inventoryhandler.Handler`
3. Add route registration inside auth group: `inventoryHandler.RegisterRoutes(r)`
4. Create handler in main(): `inventoryHandler := inventoryhandler.NewHandler(store)`

**Validation**:
```bash
cd backend && just lint && just build
```

---

### Task 4: Create frontend API client

**File**: `frontend/src/lib/api/inventory.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/lib/api/locations/index.ts` lines 31-50

**Implementation**:
```typescript
import { apiClient } from './client';

export interface SaveInventoryRequest {
  location_id: number;
  asset_ids: number[];
}

export interface SaveInventoryResponse {
  count: number;
  location_id: number;
  location_name: string;
  timestamp: string;
}

export const inventoryApi = {
  save: (data: SaveInventoryRequest) =>
    apiClient.post<{ data: SaveInventoryResponse }>('/inventory/save', data),
};
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 5: Create useInventorySave hook

**File**: `frontend/src/hooks/inventory/useInventorySave.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/hooks/locations/useLocationMutations.ts` lines 9-85

**Implementation**:
```typescript
import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { inventoryApi, type SaveInventoryRequest, type SaveInventoryResponse } from '@/lib/api/inventory';

export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      const response = await inventoryApi.save(data);
      return response.data.data;
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
    },
    onError: () => {
      toast.error('Failed to save inventory');
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 6: Wire save handler in Inventory page

**File**: `frontend/src/pages/Inventory.tsx`
**Action**: MODIFY

**Implementation**:
1. Import `useInventorySave` hook
2. Add state: `const [showClearPulse, setShowClearPulse] = useState(false)`
3. Get save function: `const { save, isSaving } = useInventorySave()`
4. Create handler:
```typescript
const handleSave = async () => {
  if (!resolvedLocationId) return;
  const saveableAssets = displayTags
    .filter(t => t.type === 'asset' && t.assetId)
    .map(t => t.assetId!);

  if (saveableAssets.length === 0) return;

  await save({ location_id: resolvedLocationId, asset_ids: saveableAssets });
  setShowClearPulse(true);
};
```
5. Update InventoryHeader props:
   - `onSave={handleSave}`
   - `isSaveDisabled={!resolvedLocationId || isSaving || saveableCount === 0}`
6. Add `onAnimationEnd` to Clear button to reset pulse state

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 7: Add pulse animation CSS

**File**: `frontend/src/index.css` or component-level styles
**Action**: MODIFY

**Implementation**:
```css
.pulse-attention {
  animation: pulse-attention 0.5s ease-in-out 4;
}

@keyframes pulse-attention {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.05); opacity: 0.8; }
}
```

**Validation**:
```bash
cd frontend && just lint && just build
```

---

### Task 8: Add backend unit tests

**File**: `backend/internal/handlers/inventory/save_test.go`
**Action**: CREATE

**Test cases**:
- Returns 400 for missing location_id
- Returns 400 for empty asset_ids
- Returns 401 for missing auth
- Returns 403 for location not in org
- Returns 403 for asset not in org
- Returns 201 with count and location name on success

**Validation**:
```bash
cd backend && just test
```

---

## Risk Assessment

- **Risk**: Location or asset IDs passed that don't exist
  **Mitigation**: Storage layer validates existence before insert; return clear error message

- **Risk**: Partial insert failure (some assets insert, some fail)
  **Mitigation**: Use transaction - all or nothing

- **Risk**: Large batch of assets causes timeout
  **Mitigation**: Batch INSERT is efficient; monitor and add pagination if needed later

## Integration Points

- **Store updates**: None (asset_scans is append-only, no cache invalidation needed)
- **Route changes**: New POST /api/v1/inventory/save endpoint
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
# Backend
cd backend && just lint && just build && just test

# Frontend
cd frontend && just lint && just typecheck && just test
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

## Validation Sequence

After each task: Run lint, typecheck/build, and test for the affected subsystem.

Final validation:
```bash
just validate
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and Linear issue
✅ Similar patterns found: locations handler, useLocationMutations hook
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ No new dependencies
✅ Database schema already exists

**Assessment**: Straightforward feature following established patterns with clear examples.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist in codebase, no novel architecture, clear validation criteria. Main risk is minor integration issues with page component state.
