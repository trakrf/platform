/**
 * Member Management E2E Tests (TRA-173)
 *
 * Tests member viewing, role changes, removal, and RBAC enforcement.
 * Uses "shared session + expendable targets" pattern from org-crud.spec.ts
 *
 * Non-admin RBAC visibility tests (deferred from TRA-172) included here.
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
  switchToOrg,
  goToMembersPage,
  addTestMemberToOrg,
  getCurrentOrgId,
} from './fixtures/org.fixture';

test.describe('Member Management', () => {
  // Shared state - created once in beforeAll
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // Signup admin user and create a team org
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Test Members Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    // Signup creates personal org automatically
    await signupTestUser(page, adminEmail, adminPassword);

    // Create team org for testing members
    await page.goto('/#create-org');
    await page.locator('input#name').fill(testOrgName);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL(/#home/, { timeout: 10000 });

    // Switch to the team org
    await switchToOrg(page, testOrgName);

    // Get the org ID for API operations
    testOrgId = await getCurrentOrgId(page);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Clear state and login fresh before each test
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    // Switch to the test org
    await switchToOrg(page, testOrgName);
  });

  // Task 5: View Members Tests
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
      // The "You" badge should be visible next to the admin's name
      await expect(page.locator('text=You')).toBeVisible();
    });

    test('should show role dropdown for admin', async ({ page }) => {
      await goToMembersPage(page);
      // Admin should see role dropdown (select element)
      await expect(page.locator('select').first()).toBeVisible();
    });
  });

  // Task 6: Role Management Tests
  test.describe('Role Management', () => {
    test('admin can change member role', async ({ page }) => {
      // Create expendable viewer
      const member = await addTestMemberToOrg(page, testOrgId, 'viewer');

      await goToMembersPage(page);

      // Find row by email, change dropdown to 'operator'
      const row = page.locator(`tr:has-text("${member.email}")`);
      await row.locator('select').selectOption('operator');

      // Verify success toast
      await expect(page.locator('text=Member role updated')).toBeVisible({
        timeout: 5000,
      });
    });

    test('role change persists after reload', async ({ page }) => {
      const member = await addTestMemberToOrg(page, testOrgId, 'viewer');
      await goToMembersPage(page);

      // Change to operator
      const row = page.locator(`tr:has-text("${member.email}")`);
      await row.locator('select').selectOption('operator');
      await expect(page.locator('text=Member role updated')).toBeVisible({
        timeout: 5000,
      });

      // Reload and verify
      await page.reload();
      await page.waitForSelector('th:has-text("Name")', { timeout: 10000 });

      // Verify the role is now operator
      const updatedRow = page.locator(`tr:has-text("${member.email}")`);
      await expect(updatedRow.locator('select')).toHaveValue('operator');
    });

    test('cannot demote last admin - shows error', async ({ page }) => {
      await goToMembersPage(page);

      // Admin's own row - try to change role to viewer
      const adminRow = page.locator(`tr:has-text("${adminEmail}")`);
      await adminRow.locator('select').selectOption('viewer');

      // Should show error about last admin
      await expect(
        page.locator('text=/[Cc]annot|[Ll]ast.*admin|[Oo]nly.*admin/')
      ).toBeVisible({ timeout: 5000 });
    });
  });

  // Task 7: Remove Members Tests
  test.describe('Remove Members', () => {
    test('admin can remove member', async ({ page }) => {
      const member = await addTestMemberToOrg(page, testOrgId, 'viewer');
      await goToMembersPage(page);

      const row = page.locator(`tr:has-text("${member.email}")`);
      // Click the remove button (trash icon)
      await row.locator('button[title="Remove member"]').click();

      // Verify success toast
      await expect(page.locator('text=Member removed')).toBeVisible({
        timeout: 5000,
      });

      // Verify member is gone from list
      await expect(row).not.toBeVisible();
    });

    test('admin cannot remove self', async ({ page }) => {
      await goToMembersPage(page);

      // Admin row should not have remove button
      const adminRow = page.locator(`tr:has-text("${adminEmail}")`);
      await expect(
        adminRow.locator('button[title="Remove member"]')
      ).not.toBeVisible();
    });
  });

  // Task 8: Non-Admin RBAC Tests (deferred from TRA-172)
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
      // Should see capitalized role text
      await expect(page.locator('td span.capitalize')).toBeVisible();
    });

    test('viewer cannot see remove button', async ({ page }) => {
      const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
      await clearAuthState(page);
      await loginTestUser(page, viewer.email, viewer.password);
      await switchToOrg(page, testOrgName);

      await goToMembersPage(page);

      // No remove buttons should be visible for non-admin
      await expect(
        page.locator('button[title="Remove member"]')
      ).not.toBeVisible();
    });

    test('viewer cannot see org delete option in settings', async ({ page }) => {
      const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
      await clearAuthState(page);
      await loginTestUser(page, viewer.email, viewer.password);
      await switchToOrg(page, testOrgName);

      await page.goto('/#org-settings');

      // Should not see danger zone / delete button
      await expect(
        page.locator('button:has-text("Delete Organization")')
      ).not.toBeVisible();
    });

    test('viewer cannot see org name edit form', async ({ page }) => {
      const viewer = await addTestMemberToOrg(page, testOrgId, 'viewer');
      await clearAuthState(page);
      await loginTestUser(page, viewer.email, viewer.password);
      await switchToOrg(page, testOrgName);

      await page.goto('/#org-settings');

      // Should not see the edit form (input#org-name)
      await expect(page.locator('input#org-name')).not.toBeVisible();
    });
  });
});
