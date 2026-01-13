# Feature: TRA-266 - Scan RFID on Hardware Trigger

## Origin
This specification addresses Linear issue [TRA-266](https://linear.app/trakrf/issue/TRA-266/scan-rfid-on-hardware-trigger). The feature extends hardware trigger scanning (already working for the Barcode tab) to the RFID Tags field in asset/location forms.

## Outcome
Users can populate RFID tag EPCs in asset/location forms by pulling the CS108 physical trigger when the tag input is focused, without clicking UI buttons.

## User Story
As a warehouse worker commissioning assets
I want to pull the CS108 trigger to scan a tag barcode directly into the RFID field
So that I can work faster with one hand holding the device

## Context

**Current State**:
- BarcodeScreen: Hardware trigger scanning works - trigger press starts barcode scan, release stops
- Asset/Location Forms: Barcode scanning works via UI button only (`useScanToInput` hook)
- Gap: Forms don't respond to physical trigger presses

**Technical Discovery**:
The existing `useScanToInput` hook activates barcode mode but doesn't subscribe to trigger state. The BarcodeScreen has trigger subscription (lines 99-129 of BarcodeScreen.tsx), but forms don't.

**Related**: TRA-197 (Barcode scanning integration) - provides the foundation this feature extends.

## Technical Requirements

### 1. Focus-Aware Trigger Scanning

When RFID Tags input is focused AND user pulls physical trigger:
- Start barcode scan (same as clicking scan button)
- Scanned EPC populates the focused/active tag input
- Works without clicking UI scan button

### 2. Implementation Approach

Extend `useScanToInput` hook to:
- Accept `triggerEnabled?: boolean` option (default: false for backwards compatibility)
- When `triggerEnabled: true`:
  - Subscribe to `useDeviceStore.triggerState`
  - On trigger press → start scan (if not already scanning)
  - On trigger release → stop scan (if scanning)

### 3. Form Integration

Update AssetForm and LocationForm:
- Pass `triggerEnabled: true` to `useScanToInput`
- Track which tag input row is focused
- Route scanned EPC to the focused row (or create new row if none focused)

## Implementation Details

### Hook Enhancement: `useScanToInput.ts`

```typescript
interface UseScanToInputOptions {
  onScan: (value: string) => void;
  autoStop?: boolean;           // default: true
  returnMode?: ReaderModeType;  // default: IDLE
  triggerEnabled?: boolean;     // NEW - default: false
}
```

Add trigger subscription when `triggerEnabled: true`:
```typescript
// Subscribe to trigger state changes
useEffect(() => {
  if (!triggerEnabled || !isConnected) return;

  const triggerState = useDeviceStore.getState().triggerState;
  if (triggerState && !scanning) {
    startBarcodeScan();
  } else if (!triggerState && scanning) {
    stopBarcodeScan();
  }
}, [triggerState, triggerEnabled, isConnected, scanning]);
```

### Form Changes: `AssetForm.tsx` / `LocationForm.tsx`

```typescript
const { startBarcodeScan, stopScan } = useScanToInput({
  onScan: (epc) => handleBarcodeScan(epc),
  autoStop: true,
  triggerEnabled: true,  // NEW
});
```

### Focus Tracking (Optional Enhancement)

For multi-tag scenarios, track which TagIdentifierInputRow is focused:
```typescript
const [focusedTagIndex, setFocusedTagIndex] = useState<number | null>(null);

// Route scanned EPC to focused row, or append new row
const handleBarcodeScan = (epc: string) => {
  if (focusedTagIndex !== null && identifiers[focusedTagIndex]) {
    updateIdentifier(focusedTagIndex, epc);
  } else {
    appendIdentifier(epc);
  }
};
```

## Files to Modify

| File | Changes |
|------|---------|
| `frontend/src/hooks/useScanToInput.ts` | Add `triggerEnabled` option and trigger subscription |
| `frontend/src/components/assets/AssetForm.tsx` | Pass `triggerEnabled: true` |
| `frontend/src/components/locations/LocationForm.tsx` | Pass `triggerEnabled: true` |
| `frontend/src/components/assets/TagIdentifierInputRow.tsx` | Optional: Add focus callbacks |

## Validation Criteria

- [ ] Physical trigger on CS108 initiates scan when asset form is open and RFID field is in view
- [ ] Scanned EPC populates the active RFID tag input (or creates new row)
- [ ] Works without clicking UI scan button
- [ ] Existing UI button scanning continues to work
- [ ] Trigger scanning only active when form is mounted (cleanup on unmount)
- [ ] No scanning interference when navigating to other screens

## Edge Cases

1. **Multiple tag inputs**: If user has added multiple tag rows, scan should populate the focused one or append new
2. **Rapid triggers**: Debounce already handled in worker (100ms)
3. **Form unmount during scan**: Hook cleanup should stop scan and restore mode
4. **Already scanning via button**: Trigger should not interfere with button-initiated scan

## Out of Scope

- RFID inventory scanning via trigger (this is barcode-to-EPC only)
- Location form trigger scanning (same pattern, include in implementation)
- BarcodeScreen changes (already works)

## References

- TRA-197: Barcode scanning integration (foundation)
- `frontend/src/components/BarcodeScreen.tsx:99-129`: Trigger subscription pattern
- `frontend/src/hooks/useScanToInput.ts`: Hook to extend
- `frontend/src/worker/cs108/reader.ts:119-192`: Worker trigger handling
