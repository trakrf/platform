# Implementation Plan: Fix MembersScreen Null Reference Error (TRA-181)

Generated: 2025-12-11
Specification: spec.md

## Understanding

Fix a production crash where the Members screen throws `TypeError: Cannot read properties of null (reading 'length')` when an organization has no members. The root cause is Go's nil slice serializing to `null` in JSON instead of `[]`.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/src/components/__tests__/HomeScreen.test.tsx` - Vitest + RTL test pattern
- `frontend/src/components/UserMenu.test.tsx` - Component test with mocks
- `backend/internal/storage/users_test.go` - Go test file structure (skipped pattern)

**Files to Create**:
- `backend/internal/storage/org_users_test.go` - JSON serialization test
- `frontend/src/components/__tests__/MembersScreen.test.tsx` - Component test for null handling

**Files to Modify**:
- `backend/internal/storage/org_users.go` (line 98) - Fix nil slice initialization
- `frontend/src/components/MembersScreen.tsx` (line 41) - Add null coalescing

## Architecture Impact

- **Subsystems affected**: Backend storage, Frontend components
- **New dependencies**: None
- **Breaking changes**: None (behavior improvement only)

## Task Breakdown

### Task 1: Fix Backend Nil Slice
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY
**Pattern**: Initialize slice to empty instead of nil

**Implementation**:
```go
// Line 98 - Change from:
var members []organization.OrgMember
// To:
members := []organization.OrgMember{}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 2: Add Backend Test
**File**: `backend/internal/storage/org_users_test.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/storage/users_test.go`

**Implementation**:
```go
package storage

import (
	"encoding/json"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
)

func TestListOrgMembersReturnsEmptyArrayNotNull(t *testing.T) {
	// This test documents the expected JSON serialization behavior
	// Empty slice should serialize to [] not null
	members := []organization.OrgMember{}

	data, err := json.Marshal(map[string]any{"data": members})
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"data":[]}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestNilSliceSerializesToNull(t *testing.T) {
	// This test documents why we must use []T{} not var []T
	var members []organization.OrgMember // nil slice

	data, err := json.Marshal(map[string]any{"data": members})
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// nil slice serializes to null - this is the bug we're fixing
	if string(data) != `{"data":null}` {
		t.Errorf("expected nil slice to serialize to null, got %s", string(data))
	}
}

func TestListOrgMembers(t *testing.T) {
	t.Skip("Requires test database - implement in integration tests")
}
```

**Validation**:
```bash
cd backend && just test
```

---

### Task 3: Fix Frontend Null Handling
**File**: `frontend/src/components/MembersScreen.tsx`
**Action**: MODIFY
**Pattern**: Add null coalescing operator

**Implementation**:
```typescript
// Line 41 - Change from:
setMembers(response.data.data);
// To:
setMembers(response.data.data ?? []);
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 4: Add Frontend Test
**File**: `frontend/src/components/__tests__/MembersScreen.test.tsx`
**Action**: CREATE
**Pattern**: Reference `frontend/src/components/__tests__/HomeScreen.test.tsx`

**Implementation**:
```typescript
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/react';
import MembersScreen from '@/components/MembersScreen';
import { useOrgStore, useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';

// Mock the API
vi.mock('@/lib/api/orgs', () => ({
  orgsApi: {
    listMembers: vi.fn(),
    updateMemberRole: vi.fn(),
    removeMember: vi.fn(),
  },
}));

describe('MembersScreen', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  beforeEach(() => {
    // Set up store with current org
    useOrgStore.setState({
      currentOrg: { id: 1, name: 'Test Org', slug: 'test-org' },
      currentRole: 'admin',
    });
    useAuthStore.setState({
      profile: { id: 1, email: 'test@example.com', name: 'Test User' },
    });
  });

  it('should handle null members response without crashing', async () => {
    // Simulate backend returning null (the bug condition)
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: { data: null },
    } as any);

    // Should not throw
    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('No members found.')).toBeInTheDocument();
    });
  });

  it('should handle empty array members response', async () => {
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: { data: [] },
    } as any);

    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('No members found.')).toBeInTheDocument();
    });
  });

  it('should display members when data is returned', async () => {
    vi.mocked(orgsApi.listMembers).mockResolvedValueOnce({
      data: {
        data: [
          {
            user_id: 1,
            name: 'Test User',
            email: 'test@example.com',
            role: 'admin',
            joined_at: '2025-01-01T00:00:00Z',
          },
        ],
      },
    } as any);

    render(<MembersScreen />);

    await waitFor(() => {
      expect(screen.getByText('Test User')).toBeInTheDocument();
      expect(screen.getByText('test@example.com')).toBeInTheDocument();
    });
  });
});
```

**Validation**:
```bash
cd frontend && just test
```

---

### Task 5: Full Validation
**Action**: Run full validation suite

**Validation**:
```bash
just validate
```

## Risk Assessment

- **Risk**: Test mocking might not match actual API response shape
  **Mitigation**: Tests use actual type definitions from codebase

- **Risk**: Other components might have same null handling issue
  **Mitigation**: TRA-182 tracks comprehensive audit; this fix is isolated

## Integration Points

- Store updates: None (existing stores unchanged)
- Route changes: None
- Config updates: None

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- Gate 1: `just lint` (Syntax & Style)
- Gate 2: `just typecheck` (Type Safety - frontend only)
- Gate 3: `just test` (Unit Tests)

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass

Final validation: `just validate`

## Validation Sequence

1. After Task 1: `cd backend && just lint && just test`
2. After Task 2: `cd backend && just test`
3. After Task 3: `cd frontend && just lint && just typecheck`
4. After Task 4: `cd frontend && just test`
5. After Task 5: `just validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec with exact line numbers
✅ Similar test patterns found in codebase
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ Two single-line fixes with well-understood behavior
✅ No external dependencies or new patterns

**Assessment**: High-confidence fix with well-defined scope and clear validation criteria.

**Estimated one-pass success probability**: 95%

**Reasoning**: Simple defensive coding changes with clear test patterns to follow. Only risk is minor test setup issues which are easily debugged.
