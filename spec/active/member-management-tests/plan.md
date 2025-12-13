# Implementation Plan: Member Management Tests (TRA-173)
Generated: 2025-12-13
Specification: spec.md

## Understanding

E2E test coverage for org member management including:
- Viewing member list (any role)
- Role changes (admin only)
- Member removal (admin only)
- Last-admin protection (UI + backend)
- Non-admin RBAC visibility (deferred from TRA-172)

**Key decisions from planning:**
1. Add backend test endpoint to expose invitation tokens (enables fast multi-user setup)
2. Test 3 roles: `admin`, `operator`, `viewer` (owner same as admin functionally)
3. Verify last-admin protection at both UI and backend levels
4. Use "shared session + expendable targets" pattern for test isolation

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/tests/e2e/org-crud.spec.ts` (lines 31-57) - beforeAll/beforeEach pattern
- `frontend/tests/e2e/fixtures/org.fixture.ts` - existing fixture helpers
- `frontend/src/components/MembersScreen.tsx` - UI selectors and behavior
- `frontend/src/components/OrgSettingsScreen.tsx` - RBAC visibility patterns
- `frontend/src/lib/api/orgs.ts` - API client methods

**Files to Create**:
- `frontend/tests/e2e/org-members.spec.ts` - main test file
- `backend/internal/handlers/test/invitations.go` - test-only endpoint for tokens

**Files to Modify**:
- `frontend/tests/e2e/fixtures/org.fixture.ts` - add multi-user helpers
- `backend/internal/routes/routes.go` - register test endpoint (dev mode only)

## Architecture Impact
- **Subsystems affected**: Frontend (E2E tests), Backend (test endpoint)
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add Backend Test Endpoint for Invitation Tokens
**File**: `backend/internal/handlers/test/invitations.go`
**Action**: CREATE

**Purpose**: Enable E2E tests to retrieve invitation tokens without email

**Implementation**:
```go
// GET /test/invitations/:id/token
// Only enabled when APP_ENV != "production"
// Returns: { "token": "abc123..." }
```

**Validation**:
```bash
just backend lint
just backend test
```

---

### Task 2: Register Test Endpoint in Routes
**File**: `backend/internal/routes/routes.go`
**Action**: MODIFY

**Purpose**: Wire up the test endpoint, guarded by environment check

**Implementation**:
```go
// Only register test routes in non-production
if os.Getenv("APP_ENV") != "production" {
    r.Route("/test", func(r chi.Router) {
        r.Get("/invitations/{id}/token", testHandler.GetInvitationToken)
    })
}
```

**Validation**:
```bash
just backend lint
just backend test
just backend build
```

---

### Task 3: Extend org.fixture.ts with Multi-User Helpers
**File**: `frontend/tests/e2e/fixtures/org.fixture.ts`
**Action**: MODIFY

**Purpose**: Add helpers for invitation flow and API-based member setup

**Implementation**:
```typescript
// New exports to add:

/** Get invitation token from test endpoint */
export async function getInviteToken(page: Page, inviteId: number): Promise<string>

/** Create invitation via API and return invite ID */
export async function createInviteViaAPI(
  page: Page,
  orgId: number,
  email: string,
  role: OrgRole
): Promise<number>

/** Accept invitation via API */
export async function acceptInviteViaAPI(page: Page, token: string): Promise<void>

/** Full flow: signup user, invite to org, accept - returns credentials */
export async function addTestMemberToOrg(
  page: Page,
  orgId: number,
  role: OrgRole
): Promise<{ email: string; password: string; userId: number }>

/** Navigate to members page */
export async function goToMembersPage(page: Page): Promise<void>
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 4: Create org-members.spec.ts - Test Structure
**File**: `frontend/tests/e2e/org-members.spec.ts`
**Action**: CREATE

**Purpose**: Set up test file with shared session pattern

**Implementation**:
```typescript
/**
 * Member Management E2E Tests (TRA-173)
 *
 * Tests member viewing, role changes, removal, and RBAC enforcement.
 * Uses "shared session + expendable targets" pattern.
 */

import { test, expect } from '@playwright/test';
import { /* helpers */ } from './fixtures/org.fixture';

test.describe('Member Management', () => {
  // Shared state - created once in beforeAll
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // 1. Signup admin user
    // 2. Create team org
    // 3. Store org ID for member operations
  });

  test.beforeEach(async ({ page }) => {
    // Login as admin before each test
  });

  test.describe('View Members', () => { /* Task 5 */ });
  test.describe('Role Management', () => { /* Task 6 */ });
  test.describe('Remove Members', () => { /* Task 7 */ });
  test.describe('Non-Admin RBAC', () => { /* Task 8 */ });
});
```

