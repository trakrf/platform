# Implementation Plan: TRA-266 - Scan RFID on Hardware Trigger

Generated: 2026-01-13
Specification: spec.md

## Understanding

Enable hardware trigger scanning for RFID tag inputs in asset/location forms. When a user focuses an RFID tag input field and pulls the CS108 physical trigger, initiate barcode scanning and populate the scanned EPC into the focused field. Include subtle visual feedback when trigger scanning is "armed" (input focused + device connected).

**Key constraints from clarifying questions:**
- Trigger only activates when RFID input is **focused** (not just form visible)
- Scanned EPC **replaces** the focused row's value
- Subtle visual indicator when trigger is armed

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/BarcodeScreen.tsx` (lines 99-129) - Trigger subscription pattern
- `frontend/src/hooks/useScanToInput.ts` - Hook to extend
- `frontend/src/stores/deviceStore.ts` - `triggerState` selector

**Files to Modify**:
- `frontend/src/hooks/useScanToInput.ts` - Add trigger support with focus awareness
- `frontend/src/components/assets/TagIdentifierInputRow.tsx` - Add focus callbacks + armed indicator
- `frontend/src/components/assets/AssetForm.tsx` - Wire up focus tracking and trigger mode
- `frontend/src/components/locations/LocationForm.tsx` - Same pattern as AssetForm

## Architecture Impact
- **Subsystems affected**: Frontend only (React components, hooks)
- **New dependencies**: None
- **Breaking changes**: None (additive, backwards compatible)

## Task Breakdown

### Task 1: Extend useScanToInput hook with trigger support

**File**: `frontend/src/hooks/useScanToInput.ts`
**Action**: MODIFY
**Pattern**: Reference `BarcodeScreen.tsx` lines 99-129 for trigger subscription

**Implementation**:

Add new options to interface:
```typescript
interface UseScanToInputOptions {
  onScan: (value: string) => void;
  autoStop?: boolean;
  returnMode?: typeof ReaderMode[keyof typeof ReaderMode];
  // NEW
  triggerEnabled?: boolean;  // Enable hardware trigger scanning
}

interface UseScanToInputReturn {
  // ... existing
  // NEW
  isTriggerArmed: boolean;  // True when ready for trigger (connected + mode set)
  setFocused: (focused: boolean) => void;  // Call on input focus/blur
}
```

Add focus and trigger state management:
```typescript
const [isFocused, setIsFocused] = useState(false);
const triggerState = useDeviceStore((s) => s.triggerState);

// Trigger subscription effect
useEffect(() => {
  if (!triggerEnabled || !isFocused || !isConnected) return;

  const handleTrigger = async () => {
    if (triggerState && !isScanningRef.current) {
      // Trigger pressed - start barcode scan
      await startBarcodeScan();
    } else if (!triggerState && isScanningRef.current) {
      // Trigger released - stop scan
      await stopScan();
    }
  };

  handleTrigger();
}, [triggerState, triggerEnabled, isFocused, isConnected]);

// Compute armed state for UI feedback
const isTriggerArmed = triggerEnabled && isFocused && isConnected && !isScanningRef.current;
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 2: Add focus callbacks and armed indicator to TagIdentifierInputRow

**File**: `frontend/src/components/assets/TagIdentifierInputRow.tsx`
**Action**: MODIFY

**Implementation**:

Add new props:
```typescript
interface TagIdentifierInputRowProps {
  // ... existing
  onFocus?: () => void;      // NEW: Called when input gains focus
  onBlur?: () => void;       // NEW: Called when input loses focus
  triggerArmed?: boolean;    // NEW: Show visual indicator when true
}
```

Add visual indicator and focus handlers to input:
```typescript
<input
  type="text"
  value={value}
  onChange={(e) => onValueChange(e.target.value)}
  onFocus={onFocus}
  onBlur={onBlur}
  disabled={disabled}
  placeholder="Enter tag number..."
  className={`flex-1 px-3 py-2 text-sm border rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 disabled:opacity-50 ${
    error
      ? 'border-red-500 focus:ring-red-500'
      : triggerArmed
        ? 'border-green-500 focus:ring-green-500 ring-2 ring-green-500/30'
        : 'border-gray-300 dark:border-gray-600 focus:ring-blue-500'
  }`}
/>

{/* Armed indicator - subtle icon next to input */}
{triggerArmed && (
  <span className="text-green-600 dark:text-green-400" title="Pull trigger to scan">
    <QrCode className="w-4 h-4" />
  </span>
)}
```

Add import for QrCode icon at top of file.

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 3: Wire trigger scanning in AssetForm

**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:

Add focus tracking state:
```typescript
const [focusedTagIndex, setFocusedTagIndex] = useState<number | null>(null);
```

Update useScanToInput call:
```typescript
const { startBarcodeScan, stopScan, isTriggerArmed, setFocused } = useScanToInput({
  onScan: (epc) => handleBarcodeScan(epc),
  autoStop: true,
  triggerEnabled: true,  // NEW
});

// Sync focus state with hook
useEffect(() => {
  setFocused(focusedTagIndex !== null);
}, [focusedTagIndex, setFocused]);
```

