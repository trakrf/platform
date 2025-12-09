# Feature: Organization RBAC - Phase 1: Database & Types

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: database, backend
**Type**: feature
**Priority**: Urgent
**Phase**: 1 of 3 (Database + RBAC Foundation)

## Outcome
Database schema supports role-based organization membership, and Go types/middleware enforce RBAC on existing endpoints.

## User Story
As a TrakRF developer
I want the database and backend types to support RBAC
So that subsequent phases can build org management and invitations on a solid foundation

## Context
**Current**: Personal orgs auto-created on signup (TRA-99). No roles, no invitation support in schema.
**Desired**: Schema with role enum, updated org_users, invitations table, and Go RBAC types.
**Next Phase**: org-rbac-api (Org/Member/Invitation endpoints)

## Builds On
- **TRA-99** (Personal org on signup) - Existing org/org_users tables

---

## Technical Requirements

### 1. Database Migration

#### Role Enum
```sql
CREATE TYPE org_role AS ENUM ('viewer', 'operator', 'manager', 'admin');
```

#### Update org_users Table
```sql
ALTER TABLE trakrf.org_users ADD COLUMN role org_role NOT NULL DEFAULT 'viewer';
```

#### Superadmin Flag
```sql
ALTER TABLE trakrf.users ADD COLUMN is_superadmin BOOLEAN DEFAULT FALSE;
```

#### Last Used Org Tracking
```sql
ALTER TABLE trakrf.users ADD COLUMN last_org_id INTEGER REFERENCES trakrf.organizations(id);
```

#### Invitations Table
```sql
CREATE TABLE trakrf.org_invitations (
  id SERIAL PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES trakrf.organizations(id) ON DELETE CASCADE,
  email VARCHAR(255) NOT NULL,
  role org_role NOT NULL DEFAULT 'viewer',
  token VARCHAR(64) NOT NULL,
  invited_by INTEGER REFERENCES trakrf.users(id),
  expires_at TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(org_id, email)
);

CREATE INDEX idx_org_invitations_token ON trakrf.org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON trakrf.org_invitations(org_id);
```

#### Soft Delete Support (if not exists)
```sql
ALTER TABLE trakrf.organizations ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
```

#### Migration Backfill
```sql
-- Set existing org creators as admin
UPDATE trakrf.org_users ou
SET role = 'admin'
FROM trakrf.organizations o
WHERE ou.org_id = o.id
  AND ou.user_id = o.created_by;
```

---

### 2. Go Types

#### OrgRole Type
```go
// internal/models/org_role.go
type OrgRole string

const (
    RoleViewer   OrgRole = "viewer"
    RoleOperator OrgRole = "operator"
    RoleManager  OrgRole = "manager"
    RoleAdmin    OrgRole = "admin"
)

// Permission checks
func (r OrgRole) CanScan() bool {
    return r == RoleOperator || r == RoleManager || r == RoleAdmin
}

func (r OrgRole) CanManageAssets() bool {
    return r == RoleManager || r == RoleAdmin
}

func (r OrgRole) CanManageUsers() bool {
    return r == RoleAdmin
}

func (r OrgRole) CanManageOrg() bool {
    return r == RoleAdmin
}

func (r OrgRole) IsValid() bool {
    switch r {
    case RoleViewer, RoleOperator, RoleManager, RoleAdmin:
        return true
    }
    return false
}
```

#### Updated User Model
```go
// Add to existing User struct
type User struct {
    // ... existing fields
    IsSuperadmin bool  `json:"is_superadmin" db:"is_superadmin"`
    LastOrgID    *int  `json:"last_org_id" db:"last_org_id"`
}
```

#### Updated OrgUser Model
```go
// Add role to existing OrgUser struct
type OrgUser struct {
    // ... existing fields
    Role OrgRole `json:"role" db:"role"`
}
```

#### Invitation Model
```go
// internal/models/invitation.go
type OrgInvitation struct {
    ID          int        `json:"id" db:"id"`
    OrgID       int        `json:"org_id" db:"org_id"`
    Email       string     `json:"email" db:"email"`
    Role        OrgRole    `json:"role" db:"role"`
    Token       string     `json:"-" db:"token"`
    InvitedBy   *int       `json:"invited_by" db:"invited_by"`
    ExpiresAt   time.Time  `json:"expires_at" db:"expires_at"`
    AcceptedAt  *time.Time `json:"accepted_at" db:"accepted_at"`
    CancelledAt *time.Time `json:"cancelled_at" db:"cancelled_at"`
    CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}
```

