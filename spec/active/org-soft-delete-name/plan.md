# Implementation Plan: Soft Delete Organization Name Collision Fix

Generated: 2025-12-13
Specification: spec.md

## Understanding

When an organization is soft-deleted, its `name` and `identifier` remain in the database, blocking creation of new orgs with the same values. This fix mangles both fields on soft delete to free them for reuse while preserving audit trail.

**Mangled format**: `*** DELETED {RFC3339 timestamp} *** {original_value}`

Example:
- `"Acme Corp"` → `"*** DELETED 2025-12-13T12:45:00Z *** Acme Corp"`
- `"acme-corp"` → `"*** DELETED 2025-12-13T12:45:00Z *** acme-corp"`

## Relevant Files

**Reference Patterns**:
- `backend/internal/services/orgs/service.go` (lines 69-84) - Current delete flow
- `backend/internal/storage/organizations.go` (lines 112-123) - Current soft delete

**Files to Modify**:
- `backend/internal/storage/organizations.go` - Add `SoftDeleteOrganizationWithMangle()` method
- `backend/internal/services/orgs/service.go` - Update `DeleteOrgWithConfirmation()` to pass mangled values

**Files to Create**:
- `backend/internal/services/orgs/service_test.go` - Unit tests for name mangling logic

## Architecture Impact

- **Subsystems affected**: Backend (storage + service layers)
- **New dependencies**: None (uses standard `time` and `fmt` packages)
- **Breaking changes**: None (behavior change is internal, API unchanged)
- **Database**: `name` VARCHAR(255) and `identifier` VARCHAR(255) - plenty of room for mangled values

## Task Breakdown

### Task 1: Add SoftDeleteOrganizationWithMangle to Storage Layer

**File**: `backend/internal/storage/organizations.go`
**Action**: MODIFY - Add new method

**Implementation**:
```go
// SoftDeleteOrganizationWithMangle marks an organization as deleted and mangles name/identifier.
func (s *Storage) SoftDeleteOrganizationWithMangle(ctx context.Context, id int, mangledName, mangledIdentifier string, deletedAt time.Time) error {
    query := `
        UPDATE trakrf.organizations
        SET name = $2, identifier = $3, deleted_at = $4
        WHERE id = $1 AND deleted_at IS NULL
    `
    result, err := s.pool.Exec(ctx, query, id, mangledName, mangledIdentifier, deletedAt)
    if err != nil {
        return fmt.Errorf("failed to delete organization: %w", err)
    }
    if result.RowsAffected() == 0 {
        return fmt.Errorf("organization not found")
    }
    return nil
}
```

**Validation**: `just backend lint && just backend build`

### Task 2: Update DeleteOrgWithConfirmation in Service Layer

**File**: `backend/internal/services/orgs/service.go`
**Action**: MODIFY - Update existing method

**Implementation**:
```go
func (s *Service) DeleteOrgWithConfirmation(ctx context.Context, orgID int, confirmName string) error {
    org, err := s.storage.GetOrganizationByID(ctx, orgID)
    if err != nil {
        return fmt.Errorf("failed to get organization: %w", err)
    }
    if org == nil {
        return fmt.Errorf("organization not found")
    }

    // Case-insensitive comparison (GitHub-style)
    if !strings.EqualFold(org.Name, confirmName) {
        return fmt.Errorf("organization name does not match")
    }

    // Mangle name and identifier to free them for reuse
    deletedAt := time.Now().UTC()
    prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))
    mangledName := prefix + org.Name
    mangledIdentifier := prefix + org.Identifier

    return s.storage.SoftDeleteOrganizationWithMangle(ctx, orgID, mangledName, mangledIdentifier, deletedAt)
}
```

**Note**: Add `"time"` to imports if not already present.

**Validation**: `just backend lint && just backend build`

### Task 3: Add Unit Tests for Name Mangling

**File**: `backend/internal/services/orgs/service_test.go`
**Action**: CREATE

**Implementation**:
```go
package orgs

import (
    "testing"
    "time"
)

func TestMangleFormat(t *testing.T) {
    // Test that the mangle format is correct
    deletedAt := time.Date(2025, 12, 13, 12, 45, 0, 0, time.UTC)
    prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))

    mangledName := prefix + "Acme Corp"
    expected := "*** DELETED 2025-12-13T12:45:00Z *** Acme Corp"

    if mangledName != expected {
        t.Errorf("expected %q, got %q", expected, mangledName)
    }
}

func TestMangledNameLength(t *testing.T) {
    // Verify mangled names fit in VARCHAR(255)
    deletedAt := time.Now().UTC()
    prefix := fmt.Sprintf("*** DELETED %s *** ", deletedAt.Format(time.RFC3339))

    // Prefix is ~36 chars, leaving 219 for original name
    longName := string(make([]byte, 200)) // 200 char name
    mangledName := prefix + longName

    if len(mangledName) > 255 {
        t.Errorf("mangled name exceeds 255 chars: %d", len(mangledName))
    }
}
```

**Validation**: `just backend test`

### Task 4: Integration Validation

**Action**: Manual verification + full test suite

**Steps**:
1. Run full backend validation: `just backend validate`
2. Verify the flow works end-to-end (if dev server available):
   - Create org "Test Org"
   - Delete org "Test Org"
   - Check DB: name should be `*** DELETED 2025-... *** Test Org`
   - Create new org "Test Org" - should succeed

**Validation**: `just backend validate`

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Long names exceed VARCHAR(255) | Low | 255 - 36 (prefix) = 219 chars for original name; most names <100 chars |
| Old `SoftDeleteOrganization` still called | Low | Search codebase for callers; only `DeleteOrgWithConfirmation` uses it |

## VALIDATION GATES (MANDATORY)

After EVERY code change:
- **Gate 1**: `just backend lint` - Must pass
- **Gate 2**: `just backend build` - Must compile
- **Gate 3**: `just backend test` - All tests pass

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task: `just backend lint && just backend build && just backend test`

Final validation: `just backend validate`

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec
- ✅ Simple modification to existing patterns
- ✅ No external dependencies
- ✅ Database schema has room (VARCHAR 255)
- ✅ Single subsystem (backend only)
- ⚠️ No existing tests for delete flow (creating new ones)

**Assessment**: Straightforward modification with clear implementation path.

**Estimated one-pass success probability**: 95%

**Reasoning**: Simple string manipulation in well-understood code paths. Only uncertainty is integration testing if dev environment isn't available.