Update handleBarcodeScan to target focused row:
```typescript
const handleBarcodeScan = async (epc: string) => {
  setIsScanning(false);

  // If a tag row is focused, update that row's value
  if (focusedTagIndex !== null && tagIdentifiers[focusedTagIndex]) {
    // Local duplicate check (excluding current row)
    if (tagIdentifiers.some((t, i) => i !== focusedTagIndex && t.value === epc)) {
      toast.error('This tag is already in the list');
      return;
    }

    // Cross-asset duplicate check
    try {
      const response = await lookupApi.byTag('rfid', epc);
      const result = response.data.data;
      const name = result.asset?.name || result.location?.name || `${result.entity_type} #${result.entity_id}`;
      // Store which index to update when confirmed
      setConfirmModal({ isOpen: true, epc, assignedTo: name });
    } catch (error: unknown) {
      const axiosError = error as { response?: { status: number } };
      if (axiosError.response?.status === 404) {
        // Update focused row directly
        const updated = [...tagIdentifiers];
        updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: epc };
        setTagIdentifiers(updated);
        toast.success('Tag updated');
      } else {
        toast.error('Failed to check tag assignment');
      }
    }
    return;
  }

  // Original behavior: append new row (button-initiated scan)
  // ... existing code for appending
};
```

Update TagIdentifierInputRow usage:
```typescript
<TagIdentifierInputRow
  key={identifier.id ?? `new-${index}`}
  type={identifier.type}
  value={identifier.value}
  onFocus={() => setFocusedTagIndex(index)}
  onBlur={() => setFocusedTagIndex(null)}
  triggerArmed={isTriggerArmed && focusedTagIndex === index}
  onTypeChange={...}
  onValueChange={...}
  onRemove={...}
  disabled={loading}
/>
```

Also update handleConfirmReassign to handle focused row case:
```typescript
const handleConfirmReassign = () => {
  if (confirmModal) {
    if (focusedTagIndex !== null && tagIdentifiers[focusedTagIndex]) {
      // Update focused row
      const updated = [...tagIdentifiers];
      updated[focusedTagIndex] = { ...updated[focusedTagIndex], value: confirmModal.epc };
      setTagIdentifiers(updated);
      toast.success('Tag updated (will be reassigned on save)');
    } else {
      // Original: append new row
      setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: confirmModal.epc }]);
      toast.success('Tag added (will be reassigned on save)');
    }
  }
  setConfirmModal(null);
};
```

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 4: Wire trigger scanning in LocationForm

**File**: `frontend/src/components/locations/LocationForm.tsx`
**Action**: MODIFY
**Pattern**: Mirror Task 3 changes exactly

**Implementation**:
Same changes as AssetForm:
1. Add `focusedTagIndex` state
2. Update `useScanToInput` with `triggerEnabled: true`
3. Add `useEffect` to sync focus state
4. Update `handleBarcodeScan` to target focused row
5. Update `handleConfirmReassign` for focused row case
6. Pass focus callbacks and `triggerArmed` to TagIdentifierInputRow

**Validation**:
```bash
just frontend typecheck
just frontend lint
```

---

### Task 5: Final validation and manual testing

**Action**: VALIDATE

Run full validation suite:
```bash
just frontend validate
```

Manual testing checklist:
- [ ] Open asset create form
- [ ] Add a tag row manually
- [ ] Focus the tag input - should see green border/icon (armed indicator)
- [ ] Pull CS108 trigger - should start barcode scan
- [ ] Scan a tag barcode - EPC should populate the focused input
- [ ] Release trigger - scanning should stop
- [ ] Test with existing tag (duplicate check should work)
- [ ] Test LocationForm with same flow
- [ ] Button-initiated scan still works
- [ ] Navigating away stops scanning cleanly

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Trigger fires when user doesn't expect | Focus requirement prevents accidental scans |
| State desync between focus and hook | `useEffect` keeps focus state in sync |
| Memory leak from trigger subscription | Effect cleanup handles unsubscribe |

## Integration Points

- **Store updates**: Reads `triggerState` from `useDeviceStore` (no writes)
- **Route changes**: None
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
just frontend lint       # Gate 1: Style
just frontend typecheck  # Gate 2: Types
just frontend test       # Gate 3: Tests (if applicable)
```

**Do not proceed to next task until current task passes all gates.**

Final validation:
```bash
just frontend validate   # All checks
just frontend build      # Production build
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and clarifying questions
- ✅ Exact pattern exists in `BarcodeScreen.tsx:99-129`
- ✅ All clarifying questions answered (focus required, replace row, visual indicator)
- ✅ Simple additive changes to existing hook
- ✅ No new dependencies
- ✅ No backend changes required

**Assessment**: High confidence - this is a straightforward extension of existing patterns with clear requirements.

**Estimated one-pass success probability**: 90%

**Reasoning**: The trigger subscription pattern is well-established in BarcodeScreen. The main complexity is wiring focus tracking through the component hierarchy, which is standard React state management.
