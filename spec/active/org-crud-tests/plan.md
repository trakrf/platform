# Implementation Plan: Playwright Core Org CRUD Tests
Generated: 2025-12-10
Specification: spec.md

## Understanding

Create E2E test coverage for organization CRUD operations (create, list, edit, delete) using Playwright. This establishes test patterns and fixtures for downstream org test issues (TRA-173, 174, 175). Tests run as a single authenticated user (org admin) with non-admin RBAC tests deferred to TRA-173.

Key decisions from planning:
- Non-admin visibility tests deferred to TRA-173 (updated in Linear)
- If org create hang bug isn't reproducible, document and ship tests anyway
- Create `fixtures/` directory to establish reusable pattern
- Signup once in `beforeAll`, reuse session across tests

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/tests/e2e/auth.spec.ts` (lines 15-26) - beforeEach cleanup, auth state clearing pattern
- `frontend/tests/e2e/auth.spec.ts` (lines 114-158) - signup form validation pattern
- `frontend/tests/e2e/auth.spec.ts` (lines 60-82) - error handling assertion pattern
- `frontend/tests/e2e/helpers/assertions.ts` - existing assertion helpers

**Files to Create**:
- `frontend/tests/e2e/fixtures/org.fixture.ts` - Reusable org test helpers (auth, org creation, cleanup)
- `frontend/tests/e2e/org-crud.spec.ts` - Main test file with 10 test scenarios

**Files to Modify**:
- None (pure test addition)

**Org Implementation Files** (for understanding selectors/behavior):
- `frontend/src/components/CreateOrgScreen.tsx` - Create form, validation rules
- `frontend/src/components/OrgSettingsScreen.tsx` - Edit form, delete button (admin only)
- `frontend/src/components/OrgSwitcher.tsx` - Org listing, switching
- `frontend/src/components/DeleteOrgModal.tsx` - Delete confirmation with name typing
- `frontend/src/stores/orgStore.ts` - createOrg action (potential hang location)
- `frontend/src/lib/api/orgs.ts` - API endpoints

## Architecture Impact
- **Subsystems affected**: Frontend E2E tests only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Create fixtures directory and org.fixture.ts
**File**: `frontend/tests/e2e/fixtures/org.fixture.ts`
**Action**: CREATE
**Pattern**: Reference `auth.spec.ts` lines 15-26 for auth state clearing

**Implementation**:
```typescript
// fixtures/org.fixture.ts
import type { Page } from '@playwright/test';

// Generate unique test identifiers
export function uniqueId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}

// Clear auth state (from auth.spec.ts pattern)
export async function clearAuthState(page: Page): Promise<void> {
  await page.evaluate(() => {
    localStorage.clear();
    sessionStorage.clear();
  });
}

