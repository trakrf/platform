# Feature: Streamline Asset Creation Flow

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: [TRA-259](https://linear.app/trakrf/issue/TRA-259/streamline-asset-creation-flow)
**Parent**: [TRA-250 - NADA launch requirements](https://linear.app/trakrf/issue/TRA-250/nada-launch-requirements)

## Outcome
Reduce friction in the asset creation workflow by removing an unnecessary interstitial modal and simplifying the form. 90% of add actions are single asset creates - the extra click adds friction for the primary use case.

## User Story
As a warehouse operator
I want to quickly add assets one at a time
So that I can efficiently register new inventory without extra clicks

## Context
**Current**:
1. User clicks FAB (+) button
2. Interstitial modal appears: "Create Single Asset" or "Bulk Upload CSV"
3. User clicks "Create Single Asset"
4. Asset form opens with fields: Identifier → Name → Type → Location → Active → Description → Dates

**Desired**:
1. User clicks FAB (+) button
2. Asset form opens directly with fields: Asset ID → Name → Description → Location → Active → Dates
   - Type hidden (defaults to "asset")
   - "Identifier" renamed to "Asset ID"
   - Description moved up below Name

**Examples**:
- Current FAB flow: `frontend/src/components/AssetsScreen.tsx:167-178`
- Interstitial modal: `frontend/src/components/assets/AssetCreateChoice.tsx`
- Asset form: `frontend/src/components/assets/AssetForm.tsx`

## Technical Requirements

### 1. FAB Behavior Change
**File**: `frontend/src/components/AssetsScreen.tsx`
- Remove `isChoiceModalOpen` state variable
- Remove `AssetCreateChoice` component import and JSX
- Change `handleCreateClick()` to directly set `isCreateModalOpen: true`
- Remove `handleSingleCreate()` and `handleBulkUpload()` handlers (no longer needed for choice modal)

### 2. Delete Interstitial Modal
**File**: `frontend/src/components/assets/AssetCreateChoice.tsx`
- Delete this file entirely
- Keep `BulkUploadModal.tsx` for future import/export feature

### 3. Form Field Changes
**File**: `frontend/src/components/assets/AssetForm.tsx`

| Change | Details |
|--------|---------|
| Rename label | "Identifier" → "Asset ID" |
| Hide Type | Remove Type dropdown from UI; keep `type: "asset"` in form state/submission |
| Reorder fields | Asset ID → Name → Description → Location → Active → Valid From → Valid To |
| Rename section | "Tag Identifiers" → "Tags" |

### 4. Not in Scope
- Bulk upload removal (keep for future import/export)
- Type column removal from database (YAGNI - hide UI only)
- Backend API changes

## Validation Criteria
- [ ] FAB click opens asset form directly (no interstitial modal)
- [ ] Form label shows "Asset ID" (not "Identifier")
- [ ] Type selector is not visible on form
- [ ] Description field appears directly after Name field
- [ ] Creating an asset works (type defaults to "asset" in payload)
- [ ] Editing an asset still works (no regressions)
- [ ] Scanner integration works for Asset ID field
- [ ] No TypeScript errors
- [ ] `just frontend build` succeeds

## Success Metrics
- [ ] Single click from FAB to form (was: 2 clicks)
- [ ] All existing asset CRUD operations pass
- [ ] Zero console errors during manual testing
- [ ] Build succeeds with no type errors

## References
- Linear issue: https://linear.app/trakrf/issue/TRA-259/streamline-asset-creation-flow
- NADA confirmed single-asset workflow, no Great Plains integration at launch
