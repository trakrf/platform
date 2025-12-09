# Feature: Organization Management with RBAC and User Invitations

> **SPLIT INTO 3 PHASES** - This spec has been split for manageable PRs:
> 1. **[org-rbac-db](../org-rbac-db/spec.md)** - Database schema + Go types + RBAC middleware
> 2. **[org-rbac-api](../org-rbac-api/spec.md)** - REST API endpoints
> 3. **[org-rbac-ui](../org-rbac-ui/spec.md)** - Frontend components
>
> Start with: `/plan org-rbac-db`

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: database, backend, frontend (multi-workspace)
**Type**: feature
**Priority**: Urgent
**Branch**: `miks2u/tra-136-organization-management-with-rbac-and-user-invitations`

## Outcome
Users can create team organizations, invite members with role-based permissions, and switch between organizations they belong to - following GitHub's multi-tenant model.

## User Story
As a TrakRF user
I want to create organizations, invite team members with specific roles, and switch between orgs
So that I can collaborate with my team while controlling access to assets and data

## Context
**Current**: Personal orgs auto-created on signup (TRA-99 done). No team orgs, no invitations, no RBAC.
**Desired**: Full multi-tenant organization management with role-based access control.
**Model**: GitHub's organization model - personal org + team orgs with invitations.

## Builds On
- **TRA-100** (Password reset) - Reuse Resend email service for invitations

## Related (Post-NADA)
- TRA-135 (Stripe) - orgs own subscriptions
- TRA-142 (White label) - org-level branding

---

## Technical Requirements

### 1. Database Schema

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
  UNIQUE(org_id, email)  -- Can't invite same email twice to same org
);

CREATE INDEX idx_org_invitations_token ON trakrf.org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON trakrf.org_invitations(org_id);
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

### 2. Role Permissions Matrix

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

### 3. Go Types

```go
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
```

---

### 4. API Endpoints

#### Organization CRUD

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs` | List user's orgs | Any authenticated |
| POST | `/api/v1/orgs` | Create team org (creator=admin) | Any authenticated |
| GET | `/api/v1/orgs/:id` | Get org details | Member |
| PUT | `/api/v1/orgs/:id` | Update org | Admin |
| DELETE | `/api/v1/orgs/:id` | Soft delete org | Admin |

#### Organization Members

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/members` | List members with roles | Member |
| PUT | `/api/v1/orgs/:id/members/:userId` | Update member role | Admin |
| DELETE | `/api/v1/orgs/:id/members/:userId` | Remove member | Admin |

#### Invitations

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/invitations` | List pending invitations | Admin |
| POST | `/api/v1/orgs/:id/invitations` | Create invitation | Admin |
| DELETE | `/api/v1/orgs/:id/invitations/:inviteId` | Cancel invitation | Admin |
| POST | `/api/v1/orgs/:id/invitations/:inviteId/resend` | Resend invitation | Admin |
| POST | `/api/v1/auth/accept-invite` | Accept invitation (public) | Token holder |

#### User Context

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/api/v1/users/me/current-org` | Set current org | Any authenticated |
| GET | `/api/v1/users/me` | Get profile + orgs + roles | Any authenticated |

---

### 5. Business Logic

#### Last Admin Protection
```go
func (s *OrgService) RemoveMember(orgID, userID, actorID int) error {
    // Check actor is admin
    // If target is admin, check there's at least one other admin
    adminCount := s.store.CountAdmins(orgID)
    if targetRole == RoleAdmin && adminCount <= 1 {
        return ErrCannotRemoveLastAdmin
    }
    // Proceed with removal
}
```

#### Superadmin Access
```go
func (m *AuthMiddleware) CheckOrgAccess(orgID int) bool {
    if m.currentUser.IsSuperadmin {
        m.logSuperadminAccess(m.currentUser.ID, orgID)
        return true
    }
    return m.store.IsOrgMember(m.currentUser.ID, orgID)
}
```

#### Invitation Flow
1. Admin creates invitation → generates token, stores with 7-day expiry
2. System sends email via Resend
3. User clicks link → `/accept-invite?token=xxx`
4. If logged in: Add to org with invited role
5. If not logged in but account exists: Prompt to login first
6. If no account: Prompt to signup first, then accept

#### User Removal Data Handling
- User removed from `org_users`
- Assets/locations they created retain `created_by` (historical)
- User loses all access to org data
- If user has no other orgs, they still have their personal org

#### Org Deletion (GitHub-style anti-footgun)
- Requires typing org name exactly to confirm (not just a checkbox)
- DELETE request body: `{ "confirm_name": "NADA AV Team" }`
- Backend validates `confirm_name` matches org name before proceeding
- Soft delete: Set `deleted_at` timestamp
- All queries filter `WHERE deleted_at IS NULL`
- Superadmin can hard delete with same confirmation
- Future: Cleanup job removes soft-deleted orgs after 90 days

#### Duplicate Invitation Handling
- If inviting email that's already a member → return error, show toast: "user@example.com is already a member of this organization"
- If inviting email with pending invitation → return error, show toast: "An invitation is already pending for user@example.com"

---

### 6. Frontend Components

#### Components
- **OrgSwitcher.tsx** - Header dropdown showing current org, list of user orgs, role badges
- **OrgSettingsPage.tsx** - Org name (editable by admin), delete button
- **MembersPage.tsx** - Table with name, email, role, actions; role dropdown for admins
- **InvitationsPage.tsx** - Pending invitations table with cancel/resend
- **InviteModal.tsx** - Email input, role dropdown
- **AcceptInvitePage.tsx** - Invitation acceptance flow
- **RoleBadge.tsx** - Visual role indicator

