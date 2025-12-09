# Feature: Organization RBAC - Phase 2: API Layer

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: backend
**Type**: feature
**Priority**: Urgent
**Phase**: 2 of 3 (Org/Member/Invitation APIs)

## Outcome
Complete REST API for organization management, member management, and invitations with RBAC enforcement.

## User Story
As a TrakRF user
I want API endpoints to manage organizations, members, and invitations
So that the frontend can build the org management UI

## Context
**Current**: Basic org endpoints exist but no member management, no invitations, no role enforcement.
**Desired**: Full CRUD for orgs, members, invitations with RBAC middleware protecting each endpoint.
**Depends On**: org-rbac-db (Phase 1 must be complete)
**Next Phase**: org-rbac-ui (Frontend components)

## Builds On
- **org-rbac-db** (Phase 1) - Database schema and RBAC middleware
- **TRA-100** (Password reset) - Resend email service for invitations

---

## Technical Requirements

### 1. Organization CRUD Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs` | List user's orgs with roles | Any authenticated |
| POST | `/api/v1/orgs` | Create team org (creator=admin) | Any authenticated |
| GET | `/api/v1/orgs/:id` | Get org details | Member |
| PUT | `/api/v1/orgs/:id` | Update org name | Admin |
| DELETE | `/api/v1/orgs/:id` | Soft delete org | Admin |

#### Create Org Request/Response
```json
// POST /api/v1/orgs
// Request:
{ "name": "NADA AV Team" }

// Response:
{
  "id": 123,
  "name": "NADA AV Team",
  "created_at": "2024-01-01T00:00:00Z"
}
```

#### Delete Org (GitHub-style confirmation)
```json
// DELETE /api/v1/orgs/:id
// Request:
{ "confirm_name": "NADA AV Team" }

// Response (success):
{ "message": "Organization deleted" }

// Response (mismatch):
{ "error": "Organization name does not match" }
```

---

### 2. Member Management Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/members` | List members with roles | Member |
| PUT | `/api/v1/orgs/:id/members/:userId` | Update member role | Admin |
| DELETE | `/api/v1/orgs/:id/members/:userId` | Remove member | Admin |

#### List Members Response
```json
// GET /api/v1/orgs/:id/members
{
  "members": [
    {
      "user_id": 1,
      "name": "Mike Stankavich",
      "email": "mike@example.com",
      "role": "admin",
      "joined_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

#### Update Role Request
```json
// PUT /api/v1/orgs/:id/members/:userId
{ "role": "manager" }
```

#### Business Rules
- Cannot remove last admin (return 400)
- Cannot demote last admin (return 400)
- Removing member removes from org_users only (data retained)

---

### 3. Invitation Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/invitations` | List pending invitations | Admin |
| POST | `/api/v1/orgs/:id/invitations` | Create invitation | Admin |
| DELETE | `/api/v1/orgs/:id/invitations/:inviteId` | Cancel invitation | Admin |
| POST | `/api/v1/orgs/:id/invitations/:inviteId/resend` | Resend invitation | Admin |
| POST | `/api/v1/auth/accept-invite` | Accept invitation | Public (token) |

#### Create Invitation
```json
// POST /api/v1/orgs/:id/invitations
// Request:
{ "email": "newuser@example.com", "role": "operator" }

// Response (success):
{
  "id": 456,
  "email": "newuser@example.com",
  "role": "operator",
  "expires_at": "2024-01-08T00:00:00Z"
}

// Response (already member):
{ "error": "newuser@example.com is already a member of this organization" }

// Response (pending invite):
{ "error": "An invitation is already pending for newuser@example.com" }
```

#### Accept Invitation
```json
// POST /api/v1/auth/accept-invite
// Request:
{ "token": "abc123..." }

// Response (success, logged in):
{ "message": "You have joined NADA AV Team", "org_id": 123 }

// Response (not logged in):
{ "error": "Please log in to accept this invitation", "redirect": "/login" }

// Response (expired):
{ "error": "This invitation has expired" }
```

---

### 4. User Context Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/api/v1/users/me/current-org` | Set current org | Authenticated |
| GET | `/api/v1/users/me` | Get profile + orgs + roles | Authenticated |

#### Get User Profile (updated)
```json
// GET /api/v1/users/me
{
  "id": 1,
  "name": "Mike Stankavich",
  "email": "mike@example.com",
  "is_superadmin": false,
  "current_org": {
    "id": 123,
    "name": "NADA AV Team",
    "role": "admin"
  },
  "orgs": [
    { "id": 123, "name": "NADA AV Team", "role": "admin" },
    { "id": 456, "name": "Personal", "role": "admin" }
  ]
}
```

#### Set Current Org
```json
// POST /api/v1/users/me/current-org
{ "org_id": 123 }
```

