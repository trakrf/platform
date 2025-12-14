/**
 * Send Invitation E2E Tests (TRA-174)
 *
 * Tests for sending organization invitations.
 * Uses "shared session + expendable targets" pattern from org-crud.spec.ts
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
  createOrgViaAPI,
  switchOrgViaAPI,
  goToMembersPage,
  addTestMemberToOrg,
} from './fixtures/org.fixture';

test.describe('Send Invitation', () => {
  // Shared state - created once in beforeAll
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // Signup admin user and create a team org
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-invite-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Invite Test Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    // Signup creates personal org automatically
    await signupTestUser(page, adminEmail, adminPassword);

    // Create team org via API (avoids UI race conditions)
    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;

    // Switch to the team org via API
    await switchOrgViaAPI(page, testOrgId);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Clear state and login fresh before each test
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    // Switch to the test org via API (more reliable than UI)
    await switchOrgViaAPI(page, testOrgId);
    // Reload to sync UI state with the new token
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('admin can send invitation with email and role', async ({ page }) => {
    const inviteEmail = `invite-target-${uniqueId()}@example.com`;

    await goToMembersPage(page);

    // Click invite button
    await page.locator('[data-testid="invite-member-button"]').click();

    // Fill in the invite modal
    await page.locator('[data-testid="invite-email-input"]').fill(inviteEmail);
    await page.locator('[data-testid="invite-role-select"]').selectOption('operator');

    // Submit
    await page.locator('[data-testid="invite-send-button"]').click();

    // Verify success toast
    await expect(page.locator('text=Invitation sent')).toBeVisible({
      timeout: 5000,
    });
  });

  test('invitation appears in pending list', async ({ page }) => {
    const inviteEmail = `invite-list-${uniqueId()}@example.com`;

    await goToMembersPage(page);

    // Send invitation
    await page.locator('[data-testid="invite-member-button"]').click();
    await page.locator('[data-testid="invite-email-input"]').fill(inviteEmail);
    await page.locator('[data-testid="invite-role-select"]').selectOption('viewer');
    await page.locator('[data-testid="invite-send-button"]').click();

    // Wait for success toast and modal to close
    await expect(page.locator('text=Invitation sent')).toBeVisible({
      timeout: 5000,
    });

    // Verify invitation appears in the pending list
    await expect(page.locator(`text=${inviteEmail}`)).toBeVisible({
      timeout: 5000,
    });
    // Verify role is shown
    await expect(page.locator('td.capitalize:has-text("viewer")')).toBeVisible();
  });

  test('non-admin cannot see invite button', async ({ page }) => {
    // Add a viewer member
    const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');

    // Login as viewer
    await clearAuthState(page);
    await loginTestUser(page, viewer.email, viewer.password);
    await switchOrgViaAPI(page, testOrgId);
    await page.reload({ waitUntil: 'networkidle' });

    await goToMembersPage(page);

    // Invite button should not be visible for non-admin
    await expect(
      page.locator('[data-testid="invite-member-button"]')
    ).not.toBeVisible();
  });

  test('cannot invite same email twice', async ({ page }) => {
    const inviteEmail = `invite-duplicate-${uniqueId()}@example.com`;

    await goToMembersPage(page);

    // Send first invitation
    await page.locator('[data-testid="invite-member-button"]').click();
    await page.locator('[data-testid="invite-email-input"]').fill(inviteEmail);
    await page.locator('[data-testid="invite-send-button"]').click();
    await expect(page.locator('text=Invitation sent')).toBeVisible({
      timeout: 5000,
    });

    // Try to send another invitation to same email
    await page.locator('[data-testid="invite-member-button"]').click();
    await page.locator('[data-testid="invite-email-input"]').fill(inviteEmail);
    await page.locator('[data-testid="invite-send-button"]').click();

    // Should show error in the modal (use specific selector to avoid matching cancel buttons)
    await expect(
      page.locator('.bg-red-900\\/20 .text-red-400')
    ).toBeVisible({ timeout: 5000 });
  });

  test('email validation rejects invalid format', async ({ page }) => {
    await goToMembersPage(page);

    // Open invite modal
    await page.locator('[data-testid="invite-member-button"]').click();

    // Enter invalid email
    await page.locator('[data-testid="invite-email-input"]').fill('not-an-email');

    // Send button should be disabled when email is invalid
    await expect(page.locator('[data-testid="invite-send-button"]')).toBeDisabled();
  });
});
