# TRA-137 Inventory Save — Remaining Work Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close test coverage gaps for the inventory save feature (TRA-137) — the core functionality is fully implemented and working.

**Architecture:** All UI, API, storage, and E2E test code is in place. This plan adds targeted unit tests to three areas: the `useInventorySave` React hook, the `LocationBar` component, and the backend handler's 403 error paths (currently stubs). No new features or behavioral changes.

**Tech Stack:** Vitest + React Testing Library (frontend), Go testify + httptest (backend)

**Branch:** Create `feature/tra-137-inventory-save-tests` from `main` before first commit.

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `frontend/src/hooks/inventory/useInventorySave.test.ts` | Unit tests for save mutation hook |
| Create | `frontend/src/components/inventory/__tests__/LocationBar.test.tsx` | Unit tests for LocationBar component |
| Modify | `backend/internal/handlers/inventory/save_test.go` | Replace stub 403 tests with real handler-level tests using a storage interface |
| Modify | `backend/internal/handlers/inventory/save.go` | Extract storage interface for testability |

---

### Task 1: Create feature branch

- [ ] **Step 1: Create and checkout feature branch**

```bash
git checkout -b feature/tra-137-inventory-save-tests main
```

- [ ] **Step 2: Verify branch**

Run: `git branch --show-current`
Expected: `feature/tra-137-inventory-save-tests`

---

### Task 2: Unit tests for `useInventorySave` hook

**Files:**
- Create: `frontend/src/hooks/inventory/useInventorySave.test.ts`
- Reference: `frontend/src/hooks/inventory/useInventorySave.ts`
- Reference: `frontend/src/lib/api/inventory.ts`
- Reference: `frontend/src/lib/auth/orgContext.ts`

The hook wraps a `useMutation` that: (1) calls `ensureOrgContext()`, (2) calls `inventoryApi.save()`, (3) on 403 retries once after token refresh, (4) shows toast on success/error.

- [ ] **Step 1: Write failing test — successful save shows toast**

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import toast from 'react-hot-toast';
import { useInventorySave } from './useInventorySave';

// Mock dependencies
vi.mock('@/lib/api/inventory', () => ({
  inventoryApi: {
    save: vi.fn(),
  },
}));
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn(),
  refreshOrgToken: vi.fn(),
}));
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

import { inventoryApi } from '@/lib/api/inventory';
import { ensureOrgContext, refreshOrgToken } from '@/lib/auth/orgContext';

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children);
}

