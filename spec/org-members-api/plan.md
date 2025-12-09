# Implementation Plan: Organization Member Management API

Generated: 2024-12-09
Specification: spec.md

## Understanding

Implement 3 API endpoints for managing organization members:
- **GET /api/v1/orgs/:id/members** - List members with roles (any member can view)
- **PUT /api/v1/orgs/:id/members/:userId** - Update member role (admin only)
- **DELETE /api/v1/orgs/:id/members/:userId** - Remove member (admin only)

Key business rules:
- Last admin protection (cannot remove/demote the only admin)
- Self-removal prevention (must have another admin remove you)
- Role validation (viewer, operator, manager, admin)

## Relevant Files

**Reference Patterns** (existing code to follow):
- `backend/internal/handlers/orgs/orgs.go` (lines 30-85) - Handler patterns for List, Create
- `backend/internal/handlers/orgs/orgs.go` (lines 186-198) - Route registration with RBAC middleware
- `backend/internal/services/orgs/service.go` (lines 66-81) - Service pattern with error handling
- `backend/internal/storage/org_users.go` (lines 17-65) - Storage query patterns

**Files to Create**:
- `backend/internal/handlers/orgs/members.go` - Member management handlers

**Files to Modify**:
- `backend/internal/apierrors/messages.go` - Add member error messages
- `backend/internal/storage/org_users.go` - Replace stubs with real implementations
- `backend/internal/services/orgs/service.go` - Add member business logic
- `backend/internal/handlers/orgs/orgs.go` - Register member routes

## Architecture Impact
- **Subsystems affected**: Backend (handlers, services, storage)
- **New dependencies**: None
- **Breaking changes**: None (new endpoints only)

---

## Task Breakdown

### Task 1: Add Member Error Messages
**File**: `backend/internal/apierrors/messages.go`
**Action**: MODIFY
**Pattern**: Follow existing org error message block (lines 93-114)

**Implementation**:
```go
// Member management error messages (add after org messages)
const (
	MemberListFailed       = "Failed to list members"
	MemberUpdateInvalidID  = "Invalid user ID"
	MemberUpdateInvalidJSON = "Invalid JSON"
	MemberUpdateValidationFail = "Validation failed"
	MemberUpdateFailed     = "Failed to update member role"
	MemberNotFound         = "Member not found"
	MemberRemoveFailed     = "Failed to remove member"
	MemberLastAdmin        = "Cannot remove or demote the last admin"
	MemberSelfRemoval      = "Cannot remove yourself"
	MemberInvalidRole      = "Invalid role"
)
```

**Validation**: `just backend lint`

---

### Task 2: Add Member Model Types
**File**: `backend/internal/models/organization/organization.go`
**Action**: MODIFY
**Pattern**: Follow existing types in same file

**Implementation**:
```go
// OrgMember represents a member in an organization for the list response
type OrgMember struct {
	UserID   int       `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// UpdateMemberRoleRequest for PUT /api/v1/orgs/:id/members/:userId
type UpdateMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=viewer operator manager admin"`
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 3: Storage - Clean Up Stubs
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY
**Pattern**: N/A - removing dead code

**Implementation**:
Delete these stub functions (lines 83-111):
- `ListOrgUsers`
- `GetOrgUser`
- `CreateOrgUser`
- `UpdateOrgUser`
- `SoftDeleteOrgUser`

**Validation**: `just backend build` (will fail until Task 4 complete)

---

### Task 4: Storage - ListOrgMembers
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY
**Pattern**: Reference `ListUserOrgs` in organizations.go for JOIN pattern

**Implementation**:
```go
// ListOrgMembers returns all members of an organization with user details
func (s *Storage) ListOrgMembers(ctx context.Context, orgID int) ([]organization.OrgMember, error) {
	query := `
		SELECT ou.user_id, u.name, u.email, ou.role, ou.created_at
		FROM trakrf.org_users ou
		JOIN trakrf.users u ON u.id = ou.user_id
		WHERE ou.org_id = $1 AND ou.deleted_at IS NULL AND u.deleted_at IS NULL
		ORDER BY ou.created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list org members: %w", err)
	}
	defer rows.Close()

	var members []organization.OrgMember
	for rows.Next() {
		var m organization.OrgMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}