**Validation**:
```bash
just frontend lint
just frontend typecheck
```

---

### Task 5: Implement View Members Tests
**File**: `frontend/tests/e2e/org-members.spec.ts`
**Action**: MODIFY (add to existing structure)

**Tests**:
1. `should display member list with name, email, role, joined date`
2. `should show "You" badge on current user row`
3. `should show role dropdown for admin users`

**Implementation**:
```typescript
test.describe('View Members', () => {
  test('should display member list with columns', async ({ page }) => {
    await goToMembersPage(page);

    // Verify table headers
    await expect(page.locator('th:has-text("Name")')).toBeVisible();
    await expect(page.locator('th:has-text("Email")')).toBeVisible();
    await expect(page.locator('th:has-text("Role")')).toBeVisible();
    await expect(page.locator('th:has-text("Joined")')).toBeVisible();
  });

  test('should show You badge on current user', async ({ page }) => {
    await goToMembersPage(page);
    await expect(page.locator('text=You')).toBeVisible();
  });

  test('should show role dropdown for admin', async ({ page }) => {
    await goToMembersPage(page);
    await expect(page.locator('select')).toBeVisible();
  });
});
```

**Validation**:
```bash
just frontend test-e2e tests/e2e/org-members.spec.ts
```

---

### Task 6: Implement Role Management Tests
**File**: `frontend/tests/e2e/org-members.spec.ts`
**Action**: MODIFY

**Tests**:
1. `admin can change member role from viewer to operator`
2. `role change persists after page reload`
3. `cannot demote last admin - UI shows disabled/error`
4. `cannot demote last admin - backend rejects`

**Implementation**:
```typescript
test.describe('Role Management', () => {
  test('admin can change member role', async ({ page }) => {
    // Create expendable viewer
    const member = await addTestMemberToOrg(page, testOrgId, 'viewer');

    await goToMembersPage(page);

    // Find row by email, change dropdown to 'operator'
    const row = page.locator(`tr:has-text("${member.email}")`);
    await row.locator('select').selectOption('operator');

    // Verify success toast
    await expect(page.locator('text=Member role updated')).toBeVisible();
  });

  test('role change persists after reload', async ({ page }) => {
    const member = await addTestMemberToOrg(page, testOrgId, 'viewer');
    await goToMembersPage(page);

    // Change to operator
    const row = page.locator(`tr:has-text("${member.email}")`);
    await row.locator('select').selectOption('operator');
    await expect(page.locator('text=Member role updated')).toBeVisible();

    // Reload and verify
    await page.reload();
    await expect(row.locator('select')).toHaveValue('operator');
  });

  test('cannot demote last admin - UI disabled', async ({ page }) => {
    await goToMembersPage(page);

    // Admin's own row - try to change role
    // Should show error or be prevented
    const adminRow = page.locator(`tr:has-text("${adminEmail}")`);
    await adminRow.locator('select').selectOption('viewer');

    // Should show error
    await expect(page.locator('text=/[Cc]annot.*last.*admin/')).toBeVisible();
  });
});
```

**Validation**:
```bash
just frontend test-e2e tests/e2e/org-members.spec.ts
```

---

### Task 7: Implement Remove Members Tests
**File**: `frontend/tests/e2e/org-members.spec.ts`
**Action**: MODIFY

**Tests**:
1. `admin can remove member`
2. `remove requires clicking trash icon`
3. `removed member disappears from list`
4. `cannot remove self`

**Implementation**:
```typescript
test.describe('Remove Members', () => {
  test('admin can remove member', async ({ page }) => {
    const member = await addTestMemberToOrg(page, testOrgId, 'viewer');
    await goToMembersPage(page);

    const row = page.locator(`tr:has-text("${member.email}")`);
    await row.locator('button[title="Remove member"]').click();

    // Verify success
    await expect(page.locator('text=Member removed')).toBeVisible();

    // Verify gone from list
    await expect(row).not.toBeVisible();
  });

  test('cannot remove self', async ({ page }) => {
    await goToMembersPage(page);

    // Admin row should not have remove button
    const adminRow = page.locator(`tr:has-text("${adminEmail}")`);
    await expect(adminRow.locator('button[title="Remove member"]')).not.toBeVisible();
  });
});
```

