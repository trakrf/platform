# Implementation Plan: Barcode Scan to RFID Identifier in Asset/Location Forms

Generated: 2025-01-13
Specification: spec.md

## Understanding

Enable users to scan a barcode on an RFID tag while creating/editing an asset or location, automatically populating the EPC as an RFID identifier. Uses existing `lookupApi.byTag()` for duplicate detection - 404 means no duplicate, 200 means already assigned elsewhere.

**Key decisions from spec review:**
1. Remove existing scan buttons from identifier field (wrong target)
2. Barcode-only (no RFID scan - broadcast mode is non-deterministic)
3. Use existing lookup API for duplicate check (no backend changes)
4. Inline feedback with Cancel button (matches existing pattern)
5. Enhance ConfirmModal with `confirmText` prop for reassign dialog

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetForm.tsx` (lines 207-222) - existing scan feedback pattern
- `frontend/src/hooks/useScanToInput.ts` - barcode scanning hook
- `frontend/src/lib/api/lookup/index.ts` - `lookupApi.byTag()` for duplicate check
- `frontend/src/components/shared/modals/ConfirmModal.tsx` - base modal to enhance

**Files to Modify:**
- `frontend/src/components/shared/modals/ConfirmModal.tsx` - add `confirmText` prop
- `frontend/src/components/assets/AssetForm.tsx` - remove old buttons, add barcode scan to RFID Tags section
- `frontend/src/components/locations/LocationForm.tsx` - mirror AssetForm changes

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None
- **Breaking changes**: None
- **Backend changes**: None (using existing lookup API)

## Task Breakdown

### Task 1: Enhance ConfirmModal with confirmText prop
**File**: `frontend/src/components/shared/modals/ConfirmModal.tsx`
**Action**: MODIFY

**Implementation**:
```typescript
interface ConfirmModalProps {
  isOpen: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  title: string;
  message: string;
  confirmText?: string;  // NEW - defaults to "Confirm"
}

// In button:
<button onClick={onConfirm} ...>
  {confirmText || 'Confirm'}
</button>
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`

---

### Task 2: Remove old scan buttons from AssetForm identifier section
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Changes**:
1. Remove `useScanToInput` hook usage (lines 43-46)
2. Remove scanner buttons section (lines 183-222)
3. Remove `isScanning` and `scanType` from input placeholder logic
4. Keep `isConnected` import for Task 3

**What to delete** (lines 183-222):
- Scanner buttons div (`mode === 'create' && isConnected && !isScanning`)
- Scanning state feedback div (`isScanning && ...`)

**Validation**:
- `just frontend lint`
- `just frontend typecheck`

---

### Task 3: Add barcode scan to AssetForm RFID Tags section
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:

1. Add imports:
```typescript
import { QrCode, Loader2 } from 'lucide-react';
import { useScanToInput } from '@/hooks/useScanToInput';
import { lookupApi } from '@/lib/api/lookup';
import { ConfirmModal } from '@/components/shared/modals/ConfirmModal';
import toast from 'react-hot-toast';
```

2. Add state and hook (after existing state declarations ~line 34):
```typescript
const [confirmModal, setConfirmModal] = useState<{
  isOpen: boolean;
  epc: string;
  assignedTo: string;
} | null>(null);

const { startBarcodeScan, stopScan, isScanning } = useScanToInput({
  onScan: (epc) => handleBarcodeScan(epc),
  autoStop: true,
});
```

3. Add handler function (before validateForm):
```typescript
const handleBarcodeScan = async (epc: string) => {
  // Local duplicate check
  if (tagIdentifiers.some(t => t.value === epc)) {
    toast.error('This tag is already in the list');
    return;
  }

  // Cross-asset duplicate check via lookup API
  try {
    const response = await lookupApi.byTag('rfid', epc);
    // 200 = found, show reassign confirmation
    const result = response.data.data;
    const name = result.asset?.name || result.location?.name || `${result.entity_type} #${result.entity_id}`;
    setConfirmModal({ isOpen: true, epc, assignedTo: name });
  } catch (error: any) {
    if (error.response?.status === 404) {
      // Not found = no duplicate, add directly
      setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: epc }]);
      toast.success('Tag added');
    } else {
      toast.error('Failed to check tag assignment');
    }
  }
};