#### Routes
```
#org-settings
#org-members
#accept-invite?token=xxx
```

#### State (Zustand)
```typescript
interface OrgState {
  currentOrg: Organization | null;
  userOrgs: { org: Organization; role: OrgRole }[];
  setCurrentOrg: (orgId: number) => Promise<void>;
}
```

---

### 7. Email Template

**Subject**: You've been invited to join {org_name} on TrakRF

```html
<h2>You've been invited to {org_name}</h2>
<p>{inviter_name} has invited you to join {org_name} as a {role} on TrakRF.</p>
<p><a href="https://app.trakrf.id/#accept-invite?token={token}">Accept Invitation</a></p>
<p>This invitation expires in 7 days.</p>
<p>If you don't have a TrakRF account yet, you'll be prompted to create one.</p>
```

---

## File Structure

```
backend/
├── internal/
│   ├── handlers/
│   │   └── orgs/
│   │       ├── orgs.go          # Org CRUD
│   │       ├── members.go       # Member management
│   │       └── invitations.go   # Invitation management
│   ├── services/
│   │   └── orgs/
│   │       └── orgs.go          # Business logic
│   ├── storage/
│   │   ├── orgs.go              # Org queries
│   │   ├── org_members.go       # Member queries
│   │   └── org_invitations.go   # Invitation queries
│   └── middleware/
│       └── rbac.go              # Role-based access control

frontend/
├── src/
│   ├── components/
│   │   ├── OrgSwitcher.tsx
│   │   ├── InviteModal.tsx
│   │   └── RoleBadge.tsx
│   ├── screens/
│   │   ├── OrgSettingsScreen.tsx
│   │   ├── MembersScreen.tsx
│   │   └── AcceptInviteScreen.tsx
│   ├── stores/
│   │   └── orgStore.ts
│   └── lib/api/
│       └── orgs.ts
```

---

## Implementation Phases

### Phase 1: Database Migration
- [ ] Create migration file with role enum, table alterations, invitations table
- [ ] Backfill existing org creators as admin
- [ ] Test up/down migrations

### Phase 2: Backend - Core RBAC
- [ ] OrgRole type and permission methods
- [ ] RBAC middleware
- [ ] Update existing endpoints to check permissions

### Phase 3: Backend - Org & Member Management
- [ ] Org CRUD handlers and storage
- [ ] Member management handlers and storage
- [ ] Last admin protection logic

### Phase 4: Backend - Invitations
- [ ] Invitation handlers and storage
- [ ] Token generation and validation
- [ ] Email integration (reuse Resend from TRA-100)
- [ ] Accept invitation endpoint

### Phase 5: Frontend - Org Switcher
- [ ] OrgSwitcher component
- [ ] Org store (Zustand)
- [ ] API client methods

### Phase 6: Frontend - Settings & Members
- [ ] OrgSettingsScreen
- [ ] MembersScreen with role management
- [ ] InvitationsPage/tab

### Phase 7: Frontend - Invitation Flow
- [ ] InviteModal
- [ ] AcceptInviteScreen
- [ ] Email template finalization

---

## Validation Criteria

### Org Management
- [ ] User can create team org (becomes admin)
- [ ] User can view orgs they belong to
- [ ] Admin can edit org name
- [ ] Admin can delete org (soft delete) by typing org name exactly
- [ ] Delete fails if confirm_name doesn't match
- [ ] Non-admin cannot edit/delete org

### Members
- [ ] Any member can view member list
- [ ] Admin can change member roles
- [ ] Admin can remove members
- [ ] Cannot remove last admin
- [ ] Cannot demote last admin

### Invitations
- [ ] Admin can send invitation
- [ ] Invitation email received
- [ ] Invited user can accept (existing account)
- [ ] Invited user can accept (new account after signup)
- [ ] Invitation expires after 7 days
- [ ] Admin can cancel invitation
- [ ] Admin can resend invitation
- [ ] Inviting existing member shows toast: "already a member"
- [ ] Inviting email with pending invite shows toast: "invitation already pending"

### Org Switching
- [ ] Header shows current org
- [ ] Dropdown shows all user orgs
- [ ] Clicking org switches context
- [ ] Last used org remembered on login

### Superadmin
- [ ] Superadmin can access any org
- [ ] Superadmin access is logged

### RBAC Enforcement
- [ ] Viewer cannot run scans or edit assets
- [ ] Operator can run scans but not edit assets
- [ ] Manager can edit assets but not manage users
- [ ] Admin has full access

---

## Success Metrics
- [ ] All 7 implementation phases complete
- [ ] All validation criteria passing
- [ ] Invitation email delivery working via Resend
- [ ] No console errors during org switching
- [ ] Role permission checks enforced on all relevant endpoints
- [ ] Backend tests cover RBAC edge cases (last admin, expired invites)

## References
- [TRA-136 Linear Issue](https://linear.app/trakrf/issue/TRA-136)
- [TRA-99](https://linear.app/trakrf/issue/TRA-99) - Personal org on signup (done)
- [TRA-100](https://linear.app/trakrf/issue/TRA-100) - Password reset (Resend setup)
- GitHub organization model for UX patterns
