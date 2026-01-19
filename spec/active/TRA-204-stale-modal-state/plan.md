# Implementation Plan: TRA-204 Fix Stale Modal State After Org Deletion

Generated: 2026-01-19
Specification: spec.md

## Understanding

After deleting an organization, the `showDeleteModal` state in `useOrgModal` hook is not reset to `false` on the success path. When the modal reopens for another organization (e.g., clicking "Members"), the stale `true` state causes the Delete confirmation dialog to appear instead of the Members tab.

**Fix**: Add `setShowDeleteModal(false)` in two locations:
1. Before `onClose()` in the `handleDeleteOrg` success path
2. At the start of `handleManageModeOpen()` to ensure clean state on modal open

**Test**: Add unit test for `useOrgModal` hook to verify state reset behavior.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/export/useExportModal.test.ts` - Hook testing pattern with `renderHook`, `act`, and mocks
- `frontend/src/hooks/orgs/useOrgSwitch.test.ts` - Pattern for testing org-related hooks with store mocks

**Files to Create**:
- `frontend/src/components/useOrgModal.test.ts` - Unit tests for the hook

**Files to Modify**:
- `frontend/src/components/useOrgModal.ts` (lines 74-80, 149-164) - Add state reset calls

## Architecture Impact

- **Subsystems affected**: Frontend UI state management (useOrgModal hook only)
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add State Reset in handleDeleteOrg Success Path

**File**: `frontend/src/components/useOrgModal.ts`
**Action**: MODIFY
**Pattern**: Existing error path at line 160 already calls `setShowDeleteModal(false)`

**Implementation**:

At line ~155, before `onClose()`:

```typescript
// Current (lines 149-164):
const handleDeleteOrg = async (confirmName: string) => {
  if (!currentOrg || isDeleting) return;
  setIsDeleting(true);
  try {
    await orgsApi.delete(currentOrg.id, confirmName);
    await fetchProfile();
    toast.success('Organization deleted');
    setShowDeleteModal(false);  // ADD THIS LINE
    onClose();
    window.location.hash = '#home';
  } catch (err) {
    setSettingsError(extractErrorMessage(err, 'Failed to delete organization'));
    setShowDeleteModal(false);
  } finally {
    setIsDeleting(false);
  }
};
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 2: Add State Reset in handleManageModeOpen

**File**: `frontend/src/components/useOrgModal.ts`
**Action**: MODIFY
**Pattern**: Defensive reset on modal open

**Implementation**:

At line ~74, add reset at start of function:

```typescript
// Current (lines 74-80):
const handleManageModeOpen = () => {
  setShowDeleteModal(false);  // ADD THIS LINE
  setActiveTab(defaultTab);
  if (currentOrg) {
    setOrgName(currentOrg.name);
    setOriginalName(currentOrg.name);
  }
  if (defaultTab === 'members') loadMembers();
};
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 3: Create Unit Tests for useOrgModal

**File**: `frontend/src/components/useOrgModal.test.ts`
**Action**: CREATE
**Pattern**: Reference `frontend/src/components/export/useExportModal.test.ts`

**Implementation**:

```typescript
/**
 * Tests for useOrgModal hook - TRA-204 regression prevention
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useOrgModal } from './useOrgModal';

// Mock dependencies
vi.mock('@/stores', () => ({
  useOrgStore: vi.fn(() => ({
    currentOrg: { id: 1, name: 'Test Org' },
    currentRole: 'owner',
    isLoading: false,
  })),
  useAuthStore: vi.fn(() => ({
    profile: { id: 1 },
    fetchProfile: vi.fn().mockResolvedValue(undefined),
  })),
}));

vi.mock('@/hooks/orgs/useOrgSwitch', () => ({
  useOrgSwitch: vi.fn(() => ({
    createOrg: vi.fn().mockResolvedValue({ id: 2, name: 'New Org' }),
  })),
}));

vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    listMembers: vi.fn().mockResolvedValue({ data: { data: [] } }),
    delete: vi.fn().mockResolvedValue({}),
  },
}));

vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), {
    success: vi.fn(),
    error: vi.fn(),
  }),
}));

describe('useOrgModal', () => {
  const mockOnClose = vi.fn();

  const defaultProps = {
    isOpen: true,
    onClose: mockOnClose,
    mode: 'manage' as const,
    defaultTab: 'members' as const,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('TRA-204: showDeleteModal state management', () => {
    it('initializes showDeleteModal as false', () => {
      const { result } = renderHook(() => useOrgModal(defaultProps));
      expect(result.current.showDeleteModal).toBe(false);
    });

    it('resets showDeleteModal when modal opens in manage mode', () => {
      const { result, rerender } = renderHook(
        ({ isOpen }) => useOrgModal({ ...defaultProps, isOpen }),
        { initialProps: { isOpen: false } }
      );

      // Simulate having stale state by opening delete modal
      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      // Close and reopen modal
      rerender({ isOpen: false });
      rerender({ isOpen: true });

      // showDeleteModal should be reset to false
      expect(result.current.showDeleteModal).toBe(false);
    });

    it('resets showDeleteModal after successful org deletion', async () => {
      const { result } = renderHook(() => useOrgModal(defaultProps));

      // Open delete modal
      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      // Perform deletion
      await act(async () => {
        await result.current.handleDeleteOrg('Test Org');
      });

      // showDeleteModal should be reset
      expect(result.current.showDeleteModal).toBe(false);
      expect(mockOnClose).toHaveBeenCalled();
    });
  });

  describe('openDeleteModal and closeDeleteModal', () => {
    it('opens and closes delete modal', () => {
      const { result } = renderHook(() => useOrgModal(defaultProps));

      expect(result.current.showDeleteModal).toBe(false);

      act(() => {
        result.current.openDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(true);

      act(() => {
        result.current.closeDeleteModal();
      });
      expect(result.current.showDeleteModal).toBe(false);
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test -- useOrgModal
```

---

### Task 4: Run Full Validation Suite

**Action**: VERIFY

**Validation**:
```bash
cd frontend && just validate
```

This runs lint, typecheck, test, and build.

## Risk Assessment

- **Risk**: Test mocks may not accurately reflect store behavior
  **Mitigation**: Keep mocks minimal, test only the state reset logic

- **Risk**: Order of `setShowDeleteModal(false)` and `onClose()` matters
  **Mitigation**: Reset state BEFORE closing modal to ensure clean state

## Integration Points

- **Store updates**: None - changes are local to useOrgModal hook
- **Route changes**: None
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just lint` - Syntax & Style
- Gate 2: `just typecheck` - Type Safety
- Gate 3: `just test` - Unit Tests

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

## Validation Sequence

After each task:
```bash
cd frontend && just lint && just typecheck && just test
```

Final validation:
```bash
just validate
```

## Plan Quality Assessment

**Complexity Score**: 1/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and Linear ticket
- ✅ Root cause identified with exact line numbers
- ✅ Similar test patterns found at `useExportModal.test.ts`
- ✅ Minimal change scope (2 lines of code + 1 test file)
- ✅ No new dependencies
- ✅ No architectural changes

**Assessment**: High confidence fix with clear test pattern to follow.

**Estimated one-pass success probability**: 95%

**Reasoning**: This is a straightforward state management bug with an obvious fix. The only uncertainty is ensuring the test mocks work correctly, but the pattern from `useExportModal.test.ts` provides a clear template.
