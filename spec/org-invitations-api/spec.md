# Feature: Organization Invitations API

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: backend
**Type**: feature
**Priority**: Urgent
**Phase**: 2c/2d of RBAC API (Invitations)

## Outcome
API endpoints for creating, managing, and accepting organization invitations with email delivery via Resend.

## User Story
As an organization admin
I want to invite users to my organization via email
So that I can grow my team and control their access level

## Context
**Current**: Phase 2a (org CRUD) and 2b (members) complete.
**Desired**: Full invitation flow with email delivery.
**Depends On**: org-rbac-api Phase 2b, TRA-100 (Resend email setup)

---

## Technical Requirements

### API Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/invitations` | List pending invitations | RequireOrgAdmin |
| POST | `/api/v1/orgs/:id/invitations` | Create invitation | RequireOrgAdmin |
| DELETE | `/api/v1/orgs/:id/invitations/:inviteId` | Cancel invitation | RequireOrgAdmin |
| POST | `/api/v1/orgs/:id/invitations/:inviteId/resend` | Resend invitation | RequireOrgAdmin |
| POST | `/api/v1/auth/accept-invite` | Accept invitation | Public (token) |

---

### Create Invitation
```json
// POST /api/v1/orgs/:id/invitations
// Request:
{ "email": "newuser@example.com", "role": "operator" }

// Response (success):
{
  "data": {
    "id": 456,
    "email": "newuser@example.com",
    "role": "operator",
    "expires_at": "2024-01-08T00:00:00Z"
  }
}

// Response (already member):
{ "error": "newuser@example.com is already a member of this organization" }

// Response (pending invite):
{ "error": "An invitation is already pending for newuser@example.com" }
```

### List Invitations
```json
// GET /api/v1/orgs/:id/invitations
{
  "data": [
    {
      "id": 456,
      "email": "newuser@example.com",
      "role": "operator",
      "invited_by": { "id": 1, "name": "Mike Stankavich" },
      "expires_at": "2024-01-08T00:00:00Z",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Cancel Invitation
```json
// DELETE /api/v1/orgs/:id/invitations/:inviteId
// Response:
{ "message": "Invitation cancelled" }
```

### Resend Invitation
```json
// POST /api/v1/orgs/:id/invitations/:inviteId/resend
// Response:
{ "message": "Invitation resent", "expires_at": "2024-01-15T00:00:00Z" }
```
- Generates new token
- Resets expiry to 7 days from now
- Sends new email

### Accept Invitation
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

// Response (invalid token):
{ "error": "Invalid invitation token" }
```

---

## Business Rules

1. **Duplicate Detection**
   - Cannot invite email that's already a member
   - Cannot invite email with pending (non-expired, non-cancelled) invitation
   - Check both conditions before creating

2. **Token Generation**
   - 64-character secure random token
   - Stored hashed or plaintext (your choice)
   - 7-day expiry from creation/resend

3. **Acceptance Flow**
   - User must be logged in to accept
   - If logged in: Add to org with invited role, mark invitation accepted
   - If not logged in: Return error with redirect hint
   - If no account: Frontend handles signup flow, then retry accept

4. **Email Delivery**
   - Use existing Resend client from TRA-100
   - Template includes: org name, inviter name, role, accept link
   - Link format: `https://app.trakrf.id/#accept-invite?token={token}`

5. **Cancellation**
   - Sets `cancelled_at` timestamp
   - Token becomes invalid
   - Does not delete row (audit trail)

---

## Email Template

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
│   │   ├── orgs/
│   │   │   └── invitations.go    # NEW: Invitation handlers
│   │   └── auth/
│   │       └── auth.go           # MODIFY: Add accept-invite endpoint
│   ├── services/
│   │   └── orgs/
│   │       └── invitations.go    # NEW: Invitation business logic
│   └── storage/
│       └── invitations.go        # NEW: Invitation queries
```

---

## Implementation Tasks

### Task 1: Storage Layer
- [ ] Create `storage/invitations.go`
- [ ] Add `CreateInvitation(ctx, orgID, email, role, invitedBy, token, expiresAt)`
- [ ] Add `ListPendingInvitations(ctx, orgID)`
- [ ] Add `GetInvitationByID(ctx, inviteID)`
- [ ] Add `GetInvitationByToken(ctx, token)`
- [ ] Add `CancelInvitation(ctx, inviteID)`
- [ ] Add `AcceptInvitation(ctx, inviteID)`
- [ ] Add `UpdateInvitationToken(ctx, inviteID, newToken, newExpiry)`
- [ ] Add `IsEmailMember(ctx, orgID, email)` check
- [ ] Add `HasPendingInvitation(ctx, orgID, email)` check

### Task 2: Service Layer
- [ ] Create `services/orgs/invitations.go`
- [ ] Implement `CreateInvitation` with duplicate checks + email send
- [ ] Implement `ResendInvitation` with new token + email send
- [ ] Implement `CancelInvitation`
- [ ] Implement `AcceptInvitation` with validation + org membership creation

### Task 3: Email Integration
- [ ] Create `sendInvitationEmail(to, orgName, inviterName, role, token)` function
- [ ] Reuse Resend client from email service
- [ ] Format HTML email template

### Task 4: Handlers
- [ ] Create `handlers/orgs/invitations.go`
- [ ] Implement GET /orgs/:id/invitations
- [ ] Implement POST /orgs/:id/invitations
- [ ] Implement DELETE /orgs/:id/invitations/:inviteId
- [ ] Implement POST /orgs/:id/invitations/:inviteId/resend

### Task 5: Accept Invite Handler
- [ ] Add POST /auth/accept-invite to auth handlers
- [ ] Validate token and check expiry
- [ ] Require authenticated user
- [ ] Add user to org with invited role

### Task 6: Route Registration
- [ ] Register invitation routes with RequireOrgAdmin middleware
- [ ] Register accept-invite route (public but checks auth)

---

## Validation Criteria

### Invitation Creation
- [ ] Admin can send invitation
- [ ] Invitation email sent via Resend
- [ ] Inviting existing member returns "already a member" error
- [ ] Inviting pending email returns "invitation already pending" error
- [ ] Non-admin cannot create invitations (403)

### Invitation Management
- [ ] Admin can list pending invitations
- [ ] Admin can cancel invitation
- [ ] Admin can resend invitation (new token, reset expiry)
- [ ] Cancelled invitation token becomes invalid

### Accept Flow
- [ ] Logged-in user can accept valid invitation
- [ ] User added to org with correct role
- [ ] Non-logged-in user gets redirect prompt
- [ ] Expired invitation returns error
- [ ] Invalid token returns error
- [ ] Already-accepted invitation returns error

### General
- [ ] `just backend validate` passes
- [ ] All error messages are user-friendly

## Success Metrics
- [ ] All 5 endpoints return correct responses
- [ ] Email delivery working via Resend
- [ ] All edge cases (expired, cancelled, duplicate) handled
- [ ] Invitation flow works end-to-end
