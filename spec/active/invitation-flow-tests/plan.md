# Implementation Plan: Invitation Flow Tests (TRA-174)

Generated: 2025-12-13
Specification: spec.md

## Understanding

Implement comprehensive E2E test coverage for the invitation lifecycle:
- Sending invitations (admin only)
- Accepting invitations (existing and new users)
- Redirect flow token preservation through auth
- Cancel/resend operations (admin only)
- Edge cases and error handling

Key decisions from planning:
1. **Expiration testing**: Verify UI shows "Expired" text + test error handling with invalid tokens (no backend test endpoint needed)
2. **New user flow**: Full UI signup to test redirect token preservation
3. **Test organization**: Multiple files (~5-8 tests each) using shared fixtures
4. **Cancel/resend targeting**: Use invitation ID from API to target test IDs directly

## Relevant Files

**Reference Patterns** (existing code to follow):
- `frontend/tests/e2e/org-crud.spec.ts` - beforeAll/beforeEach pattern, test structure
- `frontend/tests/e2e/org-members.spec.ts` - multi-user testing, role switching
- `frontend/tests/e2e/fixtures/org.fixture.ts` - existing fixtures to extend

**Files to Create**:
- `frontend/tests/e2e/org-invite-send.spec.ts` - Send invitation tests (5 tests)
- `frontend/tests/e2e/org-invite-accept.spec.ts` - Accept invitation tests (8 tests)
- `frontend/tests/e2e/org-invite-redirect.spec.ts` - Redirect flow tests (4 tests)
- `frontend/tests/e2e/org-invite-manage.spec.ts` - Cancel/resend tests (4 tests)
- `frontend/tests/e2e/org-invite-errors.spec.ts` - Edge case tests (5 tests)

**Files to Modify**:
- `frontend/tests/e2e/fixtures/org.fixture.ts` - Add 3 new fixtures

**UI Components** (reference for test IDs):
- `frontend/src/components/AcceptInviteScreen.tsx` - `accept-invite-button`, `decline-invite-button`
- `frontend/src/components/InvitationsSection.tsx` - `invite-member-button`, `cancel-invite-{id}`, `resend-invite-{id}`
- `frontend/src/components/InviteModal.tsx` - `invite-email-input`, `invite-role-select`, `invite-send-button`

## Architecture Impact

- **Subsystems affected**: Frontend E2E tests only
- **New dependencies**: None
- **Breaking changes**: None

## Task Breakdown

### Task 1: Add New Fixtures to org.fixture.ts

**File**: `frontend/tests/e2e/fixtures/org.fixture.ts`
**Action**: MODIFY
**Pattern**: Reference existing `createInviteViaAPI` (lines 173-197)

**Implementation**:
```typescript
/**
 * Cancel invitation via API
 */
export async function cancelInviteViaAPI(
  page: Page,
  orgId: number,
  inviteId: number
): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.delete(
    `${baseUrl}/orgs/${orgId}/invitations/${inviteId}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to cancel invitation: ${response.status()} - ${text}`);
  }
}

/**
 * Resend invitation via API
 */
export async function resendInviteViaAPI(
  page: Page,
  orgId: number,
  inviteId: number
): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.post(
    `${baseUrl}/orgs/${orgId}/invitations/${inviteId}/resend`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
    }
  );

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to resend invitation: ${response.status()} - ${text}`);
  }
}

/**
 * Get invitations list via API
 */
export async function getInvitationsViaAPI(
  page: Page,
  orgId: number
): Promise<Array<{ id: number; email: string; role: string; expires_at: string }>> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.get(
    `${baseUrl}/orgs/${orgId}/invitations`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to get invitations: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return data.data ?? [];
}
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`

---

### Task 2: Create org-invite-send.spec.ts

**File**: `frontend/tests/e2e/org-invite-send.spec.ts`
**Action**: CREATE
**Pattern**: Reference `org-members.spec.ts` (lines 22-64) for setup pattern

**Tests** (5 scenarios):
1. `admin can send invitation with email and role` - Open modal, fill email/role, submit, verify toast
2. `invitation appears in pending list` - After send, verify row in table with correct email/role
3. `non-admin cannot see invite button` - Login as viewer, verify button not visible
4. `cannot invite same email twice` - Send invite, try again, verify error message
5. `email validation rejects invalid format` - Enter invalid email, verify send button disabled

**Implementation**:
```typescript
import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
  createOrgViaAPI,
  switchOrgViaAPI,
  addTestMemberToOrg,
  goToMembersPage,
} from './fixtures/org.fixture';

test.describe('Send Invitation', () => {
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, adminEmail, adminPassword);

    const org = await createOrgViaAPI(page, `Invite Test Org ${id}`);
    testOrgId = org.id;
    await switchOrgViaAPI(page, testOrgId);
    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);
    await page.reload({ waitUntil: 'networkidle' });
  });

  // Tests here...
});
```

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test` (run specific file)

---

### Task 3: Create org-invite-accept.spec.ts

**File**: `frontend/tests/e2e/org-invite-accept.spec.ts`
**Action**: CREATE
**Pattern**: Reference `org-members.spec.ts` for multi-user patterns

