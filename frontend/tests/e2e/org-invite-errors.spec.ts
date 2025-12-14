/**
 * Invitation Error Handling E2E Tests (TRA-174)
 *
 * Tests for edge cases and error scenarios in invitation flow.
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
  createOrgViaAPI,
  switchOrgViaAPI,
  createInviteViaAPI,
  getInviteToken,
  acceptInviteViaAPI,
  goToMembersPage,
  cancelInviteViaAPI,
} from './fixtures/org.fixture';

test.describe('Invitation Error Handling', () => {
  // Shared state
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-errors-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Errors Test Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    await signupTestUser(page, adminEmail, adminPassword);

    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;
    await switchOrgViaAPI(page, testOrgId);

    await page.close();
  });

  test('expired invitation shows expiry info in list', async ({ page }) => {
    // This test verifies the UI shows expiration info
    // (actual expiration is 7 days, so we just verify the "Expires in X days" text)
    const inviteEmail = `expiry-${uniqueId()}@example.com`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);
    await page.reload({ waitUntil: 'networkidle' });

    // Create invitation
    await createInviteViaAPI(page, testOrgId, inviteEmail, 'viewer');

    await goToMembersPage(page);

    // Verify expiration info is shown (default is 7 days)
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({ timeout: 5000 });
    await expect(page.locator('text=Expires in')).toBeVisible();
  });

  test('invalid/malformed token shows error', async ({ page }) => {
    const id = uniqueId();
    const testUserEmail = `invalid-token-${id}@example.com`;
    const testUserPassword = 'TestPassword123!';

    // Create a user to test with
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testUserEmail, testUserPassword);

    // Navigate to accept-invite with invalid token
    await page.goto('/#accept-invite?token=invalid-token-123');

    // Try to accept
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Should show error (use specific selector to avoid matching icons)
    await expect(
      page.locator('.bg-red-900\\/20 .text-red-400')
    ).toBeVisible({ timeout: 5000 });
  });

  test('already accepted token shows error', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `already-accepted-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin and create invitation
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'viewer');
    const token = await getInviteToken(page, inviteId);

    // Create invitee and accept via API first
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Accept via API
    await acceptInviteViaAPI(page, token);

    // Now try to accept same token via UI
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Should show error (already used)
    await expect(
      page.locator('.bg-red-900\\/20 .text-red-400')
    ).toBeVisible({ timeout: 5000 });
  });

  test('cancelled invitation token shows error', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `cancelled-error-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin and create invitation
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Cancel the invitation via API
    await cancelInviteViaAPI(page, testOrgId, inviteId);

    // Create invitee
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Try to use the cancelled token
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Should show error
    await expect(
      page.locator('.bg-red-900\\/20 .text-red-400')
    ).toBeVisible({ timeout: 5000 });
  });

  // Note: "user already a member" scenario is prevented by backend validation:
  // - Cannot create invitation for existing member (409 conflict)
  // - Cannot create multiple invitations for same email
  // The UI handles this edge case if it occurs via race condition, but it's
  // not reproducible in E2E tests. The frontend code exists for robustness.

  test('no token shows invalid link error', async ({ page }) => {
    // Navigate to accept-invite without token
    await page.goto('/#accept-invite');

    // Should show invalid link error
    await expect(page.locator('text=Invalid Invitation Link')).toBeVisible();
  });
});
