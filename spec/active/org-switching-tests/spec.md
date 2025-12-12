# Feature: Org Switching + Edge Cases Tests (TRA-175)

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: https://linear.app/trakrf/issue/TRA-175
**Parent**: TRA-172 (Org CRUD Tests)

## Outcome
E2E test coverage for org context switching via header dropdown and persistence across sessions, including multi-org user scenarios.

## User Story
As a QA engineer
I want comprehensive E2E tests for org switching
So that context isolation and session persistence are verified to work correctly

## Context
**Current**: Org switcher UI exists, basic switching tested in TRA-172 via `switchToOrg()` helper. No dedicated tests for persistence, multi-org scenarios, or data isolation.
**Desired**: Full test coverage for org switcher behavior, context persistence, and multi-org user scenarios.
**Examples**:
- Existing patterns: `frontend/tests/e2e/org-crud.spec.ts`
- Fixtures: `frontend/tests/e2e/fixtures/org.fixture.ts` (`switchToOrg()` helper)

## Technical Requirements

### Multi-Org Test Setup
- Tests require user belonging to 3+ orgs with different roles
- Need API-based org creation and membership setup
- Verify data isolation between orgs

### Test Scenarios

#### Org Switcher UI
- Header shows current org name
- Dropdown shows all user's orgs
- Each org shows role badge
- "Create Organization" option visible

#### Context Switching
- Clicking different org switches context
- Data (assets, locations) reflects selected org
- API calls include correct org context
- URL/route updated appropriately (if applicable)

#### Persistence
- Last used org remembered on login
- After logout/login, returns to last org
- New user defaults to personal org

#### Multi-Org User Scenarios
- User with 3+ orgs can switch between all
- Role differs per org (admin in one, viewer in another)
- Permissions update when switching orgs

#### Superadmin (if in scope)
- Superadmin can access any org via direct URL/API
- Superadmin access is logged (verify in backend logs or audit table)
- Superadmin sees all orgs in switcher (or separate admin view)

Note: Superadmin tests may be deferred if not needed for NADA launch.

### Fixtures Required
```typescript
// Need to extend org.fixture.ts with:
- setupMultiOrgUser(numOrgs: number): Promise<{ user, orgs: Org[] }>
- getOrgDataCount(page, dataType: 'assets'|'locations'): Promise<number>
- verifyOrgContext(page, expectedOrgId): Promise<boolean>
```

### Files to Create/Modify
- `frontend/tests/e2e/org-switching.spec.ts` (new)
- `frontend/tests/e2e/fixtures/org.fixture.ts` (extend)

## Validation Criteria
- [ ] Org switcher displays all user orgs with correct roles
- [ ] Context switching works reliably
- [ ] Data isolation verified (no cross-org data leakage)
- [ ] Session persistence works across logout/login
- [ ] Tests run < 90 seconds total
- [ ] No flaky tests on context switches

## Success Metrics
- [ ] 10+ test cases covering all scenarios
- [ ] Multi-org fixture setup < 8 seconds
- [ ] Zero data isolation failures
- [ ] Context switch latency < 2 seconds

## References
- [TRA-172 Implementation](frontend/tests/e2e/org-crud.spec.ts) - establishes patterns
- [Org Fixture Helpers](frontend/tests/e2e/fixtures/org.fixture.ts) - `switchToOrg()` helper
- [Linear Issue](https://linear.app/trakrf/issue/TRA-175)
