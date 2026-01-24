# Feature: TRA-314 - Inventory Save Flow and Persistence

## Origin
This specification is derived from Linear issue TRA-314, which is a sub-task of TRA-137 (Add Tracking Save to Inventory Screen). It focuses specifically on the backend endpoint and frontend integration layer for persisting scanned inventory.

## Outcome
Backend API endpoint and frontend mutation hook that enables saving scanned RFID tags to the `asset_scans` hypertable with proper org validation, batch insert, and success feedback (toast + button animation).

## User Story
As an inventory operator
I want to save my scanned assets to the database
So that tracking data persists for reporting and location history

## Context

### Current State
- `asset_scans` hypertable exists (`backend/migrations/000011_asset_scans.up.sql`)
- No backend endpoint for saving inventory scans
- No frontend mutation hook for inventory save
- TRA-313 provides LocationBar UI with location selection (in review)
- Save button exists in InventoryHeader but not wired up

### Desired State
- POST `/api/v1/inventory/save` endpoint with org validation
- `useInventorySave` hook following `useLocationMutations` pattern
- Save button wired to mutation
- Toast: "5 assets saved to Warehouse A - Rack 12"
- Clear button flash animation after save

## Technical Requirements

### 1. Backend Endpoint

**File**: `backend/internal/handlers/inventory/save.go`

**Route**: `POST /api/v1/inventory/save`

**Request Body**:
```json
{
  "location_id": 123,
  "asset_ids": [456, 789, ...],
  "identifier_scan_ids": [111, 222, ...]
}
```

**Validation**:
- `location_id` required and must belong to org
- `asset_ids` required, non-empty array
- All asset IDs must belong to org
- `identifier_scan_ids` optional (for traceability)

**Response**:
```json
{
  "data": {
    "count": 5,
    "location_id": 123,
    "location_name": "Warehouse A - Rack 12",
    "timestamp": "2026-01-23T20:30:00Z"
  }
}
```

**Handler Pattern** (follow `backend/internal/handlers/assets/assets.go`):
```go
type Handler struct {
    storage *storage.Storage
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
    // Get org from claims
    // Validate location ownership
    // Validate asset ownership (batch)
    // Batch insert into asset_scans
    // Return count + location name
}
```

### 2. Storage Layer

**File**: `backend/internal/storage/inventory.go` (new or extend existing)

**Method**:
```go
func (s *Storage) SaveInventoryScans(ctx context.Context, orgID int, req SaveInventoryRequest) (*SaveInventoryResult, error) {
    // Validate location belongs to org
    // Validate all assets belong to org
    // Batch INSERT into asset_scans
    // Return result with count and location name
}
```

**SQL Insert**:
```sql
INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id, identifier_scan_id)
VALUES ($1, $2, $3, $4, NULL, $5)
```
- `scan_point_id` is NULL for handheld scans (no fixed scanner)
- One row per asset in the batch
- Use transaction for atomicity

### 3. Frontend Mutation Hook

**File**: `frontend/src/hooks/inventory/useInventorySave.ts`

**Pattern** (follow `frontend/src/hooks/locations/useLocationMutations.ts`):
```typescript
import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';

interface SaveInventoryRequest {
  location_id: number;
  asset_ids: number[];
  identifier_scan_ids?: number[];
}

interface SaveInventoryResponse {
  count: number;
  location_id: number;
  location_name: string;
  timestamp: string;
}

export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest) => {
      const response = await inventoryApi.save(data);
      return response.data.data;
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
      // Trigger clear button animation
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
```

### 4. API Client

**File**: `frontend/src/lib/api/inventory.ts` (new or extend)

```typescript
export const inventoryApi = {
  save: (data: SaveInventoryRequest) =>
    axios.post<{ data: SaveInventoryResponse }>('/api/v1/inventory/save', data),
};
```

### 5. Save Button Wiring

**File**: Update `InventoryHeader.tsx` or `LocationBar.tsx`

```typescript
const { save, isSaving } = useInventorySave();

const handleSave = async () => {
  if (!selectedLocationId) return;

  const saveableAssets = tags
    .filter(t => t.type === 'asset' && t.assetId)
    .map(t => t.assetId!);

  await save({
    location_id: selectedLocationId,
    asset_ids: saveableAssets,
  });

  // Trigger clear button pulse animation
  triggerClearButtonPulse();
};
```

### 6. Clear Button Animation

CSS class for pulse animation:
```css
.pulse-attention {
  animation: pulse 0.5s ease-in-out 4;
}

@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.05); opacity: 0.8; }
}
```

## Validation Criteria

### Backend
- [ ] POST endpoint validates org ownership of location
- [ ] POST endpoint validates org ownership of all assets (batch check)
- [ ] Batch insert succeeds for multiple assets
- [ ] Response includes count and location name
- [ ] Returns 400 for missing location_id
- [ ] Returns 400 for empty asset_ids
- [ ] Returns 403 for location/assets not in org

### Frontend
- [ ] `useInventorySave` hook returns `save`, `isSaving`, `saveError`
- [ ] Toast shows count and location name on success
- [ ] Clear button pulses 4 times after save
- [ ] Save button shows loading state while saving
- [ ] Error toast on failure

### Integration
- [ ] Full flow: select location → click Save → toast → clear pulse
- [ ] E2E test for save flow with mock data

## Dependencies
- TRA-313: LocationBar UI (In Review) - provides location selection state
- TRA-305: Asset enrichment pattern (Done) - provides tag type classification

## File Structure

```
backend/
  internal/
    handlers/
      inventory/
        save.go          # NEW: Save endpoint handler
        routes.go        # NEW: Route registration
    storage/
      inventory.go       # NEW: Storage methods

frontend/
  src/
    hooks/
      inventory/
        useInventorySave.ts  # NEW: Mutation hook
    lib/
      api/
        inventory.ts     # NEW: API client
```

## Implementation Order

1. **Backend Storage Layer**
   - Add `SaveInventoryScans` method to storage
   - Implement batch insert with validation

2. **Backend Handler**
   - Create inventory handler package
   - Implement Save endpoint
   - Register routes

3. **Frontend API Client**
   - Create inventory API module

4. **Frontend Mutation Hook**
   - Create `useInventorySave` hook
   - Add toast notification

5. **UI Wiring**
   - Wire Save button to mutation
   - Add clear button pulse animation

6. **Testing**
   - Backend unit tests for handler
   - Frontend hook tests
   - Integration/E2E test for full flow
