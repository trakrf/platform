# Feature: Barcode Scan to RFID Identifier in Asset/Location Forms

## Origin
This specification addresses requirement #2 from TRA-197: "Barcode Scan in Asset/Location Forms"

## Outcome
Users can scan a barcode on an RFID tag to capture the EPC and automatically populate it as an RFID identifier in asset and location create/edit forms, with duplicate validation to prevent assigning the same tag to multiple assets.

## User Story
As a warehouse operator
I want to scan a barcode on an RFID tag while editing an asset
So that I can quickly and accurately link the tag to the asset without manual data entry

## Context

**Discovery**: Barcodes on RFID tags encode the same EPC value stored in the chip. Scanning the barcode is a precise way to capture a single tag's identifier, avoiding broadcast inventory reads that pick up every tag in range.

**Current State**:
- `BarcodeScreen.tsx` exists as a standalone scanning tab
- `useScanToInput` hook supports both RFID and barcode scanning with `onScan` callback
- `AssetForm.tsx` has scanner buttons but **only in create mode** (line 190: `!initialData`)
- `tagIdentifiers` array in form state handles RFID identifiers
- `TagIdentifierInputRow.tsx` renders individual tag input rows

**Desired State**:
- Scanner buttons visible in both create AND edit modes
- Barcode scan populates EPC into a new RFID identifier row
- Duplicate EPC validation before allowing assignment
- Same functionality in `LocationForm` component

## Technical Requirements

### 1. Remove Scanner Buttons from Asset Identifier Section
**File**: `frontend/src/components/assets/AssetForm.tsx`

The existing "Scan RFID" and "Scan Barcode" buttons (lines 184-222) populate the asset `identifier` field (e.g., "LAP-001"). This is the wrong target - scanning should populate RFID tag identifiers, not asset IDs.

**Remove entirely**: Delete the scanner buttons section from the asset identifier area (lines 184-222) and the associated `useScanToInput` hook usage.

### 2. Add Barcode Scan for Tag Identifiers
**File**: `frontend/src/components/assets/AssetForm.tsx`

Add new `useScanToInput` hook for barcode scanning to tag identifiers:
```typescript
const { startBarcodeScan, stopScan, isScanning, scanType } = useScanToInput({
  onScan: (epc) => addTagIdentifier(epc),
  autoStop: true,
});
```

**New helper function**:
```typescript
const addTagIdentifier = (epc: string) => {
  // Avoid duplicates within current form
  if (tagIdentifiers.some(t => t.value === epc)) {
    toast.warning('This EPC is already in the list');
    return;
  }
  setTagIdentifiers([
    ...tagIdentifiers,
    { type: 'rfid', value: epc }
  ]);
};
```

### 3. Add Barcode Scan Button to RFID Tags Section
**Location**: `AssetForm.tsx` lines 337-385 (RFID Tags section)

Add a barcode scan icon button next to the "Add Tag" button:
```tsx
<div className="flex items-center gap-2">
  <h4>RFID Tags</h4>
  {isConnected && (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      onClick={startBarcodeScan}
      disabled={isScanning}
      title="Scan barcode to add tag"
    >
      {isScanning ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <BarcodeIcon className="h-4 w-4" />
      )}
    </Button>
  )}
</div>
```

Show scanning feedback below the section header when active:
```tsx
{isScanning && (
  <div className="flex items-center gap-2 text-sm text-muted-foreground">
    <span>Scanning barcode...</span>
    <Button variant="ghost" size="sm" onClick={stopScan}>Cancel</Button>
  </div>
)}
```

### 4. Duplicate EPC Validation (Cross-Asset)
**New API endpoint needed**: `GET /api/v1/identifiers/check?value={epc}&type=rfid`

**Response**:
```json
{
  "exists": true,
  "assigned_to": {
    "type": "asset",
    "id": 123,
    "name": "Laptop-001"
  }
}
```

**Frontend handling in `addTagIdentifier`**:
```typescript
const addTagIdentifier = async (epc: string) => {
  // Local duplicate check
  if (tagIdentifiers.some(t => t.value === epc)) {
    toast.warning('This EPC is already in the list');
    return;
  }

  // Cross-asset duplicate check
  const check = await identifiersApi.checkDuplicate(epc, 'rfid');
  if (check.exists) {
    // Show confirmation dialog
    const confirmed = await showConfirmDialog({
      title: 'Tag Already Assigned',
      message: `This tag is assigned to ${check.assigned_to.name}. Reassign to this asset?`,
      confirmText: 'Reassign',
      cancelText: 'Cancel'
    });
    if (!confirmed) return;
  }

  setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: epc }]);
};
```

