# Implementation Plan: Organization RBAC - Phase 1 (Database & Types)

Generated: 2024-12-08
Specification: spec.md

## Understanding

This phase establishes the foundation for RBAC:
1. **Database migration** - Add `org_role` enum, update `org_users.role` to use it, add `is_superadmin` and `last_org_id` to users, create `org_invitations` table
2. **Go types** - `OrgRole` type with permission methods, `OrgInvitation` model
3. **RBAC middleware** - Middleware to check org membership and role, with access denial logging

The existing `org_users.role` column uses VARCHAR with CHECK constraint (`owner, admin, member, readonly`). We'll migrate to an enum with new values (`viewer, operator, manager, admin`).

## Clarifications Applied

- OrgRole lives in `internal/models/org_role.go`
- RBAC middleware extracts org ID from URL param `:orgId`
- Migration backfill uses first user in `org_users` as admin if `created_by` is NULL
- Log all access denials (user ID, org ID, required role, timestamp)

## Relevant Files

**Reference Patterns**:
- `backend/migrations/000004_org_users.up.sql` - Existing org_users schema
- `backend/internal/middleware/middleware.go` (lines 119-169) - Auth middleware pattern
- `backend/internal/models/user/user.go` - Model struct pattern
- `backend/internal/models/org_user/org_user.go` - OrgUser model to update
- `backend/internal/storage/users.go` - Storage query patterns

**Files to Create**:
- `backend/migrations/000022_org_rbac.up.sql` - Schema migration
- `backend/migrations/000022_org_rbac.down.sql` - Rollback migration
- `backend/internal/models/org_role.go` - OrgRole type and permissions
- `backend/internal/models/invitation/invitation.go` - OrgInvitation model
- `backend/internal/middleware/rbac.go` - RBAC middleware

**Files to Modify**:
- `backend/internal/models/user/user.go` - Add IsSuperadmin, LastOrgID fields
- `backend/internal/models/org_user/org_user.go` - Change Role type to OrgRole
- `backend/internal/storage/org_users.go` - Add GetUserOrgRole, CountOrgAdmins

## Architecture Impact

- **Subsystems affected**: Database, Backend Models, Backend Middleware
- **New dependencies**: None
- **Breaking changes**: `org_users.role` values change from `owner/admin/member/readonly` to `viewer/operator/manager/admin`. Per stack.md, this is acceptable (no production deployments).

---

## Task Breakdown

### Task 1: Create Database Migration (UP)

**File**: `backend/migrations/000022_org_rbac.up.sql`
**Action**: CREATE

**Implementation**:
```sql
-- Set search path for trakrf schema
SET search_path = trakrf, public;

-- 1. Create org_role enum type
CREATE TYPE org_role AS ENUM ('viewer', 'operator', 'manager', 'admin');

-- 2. Migrate org_users.role from VARCHAR to enum
-- First, drop the existing CHECK constraint
ALTER TABLE org_users DROP CONSTRAINT IF EXISTS valid_role;

-- Map old roles to new roles and convert column
ALTER TABLE org_users
  ALTER COLUMN role DROP DEFAULT,
  ALTER COLUMN role TYPE org_role USING (
    CASE role
      WHEN 'owner' THEN 'admin'::org_role
      WHEN 'admin' THEN 'admin'::org_role
      WHEN 'member' THEN 'operator'::org_role
      WHEN 'readonly' THEN 'viewer'::org_role
      ELSE 'viewer'::org_role
    END
  ),
  ALTER COLUMN role SET DEFAULT 'viewer'::org_role;

-- 3. Add superadmin flag to users
ALTER TABLE users ADD COLUMN is_superadmin BOOLEAN NOT NULL DEFAULT FALSE;

-- 4. Add last_org_id to users for org switching
ALTER TABLE users ADD COLUMN last_org_id INTEGER REFERENCES organizations(id) ON DELETE SET NULL;

-- 5. Create org_invitations table
CREATE TABLE org_invitations (
  id SERIAL PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email VARCHAR(255) NOT NULL,
  role org_role NOT NULL DEFAULT 'viewer',
  token VARCHAR(64) NOT NULL,
  invited_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT unique_org_email UNIQUE(org_id, email)
);

CREATE INDEX idx_org_invitations_token ON org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON org_invitations(org_id);
CREATE INDEX idx_org_invitations_email ON org_invitations(email);

-- 6. Add soft delete support to organizations (if not exists)
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- 7. Backfill existing org creators as admin
-- First, try to set role based on created_by
UPDATE org_users ou
SET role = 'admin'::org_role
FROM organizations o
WHERE ou.org_id = o.id
  AND ou.user_id = o.created_by
  AND o.created_by IS NOT NULL;

-- For orgs where created_by is NULL, make the first user (by created_at) the admin
WITH first_users AS (
  SELECT DISTINCT ON (org_id) org_id, user_id
  FROM org_users
  WHERE org_id IN (
    SELECT id FROM organizations WHERE created_by IS NULL
  )
  ORDER BY org_id, created_at ASC
)
UPDATE org_users ou
SET role = 'admin'::org_role
FROM first_users fu
WHERE ou.org_id = fu.org_id
  AND ou.user_id = fu.user_id;
```

