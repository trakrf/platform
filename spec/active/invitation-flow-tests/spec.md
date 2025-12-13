# Feature: Invitation Flow Tests (TRA-174)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-174
**Parent**: TRA-172 (Org CRUD Tests)
**Consolidated**: TRA-203 (redirect flow tests folded in)

## Outcome
E2E test coverage for the full invitation lifecycle - sending, accepting, canceling, and edge cases around expiration and duplicates.

## User Story
As a QA engineer
I want comprehensive E2E tests for invitation flows
So that the org invitation system is verified to work correctly for all user scenarios

## Context
**Discovery**: During TRA-173 implementation, several member management tests were blocked by `addTestMemberToOrg` fixture failures (409 conflict - TRA-211). The invitation flow tests need to be decoupled from this issue.

**Current State**:
- Invitation system exists with UI at `AcceptInviteScreen.tsx`
- Test fixtures exist in `org.fixture.ts`:
  - `createInviteViaAPI()` - creates invitation via API
  - `getInviteToken()` - retrieves token from test endpoint
  - `acceptInviteViaAPI()` - accepts invitation
  - `addTestMemberToOrg()` - full flow (has 409 conflict issue)
- Test endpoint exists: `GET /test/invitations/{id}/token`
- Redirect logic preserves token through auth flow (`authRedirect.ts`)

**Desired**: Full test coverage for invitation send/accept/cancel flows, both existing and new user paths.

## Technical Requirements

### Email Testing Strategy
**Approach**: Skip email verification, test API/UI flow directly
- Backend test endpoint `/test/invitations/{id}/token` provides token
- No mocking of Resend API needed
- No external dependencies in CI

### Test Scenarios

#### Send Invitation (Admin Only)
- [ ] Admin can send invitation with email and role
- [ ] Invitation appears in pending list
- [ ] Non-admin cannot see invite button
- [ ] Cannot invite same email twice (error shown)
- [ ] Email validation (invalid format rejected)

#### Accept Invitation - Existing User
- [ ] Logged-in user can accept invitation via token URL
- [ ] User added to org with correct role
- [ ] User sees success message with org name
- [ ] User can navigate to dashboard after accept

#### Accept Invitation - New User
- [ ] Non-logged-in user sees login/signup options
- [ ] After signup, user returns to accept-invite screen
- [ ] New user can accept pending invitation
- [ ] New user added to org with correct role

#### Redirect Flow (from TRA-203)
- [ ] Login redirect preserves token in URL params
- [ ] Signup redirect preserves token in URL params
- [ ] After auth, user returns to accept-invite screen with token intact
- [ ] Token extraction works after redirect

#### Cancel/Resend (Admin Only)
- [ ] Admin can cancel pending invitation
- [ ] Canceled invitation removed from list
- [ ] Canceled token no longer works (shows error)
- [ ] Admin can resend invitation (new token, reset expiry)

#### Edge Cases
- [ ] Expired invitation shows error (7 days)
- [ ] Invalid/malformed token shows "invalid link" error
- [ ] Already accepted token shows "already used" error
- [ ] Cancelled invitation shows "cancelled" error
- [ ] User already a member shows "already a member" message

### Existing Fixtures Available
```typescript
// frontend/tests/e2e/fixtures/org.fixture.ts
createInviteViaAPI(page, orgId, email, role): Promise<number>  // Returns invite ID
getInviteToken(page, inviteId): Promise<string>                // From test endpoint
acceptInviteViaAPI(page, token, authToken?): Promise<void>
addTestMemberToOrg(page, orgId, role): Promise<TestUserCredentials>  // Has 409 issue
```

### New Fixtures Needed
```typescript
// To be added to org.fixture.ts:
cancelInviteViaAPI(page, orgId, inviteId): Promise<void>
resendInviteViaAPI(page, orgId, inviteId): Promise<void>
getInvitationsListViaAPI(page, orgId): Promise<Invitation[]>
```

### Files to Create/Modify
- `frontend/tests/e2e/org-invitations.spec.ts` (new - main test file)
- `frontend/tests/e2e/fixtures/org.fixture.ts` (extend with cancel/resend/list)

### UI Elements with Test IDs
- `data-testid="invite-member-button"` - Opens invite dialog
- `data-testid="accept-invite-button"` - Accepts invitation
- `data-testid="decline-invite-button"` - Declines invitation

## Validation Criteria
- [ ] All 5 send invitation scenarios pass
- [ ] All 4 existing user accept scenarios pass
- [ ] All 4 new user accept scenarios pass
- [ ] All 4 redirect flow scenarios pass
- [ ] All 5 cancel/resend scenarios pass
- [ ] All 5 edge case scenarios pass
- [ ] Tests run < 120 seconds total
- [ ] No external email service calls

## Acceptance Criteria
- Full invitation lifecycle covered
- Both existing and new user flows work
- Redirect flow preserves tokens correctly
- Edge cases handled gracefully
- Tests run < 120 seconds total

## Implementation Notes
- Pattern: "shared session + expendable targets" from `org-crud.spec.ts`
- Use `beforeAll` to create admin user and test org once
- Use `beforeEach` to clear auth state and re-login
- Avoid `addTestMemberToOrg` until TRA-211 resolved - use individual steps

## References
- [TRA-172 Implementation](frontend/tests/e2e/org-crud.spec.ts) - establishes patterns
- [TRA-173 Member Tests](frontend/tests/e2e/org-members.spec.ts) - related tests
- [Org Fixture Helpers](frontend/tests/e2e/fixtures/org.fixture.ts)
- [AcceptInviteScreen](frontend/src/components/AcceptInviteScreen.tsx) - UI component
- [Auth Redirect](frontend/src/utils/authRedirect.ts) - token preservation
- [Linear Issue](https://linear.app/trakrf/issue/TRA-174)