### 5. Apply Same Changes to LocationForm
**File**: `frontend/src/components/locations/LocationForm.tsx`

Mirror all changes from AssetForm:
- Enable scanner buttons in edit mode
- Add `tagScan` useScanToInput for tag identifiers
- Add barcode scan button to RFID Tags section
- Include duplicate validation

### 6. Backend: Identifier Duplicate Check Endpoint
**File**: `backend/internal/api/handlers/identifiers.go` (new or extend)

```go
// GET /api/v1/identifiers/check
func (h *IdentifierHandler) CheckDuplicate(w http.ResponseWriter, r *http.Request) {
    value := r.URL.Query().Get("value")
    idType := r.URL.Query().Get("type")

    identifier, err := h.service.FindByValue(r.Context(), value, idType)
    if err != nil {
        // Not found = no duplicate
        render.JSON(w, r, map[string]bool{"exists": false})
        return
    }

    // Return owner info
    render.JSON(w, r, map[string]any{
        "exists": true,
        "assigned_to": map[string]any{
            "type": identifier.EntityType,
            "id":   identifier.EntityID,
            "name": identifier.EntityName, // May need join
        },
    })
}
```

## UI/UX Requirements

### Scanner Button Placement
```
┌─────────────────────────────────────────┐
│ RFID Tags                    [📷] [+ Add Tag]
├─────────────────────────────────────────┤
│ [RFID] abc123def456...           [🗑️]  │
│ [RFID] ________________________________ │
└─────────────────────────────────────────┘

[📷] = Barcode scan icon button
```

### Scanning States
1. **Idle**: Button shows barcode icon
2. **Scanning**: Button shows spinner, "Scanning..." text appears
3. **Success**: Beep sound, new row added with EPC value
4. **Duplicate Warning**: Toast or dialog with reassign option

### Audio Feedback
Reuse existing `useBarcodeAudio` hook - beep plays automatically when barcode store updates.

## Files to Modify

| File | Change |
|------|--------|
| `frontend/src/components/assets/AssetForm.tsx` | Remove old scan buttons, add barcode scan to RFID Tags section |
| `frontend/src/components/locations/LocationForm.tsx` | Same changes as AssetForm |
| `frontend/src/api/identifiers.ts` | Add `checkDuplicate()` function |
| `backend/internal/api/handlers/identifiers.go` | Add duplicate check endpoint |
| `backend/internal/api/routes.go` | Register new route |

## Validation Criteria

- [ ] Old scanner buttons removed from asset identifier section
- [ ] Barcode scan button appears in RFID Tags section (create mode)
- [ ] Barcode scan button appears in RFID Tags section (edit mode)
- [ ] Button shows spinner while scanning, disabled state
- [ ] Scanning barcode adds new RFID identifier row with EPC value
- [ ] Beep sound plays on successful scan
- [ ] Scanning duplicate EPC within same form shows warning toast
- [ ] Scanning EPC assigned to another asset shows reassign confirmation dialog
- [ ] User can cancel reassign and EPC is not added
- [ ] User can confirm reassign and EPC is added
- [ ] Same functionality works in location create/edit forms
- [ ] Backend endpoint returns correct duplicate info with asset/location name

## Out of Scope (Handled by Other TRA-197 Subtasks)

- Barcode search in asset/location list screens (TRA-197 requirement #1)
- Removing standalone Barcode tab (TRA-197 requirement #3)

## Technical Notes

- Barcode is the *capture method*, not the identifier type
- Barcode value = EPC = `identifiers.value` where `type='rfid'`
- The `useScanToInput` hook already handles barcode scanning cleanly
- Reuse existing `useBarcodeAudio` for beep feedback (automatic when barcode store updates)

## Decisions

1. **Asset Identifier vs Tag Identifier**: Remove the existing scan buttons that populate the asset's `identifier` field. Scanning should only be used for RFID tag identifiers section.

2. **Barcode Only (No RFID Scan for Form Input)**:
   - **Barcode**: Point-and-shoot, deterministic - scan one tag, get that exact EPC ✅
   - **RFID**: Broadcast inventory mode, no discrimination - reads ALL tags in range and grabs whichever comes first ❌

   Current `useScanToInput` RFID implementation uses `ReaderMode.INVENTORY` (full broadcast) and simply takes `tags[0]`. No power control or filtering exists. This is unreliable for single-tag selection.

   **Future Enhancement**: Could add RFID scan support if we implement:
   - Low-power mode to limit read range
   - Single-tag mode or nearest-tag selection
   - Confirmation UX showing which tag was captured