// Signup and get authenticated session
export async function signupTestUser(page: Page, email: string, password: string, orgName: string): Promise<void> {
  await page.goto('/#signup');
  await page.locator('input#email').fill(email);
  await page.locator('input#password').fill(password);
  await page.locator('input#organizationName').fill(orgName);
  await page.locator('button[type="submit"]').click();
  // Wait for redirect to home
  await page.waitForURL(/#home/);
}

// Create org via UI (for testing the create flow)
export async function createOrgViaUI(page: Page, name: string): Promise<void> {
  await page.goto('/#create-org');
  await page.locator('input#name').fill(name);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/#home/);
}

// Navigate to org settings
export async function goToOrgSettings(page: Page): Promise<void> {
  await page.goto('/#org-settings');
}
```

**Validation**:
- `pnpm lint` passes
- `pnpm typecheck` passes

---

### Task 2: Create org-crud.spec.ts with test structure
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: CREATE
**Pattern**: Reference `auth.spec.ts` overall structure

**Implementation**:
```typescript
// org-crud.spec.ts - scaffold with describe blocks
import { test, expect } from '@playwright/test';
import { uniqueId, clearAuthState, signupTestUser } from './fixtures/org.fixture';

test.describe('Organization CRUD', () => {
  let testEmail: string;
  let testPassword: string;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // Signup once, reuse session
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-${id}@example.com`;
    testPassword = 'TestPassword123!';
    testOrgName = `Test Org ${id}`;

    await clearAuthState(page);
    await signupTestUser(page, testEmail, testPassword, testOrgName);
    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Login with test user before each test
    await page.goto('/#login');
    await page.locator('input#email').fill(testEmail);
    await page.locator('input#password').fill(testPassword);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);
  });

  test.describe('Org Creation', () => {
    // Tests go here
  });

  test.describe('Org Listing', () => {
    // Tests go here
  });

  test.describe('Org Edit', () => {
    // Tests go here
  });

  test.describe('Org Delete', () => {
    // Tests go here
  });
});
```

**Validation**:
- `pnpm lint` passes
- `pnpm typecheck` passes
- File structure matches auth.spec.ts pattern

---

### Task 3: Implement Org Creation tests
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: MODIFY
**Pattern**: Reference `auth.spec.ts` lines 126-158 for validation tests

**Test scenarios**:
1. User can create team org (becomes admin)
2. Org create form validation (empty name, too short, too long)
3. Note: Observe if hang occurs during creation

**Implementation**:
```typescript
test.describe('Org Creation', () => {
  test('should create new team org successfully', async ({ page }) => {
    const newOrgName = `New Org ${uniqueId()}`;
    await page.goto('/#create-org');

    await expect(page.getByRole('heading', { name: 'Create Organization' })).toBeVisible();
    await page.locator('input#name').fill(newOrgName);
    await page.locator('button[type="submit"]').click();

    // Should redirect to home
    await page.waitForURL(/#home/);

    // New org should appear in switcher
    // (verify org is in list)
  });

  test('should show validation error for empty name', async ({ page }) => {
    await page.goto('/#create-org');
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('text=Organization name is required')).toBeVisible();
  });

  test('should show validation error for name too short', async ({ page }) => {
    await page.goto('/#create-org');
    await page.locator('input#name').fill('A');
    await page.locator('input#name').blur();
    await expect(page.locator('text=Name must be at least 2 characters')).toBeVisible();
  });
});
```

**Validation**:
- `pnpm test:e2e tests/e2e/org-crud.spec.ts` - creation tests pass
- Note any hang behavior in test output

---

### Task 4: Implement Org Listing tests
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: MODIFY
**Pattern**: Reference OrgSwitcher.tsx for selectors

**Test scenarios**:
1. User can view orgs they belong to
2. Personal org shows in list after signup
3. New team org appears in list after creation

**Implementation**:
```typescript
test.describe('Org Listing', () => {
  test('should display orgs in switcher dropdown', async ({ page }) => {
    // Click org switcher to open dropdown
    await page.locator('[data-testid="org-switcher"]').click();
    // or use: page.getByRole('button').filter({ has: page.locator('svg.lucide-building-2') })

    // Should see at least the personal org from signup
    await expect(page.locator('text=Organizations')).toBeVisible();
    await expect(page.getByText(testOrgName)).toBeVisible();
  });

  test('should show newly created org in list', async ({ page }) => {
    const newOrgName = `Listed Org ${uniqueId()}`;

    // Create org
    await page.goto('/#create-org');
    await page.locator('input#name').fill(newOrgName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);

    // Open switcher
    await page.locator('[data-testid="org-switcher"]').click();

    // New org should be in list
    await expect(page.getByText(newOrgName)).toBeVisible();
  });
});
```

**Validation**:
- `pnpm test:e2e tests/e2e/org-crud.spec.ts` - listing tests pass

---

### Task 5: Implement Org Edit tests (admin only)
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: MODIFY
**Pattern**: Reference OrgSettingsScreen.tsx for selectors and validation

**Test scenarios**:
1. Admin can edit org name
2. Edit validation (empty name shows error)
3. Note: Non-admin tests deferred to TRA-173

**Implementation**:
```typescript
test.describe('Org Edit', () => {
  test('should allow admin to edit org name', async ({ page }) => {
    // First create an org to edit
    const originalName = `Edit Test Org ${uniqueId()}`;
    const newName = `Renamed Org ${uniqueId()}`;

    await page.goto('/#create-org');
    await page.locator('input#name').fill(originalName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);

    // Go to org settings
    await page.goto('/#org-settings');

    // Edit the name
    const nameInput = page.locator('input#org-name');
    await nameInput.clear();
    await nameInput.fill(newName);
    await page.locator('button[type="submit"]').click();

    // Should show success (toast or page update)
    await expect(page.locator('text=Organization name updated')).toBeVisible();
  });

  test('should show validation error for empty name on edit', async ({ page }) => {
    await page.goto('/#org-settings');

    const nameInput = page.locator('input#org-name');
    await nameInput.clear();
    await page.locator('button[type="submit"]').click();

    // Button should be disabled when no changes or invalid
    // Check that save doesn't proceed with empty name
  });

  // Non-admin visibility tests deferred to TRA-173
  // See: https://linear.app/trakrf/issue/TRA-173
});
```

**Validation**:
- `pnpm test:e2e tests/e2e/org-crud.spec.ts` - edit tests pass

---

### Task 6: Implement Org Delete tests (admin only)
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: MODIFY
**Pattern**: Reference DeleteOrgModal.tsx for selectors (has data-testid attributes)

**Test scenarios**:
1. Admin can delete org (soft delete)
2. Delete requires confirmation (type org name)
3. Deleted org disappears from list
4. Note: Non-admin tests deferred to TRA-173

**Implementation**:
```typescript
test.describe('Org Delete', () => {
  test('should show delete confirmation modal', async ({ page }) => {
    // Create org to delete
    const orgName = `Delete Test Org ${uniqueId()}`;
    await page.goto('/#create-org');
    await page.locator('input#name').fill(orgName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);

    // Go to settings and click delete
    await page.goto('/#org-settings');
    await page.locator('text=Delete Organization').click();

    // Modal should appear
    await expect(page.locator('[data-testid="delete-org-confirm-input"]')).toBeVisible();
    await expect(page.locator('text=Type')).toBeVisible();
  });

  test('should require exact name match to delete', async ({ page }) => {
    const orgName = `Confirm Delete Org ${uniqueId()}`;
    await page.goto('/#create-org');
    await page.locator('input#name').fill(orgName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);

    await page.goto('/#org-settings');
    await page.locator('text=Delete Organization').click();

    // Type wrong name
    await page.locator('[data-testid="delete-org-confirm-input"]').fill('wrong name');

    // Delete button should be disabled
    await expect(page.locator('[data-testid="delete-org-confirm-button"]')).toBeDisabled();

    // Type correct name
    await page.locator('[data-testid="delete-org-confirm-input"]').clear();
    await page.locator('[data-testid="delete-org-confirm-input"]').fill(orgName);

    // Delete button should be enabled
    await expect(page.locator('[data-testid="delete-org-confirm-button"]')).toBeEnabled();
  });

  test('should delete org and redirect to home', async ({ page }) => {
    const orgName = `Will Delete Org ${uniqueId()}`;
    await page.goto('/#create-org');
    await page.locator('input#name').fill(orgName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/);

    await page.goto('/#org-settings');
    await page.locator('text=Delete Organization').click();
    await page.locator('[data-testid="delete-org-confirm-input"]').fill(orgName);
    await page.locator('[data-testid="delete-org-confirm-button"]').click();

    // Should redirect to home
    await page.waitForURL(/#home/);

    // Deleted org should not appear in switcher
    // (may need to wait for profile refresh)
  });

  // Non-admin visibility tests deferred to TRA-173
  // See: https://linear.app/trakrf/issue/TRA-173
});
```

**Validation**:
- `pnpm test:e2e tests/e2e/org-crud.spec.ts` - delete tests pass

---

### Task 7: Add data-testid to OrgSwitcher if needed
**File**: `frontend/src/components/OrgSwitcher.tsx`
**Action**: MODIFY (conditional - only if selectors are flaky)

If tests have trouble selecting the org switcher button reliably, add:
```typescript
<Menu.Button
  data-testid="org-switcher"
  // ... rest of props
>
```

**Validation**:
- Tests can reliably target org switcher
- `pnpm lint` passes

---

### Task 8: Document hang bug findings
**File**: `frontend/tests/e2e/org-crud.spec.ts`
**Action**: MODIFY

Add a comment at the top of the file documenting hang bug investigation results:

```typescript
/**
 * Org CRUD E2E Tests
 *
 * Tests organization create, list, edit, and delete operations.
 *
 * Hang Bug Investigation (TRA-172):
 * - [Document findings here after running tests]
 * - If not reproducible in tests, note that manual testing may still trigger it
 * - Suspected location: orgStore.createOrg() or CreateOrgScreen form submit
 */
```

**Validation**:
- Findings documented
- If bug is reproducible, note exact steps

---

### Task 9: Final validation and cleanup
**Action**: VALIDATE

Run full test suite and verify:
1. All org-crud tests pass
2. Tests complete in < 60 seconds
3. No console errors
4. Lint and typecheck pass

**Validation**:
```bash
cd frontend
just lint
just typecheck
pnpm test:e2e tests/e2e/org-crud.spec.ts
```

## Risk Assessment

- **Risk**: OrgSwitcher may not have reliable selectors
  **Mitigation**: Add data-testid attributes if needed (Task 7)

- **Risk**: Test user cleanup between runs
  **Mitigation**: Use unique timestamps in email/org names to avoid collisions

- **Risk**: Hang bug may not be reproducible in headless mode
  **Mitigation**: Document findings; tests still valuable for regression coverage

- **Risk**: Session reuse in beforeAll may cause state bleed
  **Mitigation**: Each test creates its own orgs with unique names

## Integration Points
- No store modifications needed (tests only)
- No route changes
- No config updates

## VALIDATION GATES (MANDATORY)

After EVERY code change, run:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `pnpm test:e2e tests/e2e/org-crud.spec.ts`

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task:
```bash
cd frontend
just lint
just typecheck
```

Final validation:
```bash
just frontend validate
pnpm test:e2e tests/e2e/org-crud.spec.ts
```

Verify:
- All 10 test scenarios pass
- Execution time < 60 seconds
- No console errors in test output

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ Similar patterns found in codebase at `auth.spec.ts`
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ Components have data-testid attributes (DeleteOrgModal)
✅ Well-documented org implementation
⚠️ OrgSwitcher may need data-testid added

**Assessment**: Straightforward test implementation following established patterns. Main uncertainty is selector reliability for OrgSwitcher.

**Estimated one-pass success probability**: 85%

**Reasoning**: High confidence due to clear patterns in auth.spec.ts, well-structured org components with some existing data-testid attributes, and straightforward CRUD operations. Small risk around selector stability and potential need for data-testid additions.
