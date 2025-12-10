# Feature: Playwright Core Org CRUD Tests

## Metadata
**Workspace**: frontend
**Type**: feature
**Linear**: TRA-172 (child of TRA-171 epic)

## Outcome
E2E test coverage for organization CRUD operations, establishing test patterns and fixtures that subsequent org test issues (TRA-173, 174, 175) will build upon. This PR will also surface and help debug the known "org create hang" bug.

## User Story
As a developer
I want comprehensive E2E tests for organization CRUD operations
So that regressions are caught early and the org create hang bug can be isolated and fixed

## Context
**Current**: Organization functionality was implemented in TRA-136 but lacks E2E test coverage. There's a known bug where org creation hangs (suspected React state issue). 11 E2E test files exist in `frontend/tests/e2e/` with established patterns.

**Desired**: Full test coverage for create, list, edit, and delete operations with reusable fixtures for downstream issues. The hang bug should be identified (root cause documented) and ideally fixed.

**Examples**:
- `frontend/tests/e2e/auth.spec.ts` - Established patterns for auth state clearing, validation testing, error handling
- Similar test structure with `beforeEach` cleanup, semantic test names, proper assertions

## Technical Requirements

### Test Scenarios (12 tests across 4 categories)

**Org Creation**
- User can create team org (becomes admin)
- Org create form validation (empty name, too long, etc.)
- Debug: Identify org create hang (document root cause)

**Org Listing**
- User can view orgs they belong to
- Personal org shows in list after signup
- New team org appears in list after creation

**Org Edit**
- Admin can edit org name
- Non-admin cannot see edit option
- Edit validation (empty name, etc.)

**Org Delete**
- Admin can delete org (soft delete)
- Delete requires confirmation
- Non-admin cannot see delete option
- Deleted org disappears from list

### Test Infrastructure

**Files to Create:**
- `frontend/tests/e2e/org-crud.spec.ts` - Main test file
- `frontend/tests/e2e/fixtures/org.fixture.ts` - Reusable org fixtures (if needed beyond inline helpers)

**Fixture Pattern:**
```typescript
// Helper for API-level org creation (fast test setup)
async function createTestOrg(page: Page, name: string) {
  const response = await page.request.post('/api/v1/orgs', {
    data: { name }
  });
  return response.json();
}

// Reuse auth patterns from auth.spec.ts
async function loginAsAdmin(page: Page) {
  // Clear auth state, login, verify
}
```

### Constraints
- Tests must run < 60 seconds total
- No hardware dependencies (no BLE/RFID)
- Headless only (no X Windows - see frontend CLAUDE.md)
- Follow existing auth.spec.ts patterns for consistency

## Validation Criteria
- [ ] All 12 org CRUD test scenarios pass
- [ ] Org create hang root cause identified and documented
- [ ] Tests run in < 60 seconds (`pnpm test:e2e tests/e2e/org-crud.spec.ts`)
- [ ] No console errors during test runs
- [ ] Fixtures are reusable for TRA-173/174/175

## Success Metrics
- [ ] `pnpm test:e2e tests/e2e/org-crud.spec.ts` - All tests pass
- [ ] `pnpm lint` - No linting errors in new files
- [ ] `pnpm typecheck` - No type errors
- [ ] Total execution time < 60s
- [ ] If hang bug is fixable in scope, fix it; otherwise document root cause for separate PR

## Implementation Notes

### Org Create Hang Investigation
The hang likely occurs in React state management. Test approach:
1. Add console logging/debug breakpoints around org creation
2. Check if the issue is API response handling
3. Check if it's React state not updating
4. Document findings even if not fixed in this PR

### RBAC Testing Approach
- Create test user as org admin (via signup flow)
- For non-admin tests, may need to create second user via API
- Verify UI elements show/hide based on role

### Test Data Cleanup
- Use unique org names with timestamps to avoid collisions
- Consider cleanup in `afterEach` or `afterAll` hooks
- API-level deletion for fast cleanup

## References
- TRA-171: Parent epic with full test strategy
- TRA-136: Original org implementation (referenced in epic)
- `frontend/tests/e2e/auth.spec.ts`: Pattern reference
- `frontend/tests/e2e/helpers/`: Existing test utilities