---

### 3. RBAC Middleware

```go
// internal/middleware/rbac.go

// RequireOrgMember checks user is a member of the org (from URL param)
func RequireOrgMember(store storage.OrgStore) func(http.Handler) http.Handler

// RequireOrgRole checks user has at least the specified role
func RequireOrgRole(store storage.OrgStore, minRole OrgRole) func(http.Handler) http.Handler

// RequireOrgAdmin is shorthand for RequireOrgRole(store, RoleAdmin)
func RequireOrgAdmin(store storage.OrgStore) func(http.Handler) http.Handler

// Helper to get user's role in org from context
func GetOrgRole(ctx context.Context) (OrgRole, bool)

// Superadmin bypass (with logging)
func checkSuperadminAccess(user *User, orgID int) bool
```

---

### 4. Role Permissions Matrix (Reference)

| Permission | Viewer | Operator | Manager | Admin |
|------------|--------|----------|---------|-------|
| View assets/locations | ✅ | ✅ | ✅ | ✅ |
| Run scans | ❌ | ✅ | ✅ | ✅ |
| Save scan results | ❌ | ✅ | ✅ | ✅ |
| Create/edit assets | ❌ | ❌ | ✅ | ✅ |
| Create/edit locations | ❌ | ❌ | ✅ | ✅ |
| View reports | ✅ | ✅ | ✅ | ✅ |
| Export reports | ❌ | ❌ | ✅ | ✅ |
| Invite users | ❌ | ❌ | ❌ | ✅ |
| Remove users | ❌ | ❌ | ❌ | ✅ |
| Change user roles | ❌ | ❌ | ❌ | ✅ |
| Edit org settings | ❌ | ❌ | ❌ | ✅ |
| Delete org | ❌ | ❌ | ❌ | ✅ |

---

## File Structure

```
backend/
├── database/migrations/
│   ├── NNNN_org_rbac.up.sql
│   └── NNNN_org_rbac.down.sql
├── internal/
│   ├── models/
│   │   ├── org_role.go      # NEW: OrgRole type + permissions
│   │   ├── invitation.go    # NEW: OrgInvitation model
│   │   ├── user.go          # MODIFY: Add IsSuperadmin, LastOrgID
│   │   └── org.go           # MODIFY: Add DeletedAt, update OrgUser
│   └── middleware/
│       └── rbac.go          # NEW: RBAC middleware
```

---

## Implementation Tasks

### Task 1: Create Migration
- [ ] Create `NNNN_org_rbac.up.sql` with all schema changes
- [ ] Create `NNNN_org_rbac.down.sql` to reverse
- [ ] Test migration up/down locally

### Task 2: Go Models
- [ ] Create `internal/models/org_role.go` with OrgRole type and permission methods
- [ ] Create `internal/models/invitation.go` with OrgInvitation struct
- [ ] Update User model with IsSuperadmin, LastOrgID
- [ ] Update OrgUser model with Role field
- [ ] Update Organization model with DeletedAt

### Task 3: RBAC Middleware
- [ ] Create `internal/middleware/rbac.go`
- [ ] Implement RequireOrgMember, RequireOrgRole, RequireOrgAdmin
- [ ] Add superadmin bypass with logging
- [ ] Write unit tests for middleware

### Task 4: Storage Layer Updates
- [ ] Update org storage to include role in queries
- [ ] Add GetUserOrgRole(userID, orgID) method
- [ ] Add CountOrgAdmins(orgID) method (for last-admin protection)

---

## Validation Criteria

- [ ] Migration applies cleanly (`just db-migrate up`)
- [ ] Migration rolls back cleanly (`just db-migrate down`)
- [ ] Existing org creators are backfilled as admin
- [ ] OrgRole type has all 4 roles with correct permission methods
- [ ] OrgInvitation model matches schema
- [ ] User model includes IsSuperadmin and LastOrgID
- [ ] RBAC middleware compiles and has tests
- [ ] `just backend validate` passes

---

## Success Metrics
- [ ] All migration up/down tests pass
- [ ] OrgRole permission methods have 100% test coverage
- [ ] RBAC middleware has tests for each access level
- [ ] No breaking changes to existing functionality
- [ ] Backend builds and all existing tests pass

## References
- [TRA-136 Linear Issue](https://linear.app/trakrf/issue/TRA-136)
- [org-rbac-api](../org-rbac-api/spec.md) - Phase 2 (depends on this)
- [org-rbac-ui](../org-rbac-ui/spec.md) - Phase 3 (depends on Phase 2)
