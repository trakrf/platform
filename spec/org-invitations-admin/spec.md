# Feature: Organization Invitations - Admin Management

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: backend
**Type**: feature
**Priority**: Urgent
**Phase**: 2c of RBAC API (Invitation Management)

## Outcome
API endpoints for admins to create, list, cancel, and resend organization invitations with email delivery via Resend.

## User Story
As an organization admin
I want to invite users to my organization via email
So that I can grow my team and control their access level

## Context
**Current**: Phase 2a (org CRUD) and 2b (members) complete.
**Desired**: Admin-facing invitation management with email delivery.
**Depends On**: org-members-api (complete), TRA-100 (Resend email setup)
**Next Phase**: org-invitations-accept (Phase 2d)

---

## Technical Requirements

### API Endpoints

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| GET | `/api/v1/orgs/:id/invitations` | List pending invitations | RequireOrgAdmin |
| POST | `/api/v1/orgs/:id/invitations` | Create invitation | RequireOrgAdmin |
| DELETE | `/api/v1/orgs/:id/invitations/:inviteId` | Cancel invitation | RequireOrgAdmin |
| POST | `/api/v1/orgs/:id/invitations/:inviteId/resend` | Resend invitation | RequireOrgAdmin |

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

---

## Business Rules

1. **Duplicate Detection**
   - Cannot invite email that's already a member
   - Cannot invite email with pending (non-expired, non-cancelled) invitation
   - Check both conditions before creating

2. **Token Generation**
   - 64-character secure random token (crypto/rand)
   - Store token hashed (SHA-256) for security
   - 7-day expiry from creation/resend

3. **Email Delivery**
   - Use existing Resend client from TRA-100
   - Template includes: org name, inviter name, role, accept link
   - Link format: `https://app.trakrf.id/#accept-invite?token={token}`

4. **Cancellation**
   - Sets `cancelled_at` timestamp
   - Token becomes invalid
   - Does not delete row (audit trail)

5. **Resend**
   - Only valid for non-cancelled, non-accepted invitations
   - Generates completely new token
   - Resets expiry to 7 days from now

---

## Database Schema

The `org_invitations` table already exists from migration 000003. Key columns:
- `id`, `org_id`, `email`, `role`
- `token_hash` (SHA-256 of token)
- `invited_by` (user_id)
- `expires_at`, `accepted_at`, `cancelled_at`
- `created_at`, `updated_at`

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
│   ├── handlers/orgs/
│   │   └── invitations.go    # NEW: Invitation handlers
│   ├── services/orgs/
│   │   └── invitations.go    # NEW: Invitation business logic + email
│   └── storage/
│       └── invitations.go    # NEW: Invitation queries
```

---

## Implementation Tasks

### Task 1: Add Invitation Types
- [ ] Add invitation model types to `models/organization/`
- [ ] Add error messages to `apierrors/messages.go`

### Task 2: Storage Layer
- [ ] Create `storage/invitations.go`
- [ ] `CreateInvitation(ctx, orgID, email, role, tokenHash, invitedBy, expiresAt)`
- [ ] `ListPendingInvitations(ctx, orgID)` - non-expired, non-cancelled, non-accepted
- [ ] `GetInvitationByID(ctx, inviteID)`
- [ ] `CancelInvitation(ctx, inviteID)` - sets cancelled_at
- [ ] `UpdateInvitationToken(ctx, inviteID, newTokenHash, newExpiry)`
- [ ] `IsEmailMember(ctx, orgID, email)` - check org_users
- [ ] `HasPendingInvitation(ctx, orgID, email)` - check for active invite

### Task 3: Service Layer
- [ ] Create `services/orgs/invitations.go`
- [ ] `CreateInvitation` - duplicate checks, token gen, email send
- [ ] `ListInvitations` - fetch with inviter details
- [ ] `CancelInvitation` - validation + cancel
- [ ] `ResendInvitation` - new token, reset expiry, email send

### Task 4: Email Integration
- [ ] Create `sendInvitationEmail(to, orgName, inviterName, role, token)`
- [ ] Reuse Resend client from email service
- [ ] Format HTML email template

### Task 5: Handlers
- [ ] Create `handlers/orgs/invitations.go`
- [ ] GET /orgs/:id/invitations
- [ ] POST /orgs/:id/invitations
- [ ] DELETE /orgs/:id/invitations/:inviteId
- [ ] POST /orgs/:id/invitations/:inviteId/resend

### Task 6: Route Registration
- [ ] Register invitation routes with RequireOrgAdmin middleware
- [ ] Add route tests to main_test.go

---

## Validation Criteria

- [ ] Admin can create invitation (email sent)
- [ ] Admin can list pending invitations
- [ ] Admin can cancel invitation
- [ ] Admin can resend invitation (new token, new email)
- [ ] Cannot invite existing member (400)
- [ ] Cannot invite email with pending invitation (400)
- [ ] Non-admin cannot access any endpoint (403)
- [ ] `just backend validate` passes

## Success Metrics
- [ ] All 4 admin endpoints return correct responses
- [ ] Email delivery working via Resend
- [ ] Duplicate detection working
- [ ] Cancelled invitations tracked (audit trail)