**Validation**:
```bash
just db-migrate up
# Verify: psql -c "SELECT * FROM trakrf.org_users LIMIT 5;"
# Verify: psql -c "\d trakrf.org_invitations"
```

---

### Task 2: Create Database Migration (DOWN)

**File**: `backend/migrations/000022_org_rbac.down.sql`
**Action**: CREATE

**Implementation**:
```sql
SET search_path = trakrf, public;

-- 1. Drop invitations table
DROP TABLE IF EXISTS org_invitations;

-- 2. Remove new user columns
ALTER TABLE users DROP COLUMN IF EXISTS last_org_id;
ALTER TABLE users DROP COLUMN IF EXISTS is_superadmin;

-- 3. Convert org_users.role back to VARCHAR
ALTER TABLE org_users
  ALTER COLUMN role DROP DEFAULT,
  ALTER COLUMN role TYPE VARCHAR(50) USING (
    CASE role::text
      WHEN 'admin' THEN 'admin'
      WHEN 'manager' THEN 'admin'
      WHEN 'operator' THEN 'member'
      WHEN 'viewer' THEN 'readonly'
      ELSE 'member'
    END
  ),
  ALTER COLUMN role SET DEFAULT 'member';

-- Restore CHECK constraint
ALTER TABLE org_users ADD CONSTRAINT valid_role
  CHECK (role IN ('owner', 'admin', 'member', 'readonly'));

-- 4. Drop enum type
DROP TYPE IF EXISTS org_role;

-- Note: deleted_at on organizations is left in place (harmless)
```

**Validation**:
```bash
just db-migrate down
just db-migrate up
# Verify migration is reversible
```

---

### Task 3: Create OrgRole Type

**File**: `backend/internal/models/org_role.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/user/user.go`

**Implementation**:
```go
package models

import (
	"database/sql/driver"
	"fmt"
)

// OrgRole represents a user's role within an organization
type OrgRole string

const (
	RoleViewer   OrgRole = "viewer"
	RoleOperator OrgRole = "operator"
	RoleManager  OrgRole = "manager"
	RoleAdmin    OrgRole = "admin"
)

// AllRoles returns all valid org roles in order of increasing privilege
func AllRoles() []OrgRole {
	return []OrgRole{RoleViewer, RoleOperator, RoleManager, RoleAdmin}
}

// IsValid checks if the role is a valid OrgRole value
func (r OrgRole) IsValid() bool {
	switch r {
	case RoleViewer, RoleOperator, RoleManager, RoleAdmin:
		return true
	}
	return false
}

// String returns the string representation of the role
func (r OrgRole) String() string {
	return string(r)
}

// Permission checks

// CanView returns true if the role can view assets and locations
func (r OrgRole) CanView() bool {
	return r.IsValid() // All valid roles can view
}

// CanScan returns true if the role can run and save scans
func (r OrgRole) CanScan() bool {
	return r == RoleOperator || r == RoleManager || r == RoleAdmin
}

// CanManageAssets returns true if the role can create/edit assets and locations
func (r OrgRole) CanManageAssets() bool {
	return r == RoleManager || r == RoleAdmin
}

// CanExportReports returns true if the role can export reports
func (r OrgRole) CanExportReports() bool {
	return r == RoleManager || r == RoleAdmin
}

// CanManageUsers returns true if the role can invite/remove users and change roles
func (r OrgRole) CanManageUsers() bool {
	return r == RoleAdmin
}

// CanManageOrg returns true if the role can edit org settings and delete org
func (r OrgRole) CanManageOrg() bool {
	return r == RoleAdmin
}

// HasAtLeast returns true if this role has at least the permissions of minRole
func (r OrgRole) HasAtLeast(minRole OrgRole) bool {
	roleOrder := map[OrgRole]int{
		RoleViewer:   1,
		RoleOperator: 2,
		RoleManager:  3,
		RoleAdmin:    4,
	}
	return roleOrder[r] >= roleOrder[minRole]
}

// Scan implements sql.Scanner for database reads
func (r *OrgRole) Scan(value interface{}) error {
	if value == nil {
		*r = RoleViewer
		return nil
	}
	switch v := value.(type) {
	case string:
		*r = OrgRole(v)
	case []byte:
		*r = OrgRole(string(v))
	default:
		return fmt.Errorf("cannot scan %T into OrgRole", value)
	}
	if !r.IsValid() {
		return fmt.Errorf("invalid org role: %s", *r)
	}
	return nil
}

// Value implements driver.Valuer for database writes
func (r OrgRole) Value() (driver.Value, error) {
	if !r.IsValid() {
		return nil, fmt.Errorf("invalid org role: %s", r)
	}
	return string(r), nil
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 4: Create OrgInvitation Model

**File**: `backend/internal/models/invitation/invitation.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/models/user/user.go`

**Implementation**:
```go
package invitation

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models"
)

