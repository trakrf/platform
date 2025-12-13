# Feature: Invitation Flow Tests (TRA-174)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-174
**Parent**: TRA-172 (Org CRUD Tests)

## Outcome
E2E test coverage for the full invitation lifecycle - sending, accepting, canceling, and edge cases around expiration and duplicates.

## User Story
As a QA engineer
I want comprehensive E2E tests for invitation flows
So that the org invitation system is verified to work correctly for all user scenarios

## Context
**Current**: Invitation system exists but has no E2E coverage. Most complex flow due to email verification and multi-user state.
**Desired**: Full test coverage for invitation send/accept/cancel flows, both existing and new user paths.
**Examples**:
- Existing patterns: `frontend/tests/e2e/org-crud.spec.ts`
- Fixtures: `frontend/tests/e2e/fixtures/org.fixture.ts`

## Technical Requirements

### Email Testing Strategy
**Recommended approach**: Mock Resend API in tests
- Intercept API calls to verify payload
- Extract invitation token from mock response
- Avoid external email service dependencies in CI

Alternative for local debugging: Mailhog/Mailpit SMTP trap

### Test Scenarios

#### Send Invitation (Admin Only)
- Admin can send invitation with email and role
- Invitation appears in pending list
- Non-admin cannot see invite button
- Cannot invite same email twice (error shown)
- Email validation (invalid format rejected)

#### Accept Invitation - Existing User
- User receives invite link
- Logged-in user can accept invitation
- User added to org with correct role
- User redirected to org after accept

#### Accept Invitation - New User
- Non-logged-in user prompted to login/signup
- After signup, can accept pending invitation
- New user added to org with correct role

#### Cancel/Resend (Admin Only)
- Admin can cancel pending invitation
- Canceled invitation removed from list
- Canceled token no longer works
- Admin can resend invitation (new token, reset expiry)

#### Edge Cases
- Expired invitation shows error (7 days)
- Invalid token shows error
- Already-accepted invitation shows appropriate message

### Fixtures Required
```typescript
// Need to extend org.fixture.ts with:
- sendInvitationViaAPI(orgId, email, role): Promise<{ token }>
- getInvitationToken(email): Promise<string>
- acceptInvitation(page, token): Promise<void>
- cancelInvitation(orgId, invitationId): Promise<void>
```

### Files to Create/Modify
- `frontend/tests/e2e/org-invitations.spec.ts` (new)
- `frontend/tests/e2e/fixtures/org.fixture.ts` (extend)

## Validation Criteria
- [ ] All send invitation scenarios pass
- [ ] Existing user accept flow works
- [ ] New user accept flow works (signup → accept)
- [ ] Cancel and resend flows work
- [ ] Edge cases (expired, invalid, duplicate) handled gracefully
- [ ] Tests run < 120 seconds total
- [ ] No external email service calls in CI

## Success Metrics
- [ ] 15+ test cases covering all scenarios
- [ ] Invitation token extraction works reliably
- [ ] Zero network calls to actual email service
- [ ] All role assignments verified post-accept

## References
- [TRA-172 Implementation](frontend/tests/e2e/org-crud.spec.ts) - establishes patterns
- [Org Fixture Helpers](frontend/tests/e2e/fixtures/org.fixture.ts)
- [Linear Issue](https://linear.app/trakrf/issue/TRA-174)
