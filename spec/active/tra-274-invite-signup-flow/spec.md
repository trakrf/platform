# Feature: Improve Signup Flow for Invited Users (TRA-274)

## Metadata
**Workspace**: frontend + backend
**Type**: fix
**Linear Issue**: [TRA-274](https://linear.app/trakrf/issue/TRA-274)
**Priority**: Urgent

## Outcome
When a new user signs up via an org invitation link, they see the inviting organization name and understand they're joining an existing org, not creating a new one.

## User Story
As an invited user
I want to see which organization I'm joining when I sign up
So that I'm not confused by a "create organization" form when I expected to join an existing org

## Context

**Current**: When an unauthenticated user clicks an org invitation link:
1. AcceptInviteScreen shows "You've Been Invited!" with no org name visible
2. Clicking "Create Account" goes to SignupScreen
3. SignupScreen shows a required "Organization Name" field
4. User must enter a name to create a "personal org" before they can accept the invitation
5. This is confusing - user expects to join the invited org, not create their own

**Desired**: The signup-via-invite flow should:
1. Show the inviting org name on AcceptInviteScreen (before login/signup)
2. During signup, make it clear the user is joining [OrgName], not creating a new org
3. Either make the personal org name optional/auto-generated, or clearly label it as "Your Personal Workspace"

**Key Files**:
- `frontend/src/components/AcceptInviteScreen.tsx` - shows invite UI (lines 108-144 for unauthenticated state)
- `frontend/src/components/SignupScreen.tsx` - signup form with org name field
- `backend/internal/handlers/auth/auth.go` - auth endpoints
- `backend/internal/services/orgs/invitations.go` - invitation service

## Technical Requirements

### Backend
1. **New endpoint**: `GET /api/v1/auth/invitation-info?token={token}`
   - Returns invitation details without requiring authentication
   - Response: `{ org_name, org_identifier, role, inviter_name (optional) }`
   - Security: Token already proves authorization (was sent via email to recipient)
   - Errors: 404 for invalid/expired/cancelled tokens

2. **New signup mode**: `POST /api/v1/auth/signup` with optional `invitation_token`
   - When `invitation_token` is provided:
     - Create user account only (no personal org)
     - Validate token, add user to invited org with invitation role
     - Mark invitation as accepted
     - Return JWT with invited org_id in claims
   - When `invitation_token` is NOT provided: keep current behavior (create user + personal org)

### Frontend
3. **AcceptInviteScreen enhancements**:
   - On mount (when not authenticated), call invitation-info endpoint
   - Display: "You've been invited to join **{OrgName}** as a **{Role}**"
   - Show inviter name if available: "Invited by {InviterName}"
   - Pass org info to signup via URL params

4. **SignupScreen conditional behavior**:
   - Detect invite context via URL params (`returnTo=accept-invite&token=...`)
   - When in invite context:
     - Fetch invitation-info to get org name
     - Display org name as read-only label (not input field)
     - Hide the "Organization Name" input entirely
     - On submit: call signup with `invitation_token` parameter
     - On success: redirect directly to dashboard (skip AcceptInviteScreen)
   - When NOT in invite context: keep current behavior (org name required)

### UX Flow (Target State)
```
1. User clicks invite link → AcceptInviteScreen
   - Shows: "You've been invited to join Acme Corp as Operator"
   - Buttons: [Sign In] [Create Account]

2. User clicks "Create Account" → SignupScreen
   - Label: "Joining: Acme Corp" (read-only, not an input)
   - Fields: Email, Password only
   - Button: [Create Account]

3. After signup → Redirect to dashboard
   - User is already a member of Acme Corp (no separate accept step needed)
   - Toast: "Welcome to Acme Corp!"
```

## Validation Criteria
- [ ] AcceptInviteScreen (unauthenticated) shows inviting org name and role
- [ ] SignupScreen hides org name field when accessed via invite link
- [ ] SignupScreen displays inviting org name as read-only label
- [ ] Signup via invite creates user WITHOUT personal org
- [ ] Signup via invite adds user directly to invited org
- [ ] After signup via invite, user lands on dashboard as member of invited org
- [ ] Regular signup (not via invite) works unchanged (creates user + personal org)
- [ ] Invalid/expired tokens show appropriate error on AcceptInviteScreen

## Success Metrics
- [ ] New user invited via email can complete signup and join org in 2 steps (view invite → signup)
- [ ] Zero confusion about "which org am I creating/joining"
- [ ] All existing auth tests continue to pass
- [ ] New tests cover invite-signup flow (backend + frontend)

## Edge Cases
- Token expired between viewing AcceptInviteScreen and completing signup → Show error on signup submit
- User enters different email than invitation target → Show email mismatch error
- User refreshes SignupScreen → Preserve invite context via URL params (token in URL)
- Existing user clicks signup link → Should use login flow instead (frontend can suggest)

## References
- AcceptInviteScreen unauthenticated state: `frontend/src/components/AcceptInviteScreen.tsx:108-144`
- SignupScreen org name field: `frontend/src/components/SignupScreen.tsx:122-146`
- Invitation creation: `backend/internal/services/orgs/invitations.go:19-83`
- Accept invitation logic: `backend/internal/services/auth/auth.go:278-349`