// OrgInvitation represents an invitation to join an organization
type OrgInvitation struct {
	ID          int            `json:"id"`
	OrgID       int            `json:"org_id"`
	Email       string         `json:"email"`
	Role        models.OrgRole `json:"role"`
	Token       string         `json:"-"` // Never expose token in JSON responses
	InvitedBy   *int           `json:"invited_by,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at"`
	AcceptedAt  *time.Time     `json:"accepted_at,omitempty"`
	CancelledAt *time.Time     `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`

	// Joined fields (populated by queries with JOINs)
	OrgName     string `json:"org_name,omitempty"`
	InviterName string `json:"inviter_name,omitempty"`
}

// IsPending returns true if the invitation has not been accepted or cancelled
func (i *OrgInvitation) IsPending() bool {
	return i.AcceptedAt == nil && i.CancelledAt == nil
}

// IsExpired returns true if the invitation has passed its expiry time
func (i *OrgInvitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

// IsValid returns true if the invitation can be accepted
func (i *OrgInvitation) IsValid() bool {
	return i.IsPending() && !i.IsExpired()
}

// CreateInvitationRequest represents a request to create an invitation
type CreateInvitationRequest struct {
	Email string         `json:"email" validate:"required,email"`
	Role  models.OrgRole `json:"role" validate:"required"`
}

// AcceptInvitationRequest represents a request to accept an invitation
type AcceptInvitationRequest struct {
	Token string `json:"token" validate:"required,len=64"`
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 5: Update User Model

**File**: `backend/internal/models/user/user.go`
**Action**: MODIFY
**Pattern**: Add new fields following existing struct pattern

**Changes**:
Add two fields to the `User` struct:
```go
type User struct {
	ID           int        `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	Settings     any        `json:"settings"`
	Metadata     any        `json:"metadata"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// New fields for RBAC
	IsSuperadmin bool `json:"is_superadmin"`
	LastOrgID    *int `json:"last_org_id,omitempty"`
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 6: Update OrgUser Model

**File**: `backend/internal/models/org_user/org_user.go`
**Action**: MODIFY

**Changes**:
Change `Role` field type from `string` to `models.OrgRole`:
```go
import (
	"time"

	"github.com/trakrf/platform/backend/internal/models"
)

type OrgUser struct {
	OrgID     int            `json:"org_id"`
	UserID    int            `json:"user_id"`
	Role      models.OrgRole `json:"role"` // Changed from string
	Status    string         `json:"status"`
	Settings  any            `json:"settings"`
	Metadata  any            `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt *time.Time     `json:"deleted_at,omitempty"`
	UserEmail string         `json:"user_email"`
	UserName  string         `json:"user_name"`
}
```

Also update `CreateOrgUserRequest` and `UpdateOrgUserRequest`:
```go
type CreateOrgUserRequest struct {
	OrgID  int            `json:"org_id" validate:"required"`
	UserID int            `json:"user_id" validate:"required"`
	Role   models.OrgRole `json:"role" validate:"required"`
}

type UpdateOrgUserRequest struct {
	Role   *models.OrgRole `json:"role" validate:"omitempty"`
	Status *string         `json:"status" validate:"omitempty,oneof=active inactive"`
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 7: Create RBAC Middleware

**File**: `backend/internal/middleware/rbac.go`
**Action**: CREATE
**Pattern**: Reference `backend/internal/middleware/middleware.go` (lines 119-169)

**Implementation**:
```go
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type contextKey string

const orgRoleKey contextKey = "org_role"

// OrgRoleStore defines the storage methods needed by RBAC middleware
type OrgRoleStore interface {
	GetUserOrgRole(ctx context.Context, userID, orgID int) (models.OrgRole, error)
	IsUserSuperadmin(ctx context.Context, userID int) (bool, error)
}

// RequireOrgMember checks that the authenticated user is a member of the org
// specified by the :orgId URL parameter. Sets the user's role in context.
func RequireOrgMember(store OrgRoleStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := GetRequestID(ctx)

			// Get user claims from auth middleware
			claims := GetUserClaims(r)
			if claims == nil {
				httputil.WriteJSONError(w, r, http.StatusUnauthorized,
					errors.ErrUnauthorized, "Unauthorized", "Authentication required", requestID)
				return
			}

			// Extract org ID from URL
			orgIDStr := chi.URLParam(r, "orgId")
			if orgIDStr == "" {
				// Try alternate param name
				orgIDStr = chi.URLParam(r, "id")
			}
			if orgIDStr == "" {
				httputil.WriteJSONError(w, r, http.StatusBadRequest,
					errors.ErrBadRequest, "Bad Request", "Organization ID required", requestID)
				return
			}

			orgID, err := strconv.Atoi(orgIDStr)
			if err != nil {
				httputil.WriteJSONError(w, r, http.StatusBadRequest,
					errors.ErrBadRequest, "Bad Request", "Invalid organization ID", requestID)
				return
			}

			// Check for superadmin bypass
			isSuperadmin, err := store.IsUserSuperadmin(ctx, claims.UserID)
			if err != nil {
				slog.Error("failed to check superadmin status",
					"user_id", claims.UserID,
					"error", err)
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Failed to check permissions", requestID)
				return
			}

			if isSuperadmin {
				// Log superadmin access
				slog.Warn("superadmin org access",
					"user_id", claims.UserID,
					"org_id", orgID,
					"path", r.URL.Path,
					"method", r.Method)
				// Grant admin role to superadmin
				ctx = context.WithValue(ctx, orgRoleKey, models.RoleAdmin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Get user's role in the org
			role, err := store.GetUserOrgRole(ctx, claims.UserID, orgID)
			if err != nil {
				if err == storage.ErrOrgUserNotFound {
					logAccessDenied(claims.UserID, orgID, "member", r)
					httputil.WriteJSONError(w, r, http.StatusForbidden,
						errors.ErrUnauthorized, "Forbidden", "You are not a member of this organization", requestID)
					return
				}
				slog.Error("failed to get user org role",
					"user_id", claims.UserID,
					"org_id", orgID,
					"error", err)
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Failed to check permissions", requestID)
				return
			}

			// Store role in context for handlers
			ctx = context.WithValue(ctx, orgRoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireOrgRole checks that the user has at least the specified role
func RequireOrgRole(store OrgRoleStore, minRole models.OrgRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// First ensure user is a member
		memberCheck := RequireOrgMember(store)
		return memberCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := GetRequestID(ctx)
			claims := GetUserClaims(r)

			role, ok := GetOrgRole(ctx)
			if !ok {
				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal Error", "Role not found in context", requestID)
				return
			}

			if !role.HasAtLeast(minRole) {
				orgIDStr := chi.URLParam(r, "orgId")
				if orgIDStr == "" {
					orgIDStr = chi.URLParam(r, "id")
				}
				orgID, _ := strconv.Atoi(orgIDStr)
				logAccessDenied(claims.UserID, orgID, minRole.String(), r)

				httputil.WriteJSONError(w, r, http.StatusForbidden,
					errors.ErrUnauthorized, "Forbidden",
					"Insufficient permissions. Required role: "+minRole.String(), requestID)
				return
			}

			next.ServeHTTP(w, r)
		}))
	}
}

// RequireOrgAdmin is a convenience wrapper for RequireOrgRole(store, RoleAdmin)
func RequireOrgAdmin(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleAdmin)
}

// RequireOrgManager is a convenience wrapper for RequireOrgRole(store, RoleManager)
func RequireOrgManager(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleManager)
}

// RequireOrgOperator is a convenience wrapper for RequireOrgRole(store, RoleOperator)
func RequireOrgOperator(store OrgRoleStore) func(http.Handler) http.Handler {
	return RequireOrgRole(store, models.RoleOperator)
}

// GetOrgRole retrieves the user's org role from context
func GetOrgRole(ctx context.Context) (models.OrgRole, bool) {
	role, ok := ctx.Value(orgRoleKey).(models.OrgRole)
	return role, ok
}

// logAccessDenied logs denied access attempts for audit purposes
func logAccessDenied(userID, orgID int, requiredRole string, r *http.Request) {
	slog.Warn("access denied",
		"user_id", userID,
		"org_id", orgID,
		"required_role", requiredRole,
		"path", r.URL.Path,
		"method", r.Method,
		"request_id", GetRequestID(r.Context()))
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 8: Add Storage Methods for RBAC

**File**: `backend/internal/storage/org_users.go`
**Action**: MODIFY
**Pattern**: Reference `backend/internal/storage/users.go`

**Add these methods**:
```go
import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/trakrf/platform/backend/internal/models"
)

// ErrOrgUserNotFound is returned when a user is not a member of an org
var ErrOrgUserNotFound = fmt.Errorf("user is not a member of this organization")

// GetUserOrgRole returns the user's role in the specified organization
func (s *Storage) GetUserOrgRole(ctx context.Context, userID, orgID int) (models.OrgRole, error) {
	query := `
		SELECT role
		FROM trakrf.org_users
		WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
	`
	var role models.OrgRole
	err := s.pool.QueryRow(ctx, query, userID, orgID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrOrgUserNotFound
		}
		return "", fmt.Errorf("failed to get user org role: %w", err)
	}
	return role, nil
}

// IsUserSuperadmin checks if the user has the superadmin flag set
func (s *Storage) IsUserSuperadmin(ctx context.Context, userID int) (bool, error) {
	query := `
		SELECT is_superadmin
		FROM trakrf.users
		WHERE id = $1 AND deleted_at IS NULL
	`
	var isSuperadmin bool
	err := s.pool.QueryRow(ctx, query, userID).Scan(&isSuperadmin)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check superadmin status: %w", err)
	}
	return isSuperadmin, nil
}

// CountOrgAdmins returns the number of admins in an organization
func (s *Storage) CountOrgAdmins(ctx context.Context, orgID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM trakrf.org_users
		WHERE org_id = $1 AND role = 'admin' AND deleted_at IS NULL
	`
	var count int
	err := s.pool.QueryRow(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count org admins: %w", err)
	}
	return count, nil
}
```

**Validation**:
```bash
just backend lint
just backend build
```

---

### Task 9: Update User Storage Queries

**File**: `backend/internal/storage/users.go`
**Action**: MODIFY

**Update SELECT queries to include new fields**:

In `GetUserByID`, `GetUserByEmail`, `CreateUser`, `UpdateUser` - add `is_superadmin, last_org_id` to:
1. SELECT column list
2. Scan parameters

Example for `GetUserByEmail`:
```go
query := `
    SELECT id, email, name, password_hash, last_login_at, settings, metadata,
           created_at, updated_at, is_superadmin, last_org_id
    FROM trakrf.users
    WHERE email = $1 AND deleted_at IS NULL
`
var usr user.User
err := s.pool.QueryRow(ctx, query, email).Scan(
    &usr.ID, &usr.Email, &usr.Name, &usr.PasswordHash, &usr.LastLoginAt,
    &usr.Settings, &usr.Metadata, &usr.CreatedAt, &usr.UpdatedAt,
    &usr.IsSuperadmin, &usr.LastOrgID)
```

**Validation**:
```bash
just backend lint
just backend test
```

---

### Task 10: Write Unit Tests for OrgRole

**File**: `backend/internal/models/org_role_test.go`
**Action**: CREATE

**Implementation**:
```go
package models

import (
	"testing"
)

func TestOrgRole_IsValid(t *testing.T) {
	tests := []struct {
		role     OrgRole
		expected bool
	}{
		{RoleViewer, true},
		{RoleOperator, true},
		{RoleManager, true},
		{RoleAdmin, true},
		{OrgRole("invalid"), false},
		{OrgRole(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.expected {
				t.Errorf("OrgRole(%q).IsValid() = %v, want %v", tt.role, got, tt.expected)
			}
		})
	}
}

func TestOrgRole_Permissions(t *testing.T) {
	tests := []struct {
		role            OrgRole
		canScan         bool
		canManageAssets bool
		canManageUsers  bool
		canManageOrg    bool
	}{
		{RoleViewer, false, false, false, false},
		{RoleOperator, true, false, false, false},
		{RoleManager, true, true, false, false},
		{RoleAdmin, true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := tt.role.CanScan(); got != tt.canScan {
				t.Errorf("%s.CanScan() = %v, want %v", tt.role, got, tt.canScan)
			}
			if got := tt.role.CanManageAssets(); got != tt.canManageAssets {
				t.Errorf("%s.CanManageAssets() = %v, want %v", tt.role, got, tt.canManageAssets)
			}
			if got := tt.role.CanManageUsers(); got != tt.canManageUsers {
				t.Errorf("%s.CanManageUsers() = %v, want %v", tt.role, got, tt.canManageUsers)
			}
			if got := tt.role.CanManageOrg(); got != tt.canManageOrg {
				t.Errorf("%s.CanManageOrg() = %v, want %v", tt.role, got, tt.canManageOrg)
			}
		})
	}
}