```

**Validation**: `just backend build`

---

### Task 5: Storage - UpdateMemberRole
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY

**Implementation**:
```go
// UpdateMemberRole updates a member's role in an organization
func (s *Storage) UpdateMemberRole(ctx context.Context, orgID, userID int, role models.OrgRole) error {
	query := `
		UPDATE trakrf.org_users
		SET role = $3, updated_at = NOW()
		WHERE org_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`
	result, err := s.pool.Exec(ctx, query, orgID, userID, role)
	if err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgUserNotFound
	}
	return nil
}
```

**Validation**: `just backend build`

---

### Task 6: Storage - RemoveMember
**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY

**Implementation**:
```go
// RemoveMember removes a user from an organization (hard delete)
func (s *Storage) RemoveMember(ctx context.Context, orgID, userID int) error {
	query := `
		DELETE FROM trakrf.org_users
		WHERE org_id = $1 AND user_id = $2
	`
	result, err := s.pool.Exec(ctx, query, orgID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOrgUserNotFound
	}
	return nil
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 7: Service - ListMembers
**File**: `backend/internal/services/orgs/service.go`
**Action**: MODIFY
**Pattern**: Reference `GetUserProfile` (lines 84-139) for storage call pattern

**Implementation**:
```go
// ListMembers returns all members of an organization
func (s *Service) ListMembers(ctx context.Context, orgID int) ([]organization.OrgMember, error) {
	members, err := s.storage.ListOrgMembers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	return members, nil
}
```

**Validation**: `just backend build`

---

### Task 8: Service - UpdateMemberRole
**File**: `backend/internal/services/orgs/service.go`
**Action**: MODIFY

**Implementation**:
```go
// UpdateMemberRole updates a member's role with last-admin protection
func (s *Service) UpdateMemberRole(ctx context.Context, orgID, targetUserID int, newRole models.OrgRole) error {
	// Get current role
	currentRole, err := s.storage.GetUserOrgRole(ctx, targetUserID, orgID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// If demoting from admin, check if they're the last admin
	if currentRole == models.RoleAdmin && newRole != models.RoleAdmin {
		adminCount, err := s.storage.CountOrgAdmins(ctx, orgID)
		if err != nil {
			return fmt.Errorf("failed to check admin count: %w", err)
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot demote the last admin")
		}
	}

	return s.storage.UpdateMemberRole(ctx, orgID, targetUserID, newRole)
}
```

**Validation**: `just backend build`

---

### Task 9: Service - RemoveMember
**File**: `backend/internal/services/orgs/service.go`
**Action**: MODIFY

**Implementation**:
```go
// RemoveMember removes a member with last-admin and self-removal protection
func (s *Service) RemoveMember(ctx context.Context, orgID, targetUserID, actorUserID int) error {
	// Prevent self-removal
	if targetUserID == actorUserID {
		return fmt.Errorf("cannot remove yourself")
	}

	// Check if target is a member
	targetRole, err := s.storage.GetUserOrgRole(ctx, targetUserID, orgID)
	if err != nil {
		return fmt.Errorf("member not found")
	}

	// If removing an admin, check if they're the last admin
	if targetRole == models.RoleAdmin {
		adminCount, err := s.storage.CountOrgAdmins(ctx, orgID)
		if err != nil {
			return fmt.Errorf("failed to check admin count: %w", err)
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot remove the last admin")
		}
	}

	return s.storage.RemoveMember(ctx, orgID, targetUserID)
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 10: Create Member Handlers
**File**: `backend/internal/handlers/orgs/members.go`
**Action**: CREATE
**Pattern**: Reference `orgs.go` handlers for structure

**Implementation**:
```go
package orgs

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// ListMembers returns all members of an organization.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	members, err := h.service.ListMembers(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberListFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": members})
}