const handleConfirmReassign = () => {
  if (confirmModal) {
    setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: confirmModal.epc }]);
    toast.success('Tag added (will be reassigned on save)');
  }
  setConfirmModal(null);
};
```

4. Modify RFID Tags section header (around line 339):
```tsx
<div className="flex items-center justify-between mb-4">
  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300">
    RFID Tags
  </label>
  <div className="flex items-center gap-2">
    {isConnected && (
      <button
        type="button"
        onClick={isScanning ? stopScan : startBarcodeScan}
        disabled={loading}
        className={`flex items-center gap-1 px-3 py-1.5 text-sm font-medium rounded-lg transition-colors disabled:opacity-50 ${
          isScanning
            ? 'text-red-600 hover:text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20'
            : 'text-green-600 hover:text-green-700 dark:text-green-400 hover:bg-green-50 dark:hover:bg-green-900/20'
        }`}
      >
        {isScanning ? (
          <>
            <Loader2 className="w-4 h-4 animate-spin" />
            Cancel
          </>
        ) : (
          <>
            <QrCode className="w-4 h-4" />
            Scan
          </>
        )}
      </button>
    )}
    <button
      type="button"
      onClick={() => setTagIdentifiers([...tagIdentifiers, { type: 'rfid', value: '' }])}
      disabled={loading}
      className="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded-lg transition-colors disabled:opacity-50"
    >
      <Plus className="w-4 h-4" />
      Add Tag
    </button>
  </div>
</div>
```

5. Add scanning feedback (after the header div, before the tag list):
```tsx
{isScanning && (
  <div className="flex items-center gap-2 mb-4 p-3 bg-blue-50 dark:bg-blue-900/20 rounded-lg">
    <Loader2 className="w-4 h-4 animate-spin text-blue-600 dark:text-blue-400" />
    <span className="text-sm text-blue-600 dark:text-blue-400">
      Scanning barcode... Point at tag barcode
    </span>
  </div>
)}
```

6. Add ConfirmModal at end of form (before closing </form>):
```tsx
{confirmModal && (
  <ConfirmModal
    isOpen={confirmModal.isOpen}
    title="Tag Already Assigned"
    message={`This tag is currently assigned to "${confirmModal.assignedTo}". Do you want to reassign it to this asset?`}
    confirmText="Reassign"
    onConfirm={handleConfirmReassign}
    onCancel={() => setConfirmModal(null)}
  />
)}
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 4: Apply same changes to LocationForm
**File**: `frontend/src/components/locations/LocationForm.tsx`
**Action**: MODIFY

Mirror all changes from Task 2 and Task 3:
1. Remove old scan buttons from identifier section (lines 203-242)
2. Add imports (QrCode, Loader2, lookupApi, ConfirmModal, toast)
3. Add confirmModal state and useScanToInput hook
4. Add handleBarcodeScan and handleConfirmReassign functions
5. Modify Tag Identifiers section header with scan button
6. Add scanning feedback
7. Add ConfirmModal

**Note**: LocationForm uses "Tag Identifiers" label (line 365) vs AssetForm's "RFID Tags"

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 5: Manual testing verification
**Action**: TEST

Test scenarios:
1. Create asset → scan barcode → tag added
2. Edit asset → scan barcode → tag added
3. Scan duplicate EPC in same form → toast warning
4. Scan EPC assigned to another asset → reassign dialog appears
5. Click Cancel in reassign dialog → tag not added
6. Click Reassign in reassign dialog → tag added
7. Same tests for Location forms
8. Cancel button stops scanning

**Validation**:
- `just frontend build`
- Manual testing on device

## Risk Assessment

- **Risk**: Barcode scanner not in expected mode when form opens
  **Mitigation**: `useScanToInput` handles mode switching automatically, returns to IDLE on unmount

- **Risk**: Network latency during duplicate check feels slow
  **Mitigation**: Show toast.loading() during API call if needed (can add in polish pass)

## VALIDATION GATES (MANDATORY)

After EVERY task:
1. `just frontend lint` - must pass
2. `just frontend typecheck` - must pass
3. `just frontend test` - must pass

Final validation:
- `just frontend build` - must succeed
- `just validate` - full stack validation

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Existing `useScanToInput` hook handles all scanner complexity
- ✅ Existing `lookupApi.byTag()` for duplicate check - no backend work
- ✅ Existing `ConfirmModal` component to enhance
- ✅ AssetForm and LocationForm are nearly identical - predictable changes
- ✅ Existing scan feedback pattern to follow (lines 207-222)

**Assessment**: Straightforward UI changes following established patterns with no backend work needed.

**Estimated one-pass success probability**: 90%

**Reasoning**: All pieces exist - just wiring them together differently. The only uncertainty is ensuring the modal state management integrates cleanly with the async scan handler.
