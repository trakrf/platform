# Implementation Plan: Asset Form Tag Identifiers

Generated: 2025-12-19
Specification: spec.md

## Understanding

Users need to add tag identifiers (RFID, BLE, Barcode) when creating assets, and remove existing identifiers via the Tag Identifiers modal. Based on clarifying questions:

1. **Form location**: Tag identifiers section after Valid From/To dates
2. **Create form**: Add-only (no remove button)
3. **Remove functionality**: In TagIdentifiersModal with confirmation dialog
4. **Remove current read-only display**: Replace with editable section

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/assets/AssetForm.tsx` (lines 140-283) - form field patterns
- `frontend/src/components/shared/modals/ConfirmModal.tsx` - confirmation dialog pattern
- `frontend/src/components/assets/TagIdentifierList.tsx` - tag type display pattern

**Files to Create**:
- `frontend/src/components/assets/TagIdentifierInputRow.tsx` - reusable input row component

**Files to Modify**:
- `frontend/src/components/assets/AssetForm.tsx` - add tag identifiers section
- `frontend/src/components/assets/TagIdentifiersModal.tsx` - add remove functionality
- `frontend/src/lib/api/assets.ts` - add removeIdentifier API function

## Architecture Impact
- **Subsystems affected**: Frontend (UI components, API layer)
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add removeIdentifier API function
**File**: `frontend/src/lib/api/assets.ts`
**Action**: MODIFY
**Pattern**: Reference existing API functions in same file

**Implementation**:
```typescript
// Add to existing assets API file
export async function removeAssetIdentifier(assetId: number, identifierId: number): Promise<boolean> {
  const response = await apiClient.delete(`/assets/${assetId}/identifiers/${identifierId}`);
  return response.data.deleted;
}
```

**Validation**:
- `just frontend typecheck`

---

### Task 2: Create TagIdentifierInputRow component
**File**: `frontend/src/components/assets/TagIdentifierInputRow.tsx`
**Action**: CREATE
**Pattern**: Reference AssetForm.tsx form field styling

**Implementation**:
```typescript
interface TagIdentifierInputRowProps {
  type: 'rfid' | 'ble' | 'barcode';
  value: string;
  onTypeChange: (type: 'rfid' | 'ble' | 'barcode') => void;
  onValueChange: (value: string) => void;
  onRemove?: () => void;  // Optional - only shown in edit mode
  disabled?: boolean;
  error?: string;
}

// Type dropdown + Value input + optional Remove button
// Uses same styling as AssetForm fields
```

**Validation**:
- `just frontend typecheck`
- `just frontend lint`

---

### Task 3: Add tag identifiers state and UI to AssetForm
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY
**Pattern**: Follow existing form field patterns (lines 140-283)

**Implementation**:
1. Remove read-only TagIdentifierList display (lines 300-309)
2. Add state: `const [tagIdentifiers, setTagIdentifiers] = useState<TagIdentifierInput[]>([])`
3. Add TAG_TYPES constant
4. Add section after valid_to field with:
   - Header with "Tag Identifiers" and "+ Add" button
   - List of TagIdentifierInputRow components
   - No remove button in create mode
5. Include identifiers in submit data
6. Add validation for empty values

**Validation**:
- `just frontend typecheck`
- `just frontend lint`

---

### Task 4: Add remove functionality to TagIdentifiersModal
**File**: `frontend/src/components/assets/TagIdentifiersModal.tsx`
**Action**: MODIFY
**Pattern**: Reference ConfirmModal pattern

**Implementation**:
1. Add props: `assetId: number`, `onIdentifierRemoved?: (id: number) => void`
2. Add state for confirmation dialog and loading
3. Add remove button (Trash2 icon) to each identifier row
4. On remove click: show confirmation dialog
5. On confirm: call removeAssetIdentifier API, then callback
6. Handle loading and error states

**Validation**:
- `just frontend typecheck`
- `just frontend lint`

---

### Task 5: Update AssetCard to pass assetId to modal
**File**: `frontend/src/components/assets/AssetCard.tsx`
**Action**: MODIFY

**Implementation**:
Pass `asset.id` to TagIdentifiersModal, handle `onIdentifierRemoved` to refresh asset data

**Validation**:
- `just frontend typecheck`
- `just frontend lint`

---

### Task 6: Final validation and testing
**Action**: VALIDATE

**Validation**:
- `just frontend validate` (all checks)

---

### Task 7: Playwright MCP UI Screenshots
**Action**: VALIDATE WITH SCREENSHOTS
**Output**: `frontend/dist/screenshots/`

**Steps**:
1. Build frontend: `just frontend build`
2. Serve built files: `python3 -m http.server 3001 --directory frontend/dist`
3. Login with Playwright MCP (test1@test.com / password)
4. Capture screenshots:

**Screenshots to capture**:

| Screenshot | Description | Filename |
|------------|-------------|----------|
| Create Form | Asset create form with tag identifiers section visible | `20-create-form-tag-section.png` |
| Create Form + Tag | Form with one tag identifier added | `21-create-form-with-tag.png` |
| Edit Form | Asset edit form showing existing identifiers | `22-edit-form-identifiers.png` |
| Tags Modal | Tag identifiers modal with remove buttons | `23-tags-modal-remove.png` |
| Confirm Dialog | Confirmation dialog before deletion | `24-confirm-delete-dialog.png` |
| After Delete | Modal after identifier removed | `25-after-delete.png` |

**Validation**:
- All 6 screenshots captured successfully
- UI matches spec wireframe
- Mobile responsive (test at 375px width for key screens)

## Risk Assessment

- **Risk**: Form submit might not include identifiers correctly
  **Mitigation**: Test with network tab to verify request payload

- **Risk**: Confirmation modal might not match app styling
  **Mitigation**: Use existing ConfirmModal pattern but adapt for dark mode

## Integration Points

- API: Uses existing `DELETE /api/v1/assets/{id}/identifiers/{identifierId}` endpoint
- Store updates: May need to invalidate asset cache after identifier removal

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test`

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: `just frontend typecheck && just frontend lint`

Final validation: `just frontend validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- Clear requirements from spec and clarifying questions
- Similar patterns found in AssetForm.tsx (form fields)
- Existing ConfirmModal pattern to follow
- Backend API already implemented and tested
- No new dependencies needed

**Assessment**: Well-scoped feature with clear patterns to follow. Backend support exists.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist in codebase, backend API is ready, scope is limited to 2 files to modify and 1 small component to create.

## Task Summary

| Task | Description | Type |
|------|-------------|------|
| 1 | Add removeIdentifier API function | CODE |
| 2 | Create TagIdentifierInputRow component | CODE |
| 3 | Add tag identifiers to AssetForm | CODE |
| 4 | Add remove functionality to TagIdentifiersModal | CODE |
| 5 | Update AssetCard to pass assetId | CODE |
| 6 | Final validation | VALIDATE |
| 7 | Playwright MCP UI Screenshots | SCREENSHOTS |

**Total Tasks**: 7
