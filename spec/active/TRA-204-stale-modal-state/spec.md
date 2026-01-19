# TRA-204: Fix Stale Modal State After Org Deletion

## Metadata
**Workspace**: frontend
**Type**: fix
**Linear**: [TRA-204](https://linear.app/trakrf/issue/TRA-204)

## Outcome
After deleting an organization, navigating to Members for another org shows the correct Members tab instead of the stale Delete confirmation dialog.

## User Story
As an organization owner
I want modal state to reset properly after org deletion
So that I can manage other organizations without encountering stale UI states

## Context

### Bug Description
When a user deletes an organization, the context picker automatically switches to another org. If the user then clicks "Members" from the dropdown, the Delete confirmation modal appears instead of the Members tab.

### Current Implementation

**File**: `frontend/src/components/useOrgModal.ts`

The `handleDeleteOrg` function (lines 149-164) has asymmetric state handling:

```typescript
const handleDeleteOrg = async (confirmName: string) => {
  if (!currentOrg || isDeleting) return;
  setIsDeleting(true);
  try {
    await orgsApi.delete(currentOrg.id, confirmName);
    await fetchProfile();
    toast.success('Organization deleted');
    onClose();                           // Modal closes
    window.location.hash = '#home';
    // BUG: showDeleteModal NOT reset on success
  } catch (err) {
    setSettingsError(extractErrorMessage(err, 'Failed to delete organization'));
    setShowDeleteModal(false);           // Only reset on error
  } finally {
    setIsDeleting(false);
  }
};
```

**Root Cause**: On successful deletion, `showDeleteModal` remains `true`. When the modal reopens for a different org, the stale state causes the Delete confirmation to appear.

**Additional Issue**: The `handleManageModeOpen` function (lines 74-80) doesn't reset `showDeleteModal`:

```typescript
const handleManageModeOpen = () => {
  setActiveTab(defaultTab);
  if (currentOrg) {
    setOrgName(currentOrg.name);
    setOriginalName(currentOrg.name);
  }
  if (defaultTab === 'members') loadMembers();
  // Missing: setShowDeleteModal(false);
};
```

### Bug Reproduction Steps
1. Log in with an account that has multiple organizations
2. Open Organization Settings for one org
3. Click "Delete Organization" button (`showDeleteModal = true`)
4. Confirm deletion by typing org name and clicking Delete
5. Deletion succeeds, modal closes, context switches to another org
6. Click "Members" from the org dropdown menu
7. **Bug**: Delete confirmation dialog appears instead of Members tab

### Desired Behavior
After org deletion completes:
1. All modal state should be reset (`showDeleteModal = false`)
2. Opening modal for another org should show clean state
3. Members tab should display member list, not delete dialog

## Technical Requirements

### Fix 1: Reset `showDeleteModal` on Successful Deletion
**File**: `frontend/src/components/useOrgModal.ts`
**Location**: Line ~155 (inside `handleDeleteOrg` try block)

```typescript
// Before onClose()
setShowDeleteModal(false);
```

### Fix 2: Reset `showDeleteModal` on Modal Open
**File**: `frontend/src/components/useOrgModal.ts`
**Location**: Start of `handleManageModeOpen` function

```typescript
const handleManageModeOpen = () => {
  setShowDeleteModal(false);  // Add this line
  setActiveTab(defaultTab);
  // ... rest of function
};
```

## Validation Criteria

- [ ] Delete org → Switch to another org → Click Members → Members tab displays correctly
- [ ] Delete org (error case) → Modal closes with no stale state
- [ ] Opening OrgModal in 'manage' mode always starts with `showDeleteModal = false`
- [ ] Existing delete org functionality unchanged (confirmation still works)
- [ ] No console errors during org deletion flow
- [ ] Unit tests pass for useOrgModal hook

## Success Metrics

- [ ] Bug reproduction steps no longer produce stale modal state
- [ ] `pnpm test` passes (all existing tests)
- [ ] `pnpm typecheck` passes
- [ ] `pnpm lint` passes

## File Changes Summary

| File | Lines Changed | Description |
|------|---------------|-------------|
| `frontend/src/components/useOrgModal.ts` | ~2 | Add `setShowDeleteModal(false)` in two locations |

## Testing Plan

### Unit Test (New)
Add test case to verify modal state reset:

```typescript
// useOrgModal.test.ts
describe('useOrgModal', () => {
  it('resets showDeleteModal when modal opens in manage mode', () => {
    // Test that handleManageModeOpen resets showDeleteModal
  });

  it('resets showDeleteModal after successful org deletion', () => {
    // Test that handleDeleteOrg success path resets state
  });
});
```

### Manual Testing
1. Create two orgs (Org A, Org B)
2. Switch to Org A
3. Open Settings → Delete Organization
4. Type confirmation name → Delete
5. After redirect, verify current org is Org B
6. Click "Members" from dropdown
7. Verify Members tab displays (not Delete dialog)

## References
- Component: `frontend/src/components/OrgModal.tsx`
- Hook: `frontend/src/components/useOrgModal.ts`
- Org Store: `frontend/src/stores/orgStore.ts`
- Org Switcher: `frontend/src/components/OrgSwitcher.tsx`
