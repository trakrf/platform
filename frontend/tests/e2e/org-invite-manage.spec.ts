/**
 * Manage Invitations E2E Tests (TRA-174)
 *
 * Tests for cancel and resend invitation operations (admin only).
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
  goToMembersPage,
} from './fixtures/org.fixture';

test.describe('Manage Invitations', () => {
  // Shared state
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-manage-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Manage Invite Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    await signupTestUser(page, adminEmail, adminPassword);

    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;
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

  test('admin can cancel pending invitation', async ({ page }) => {
    const inviteEmail = `cancel-${uniqueId()}@example.com`;

    // Create invitation via API
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteEmail, 'viewer');

    await goToMembersPage(page);

    // Wait for invitation to appear in list
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({ timeout: 5000 });

    // Click cancel button for this invitation
    await page.locator(`[data-testid="cancel-invite-${inviteId}"]`).click();

    // Verify success toast
    await expect(page.locator('text=Invitation cancelled')).toBeVisible({
      timeout: 5000,
    });
  });

  test('canceled invitation removed from list', async ({ page }) => {
    const inviteEmail = `cancel-list-${uniqueId()}@example.com`;

    // Create invitation via API
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteEmail, 'operator');

    await goToMembersPage(page);

    // Verify invitation is in list
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({ timeout: 5000 });

    // Cancel invitation
    await page.locator(`[data-testid="cancel-invite-${inviteId}"]`).click();

    // Wait for toast
    await expect(page.locator('text=Invitation cancelled')).toBeVisible({
      timeout: 5000,
    });

    // Verify invitation is gone from list
    await expect(page.locator(`text=${inviteEmail}`)).not.toBeVisible();
  });

  test('canceled token no longer works', async ({ page }) => {
    const id = uniqueId();
    const inviteEmail = `cancel-token-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Create invitation and get token
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteEmail, 'viewer');
    const token = await getInviteToken(page, inviteId);

    await goToMembersPage(page);

    // Cancel the invitation via UI
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({ timeout: 5000 });
    await page.locator(`[data-testid="cancel-invite-${inviteId}"]`).click();
    await expect(page.locator('text=Invitation cancelled')).toBeVisible({
      timeout: 5000,
    });

    // Create invitee account
    await clearAuthState(page);
    await signupTestUser(page, inviteEmail, inviteePassword);

    // Try to use the canceled token
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Should show error (use specific selector to avoid matching icons)
    await expect(
      page.locator('.bg-red-900\\/20 .text-red-400')
    ).toBeVisible({ timeout: 5000 });
  });

  test('admin can resend invitation', async ({ page }) => {
    const inviteEmail = `resend-${uniqueId()}@example.com`;

    // Create invitation via API
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteEmail, 'manager');

    await goToMembersPage(page);

    // Wait for invitation to appear
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({ timeout: 5000 });

    // Click resend button
    await page.locator(`[data-testid="resend-invite-${inviteId}"]`).click();

    // Verify success toast
    await expect(page.locator('text=Invitation resent')).toBeVisible({ timeout: 5000 });
  });
});
