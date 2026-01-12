# Implementation Plan: Streamline Asset Creation Flow
Generated: 2026-01-12
Specification: spec.md

## Understanding
Remove the Single/Bulk interstitial modal so the FAB opens the asset form directly. Simplify the form by renaming "Identifier" to "Asset ID", hiding the Type dropdown (defaulting to "asset"), moving Description up below Name, and renaming "Tag Identifiers" to "Tags".

## Relevant Files

**Reference Patterns**:
- `frontend/src/components/assets/AssetForm.tsx` (current form structure)
- `frontend/src/components/AssetsScreen.tsx` (FAB and modal orchestration)

**Files to Delete**:
- `frontend/src/components/assets/AssetCreateChoice.tsx` - Interstitial modal no longer needed

**Files to Modify**:
- `frontend/src/components/AssetsScreen.tsx` - Remove choice modal state/handlers/JSX
- `frontend/src/components/assets/AssetForm.tsx` - Rename labels, hide Type, reorder fields

## Architecture Impact
- **Subsystems affected**: Frontend UI only
- **New dependencies**: None
- **Breaking changes**: None (UX improvement, same data model)

## Task Breakdown

### Task 1: Simplify AssetsScreen - Remove Choice Modal
**File**: `frontend/src/components/AssetsScreen.tsx`
**Action**: MODIFY

**Implementation**:
1. Remove import of `AssetCreateChoice` (line 12)
2. Remove `isChoiceModalOpen` state (line 20)
3. Change `handleCreateClick` to directly open form:
   ```typescript
   const handleCreateClick = () => {
     setIsCreateModalOpen(true);
   };
   ```
4. Remove `handleSingleCreate` function (lines 90-93)
5. Remove `handleBulkUpload` function (lines 95-98)
6. Remove `<AssetCreateChoice ... />` JSX (lines 173-178)

**Validation**: `just frontend typecheck`

---

### Task 2: Delete AssetCreateChoice Component
**File**: `frontend/src/components/assets/AssetCreateChoice.tsx`
**Action**: DELETE

**Implementation**: Delete the file entirely.

**Validation**: `just frontend typecheck` (verify no import errors)

---

### Task 3: Rename "Identifier" to "Asset ID"
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:
1. Line 160: Change label from "Identifier" to "Asset ID"
2. Line 83: Change error message from "Identifier is required" to "Asset ID is required"

**Validation**: `just frontend typecheck`

---

### Task 4: Hide Type Dropdown
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:
Remove the entire Type dropdown block (lines 244-261):
```tsx
<div>
  <label htmlFor="type" ...>Type ...</label>
  <select id="type" ...>...</select>
</div>
```

Keep the form state (`type: asset`) and submission logic unchanged - it defaults correctly.

**Validation**: `just frontend typecheck`

---

### Task 5: Reorder Fields - Move Description After Name
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:
Move the Description block (currently lines 301-314) to directly after the Name field block (after line 242), before Location.

New field order in the grid:
1. Asset ID (with scanner buttons)
2. Name
3. Description (moved - full width below the grid)
4. Location
5. Active checkbox
6. Valid From / Valid To

Actually, looking at the layout - Description is a full-width textarea outside the 2-col grid. Move it to appear right after the first grid closes (after Name), before Location/Active grid row.

**Validation**: `just frontend typecheck`

---

### Task 6: Rename "Tag Identifiers" to "Tags"
**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:
1. Line 357: Change label from "Tag Identifiers" to "Tags"
2. Line 374: Update empty state text from "No tag identifiers added..." to "No tags added. Click "Add Tag" to link RFID tags."

**Validation**: `just frontend typecheck && just frontend build`

---

## Risk Assessment
- **Risk**: Removing choice modal breaks bulk upload access
  **Mitigation**: Keep `BulkUploadModal` and `isBulkUploadOpen` state for future. Bulk upload will be accessible via separate import/export feature later.

- **Risk**: Type field removal causes API errors
  **Mitigation**: Type stays in form state with default "asset" - only hiding UI, not removing data.

## VALIDATION GATES (MANDATORY)

After EVERY task:
```bash
just frontend typecheck
```

After all tasks complete:
```bash
just frontend lint
just frontend build
```

**Enforcement**: If any gate fails, fix immediately before proceeding.

## Validation Sequence

1. After Task 1-2: `just frontend typecheck` (verify imports clean)
2. After Task 3-6: `just frontend typecheck`
3. Final: `just frontend lint && just frontend build`

Manual testing:
- Click FAB → Form opens directly (no interstitial)
- Form shows "Asset ID" label
- Type dropdown not visible
- Description appears after Name
- "Tags" section at bottom
- Create asset → succeeds
- Edit asset → succeeds

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Simple UI changes - no new patterns
✅ All clarifying questions answered
✅ No external dependencies
✅ No backend changes required
✅ Well-defined file locations

**Assessment**: Straightforward UI simplification with no architectural changes.

**Estimated one-pass success probability**: 95%

**Reasoning**: Pure frontend changes to existing components. No new patterns, no API changes, no state management changes. Risk is minimal.
