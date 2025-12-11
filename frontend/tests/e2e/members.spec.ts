/**
 * Members Screen E2E Tests (TRA-181)
 *
 * Tests that the Members screen renders without crashing when:
 * - Members list is empty (API returns [])
 * - Invitations list is empty (API returns [])
 *
 * Bug context: The screen crashed with "Cannot read properties of null (reading 'length')"
 * when the backend returned null instead of [] for empty collections.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *
 * Run with: pnpm test:e2e tests/e2e/members.spec.ts
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
} from './fixtures/org.fixture';

test.describe('Members Screen (TRA-181)', () => {
  let testEmail: string;
  let testPassword: string;

  test.beforeAll(async ({ browser }) => {
    // Signup once, reuse session across tests
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-members-${id}@example.com`;
    testPassword = 'TestPassword123!';

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

  test('should render members screen without null crash', async ({ page }) => {
    // Navigate to members screen
    await page.goto('/#org-members');

    // Wait for the page to load - should see heading
    await expect(
      page.getByRole('heading', { name: /Members/i })
    ).toBeVisible({ timeout: 10000 });

    // Verify NO React error about null.length
    const pageContent = await page.content();
    expect(pageContent).not.toContain('Cannot read properties of null');
    expect(pageContent).not.toContain('TypeError');

    // Should see either:
    // - The members table (if members exist)
    // - OR "No members" empty state
    // - OR loading state that eventually resolves
    const membersTable = page.locator('table');
    const emptyState = page.getByText(/no members|add your first member/i);
    const loadingState = page.getByText(/loading/i);

    // Wait for loading to complete
    await expect(loadingState).not.toBeVisible({ timeout: 10000 }).catch(() => {
      // Loading might have already finished, that's ok
    });

    // Either table or empty state should be visible (but not an error)
    const hasTable = await membersTable.isVisible();
    const hasEmptyState = await emptyState.isVisible().catch(() => false);

    expect(hasTable || hasEmptyState).toBeTruthy();
  });

  test('should render invitations section without null crash', async ({ page }) => {
    // Navigate to members screen (which includes InvitationsSection)
    await page.goto('/#org-members');

    // Wait for the page to load
    await expect(
      page.getByRole('heading', { name: /Members/i })
    ).toBeVisible({ timeout: 10000 });

    // Wait for invitations section to appear (admin only)
    // Should see "Pending Invitations" heading
    const invitationsHeading = page.getByRole('heading', { name: /Pending Invitations/i });

    // Wait for loading to complete
    await page.waitForTimeout(2000); // Give time for API calls

    // Verify NO React error about null.length
    const pageContent = await page.content();
    expect(pageContent).not.toContain('Cannot read properties of null');
    expect(pageContent).not.toContain('TypeError');

    // If invitations section is visible (admin), check for content
    if (await invitationsHeading.isVisible().catch(() => false)) {
      // Should see either:
      // - Invitations table
      // - OR "No pending invitations" empty state
      const invitationsTable = page.locator('table').nth(1); // Second table is invitations
      const invitationsEmptyState = page.getByText(/no pending invitations/i);

      const hasInvitationsTable = await invitationsTable.isVisible().catch(() => false);
      const hasInvitationsEmptyState = await invitationsEmptyState.isVisible().catch(() => false);

      expect(hasInvitationsTable || hasInvitationsEmptyState).toBeTruthy();
    }
  });

  test('should show invite member button for admin', async ({ page }) => {
    // Navigate to members screen
    await page.goto('/#org-members');

    // Wait for the page to load
    await expect(
      page.getByRole('heading', { name: /Members/i })
    ).toBeVisible({ timeout: 10000 });

    // Wait for content to load
    await page.waitForTimeout(2000);

    // Admin should see "Invite Member" button
    const inviteButton = page.getByTestId('invite-member-button');
    await expect(inviteButton).toBeVisible({ timeout: 5000 });
  });
});