---

### 5. Invitation Email (Resend)

Reuse email service from TRA-100:

```go
func sendInvitationEmail(to, orgName, inviterName, role, token string) error {
    resetURL := fmt.Sprintf("https://app.trakrf.id/#accept-invite?token=%s", token)

    _, err := resendClient.Emails.Send(&resend.SendEmailRequest{
        From:    "TrakRF <noreply@trakrf.id>",
        To:      []string{to},
        Subject: fmt.Sprintf("You've been invited to join %s on TrakRF", orgName),
        Html:    fmt.Sprintf(`
            <h2>You've been invited to %s</h2>
            <p>%s has invited you to join %s as a %s on TrakRF.</p>
            <p><a href="%s">Accept Invitation</a></p>
            <p>This invitation expires in 7 days.</p>
            <p>If you don't have a TrakRF account yet, you'll be prompted to create one.</p>
        `, orgName, inviterName, orgName, role, resetURL),
    })
    return err
}
```

---

## File Structure

```
backend/
├── internal/
│   ├── handlers/
│   │   └── orgs/
│   │       ├── orgs.go          # NEW: Org CRUD handlers
│   │       ├── members.go       # NEW: Member management handlers
│   │       └── invitations.go   # NEW: Invitation handlers
│   ├── services/
│   │   └── orgs/
│   │       └── orgs.go          # NEW: Business logic
│   ├── storage/
│   │   ├── orgs.go              # MODIFY: Add new queries
│   │   ├── org_members.go       # NEW: Member queries
│   │   └── org_invitations.go   # NEW: Invitation queries
│   └── routes/
│       └── routes.go            # MODIFY: Register new endpoints
```

---

## Implementation Tasks

### Task 1: Storage Layer
- [ ] Create `storage/org_members.go` (list, update role, remove)
- [ ] Create `storage/org_invitations.go` (CRUD, token lookup)
- [ ] Update `storage/orgs.go` for soft delete, list with roles

### Task 2: Service Layer
- [ ] Create `services/orgs/orgs.go` with business logic
- [ ] Implement last-admin protection
- [ ] Implement duplicate invitation detection
- [ ] Integrate Resend email service

### Task 3: Org Handlers
- [ ] Create `handlers/orgs/orgs.go` (list, create, get, update, delete)
- [ ] Wire RBAC middleware per endpoint

### Task 4: Member Handlers
- [ ] Create `handlers/orgs/members.go` (list, update role, remove)
- [ ] Wire RequireOrgAdmin middleware

### Task 5: Invitation Handlers
- [ ] Create `handlers/orgs/invitations.go` (list, create, cancel, resend)
- [ ] Create accept-invite handler in auth handlers

### Task 6: User Context
- [ ] Update GET /users/me to include orgs with roles
- [ ] Add POST /users/me/current-org endpoint

### Task 7: Route Registration
- [ ] Register all new routes with appropriate middleware

---

## Validation Criteria

### Org Management
- [ ] User can create team org (becomes admin)
- [ ] User can view orgs they belong to
- [ ] Admin can edit org name
- [ ] Admin can delete org by typing name exactly
- [ ] Delete fails if confirm_name doesn't match
- [ ] Non-admin cannot edit/delete org (403)

### Members
- [ ] Any member can view member list
- [ ] Admin can change member roles
- [ ] Admin can remove members
- [ ] Cannot remove last admin (400)
- [ ] Cannot demote last admin (400)

### Invitations
- [ ] Admin can send invitation
- [ ] Invitation email sent via Resend
- [ ] Inviting existing member returns error with message
- [ ] Inviting pending email returns error with message
- [ ] Logged-in user can accept invitation
- [ ] Non-logged-in user gets redirect prompt
- [ ] Expired invitation returns error
- [ ] Admin can cancel invitation
- [ ] Admin can resend invitation (new token, reset expiry)

### User Context
- [ ] GET /users/me returns orgs with roles
- [ ] POST /users/me/current-org updates last_org_id
- [ ] current_org reflects last_org_id or first org

---

## Success Metrics
- [ ] All API endpoints return correct responses
- [ ] RBAC middleware correctly enforces access
- [ ] Invitation emails delivered via Resend
- [ ] All edge cases (last admin, expired invite) handled
- [ ] `just backend validate` passes
- [ ] Integration tests cover happy path and error cases

## References
- [TRA-136 Linear Issue](https://linear.app/trakrf/issue/TRA-136)
- [org-rbac-db](../org-rbac-db/spec.md) - Phase 1 (prerequisite)
- [org-rbac-ui](../org-rbac-ui/spec.md) - Phase 3 (depends on this)
- [TRA-100](https://linear.app/trakrf/issue/TRA-100) - Resend email setup