**Tests** (8 scenarios):

**Existing User** (4 tests):
1. `logged-in user can accept invitation via token URL` - Create invite, get token, navigate, click accept
2. `user added to org with correct role` - After accept, verify org membership via API
3. `user sees success message with org name` - Verify "Welcome to {org}!" screen
4. `user can navigate to dashboard after accept` - Click "Go to Dashboard", verify redirect

**New User** (4 tests):
1. `non-logged-in user sees login/signup options` - Navigate to accept-invite without auth, verify buttons
2. `after signup user returns to accept-invite screen` - Click signup, complete, verify return with token
3. `new user can accept pending invitation` - Complete signup flow, accept invitation
4. `new user added to org with correct role` - Verify membership after full flow

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 4: Create org-invite-redirect.spec.ts

**File**: `frontend/tests/e2e/org-invite-redirect.spec.ts`
**Action**: CREATE
**Pattern**: Reference `authRedirect.ts` (lines 1-39) for redirect logic

**Tests** (4 scenarios):
1. `login redirect preserves token in URL params` - Start at accept-invite, click login, verify URL has token
2. `signup redirect preserves token in URL params` - Start at accept-invite, click signup, verify URL has token
3. `after auth user returns to accept-invite with token intact` - Complete auth, verify redirect URL
4. `token extraction works after redirect` - Full flow, verify accept button works with preserved token

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 5: Create org-invite-manage.spec.ts

**File**: `frontend/tests/e2e/org-invite-manage.spec.ts`
**Action**: CREATE
**Pattern**: Use invitation ID from API to target buttons

**Tests** (4 scenarios):
1. `admin can cancel pending invitation` - Create invite, click cancel button, verify toast
2. `canceled invitation removed from list` - After cancel, verify row gone from table
3. `canceled token no longer works` - Cancel invite, try to accept token, verify error
4. `admin can resend invitation` - Create invite, click resend, verify toast + updated expiry

**Implementation note**: Use `[data-testid="cancel-invite-${inviteId}"]` selector pattern

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 6: Create org-invite-errors.spec.ts

**File**: `frontend/tests/e2e/org-invite-errors.spec.ts`
**Action**: CREATE
**Pattern**: Reference `AcceptInviteScreen.tsx` (lines 192-223) for error messages

**Tests** (5 scenarios):
1. `expired invitation shows Expired text in list` - Create invite, check UI shows expiry info (verify formatExpiry works)
2. `invalid/malformed token shows error` - Navigate to `/#accept-invite?token=invalid123`, verify error message
3. `already accepted token shows error` - Accept invite, try again with same token, verify error
4. `cancelled invitation token shows error` - Cancel invite via API, try to accept, verify error
5. `user already a member shows message` - User already in org tries to accept invite, verify message

**Validation**:
- `just frontend lint`
- `just frontend typecheck`
- `just frontend test`

---

### Task 7: Final Validation

**Action**: Run full test suite and verify all tests pass

**Commands**:
```bash
# Run all invitation tests
cd frontend && pnpm test:e2e tests/e2e/org-invite-*.spec.ts

# Full validation
just frontend validate
```

**Success criteria**:
- All 26 tests pass
- No lint errors
- No type errors
- Tests run < 120 seconds total

## Risk Assessment

- **Risk**: Tests may be flaky due to timing issues with UI updates
  **Mitigation**: Use proper waitFor patterns, avoid arbitrary delays

- **Risk**: Token preservation through redirect may have race conditions
  **Mitigation**: Use `waitForURL` with proper patterns, verify token in URL before proceeding

- **Risk**: Multi-user tests may have auth state leakage
  **Mitigation**: Always `clearAuthState` in beforeEach, use fresh browser contexts

## Integration Points

- **Store updates**: Tests verify orgStore membership via API, not direct store access
- **Route changes**: Tests verify hash navigation (`#accept-invite`, `#home`, `#org-members`)
- **Config updates**: None required

## VALIDATION GATES (MANDATORY)

**CRITICAL**: These are not suggestions - they are GATES that block progress.

After EVERY code change:
- Gate 1: `just frontend lint`
- Gate 2: `just frontend typecheck`
- Gate 3: `just frontend test` (run specific file during development)

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

**Do not proceed to next task until current task passes all gates.**

## Validation Sequence

After each task:
```bash
just frontend lint
just frontend typecheck
just frontend test tests/e2e/org-invite-*.spec.ts
```

Final validation:
```bash
just frontend validate
```

## Plan Quality Assessment

**Complexity Score**: 3/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- ✅ Clear requirements from spec with explicit test scenarios
- ✅ Similar patterns found at `org-crud.spec.ts`, `org-members.spec.ts`
- ✅ All clarifying questions answered
- ✅ Existing test fixtures to extend at `org.fixture.ts`
- ✅ UI components have test IDs already in place
- ✅ API client methods already exist in `orgs.ts`

**Assessment**: Well-defined testing task with clear patterns to follow. All infrastructure exists.

**Estimated one-pass success probability**: 90%

**Reasoning**: Existing patterns are well-established, test IDs are in place, API methods exist. Main risk is timing/flakiness in multi-user flows, but mitigation strategies are clear.
