# Feature: Member Management Tests (TRA-173)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-173
**Parent**: TRA-172 (Org CRUD Tests)

## Outcome
E2E test coverage for org member management - viewing members, role changes, and removal with last-admin protection and non-admin RBAC visibility.

## User Story
As a QA engineer
I want comprehensive E2E tests for member management
So that role-based access control and member operations are verified to work correctly

## Context
**Current**: TRA-172 established core org CRUD patterns. Non-admin RBAC visibility tests were deferred because they require multi-user setup.
**Desired**: Full test coverage for member list viewing, role management, member removal, and RBAC enforcement.
**Examples**:
- Existing patterns: `frontend/tests/e2e/org-crud.spec.ts`
- Fixtures: `frontend/tests/e2e/fixtures/org.fixture.ts`

## Technical Requirements

### Multi-User Test Setup
- Tests require multiple users in the same org with different roles
- Need API-based user creation and org membership setup (UI creation too slow for fixtures)
- Roles to test: admin, manager, operator, viewer

### Test Scenarios

#### View Members
- Any member can view member list
- Member list shows name, email, role
- Current user's role is visible

#### Role Management (Admin Only)
- Admin can change member roles (viewer → operator → manager → admin)
- Non-admin cannot see role change controls
- Role change persists after page reload
- Cannot demote last admin (error message shown)

#### Remove Members (Admin Only)
- Admin can remove members
- Remove requires confirmation
- Non-admin cannot see remove button
- Cannot remove last admin (error message shown)
- Removed member no longer appears in list
- Removed member loses access (verify with login as removed user)

#### Non-Admin Org Settings Visibility (Deferred from TRA-172)
- Non-admin cannot see org edit option in settings
- Non-admin cannot see org delete option in settings

### Fixtures Required
```typescript
// Need to extend org.fixture.ts with:
- createUserViaAPI(email, password): Promise<User>
- addMemberToOrgViaAPI(orgId, userId, role): Promise<void>
- setupMultiUserOrg(): Promise<{ admin, viewer, operator, org }>
```

### Files to Create/Modify
- `frontend/tests/e2e/org-members.spec.ts` (new)
- `frontend/tests/e2e/fixtures/org.fixture.ts` (extend)

## Validation Criteria
- [ ] All member viewing scenarios pass
- [ ] Role change flows work for all role transitions
- [ ] Last admin protection verified (cannot demote or remove)
- [ ] Non-admin users correctly see restricted UI
- [ ] Tests run < 90 seconds total
- [ ] No flaky tests (run 3x without failure)

## Success Metrics
- [ ] 12+ test cases covering all scenarios
- [ ] Multi-user fixture setup < 5 seconds
- [ ] Zero false positives on RBAC checks
- [ ] Follows existing org.fixture.ts patterns

## References
- [TRA-172 Implementation](frontend/tests/e2e/org-crud.spec.ts) - establishes patterns
- [Org Fixture Helpers](frontend/tests/e2e/fixtures/org.fixture.ts)
- [Linear Issue](https://linear.app/trakrf/issue/TRA-173)
