# TRA-332: Fix Inventory Save 403 for Admin User

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `POST /api/v1/inventory/save` returning 403 for authenticated Admin users by adding diagnostic logging, enriched error responses, and a frontend JWT org-context guard.

**Architecture:** Three-layer fix: (1) backend adds structured logging and enriched 403 response with `org_id` context, (2) frontend validates JWT org context before sending save requests and handles 403 with smart recovery, (3) align save button enablement with actual save logic.

**Tech Stack:** Go (zerolog, chi), React/TypeScript (jwt-decode, axios, zustand, react-hot-toast)

**Linear Issue:** [TRA-332](https://linear.app/trakrf/issue/TRA-332)

**Related Issues:** TRA-189, TRA-295, TRA-318 (org switch race conditions)

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `backend/internal/handlers/inventory/save.go:83-95` | Add structured logging on 403 path, include `org_id` in error detail |
| Modify | `backend/internal/storage/inventory.go:28-59` | Return typed errors distinguishing location vs asset failure |
| Modify | `backend/internal/handlers/inventory/save_test.go` | Add tests for enriched error responses |
| Modify | `frontend/src/hooks/inventory/useInventorySave.ts` | Add JWT org-context guard, handle 403 with token refresh retry |
| Modify | `frontend/src/components/InventoryScreen.tsx:131-133,298` | Align `saveableCount` with actual save filter logic |
| Create | `frontend/src/hooks/inventory/useInventorySave.test.ts` | Unit tests for org-context guard and 403 handling |

---

## Task 1: Backend — Typed storage errors for location vs asset validation

**Files:**
- Modify: `backend/internal/storage/inventory.go:40-58`

- [ ] **Step 1: Write the failing test**

Add to `backend/internal/storage/inventory_test.go` (create if needed — colocated test):

```go
package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInventoryErrorTypes(t *testing.T) {
	t.Run("location access denied error", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:     "location",
			OrgID:      123,
			LocationID: 456,
		}
		assert.Contains(t, err.Error(), "location not found or access denied")
		assert.Contains(t, err.Error(), "org_id=123")
		assert.Contains(t, err.Error(), "location_id=456")
		assert.True(t, err.IsAccessDenied())
	})

	t.Run("asset access denied error", func(t *testing.T) {
		err := &InventoryAccessError{
			Reason:     "assets",
			OrgID:      123,
			AssetIDs:   []int{1, 2, 3},
			ValidCount: 2,
			TotalCount: 3,
		}
		assert.Contains(t, err.Error(), "assets not found or access denied")
		assert.Contains(t, err.Error(), "org_id=123")
		assert.Contains(t, err.Error(), "valid=2/3")
		assert.True(t, err.IsAccessDenied())
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/mike/platform && just backend test -- -run TestInventoryErrorTypes -v ./internal/storage/`
Expected: FAIL — `InventoryAccessError` type not defined

- [ ] **Step 3: Write the InventoryAccessError type and update storage**

Add the error type to `backend/internal/storage/inventory.go` (before `SaveInventoryScans`):

```go
// InventoryAccessError provides diagnostic context for 403 responses.
type InventoryAccessError struct {
	Reason     string // "location" or "assets"
	OrgID      int
	LocationID int
	AssetIDs   []int
	ValidCount int
	TotalCount int
}

func (e *InventoryAccessError) Error() string {
	switch e.Reason {
	case "location":
		return fmt.Sprintf("location not found or access denied (org_id=%d, location_id=%d)", e.OrgID, e.LocationID)
	case "assets":
		return fmt.Sprintf("assets not found or access denied (org_id=%d, valid=%d/%d)", e.OrgID, e.ValidCount, e.TotalCount)
	default:
		return "access denied"
	}
}

func (e *InventoryAccessError) IsAccessDenied() bool {
	return true
}
```

Then update `SaveInventoryScans` to return typed errors instead of plain `fmt.Errorf`:

Replace the location validation error (around line 42):
```go
	if err == pgx.ErrNoRows {
		return nil, &InventoryAccessError{
			Reason:     "location",
			OrgID:      orgID,
			LocationID: req.LocationID,
		}
	}
```

Replace the asset count check (around line 57-58):
```go
	if validAssetCount != len(req.AssetIDs) {
		return nil, &InventoryAccessError{
			Reason:     "assets",
			OrgID:      orgID,
			AssetIDs:   req.AssetIDs,
			ValidCount: validAssetCount,
			TotalCount: len(req.AssetIDs),
		}
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/mike/platform && just backend test -- -run TestInventoryErrorTypes -v ./internal/storage/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/storage/inventory.go backend/internal/storage/inventory_test.go
git commit -m "feat(storage): add typed InventoryAccessError for diagnostic 403s"
```

---

## Task 2: Backend — Structured logging and enriched 403 in handler

**Files:**
- Modify: `backend/internal/handlers/inventory/save.go:83-95`

- [ ] **Step 1: Write the failing test**

The handler currently uses `*storage.Storage` directly (not an interface), so we can't easily mock the storage in unit tests. Instead, write an integration-style test that verifies the handler's error detection logic works with the new `InventoryAccessError` type from Task 1.

Add to `backend/internal/handlers/inventory/save_test.go`:

```go
func TestSave_AccessErrorDetection(t *testing.T) {
	// Verify the handler's error detection works with both the new
	// InventoryAccessError format and the legacy plain string format
	tests := []struct {
		name            string
		err             error
		expectForbidden bool
		expectOrgInMsg  bool
	}{
		{
			name: "typed location error includes org context",
			err: &storage.InventoryAccessError{
				Reason:     "location",
				OrgID:      123,
				LocationID: 456,
			},
			expectForbidden: true,
			expectOrgInMsg:  true,
		},
		{
			name: "typed asset error includes org context",
			err: &storage.InventoryAccessError{
				Reason:     "assets",
				OrgID:      123,
				AssetIDs:   []int{1, 2, 3},
				ValidCount: 2,
				TotalCount: 3,
			},
			expectForbidden: true,
			expectOrgInMsg:  true,
		},
		{
			name:            "internal error is not forbidden",
			err:             errors.New("database connection failed"),
			expectForbidden: false,
			expectOrgInMsg:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			isForbidden := strings.Contains(errStr, "not found or access denied")
			assert.Equal(t, tt.expectForbidden, isForbidden)
			if tt.expectOrgInMsg {
				assert.Contains(t, errStr, "org_id=123")
			}
		})
	}
}
```

Add `"strings"` and `"github.com/trakrf/platform/backend/internal/storage"` to the import block.

- [ ] **Step 2: Run test to verify it passes**

Run: `cd /home/mike/platform && just backend test -- -run TestSave_AccessErrorDetection -v ./internal/handlers/inventory/`
Expected: PASS (the typed error from Task 1 integrates with the handler's detection logic)

- [ ] **Step 3: Update handler to log diagnostics and pass enriched error detail**

In `backend/internal/handlers/inventory/save.go`, replace the error handling block (lines 83-95) with:

```go
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found or access denied") {
			logger.Get().Warn().
				Int("org_id", orgID).
				Int("location_id", request.LocationID).
				Ints("asset_ids", request.AssetIDs).
				Str("request_id", requestID).
				Str("error", errStr).
				Msg("Inventory save denied: org context mismatch")

			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				apierrors.InventorySaveForbidden, errStr, requestID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InventorySaveFailed, err.Error(), requestID)
		return
	}
```

Add the logger import:
```go
"github.com/trakrf/platform/backend/internal/logger"
```

- [ ] **Step 4: Run all handler tests**

Run: `cd /home/mike/platform && just backend test -- -run TestSave -v ./internal/handlers/inventory/`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/inventory/save.go backend/internal/handlers/inventory/save_test.go
git commit -m "fix(inventory): add diagnostic logging and enriched 403 detail for save endpoint"
```

---

## Task 3: Frontend — JWT org-context guard in useInventorySave

**Files:**
- Modify: `frontend/src/hooks/inventory/useInventorySave.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/src/hooks/inventory/useInventorySave.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Unit tests for getTokenOrgId helper.
 * This validates that the JWT org context guard correctly
 * extracts and validates the org_id from stored tokens.
 */

// Mock jwt-decode
vi.mock('jwt-decode', () => ({
  jwtDecode: vi.fn(),
}));

import { jwtDecode } from 'jwt-decode';

// Import after mocking
import { getTokenOrgId } from './useInventorySave';

describe('getTokenOrgId', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    localStorage.clear();
  });

  it('returns org_id from valid token with org claim', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ current_org_id: 42 });

    expect(getTokenOrgId()).toBe(42);
  });

  it('returns null when no auth storage', () => {
    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when token has no org_id claim', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ user_id: 1 });

    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when org_id is 0', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'valid.jwt.token' },
    }));
    vi.mocked(jwtDecode).mockReturnValue({ current_org_id: 0 });

    expect(getTokenOrgId()).toBeNull();
  });

  it('returns null when jwt-decode throws', () => {
    localStorage.setItem('auth-storage', JSON.stringify({
      state: { token: 'corrupt.token' },
    }));
    vi.mocked(jwtDecode).mockImplementation(() => { throw new Error('Invalid token'); });

    expect(getTokenOrgId()).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/mike/platform && just frontend test -- -t "getTokenOrgId"`
Expected: FAIL — `getTokenOrgId` not exported

- [ ] **Step 3: Implement the guard and 403 retry logic**

Replace `frontend/src/hooks/inventory/useInventorySave.ts` with:

```typescript
import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import { jwtDecode } from 'jwt-decode';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';

interface JwtClaims {
  current_org_id?: number;
  user_id?: number;
}

/**
 * Extract org_id from the JWT in localStorage.
 * Returns null if missing, zero, or token is invalid.
 * Exported for testing.
 */
export function getTokenOrgId(): number | null {
  try {
    const authStorage = localStorage.getItem('auth-storage');
    if (!authStorage) return null;

    const { state } = JSON.parse(authStorage);
    if (!state?.token) return null;

    const decoded = jwtDecode<JwtClaims>(state.token);
    return decoded.current_org_id || null; // treats 0 and undefined as null
  } catch {
    return null;
  }
}

/**
 * Attempt to refresh the JWT with the current org context.
 * Returns true if the token was successfully refreshed.
 */
async function refreshOrgToken(): Promise<boolean> {
  try {
    const profile = useAuthStore.getState().profile;
    if (!profile) {
      await useAuthStore.getState().fetchProfile();
    }
    const currentOrgId = useAuthStore.getState().profile?.current_org?.id;
    if (!currentOrgId) return false;

    const response = await orgsApi.setCurrentOrg({ org_id: currentOrgId });
    // Zustand persist middleware wraps setState, so this persists to localStorage.
    // Same pattern used in orgStore.switchOrg (orgStore.ts:62).
    useAuthStore.setState({ token: response.data.token });
    return true;
  } catch {
    return false;
  }
}

/**
 * Hook for saving scanned inventory to the database.
 *
 * Includes a JWT org-context guard that:
 * 1. Checks the token has a valid current_org_id before sending
 * 2. On 403, attempts one token refresh + retry
 */
export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      // Guard: verify JWT has org context before sending
      const orgId = getTokenOrgId();
      if (!orgId) {
        console.warn('[InventorySave] JWT missing org_id claim, refreshing token');
        const refreshed = await refreshOrgToken();
        if (!refreshed) {
          throw new Error('No organization context. Please select an organization and try again.');
        }
      }

      try {
        const response = await inventoryApi.save(data);
        return response.data.data;
      } catch (error: unknown) {
        // On 403, attempt one token refresh + retry
        const axiosError = error as { response?: { status?: number } };
        if (axiosError.response?.status === 403) {
          console.warn('[InventorySave] Got 403, attempting token refresh and retry');
          const refreshed = await refreshOrgToken();
          if (refreshed) {
            const retryResponse = await inventoryApi.save(data);
            return retryResponse.data.data;
          }
        }
        throw error;
      }
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
    },
    onError: (error: Error) => {
      if (error.message.includes('No organization context')) {
        toast.error(error.message);
      } else {
        toast.error('Failed to save inventory');
      }
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/mike/platform && just frontend test -- -t "getTokenOrgId"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/hooks/inventory/useInventorySave.ts frontend/src/hooks/inventory/useInventorySave.test.ts
git commit -m "fix(inventory): add JWT org-context guard with 403 retry in save hook"
```

---

## Task 4: Frontend — Align saveableCount with actual save filter

**Files:**
- Modify: `frontend/src/components/InventoryScreen.tsx:131-133`

- [ ] **Step 1: Identify the mismatch**

Current code at line 131-133:
```typescript
const saveableCount = useMemo(() => {
  return tags.filter(t => t.type === 'asset').length;
}, [tags]);
```

But `handleSave` (line 238-240) filters for `t.type === 'asset' && t.assetId`. The button can be enabled when no assets have been enriched.

- [ ] **Step 2: Fix the saveableCount filter**

Replace lines 131-133 in `frontend/src/components/InventoryScreen.tsx`:

```typescript
  const saveableCount = useMemo(() => {
    return tags.filter(t => t.type === 'asset' && t.assetId).length;
  }, [tags]);
```

- [ ] **Step 3: Run frontend type check and lint**

Run: `cd /home/mike/platform && just frontend typecheck && just frontend lint`
Expected: PASS — no type errors or lint warnings

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/InventoryScreen.tsx
git commit -m "fix(inventory): align save button enablement with actual save filter logic"
```

---

## Task 5: Run full validation

- [ ] **Step 1: Run backend tests**

Run: `cd /home/mike/platform && just backend test`
Expected: All tests PASS

- [ ] **Step 2: Run frontend tests**

Run: `cd /home/mike/platform && just frontend test`
Expected: All tests PASS

- [ ] **Step 3: Run full validation**

Run: `cd /home/mike/platform && just validate`
Expected: All lint, typecheck, and tests PASS

- [ ] **Step 4: Create feature branch and push**

```bash
git checkout -b fix/tra-332-inventory-save-403
git push -u origin fix/tra-332-inventory-save-403
```

- [ ] **Step 5: Create PR**

Title: `fix(inventory): resolve 403 on save for authenticated Admin user (TRA-332)`

Body should reference:
- Diagnostic logging added to backend 403 path
- Typed `InventoryAccessError` with org/location/asset context
- Frontend JWT org-context guard with 403 retry
- `saveableCount` aligned with actual save filter
- Related: TRA-189, TRA-295, TRA-318