describe('useInventorySave', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ensureOrgContext).mockResolvedValue(1);
  });

  it('shows success toast with count and location name', async () => {
    vi.mocked(inventoryApi.save).mockResolvedValue({
      data: { data: { count: 5, location_id: 1, location_name: 'Warehouse A', timestamp: '2026-03-24T00:00:00Z' } },
    } as any);

    const { result } = renderHook(() => useInventorySave(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.save({ location_id: 1, asset_ids: [1, 2, 3, 4, 5] });
    });

    expect(toast.success).toHaveBeenCalledWith('5 assets saved to Warehouse A');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just frontend test -- src/hooks/inventory/useInventorySave.test.ts`
Expected: FAIL (file doesn't exist yet, or test fails if file was just created)

- [ ] **Step 3: Create the test file with all test cases**

Full test file should cover:
- Successful save shows toast with count and location
- `ensureOrgContext()` is called before API call
- On 403, retries once after `refreshOrgToken()`
- On 403 where refresh fails, throws error
- Error toast on failure
- `isSaving` reflects pending state
- Org context error shows specific message

- [ ] **Step 4: Run tests to verify they pass**

Run: `just frontend test -- src/hooks/inventory/useInventorySave.test.ts`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/inventory/useInventorySave.test.ts
git commit -m "test(inventory): add unit tests for useInventorySave hook"
```

---

### Task 3: Unit tests for `LocationBar` component

**Files:**
- Create: `frontend/src/components/inventory/__tests__/LocationBar.test.tsx`
- Reference: `frontend/src/components/inventory/LocationBar.tsx`
- Reference: `frontend/src/types/locations/index.ts` (for Location type shape)

LocationBar displays detected/selected location with a dropdown. Key behaviors: shows detected location name, shows "No location tag detected" fallback, dropdown with Change/Select, "Use detected" revert option, detection method subtext.

- [ ] **Step 1: Write failing test — renders detected location name**

```tsx
import '@testing-library/jest-dom';
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { LocationBar } from '../LocationBar';

const mockLocations = [
  { id: 1, name: 'Warehouse A', path: 'warehouse-a', depth: 0 },
  { id: 2, name: 'Rack 12', path: 'warehouse-a.rack-12', depth: 1 },
] as any[];

describe('LocationBar', () => {
  it('renders detected location name', () => {
    render(
      <LocationBar
        detectedLocation={{ id: 1, name: 'Warehouse A' }}
        detectionMethod="tag"
        selectedLocationId={null}
        onLocationChange={vi.fn()}
        locations={mockLocations}
        isAuthenticated={true}
      />
    );

    expect(screen.getByText('Warehouse A')).toBeInTheDocument();
    expect(screen.getByText('via location tag (strongest signal)')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `just frontend test -- src/components/inventory/__tests__/LocationBar.test.tsx`
Expected: FAIL (file doesn't exist yet)

- [ ] **Step 3: Create the test file with all test cases**

Full test file should cover:
- Renders detected location name with "via location tag" subtext
- Renders "No location tag detected" when no detection
- Shows "manually selected" subtext for manual override
- Shows "Change" button when location detected, "Select" when not
- Hides dropdown for unauthenticated users
- Shows "Use detected" revert option when manual override differs from detected
- Sorts locations by path in dropdown

- [ ] **Step 4: Run tests to verify they pass**

Run: `just frontend test -- src/components/inventory/__tests__/LocationBar.test.tsx`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/inventory/__tests__/LocationBar.test.tsx
git commit -m "test(inventory): add unit tests for LocationBar component"
```

---

### Task 4: Backend handler 403 tests — extract storage interface

**Files:**
- Modify: `backend/internal/handlers/inventory/save.go` (extract interface)
- Modify: `backend/internal/handlers/inventory/save_test.go` (replace stubs)

The current handler takes `*storage.Storage` directly, making it impossible to mock the storage layer for 403 path tests. Extract an interface so the handler can be tested with a mock.

**Note on error detection:** The handler detects 403-worthy errors via `strings.Contains(errStr, "not found or access denied")`, not type assertion. `InventoryAccessError.Error()` produces messages containing that substring, so returning `&storage.InventoryAccessError{...}` from the mock triggers the 403 path correctly.

- [ ] **Step 1: Write failing test — handler returns 403 when location not owned by org**

In `save_test.go`, add a test that creates the handler with a mock storage returning `&storage.InventoryAccessError{Reason: "location", ...}` and asserts 403 response. This will fail because the handler currently requires `*storage.Storage`, not an interface.

- [ ] **Step 2: Extract storage interface in save.go**

```go
// InventoryStorage defines the storage methods needed by the inventory handler
type InventoryStorage interface {
	SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error)
}

type Handler struct {
	storage InventoryStorage
}

func NewHandler(storage InventoryStorage) *Handler {
	return &Handler{storage: storage}
}
```

Ensure `*storage.Storage` satisfies this interface (it already has the method).

- [ ] **Step 3: Run test to verify it now compiles but may need adjustment**

Run: `cd backend && go test -run TestSave -v ./internal/handlers/inventory/`
Expected: Compiles, new test should pass

- [ ] **Step 4: Add test for 403 asset access denied**

Test with mock returning `&storage.InventoryAccessError{Reason: "assets", ...}` — assert 403 with diagnostic message.

- [ ] **Step 5: Add test for 500 internal storage error**

Test with mock returning generic error — assert 500.

- [ ] **Step 6: Add test for successful save returns 201**

Test with mock returning valid `SaveInventoryResult` — assert 201 with correct JSON body.

- [ ] **Step 7: Run all inventory handler tests**

Run: `cd backend && go test -run TestSave -v ./internal/handlers/inventory/`
Expected: All PASS

- [ ] **Step 8: Remove stale stub tests**

Remove `TestSave_LocationNotOwnedByOrg` and `TestSave_AssetNotOwnedByOrg` stubs (replaced by real tests). Also remove the unused `mockStorage` struct and `testHandler` struct at the top of the test file.

- [ ] **Step 9: Run full backend test suite**

Run: `just backend test`
Expected: All inventory tests PASS (pre-existing reports failure is unrelated)

- [ ] **Step 10: Commit**

```bash
git add backend/internal/handlers/inventory/save.go backend/internal/handlers/inventory/save_test.go
git commit -m "test(inventory): add handler-level 403/500/201 tests with storage interface"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run full frontend test suite**

Run: `just frontend test`
Expected: All PASS

- [ ] **Step 2: Run frontend typecheck**

Run: `just frontend typecheck`
Expected: No errors

- [ ] **Step 3: Run backend test suite**

Run: `just backend test`
Expected: All inventory tests PASS

- [ ] **Step 4: Run lint**

Run: `just lint`
Expected: No new lint errors
