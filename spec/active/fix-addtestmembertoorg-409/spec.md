# Feature: Fix addTestMemberToOrg 409 Conflict (TRA-211)

## Metadata
**Workspace**: frontend + backend (investigation)
**Type**: bug
**Linear**: https://linear.app/trakrf/issue/TRA-211
**Blocking**: TRA-173, TRA-174 (org member and invitation tests)

## Outcome
Fix the `addTestMemberToOrg` E2E test fixture so multi-user tests can run without 409 "already a member" conflicts.

**Potential secondary issue**: A 409 was also observed in interactive testing - need to determine if there's an underlying backend bug beyond the test fixture.

## User Story
As a developer
I want the `addTestMemberToOrg` fixture to work reliably
So that multi-user E2E tests can pass and unblock TRA-173/TRA-174

## Context

### Discovery
Analysis of the test flow reveals a race condition in `switchToOrg`:

1. `beforeAll` creates admin + team org
2. `switchToOrg` clicks dropdown and waits for menu to close
3. `getCurrentOrgId` fetches `/api/v1/users/me` to get `current_org.id`
4. Test calls `createInviteViaAPI` with `testOrgId`

**The bug**: `switchToOrg` only waits for the dropdown UI to close, but doesn't wait for:
- The `setCurrentOrg` API call to complete
- The profile refetch to complete
- The localStorage token to be updated

This means `getCurrentOrgId` may read stale data and return the **first org ID** (created during signup) instead of the **second org ID** (the team org created for testing).

### Current State
The flow in `addTestMemberToOrg`:
```typescript
// 1. Create invitation in team org (but orgId might be wrong!)
const inviteId = await createInviteViaAPI(page, orgId, email, role);

// 2. Get token
const inviteToken = await getInviteToken(page, inviteId);

// 3. New user signs up (creates their own org)
const newUserToken = await signupViaAPI(page, email, password, memberOrgName);

// 4. Accept fails with 409 - likely due to stale testOrgId or test data pollution
await acceptInviteViaAPI(page, inviteToken, newUserToken);
```

### Root Cause
If `testOrgId` = admin's first org ID (created during signup, not the team org), then:
- Invitation is created for admin's first org
- New user signs up with invited email → creates their own org
- New user tries to accept invitation
- If new user's email somehow matches an existing member of admin's first org, 409 occurs
- More likely: test data pollution from previous runs where same org/email combinations exist

### Evidence
The `switchToOrg` fixture:
```typescript
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  await page.locator(`button:has-text("${orgName}")`).click();
  // Only waits for dropdown to close - NOT for API to complete!
  await page.waitForSelector('[role="menu"]', { state: 'hidden', timeout: 5000 });
}
```

## Technical Requirements

### Fix 1: Wait for org switch API to complete
Update `switchToOrg` to wait for the backend state change:

```typescript
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  await page.locator(`button:has-text("${orgName}")`).click();

  // Wait for dropdown to close
  await page.waitForSelector('[role="menu"]', { state: 'hidden', timeout: 5000 });

  // Wait for API call to complete and localStorage to update
  await page.waitForFunction(
    (expectedOrgName) => {
      const authStorage = localStorage.getItem('auth-storage');
      if (!authStorage) return false;
      try {
        const { state } = JSON.parse(authStorage);
        return state?.profile?.current_org?.name === expectedOrgName;
      } catch {
        return false;
      }
    },
    orgName,
    { timeout: 10000 }
  );
}
```

### Fix 2: Alternative - Use API-based org switch
Bypass UI entirely for test setup reliability:

```typescript
export async function switchOrgViaAPI(page: Page, orgId: number): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.put(`${baseUrl}/users/current-org`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: { org_id: orgId },
  });

  if (!response.ok()) {
    throw new Error(`Failed to switch org: ${response.status()}`);
  }

  // Update localStorage with new token
  const data = await response.json();
  await page.evaluate((newToken) => {
    const authStorage = localStorage.getItem('auth-storage');
    if (authStorage) {
      const parsed = JSON.parse(authStorage);
      parsed.state.token = newToken;
      localStorage.setItem('auth-storage', JSON.stringify(parsed));
    }
  }, data.token);
}
```

### Fix 3: Get org ID directly after creation
Don't rely on `getCurrentOrgId` after switch - get it from creation response:

