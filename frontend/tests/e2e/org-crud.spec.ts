/**
 * Org CRUD E2E Tests
 *
 * Tests organization create, list, edit, and delete operations.
 * Part of TRA-172 - establishes patterns for TRA-173, 174, 175.
 *
 * Hang Bug Investigation (TRA-172):
 * - Code inspection shows proper async/await patterns in orgStore.createOrg()
 * - CreateOrgScreen.tsx has correct form submission with try/catch
 * - If hang occurs, likely candidates:
 *   1. Network timeout on orgsApi.create() call
 *   2. Profile refetch (fetchProfile) blocking if API slow
 *   3. React state update race condition in store subscription
 * - E2E tests will help isolate if hang is reproducible in headless mode
 * - If not reproducible in tests, manual debugging with React DevTools recommended
 *
 * Note: Non-admin RBAC visibility tests deferred to TRA-173
 * See: https://linear.app/trakrf/issue/TRA-173
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
  openOrgSwitcher,
  switchToOrg,
} from './fixtures/org.fixture';

test.describe('Organization CRUD', () => {
  let testEmail: string;
  let testPassword: string;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // Signup once, reuse session across tests
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-${id}@example.com`;
    testPassword = 'TestPassword123!';
    testOrgName = `Test Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testEmail, testPassword);
    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Clear state and login fresh before each test
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);
  });

  test.describe('Org Creation', () => {
    test('should create new team org successfully', async ({ page }) => {
      const newOrgName = `New Org ${uniqueId()}`;
      await page.goto('/#create-org');

      // Verify we're on the create org page
      await expect(
        page.getByRole('heading', { name: 'Create Organization' })
      ).toBeVisible();

      // Fill in org name and submit
      await page.locator('input#name').fill(newOrgName);
      await page.locator('button[type="submit"]').click();

      // Should redirect to home after successful creation
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Verify we're on home page
      await expect(page).toHaveURL(/#home/);
    });

    test('should show validation error for empty name', async ({ page }) => {
      await page.goto('/#create-org');

      // Click submit without filling in name
      await page.locator('button[type="submit"]').click();

      // Should show validation error
      await expect(
        page.locator('text=Organization name is required')
      ).toBeVisible();
    });

    test('should show validation error for name too short', async ({ page }) => {
      await page.goto('/#create-org');

      // Enter a single character
      await page.locator('input#name').fill('A');
      await page.locator('input#name').blur();

      // Should show validation error
      await expect(
        page.locator('text=Name must be at least 2 characters')
      ).toBeVisible();
    });
  });

  test.describe('Org Listing', () => {
    test('should display orgs in switcher dropdown', async ({ page }) => {
      // Open the org switcher dropdown
      await openOrgSwitcher(page);

      // Should see the "Organizations" header in dropdown
      await expect(page.locator('text=Organizations')).toBeVisible();

      // Should see the personal org from signup (named after email)
      await expect(page.getByRole('menuitem', { name: testEmail })).toBeVisible();
    });

    test('should show newly created org in list', async ({ page }) => {
      const newOrgName = `Listed Org ${uniqueId()}`;

      // Create a new org
      await page.goto('/#create-org');
      await page.locator('input#name').fill(newOrgName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Open the org switcher
      await openOrgSwitcher(page);

      // The new org should appear in the list - use menuitem role to be specific
      await expect(page.getByRole('menuitem', { name: newOrgName })).toBeVisible();
    });
  });

  test.describe('Org Edit', () => {
    test('should allow admin to edit org name', async ({ page }) => {
      // First create an org to edit
      const originalName = `Edit Test Org ${uniqueId()}`;
      const newName = `Renamed Org ${uniqueId()}`;

      await page.goto('/#create-org');
      await page.locator('input#name').fill(originalName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Switch to the newly created org
      await switchToOrg(page, originalName);

      // Go to org settings
      await page.goto('/#org-settings');

      // Wait for the settings page to load and the form to be ready
      const nameInput = page.locator('input#org-name');
      await expect(nameInput).toBeVisible({ timeout: 10000 });

      // Edit the name
      await nameInput.clear();
      await nameInput.fill(newName);

      // Submit the form
      await page.locator('button[type="submit"]').click();

      // Should show success toast
      await expect(page.locator('text=Organization name updated')).toBeVisible({
        timeout: 5000,
      });
    });

    test('should disable save button when name is empty', async ({ page }) => {
      // Create an org first
      const orgName = `Empty Edit Org ${uniqueId()}`;
      await page.goto('/#create-org');
      await page.locator('input#name').fill(orgName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Switch to the newly created org
      await switchToOrg(page, orgName);

      // Go to org settings
      await page.goto('/#org-settings');

      // Wait for the input to be ready
      const nameInput = page.locator('input#org-name');
      await expect(nameInput).toBeVisible({ timeout: 10000 });

      // Clear the name field
      await nameInput.clear();

      // The save button should be disabled when the form is invalid (empty name)
      const saveButton = page.locator('button[type="submit"]');
      await expect(saveButton).toBeDisabled();
    });

    // Non-admin visibility tests deferred to TRA-173
    // See: https://linear.app/trakrf/issue/TRA-173
  });

  test.describe('Org Delete', () => {
    test('should show delete confirmation modal', async ({ page }) => {
      // Create an org to delete
      const orgName = `Delete Test Org ${uniqueId()}`;
      await page.goto('/#create-org');
      await page.locator('input#name').fill(orgName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Switch to the newly created org
      await switchToOrg(page, orgName);

      // Go to org settings
      await page.goto('/#org-settings');

      // Wait for the Delete Organization button and click it
      const deleteButton = page.locator('button:has-text("Delete Organization")');
      await expect(deleteButton).toBeVisible({ timeout: 10000 });
      await deleteButton.click();

      // Modal should appear with confirmation input
      await expect(
        page.locator('[data-testid="delete-org-confirm-input"]')
      ).toBeVisible();
    });

    test('should require exact name match to delete', async ({ page }) => {
      // Create an org to delete
      const orgName = `Confirm Delete Org ${uniqueId()}`;
      await page.goto('/#create-org');
      await page.locator('input#name').fill(orgName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Switch to the newly created org
      await switchToOrg(page, orgName);

      // Go to org settings and open delete modal
      await page.goto('/#org-settings');
      const deleteButton = page.locator('button:has-text("Delete Organization")');
      await expect(deleteButton).toBeVisible({ timeout: 10000 });
      await deleteButton.click();

      // Type wrong name - delete button should be disabled
      await page
        .locator('[data-testid="delete-org-confirm-input"]')
        .fill('wrong name');
      await expect(
        page.locator('[data-testid="delete-org-confirm-button"]')
      ).toBeDisabled();

      // Type correct name - delete button should be enabled
      await page.locator('[data-testid="delete-org-confirm-input"]').clear();
      await page
        .locator('[data-testid="delete-org-confirm-input"]')
        .fill(orgName);
      await expect(
        page.locator('[data-testid="delete-org-confirm-button"]')
      ).toBeEnabled();
    });

    test('should delete org and redirect to home', async ({ page }) => {
      // Create an org specifically for deletion
      const orgName = `Will Delete Org ${uniqueId()}`;
      await page.goto('/#create-org');
      await page.locator('input#name').fill(orgName);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#home/, { timeout: 10000 });

      // Switch to the newly created org
      await switchToOrg(page, orgName);

      // Go to org settings and delete
      await page.goto('/#org-settings');
      const deleteButton = page.locator('button:has-text("Delete Organization")');
      await expect(deleteButton).toBeVisible({ timeout: 10000 });
      await deleteButton.click();

      // Confirm deletion
      await page
        .locator('[data-testid="delete-org-confirm-input"]')
        .fill(orgName);
      await page.locator('[data-testid="delete-org-confirm-button"]').click();

      // Should redirect to home after deletion
      await page.waitForURL(/#home/, { timeout: 10000 });

      // The deleted org should no longer be the current org
      // (user will be switched to another org or see "no organization" state)
    });

    // Non-admin visibility tests deferred to TRA-173
    // See: https://linear.app/trakrf/issue/TRA-173
  });
});