// UpdateMemberRole updates a member's role in an organization.
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "userId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	var request organization.UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidJSON, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.MemberUpdateValidationFail, err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	role := models.OrgRole(request.Role)
	if !role.IsValid() {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.MemberInvalidRole, "", middleware.GetRequestID(r.Context()))
		return
	}

	err = h.service.UpdateMemberRole(r.Context(), orgID, userID, role)
	if err != nil {
		if err.Error() == "member not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.MemberNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "cannot demote the last admin" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberLastAdmin, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberUpdateFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Role updated"})
}

// RemoveMember removes a member from an organization.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserClaims(r)
	if claims == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			"Unauthorized", "", middleware.GetRequestID(r.Context()))
		return
	}

	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.OrgGetInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	userID, err := strconv.Atoi(chi.URLParam(r, "userId"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.MemberUpdateInvalidID, "", middleware.GetRequestID(r.Context()))
		return
	}

	err = h.service.RemoveMember(r.Context(), orgID, userID, claims.UserID)
	if err != nil {
		if err.Error() == "cannot remove yourself" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberSelfRemoval, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "member not found" {
			httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
				apierrors.MemberNotFound, "", middleware.GetRequestID(r.Context()))
			return
		}
		if err.Error() == "cannot remove the last admin" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.MemberLastAdmin, "", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.MemberRemoveFailed, "", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"message": "Member removed"})
}
```

**Validation**: `just backend lint && just backend build`

---

### Task 11: Register Member Routes
**File**: `backend/internal/handlers/orgs/orgs.go`
**Action**: MODIFY
**Pattern**: Reference existing route registration (lines 186-198)

**Implementation**:
Add to `RegisterRoutes` function, inside the `/api/v1/orgs/{id}` route group:

```go
// Member management routes
r.With(middleware.RequireOrgMember(store)).Get("/members", h.ListMembers)
r.With(middleware.RequireOrgAdmin(store)).Put("/members/{userId}", h.UpdateMemberRole)
r.With(middleware.RequireOrgAdmin(store)).Delete("/members/{userId}", h.RemoveMember)
```

**Validation**: `just backend lint && just backend build`

---

### Task 12: Update Route Tests
**File**: `backend/main_test.go`
**Action**: MODIFY
**Pattern**: Reference existing route tests

**Implementation**:
Add to the tests slice in `TestRouterRegistration`:

```go
{"GET", "/api/v1/orgs/1/members"},
{"PUT", "/api/v1/orgs/1/members/1"},
{"DELETE", "/api/v1/orgs/1/members/1"},
```

**Validation**: `just backend test`

---

### Task 13: Final Validation
**Action**: VALIDATE

Run full validation suite:
```bash
just backend lint
just backend build
just backend test
```

Verify:
- All 3 new endpoints are registered
- No lint errors
- Build succeeds
- Route tests pass

---

## Risk Assessment

- **Risk**: Last admin check race condition
  **Mitigation**: The check and update are not atomic, but this is acceptable for MVP. A transaction-based approach could be added later if needed.

- **Risk**: Removing member doesn't clean up their last_org_id
  **Mitigation**: The `/users/me` endpoint already handles this gracefully by falling back to the first org if last_org_id is invalid.

## Integration Points

- **Store updates**: None - using existing Storage struct
- **Route changes**: 3 new routes under `/api/v1/orgs/{id}/members`
- **Config updates**: None

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
- Gate 1: `just backend lint` - Syntax & Style
- Gate 2: `just backend build` - Compiles successfully
- Gate 3: `just backend test` - All tests pass

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- Do not proceed to next task until current task passes all gates

## Validation Sequence

After each task: `just backend lint && just backend build`
After Task 12: `just backend test`
Final validation: `just backend validate`

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase at `handlers/orgs/orgs.go`
✅ All clarifying questions answered
✅ Existing storage patterns to follow at `storage/org_users.go`
✅ OrgRole validation already exists at `models/org_role.go`
✅ CountOrgAdmins already implemented
✅ RBAC middleware already tested from Phase 2a

**Assessment**: High confidence - straightforward extension of existing patterns with clear business rules.

**Estimated one-pass success probability**: 90%

**Reasoning**: All patterns exist, no new dependencies, clear validation criteria. Only minor risk is typos or missed imports.
