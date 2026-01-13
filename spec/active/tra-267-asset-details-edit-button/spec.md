# Feature: Add Edit Button to Asset Details Modal

## Origin
Linear issue TRA-267 - reducing friction in the view-then-edit workflow.

## Outcome
Users can seamlessly transition from viewing asset details to editing without navigating back to the asset list.

## User Story
As a **warehouse operator**
I want to **click Edit directly from the Asset Details modal**
So that **I can quickly update asset information without extra navigation steps**

## Context

**Current State:**
- `AssetDetailsModal` is view-only with Close button (X in header + "Close" in footer)
- To edit, user must: Close modal → Find asset in list → Click pencil icon
- Edit state managed in `AssetsScreen` via `editingAsset` state

**Desired State:**
- Add "Edit" button to `AssetDetailsModal` footer, next to "Close"
- Clicking Edit closes Details modal and opens `AssetFormModal` in edit mode
- Same workflow as clicking the pencil icon, just fewer steps

## Technical Requirements

### Component Changes

**1. AssetDetailsModal.tsx**
- Add `onEdit?: (asset: Asset) => void` prop
- Add "Edit" button to footer, positioned before "Close"
- Button styling should match primary action pattern (blue/indigo)

**2. AssetsScreen.tsx**
- Pass `onEdit={handleEditAsset}` to `AssetDetailsModal`
- Handler already exists - reuse the same function used by `AssetCard`

### UI Specification

**Button Layout (footer):**
```
[Edit]  [Close]
  ↑        ↑
primary  secondary
```

**Edit Button Styling** (match existing primary patterns):
```typescript
className="px-4 py-2 bg-indigo-600 text-white rounded-lg
           hover:bg-indigo-700 font-medium transition-colors"
```

**Close Button Styling** (existing - no change):
```typescript
className="px-4 py-2 bg-gray-200 dark:bg-gray-700
           text-gray-900 dark:text-gray-100 rounded-lg
           hover:bg-gray-300 dark:hover:bg-gray-600
           font-medium transition-colors"
```

### Behavior

1. User opens Asset Details modal (viewing)
2. User clicks "Edit" button
3. `onEdit(asset)` callback fires
4. Details modal closes (`onClose` should be called internally or via parent)
5. Edit modal opens with asset data
6. Edit modal fetches fresh data (existing behavior - prevents stale state)

## Code References

| File | Purpose |
|------|---------|
| `frontend/src/components/assets/AssetDetailsModal.tsx` | Add Edit button + prop |
| `frontend/src/components/AssetsScreen.tsx` | Wire up onEdit callback |
| `frontend/src/components/assets/AssetForm.tsx` | Move setFocus to Add Tag handler |

## Validation Criteria

**Edit Button:**
- [ ] Edit button appears in Asset Details modal footer
- [ ] Edit button styled as primary action (indigo/blue)
- [ ] Close button remains as secondary action (gray)
- [ ] Clicking Edit closes Details and opens Edit modal
- [ ] Edit modal shows correct asset data
- [ ] Keyboard accessibility preserved (tab order: Edit → Close)
- [ ] Dark mode styling correct for both buttons

**Tag Focus Fix:**
- [ ] Opening Edit modal does NOT auto-focus the tag input field
- [ ] Clicking "Add Tag" button DOES focus the new tag input field

## Edge Cases

- **Asset deleted while viewing**: Edit modal handles this (existing error handling)
- **Rapid clicks**: Modal transition handles this naturally via state
- **Mobile view**: Buttons should stack or fit comfortably (existing responsive patterns)

## Additional UX Fix: Tag Input Focus Behavior

**Problem:** Currently, focus is set on the tag number input when a row is added. This fires both on explicit "Add Tag" button clicks AND on auto-add during edit modal mount, causing unexpected focus jumps.

**Solution:** Move the `setFocus` call from the row-add logic to the Add Tag button handler only.

**Current behavior:**
```typescript
// In row add logic (fires on mount + button click)
append({ ... });
setFocus(`identifiers.${index}.identifier`);  // ❌ Always focuses
```

**Desired behavior:**
```typescript
// In row add logic
append({ ... });
// No focus here

// In Add Tag button handler only
const handleAddTag = () => {
  append({ ... });
  setFocus(`identifiers.${fields.length}.identifier`);  // ✅ Only on explicit click
};
```

**Files affected:** `AssetForm.tsx` (or wherever tag row management lives)

## Out of Scope

- Inline editing within Details modal
- Combining modals into single view/edit modal
- Any changes to AssetFormModal behavior

## Implementation Notes

- Small change across 3 files (~20 lines total)
- No new state management needed
- Reuses existing patterns and handlers