```typescript
// In test setup
const createResponse = await page.request.post(`${baseUrl}/orgs`, {
  headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
  data: { name: testOrgName },
});
const { data: newOrg } = await createResponse.json();
testOrgId = newOrg.id; // Direct from creation, no switch needed
```

### Files to Modify
- `frontend/tests/e2e/fixtures/org.fixture.ts` - Fix `switchToOrg` or add `switchOrgViaAPI`

## Validation Criteria
- [ ] `addTestMemberToOrg` completes without 409 error
- [ ] `getCurrentOrgId` returns correct team org ID after switch
- [ ] All skipped tests in org-members.spec.ts can be unskipped:
  - [ ] admin can change member role
  - [ ] role change persists after reload
  - [ ] admin can remove member
  - [ ] viewer cannot see role dropdown
  - [ ] viewer cannot see remove button
  - [ ] viewer cannot see org delete option in settings
  - [ ] viewer cannot see org name edit form
- [ ] Email mismatch returns 403 with clear message (not confusing 409)
- [ ] User with wrong email cannot accept invitation intended for someone else

## Recommended Approach
**Fix 1 + Fix 3 + Fix 4**:
1. Update `switchToOrg` to wait for localStorage sync (defensive)
2. Use direct org ID from creation response (primary fix for test fixture)
3. Add `switchOrgViaAPI` helper for future tests that need reliable org switching
4. Enforce email match on invitation accept (backend security fix)

## Implementation Order
1. **Backend first** - Implement Fix 4 (email verification on accept)
2. **Frontend** - Handle new `email_mismatch` error in AcceptInviteScreen
3. **Test fixture** - Implement Fix 3 (get org ID from creation response)
4. **Test fixture** - Update `switchToOrg` with proper wait (Fix 1)
5. Verify one test passes
6. Unskip remaining tests
7. Run full test suite

## Fix 4: Enforce Email Match on Accept (Backend)

**Decision**: Only allow the invited email to accept the invitation. If sent to wrong email, admin should cancel and resend.

### Confirmed Scenario (Interactive Testing)
Admin sent invitation, then clicked the invite link while still logged in as admin → got confusing 409 "already a member" because admin is already in their own org. Should have been a clear "wrong email" error.

### Current Behavior
Anyone with the invite link can accept, leading to confusing 409 when existing member clicks someone else's link.

### New Behavior
Backend verifies accepting user's email matches invitation email. Clear error if mismatch:
- **Before**: Admin clicks invite → 409 "already a member" (confusing)
- **After**: Admin clicks invite → 403 "This invitation was sent to a different email address" (clear)

### Implementation
In `backend/internal/services/auth/auth.go` `AcceptInvitation()`:

```go
// After looking up invitation, before checking membership:

// Get accepting user's email
user, err := s.storage.GetUserByID(ctx, userID)
if err != nil {
    return nil, fmt.Errorf("failed to get user: %w", err)
}

// Verify email matches invitation
if !strings.EqualFold(user.Email, inv.Email) {
    return nil, fmt.Errorf("email_mismatch")
}
```

In `backend/internal/handlers/auth/auth.go` `AcceptInvite()`:

```go
case "email_mismatch":
    httputil.WriteJSONError(w, r, http.StatusForbidden, errors.ErrForbidden,
        "This invitation was sent to a different email address", "", middleware.GetRequestID(r.Context()))
```

### Frontend Update
In `frontend/src/components/AcceptInviteScreen.tsx` `extractErrorMessage()`:

```typescript
if (detail.toLowerCase().includes('different email') || title.toLowerCase().includes('different email')) {
  return 'This invitation was sent to a different email address. Please log in with the correct account.';
}
```

### Files to Modify
- `backend/internal/services/auth/auth.go` - Add email verification
- `backend/internal/handlers/auth/auth.go` - Add error case
- `frontend/src/components/AcceptInviteScreen.tsx` - Handle new error

## References
- [org.fixture.ts](frontend/tests/e2e/fixtures/org.fixture.ts) - Current fixtures
- [org-members.spec.ts](frontend/tests/e2e/org-members.spec.ts) - Blocked tests
- [orgStore.ts](frontend/src/stores/orgStore.ts) - switchOrg implementation
- [invitations.go (service)](backend/internal/services/orgs/invitations.go) - Invitation creation
- [auth.go (service)](backend/internal/services/auth/auth.go) - AcceptInvitation logic
- [Linear Issue](https://linear.app/trakrf/issue/TRA-211)
