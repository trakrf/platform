# Implementation Plan: Add Edit Button to Asset Details Modal

Generated: 2026-01-13
Specification: spec.md

## Understanding

Add an "Edit" button to the Asset Details modal footer that opens the Edit Asset modal directly, reducing the view-to-edit workflow from 3+ clicks to 1. Additionally, fix the tag input auto-focus behavior so focus only occurs on explicit "Add Tag" button clicks, not on form mount.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetDetailsModal.tsx` (lines 168-175) - Footer button pattern
- `frontend/src/components/AssetsScreen.tsx` (lines 59-61) - `handleEditAsset` handler to reuse

**Files to Modify**:
- `frontend/src/components/assets/AssetDetailsModal.tsx` - Add `onEdit` prop + Edit button
- `frontend/src/components/AssetsScreen.tsx` - Pass `onEdit` callback to modal
- `frontend/src/components/assets/AssetForm.tsx` - Remove auto-focus on mount

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add onEdit prop and Edit button to AssetDetailsModal

**File**: `frontend/src/components/assets/AssetDetailsModal.tsx`
**Action**: MODIFY
**Pattern**: Reference existing footer button at lines 168-175

**Implementation**:

1. Add `onEdit` prop to interface (line 8-12):
```typescript
interface AssetDetailsModalProps {
  asset: Asset | null;
  isOpen: boolean;
  onClose: () => void;
  onEdit?: (asset: Asset) => void;  // Add this
}
```

2. Destructure `onEdit` in component signature (line 14):
```typescript
export function AssetDetailsModal({ asset, isOpen, onClose, onEdit }: AssetDetailsModalProps) {
```

3. Add Edit button handler and button to footer (lines 168-175):
```typescript
<div className="flex justify-end gap-3 px-6 py-4 border-t border-gray-200 dark:border-gray-700">
  {onEdit && asset && (
    <button
      onClick={() => {
        onEdit(asset);
        onClose();
      }}
      className="px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 font-medium transition-colors"
    >
      Edit
    </button>
  )}
  <button
    onClick={onClose}
    className="px-4 py-2 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 font-medium transition-colors"
  >
    Close
  </button>
</div>
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

### Task 2: Wire up onEdit callback in AssetsScreen

**File**: `frontend/src/components/AssetsScreen.tsx`
**Action**: MODIFY
**Pattern**: Reference existing `handleEditAsset` at lines 59-61

**Implementation**:

Update the `AssetDetailsModal` usage (lines 190-194) to pass the existing handler:
```typescript
<AssetDetailsModal
  asset={viewingAsset}
  isOpen={!!viewingAsset}
  onClose={() => setViewingAsset(null)}
  onEdit={handleEditAsset}  // Add this line
/>
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

### Task 3: Remove auto-focus on form mount

**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:

Remove the `setAutoFocusIndex` calls that fire on mount:

1. Edit mode (line 85) - Remove this line:
```typescript
// DELETE: setAutoFocusIndex(existingTags.length);
```

2. Create mode (line 89) - Remove this line:
```typescript
// DELETE: setAutoFocusIndex(0);
```

The Add Tag button handler (lines 436-440) already sets `setAutoFocusIndex(newIndex)` correctly - this remains unchanged.

**Before** (lines 78-91):
```typescript
if (asset && mode === 'edit') {
  // ... existing code ...
  setTagIdentifiers([...existingTags, { type: 'rfid', value: '' }]);
  setAutoFocusIndex(existingTags.length); // Focus the new blank row  <-- DELETE
} else if (mode === 'create') {
  setTagIdentifiers([{ type: 'rfid', value: '' }]);
  setAutoFocusIndex(0);  // <-- DELETE
}
```

**After**:
```typescript
if (asset && mode === 'edit') {
  // ... existing code ...
  setTagIdentifiers([...existingTags, { type: 'rfid', value: '' }]);
  // Auto-focus removed - only Add Tag button triggers focus
} else if (mode === 'create') {
  setTagIdentifiers([{ type: 'rfid', value: '' }]);
  // Auto-focus removed - only Add Tag button triggers focus
}
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

### Task 4: Full validation

**Action**: VALIDATE

Run full frontend validation:
```bash
cd frontend && just validate
```

This runs lint, typecheck, test, and build.

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Edit button breaks modal layout on mobile | Existing `gap-3` and `flex` handle multiple buttons; test on mobile viewport |
| Focus removal affects accessibility | Tab order still works; users can manually focus inputs |

## Integration Points

- **Store updates**: None - reuses existing `handleEditAsset` which sets `editingAsset` state
- **Route changes**: None
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
cd frontend && just lint && just typecheck
```

After all tasks complete:
```bash
cd frontend && just validate
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

## Validation Sequence

| Task | Validation |
|------|------------|
| Task 1 | `just lint && just typecheck` |
| Task 2 | `just lint && just typecheck` |
| Task 3 | `just lint && just typecheck` |
| Task 4 | `just validate` (full suite) |

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 10/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and user answers
- ✅ Exact patterns found in codebase (button styling, handler pattern)
- ✅ All clarifying questions answered
- ✅ No new dependencies or architectural changes
- ✅ Simple prop addition and line deletions

**Assessment**: Straightforward UI enhancement with clear patterns to follow.

**Estimated one-pass success probability**: 95%

**Reasoning**: All changes are additive or simple deletions. Existing patterns are directly reusable. No complex state management or new abstractions needed.
