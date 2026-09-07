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
    // RENDERED TEXT, not `page.content()`. This asserted over the whole HTML
    // document, which under `dev:bridge` includes the ble-mcp-test mock inlined
    // as a <script> — and that source contains three literal `throw new
    // TypeError(...)` statements which never execute. So the bare `TypeError`
    // grep matched the instrument rather than the page, and these two tests
    // failed on every bridge-mode run while the crash they exist to catch was
    // absent (the `Cannot read properties of null` limb above passed the whole
    // time). Verified 2026-09-07: the served page carries 3 `throw new
    // TypeError` and 0 `Cannot read properties of null`. TRA-1253.
    //
    // `innerText` is the surface the bug actually shows on — a React tree that
    // threw renders the error text — and it excludes script bodies by
    // construction, so the assertion can no longer match its own harness.
    const renderedText = await page.locator('body').innerText();
    expect(renderedText).not.toContain('Cannot read properties of null');
    expect(renderedText).not.toContain('TypeError');

    // Should see either:
    // - The members list (if members exist)
    // - OR "No members" empty state
    // - OR loading state that eventually resolves
    const membersList = page.getByTestId('members-list');
    const emptyState = page.getByText(/no members|add your first member/i);
    const loadingState = page.getByText(/loading/i);

    // Wait for loading to complete
    await expect(loadingState).not.toBeVisible({ timeout: 10000 }).catch(() => {
      // Loading might have already finished, that's ok
    });

    /*
     * Retry this, rather than sampling both locators once.
     *
     * `isVisible()` is a point-in-time read with no retry, and the heading this
     * test waits for above renders before the members query resolves. So the
     * table could still be in flight when both samples were taken, which made
     * the assertion fail against a page that rendered the list correctly a beat
     * later — the failure screenshot showed the populated table. Found while
     * auditing specs adjacent to TRA-1246; it is not one of the nine, and it
     * fails only sometimes, which is exactly why it had never been chased.
     */
    await expect(membersList.or(emptyState).first()).toBeVisible({ timeout: 10000 });
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
    // RENDERED TEXT, not `page.content()`. This asserted over the whole HTML
    // document, which under `dev:bridge` includes the ble-mcp-test mock inlined
    // as a <script> — and that source contains three literal `throw new
    // TypeError(...)` statements which never execute. So the bare `TypeError`
    // grep matched the instrument rather than the page, and these two tests
    // failed on every bridge-mode run while the crash they exist to catch was
    // absent (the `Cannot read properties of null` limb above passed the whole
    // time). Verified 2026-09-07: the served page carries 3 `throw new
    // TypeError` and 0 `Cannot read properties of null`. TRA-1253.
    //
    // `innerText` is the surface the bug actually shows on — a React tree that
    // threw renders the error text — and it excludes script bodies by
    // construction, so the assertion can no longer match its own harness.
    const renderedText = await page.locator('body').innerText();
    expect(renderedText).not.toContain('Cannot read properties of null');
    expect(renderedText).not.toContain('TypeError');

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