func TestOrgRole_HasAtLeast(t *testing.T) {
	tests := []struct {
		role     OrgRole
		minRole  OrgRole
		expected bool
	}{
		{RoleAdmin, RoleViewer, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleManager, RoleOperator, true},
		{RoleOperator, RoleManager, false},
		{RoleViewer, RoleAdmin, false},
		{RoleViewer, RoleViewer, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.role)+"_vs_"+string(tt.minRole), func(t *testing.T) {
			if got := tt.role.HasAtLeast(tt.minRole); got != tt.expected {
				t.Errorf("%s.HasAtLeast(%s) = %v, want %v", tt.role, tt.minRole, got, tt.expected)
			}
		})
	}
}

func TestOrgRole_ScanValue(t *testing.T) {
	var role OrgRole

	// Test scanning string
	err := role.Scan("admin")
	if err != nil || role != RoleAdmin {
		t.Errorf("Scan(\"admin\") failed: err=%v, role=%v", err, role)
	}

	// Test scanning []byte
	err = role.Scan([]byte("viewer"))
	if err != nil || role != RoleViewer {
		t.Errorf("Scan([]byte) failed: err=%v, role=%v", err, role)
	}

	// Test scanning nil
	err = role.Scan(nil)
	if err != nil || role != RoleViewer {
		t.Errorf("Scan(nil) failed: err=%v, role=%v", err, role)
	}

	// Test Value
	val, err := RoleManager.Value()
	if err != nil || val != "manager" {
		t.Errorf("RoleManager.Value() failed: err=%v, val=%v", err, val)
	}

	// Test invalid role
	err = role.Scan("invalid_role")
	if err == nil {
		t.Error("Scan(\"invalid_role\") should have returned error")
	}
}
```

**Validation**:
```bash
just backend test
```

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Migration breaks existing data | Careful role mapping (owner→admin, member→operator, readonly→viewer). Test with `just db-migrate down && just db-migrate up` |
| OrgRole type not compatible with pgx | Implement `sql.Scanner` and `driver.Valuer` interfaces |
| RBAC middleware doesn't find org ID | Support both `:orgId` and `:id` URL params |
| Existing code breaks with new User fields | Add fields with defaults, update all SELECT/Scan calls |

---

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
```bash
just backend lint      # Gate 1: Syntax & Style
just backend build     # Gate 2: Compilation
just backend test      # Gate 3: Unit Tests
```

**Final validation**:
```bash
just backend validate  # All checks
just db-migrate down && just db-migrate up  # Migration reversibility
```

---

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase (middleware, models, storage)
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ No new external dependencies
⚠️ RBAC middleware is new pattern (but follows auth middleware closely)

**Assessment**: Well-scoped foundation work with clear patterns to follow. Main risk is migration correctness.

**Estimated one-pass success probability**: 85%

**Reasoning**: All patterns exist in codebase, migration is straightforward ALTER/CREATE, Go types are simple. Middleware is the only novel component but follows existing auth middleware pattern closely.
