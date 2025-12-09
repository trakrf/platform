# Feature: Organization Invitations - Accept Flow

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: backend
**Type**: feature
**Priority**: Urgent
**Phase**: 2d of RBAC API (Accept Invitation)

## Outcome
API endpoint for users to accept organization invitations, completing the invitation flow.

## User Story
As an invited user
I want to accept an invitation via a token link
So that I can join the organization with my assigned role

## Context
**Current**: Phase 2c (invitation admin management) complete.
**Desired**: Users can accept invitations and join organizations.
**Depends On**: org-invitations-admin (Phase 2c)

---

## Technical Requirements

### API Endpoint

| Method | Endpoint | Description | Access |
|--------|----------|-------------|--------|
| POST | `/api/v1/auth/accept-invite` | Accept invitation | Authenticated |

---

### Accept Invitation
```json
// POST /api/v1/auth/accept-invite
// Request:
{ "token": "abc123...64chars..." }

// Response (success):
{
  "message": "You have joined NADA AV Team",
  "org_id": 123,
  "role": "operator"
}

// Response (not logged in):
{ "error": "Please log in to accept this invitation", "redirect": "/login" }

// Response (expired):
{ "error": "This invitation has expired" }

// Response (cancelled):
{ "error": "This invitation has been cancelled" }

// Response (already accepted):
{ "error": "This invitation has already been accepted" }

// Response (invalid token):
{ "error": "Invalid invitation token" }

// Response (already member):
{ "error": "You are already a member of this organization" }
```

---

## Business Rules

1. **Authentication Required**
   - User must be logged in to accept
   - If not logged in: Return 401 with redirect hint
   - Frontend handles signup flow, then retries accept

2. **Token Validation**
   - Hash incoming token with SHA-256
   - Look up by token_hash
   - Check: not expired, not cancelled, not already accepted

3. **Acceptance Flow**
   - Verify user is not already a member of the org
   - Add user to org_users with invited role
   - Set `accepted_at` timestamp on invitation
   - Return org details for frontend redirect

4. **Idempotency**
   - If same user tries to accept same invitation twice: "already accepted"
   - If user is already member (via different path): "already a member"

---

## File Structure

```
backend/
├── internal/
│   ├── handlers/auth/
│   │   └── auth.go           # MODIFY: Add AcceptInvite handler
│   ├── services/orgs/
│   │   └── invitations.go    # MODIFY: Add AcceptInvitation method
│   └── storage/
│       └── invitations.go    # MODIFY: Add GetInvitationByToken, AcceptInvitation
```

---

## Implementation Tasks

### Task 1: Storage Layer Additions
- [ ] Add `GetInvitationByTokenHash(ctx, tokenHash)` to storage
- [ ] Add `AcceptInvitation(ctx, inviteID, userID)` - sets accepted_at, adds to org_users
- [ ] Transaction: accept invite + add user to org atomically

### Task 2: Service Layer Addition
- [ ] Add `AcceptInvitation(ctx, token, userID)` to invitations service
- [ ] Token hashing and lookup
- [ ] Validation (expired, cancelled, already accepted, already member)
- [ ] Return org details on success

### Task 3: Auth Handler
- [ ] Add `AcceptInvite` handler to `handlers/auth/auth.go`
- [ ] Require authentication (check claims)
- [ ] Parse token from request body
- [ ] Call service, return appropriate response

### Task 4: Route Registration
- [ ] Register POST /api/v1/auth/accept-invite route
- [ ] Requires JWT middleware (authenticated)
- [ ] Add route test to main_test.go

---

## Validation Criteria

- [ ] Logged-in user can accept valid invitation
- [ ] User added to org with correct role
- [ ] Invitation marked as accepted (accepted_at set)
- [ ] Non-logged-in user gets 401 with redirect hint
- [ ] Expired invitation returns 400 "expired"
- [ ] Cancelled invitation returns 400 "cancelled"
- [ ] Already-accepted invitation returns 400 "already accepted"
- [ ] Invalid token returns 400 "invalid"
- [ ] Already-member returns 400 "already a member"
- [ ] `just backend validate` passes

## Success Metrics
- [ ] Accept endpoint works end-to-end
- [ ] All edge cases return correct error messages
- [ ] Invitation flow complete: create → email → click → accept → member