**Validation**:
```bash
just frontend test-e2e tests/e2e/org-members.spec.ts
```

---

### Task 8: Implement Non-Admin RBAC Tests
**File**: `frontend/tests/e2e/org-members.spec.ts`
**Action**: MODIFY

**Tests**:
1. `non-admin cannot see role dropdown (shows text instead)`
2. `non-admin cannot see remove button`
3. `non-admin cannot see org edit in settings`
4. `non-admin cannot see org delete in settings`

**Implementation**:
```typescript
test.describe('Non-Admin RBAC', () => {
  test('viewer cannot see role dropdown', async ({ page }) => {
    // Create and login as viewer
    const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
    await clearAuthState(page);
    await loginTestUser(page, viewer.email, viewer.password);
    await switchToOrg(page, testOrgName);

    await goToMembersPage(page);

    // Should see text role, not dropdown
    await expect(page.locator('select')).not.toBeVisible();
    await expect(page.locator('td:has-text("Viewer")')).toBeVisible();
  });

  test('viewer cannot see remove button', async ({ page }) => {
    const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
    await clearAuthState(page);
    await loginTestUser(page, viewer.email, viewer.password);
    await switchToOrg(page, testOrgName);

    await goToMembersPage(page);

    await expect(page.locator('button[title="Remove member"]')).not.toBeVisible();
  });

  test('viewer cannot see org delete option', async ({ page }) => {
    const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
    await clearAuthState(page);
    await loginTestUser(page, viewer.email, viewer.password);
    await switchToOrg(page, testOrgName);

    await page.goto('/#org-settings');

    // Should not see danger zone / delete button
    await expect(page.locator('text=Delete Organization')).not.toBeVisible();
    await expect(page.locator('text=Danger Zone')).not.toBeVisible();
  });
});
```

**Validation**:
```bash
just frontend test-e2e tests/e2e/org-members.spec.ts
```

---

### Task 9: Final Validation
**Action**: VALIDATE

Run full test suite and ensure all criteria met:

```bash
# All org-members tests pass
just frontend test-e2e tests/e2e/org-members.spec.ts

# Full frontend validation
just frontend validate

# Backend validation (for new test endpoint)
just backend validate

# Verify test runtime < 90 seconds
time just frontend test-e2e tests/e2e/org-members.spec.ts
```

**Success Criteria**:
- [ ] 12+ test cases passing
- [ ] Runtime < 90 seconds
- [ ] No flaky tests (run 3x)
- [ ] All RBAC scenarios covered

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Test endpoint leaks to production | Low | High | Guard with `APP_ENV != "production"` check |
| Invitation token timing issues | Medium | Medium | Add retries/waits in fixture helpers |
| Test pollution between describe blocks | Medium | Medium | Use expendable targets pattern |
| Slow test runtime | Low | Medium | API-based setup vs UI signup |

## Integration Points

- **Backend routes**: New `/test/invitations/:id/token` endpoint
- **Frontend fixtures**: Extended `org.fixture.ts` with multi-user helpers
- **Existing tests**: No changes to `org-crud.spec.ts`

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
# Backend tasks (1-2)
just backend lint && just backend test

# Frontend tasks (3-8)
just frontend lint && just frontend typecheck

# E2E tests (tasks 5-8)
just frontend test-e2e tests/e2e/org-members.spec.ts
```

**Enforcement**: If ANY gate fails → Fix → Re-run → Repeat until pass

## Final Validation Sequence

```bash
# Full stack validation
just validate

# E2E test timing check
time just frontend test-e2e tests/e2e/org-members.spec.ts

# Flakiness check (run 3x)
for i in 1 2 3; do just frontend test-e2e tests/e2e/org-members.spec.ts || exit 1; done
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec and Linear issue
- ✅ Existing patterns in `org-crud.spec.ts` to follow
- ✅ All clarifying questions answered
- ✅ UI components already exist (`MembersScreen.tsx`)
- ✅ API endpoints already exist (`orgsApi.ts`)
- ⚠️ New backend test endpoint (simple, but cross-stack)

**Assessment**: Well-scoped E2E test implementation following established patterns. Main risk is the backend endpoint, but it's minimal code with clear guard.

**Estimated one-pass success probability**: 85%

**Reasoning**: Strong existing patterns, clear UI to test against, API layer complete. Only unknowns are edge cases in last-admin protection behavior.
