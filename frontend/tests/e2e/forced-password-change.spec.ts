/**
 * Forced password change E2E
 *
 * TRA-1135: an account provisioned with an operator-chosen bootstrap password
 * must not be able to use the app until the user sets their own.
 *
 * The flag is driven here by rewriting the /users/me response rather than by
 * setting it server-side, because setting it server-side needs a superadmin
 * session and this suite has no superadmin fixture (the same gap kits.spec.ts
 * records). That the server stores, clears and serves the flag is covered by
 * the Go integration tests; what only a browser can show is the part below —
 * that the gate replaces the whole app, that nothing routes around it, and that
 * a real change-password call takes it down.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *   (or PLAYWRIGHT_BASE_URL pointed at preview)
 *
 * Run with: pnpm test:e2e tests/e2e/forced-password-change.spec.ts
 */

import { test, expect, type Page } from '@playwright/test';
import { clearAuthState, uniqueId, signupTestUser } from './fixtures/org.fixture';

const BOOTSTRAP_PASSWORD = 'Changeme!1';
const CHOSEN_PASSWORD = 'chosen-pass-1';

/**
 * Rewrite /users/me so it reports the account as still holding its bootstrap
 * password. Stands in for the superadmin toggle.
 */
async function forceFlagOnProfile(page: Page): Promise<void> {
  await page.route('**/api/v1/users/me', async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    if (body?.data) {
      body.data.must_change_password = true;
    }
    await route.fulfill({ response, json: body });
  });
}

test.describe('Forced password change', () => {
  test('gates the app until the user sets their own password', async ({ page }) => {
    const email = `forced-${uniqueId()}@example.com`;

    // Sign up normally first: this is an ordinary, unflagged account, which is
    // what makes the assertions below about the flag rather than about signup.
    await page.goto('/');
    await clearAuthState(page);
    await signupTestUser(page, email, BOOTSTRAP_PASSWORD);
    await expect(page.getByTestId('org-switcher')).toBeVisible();

    // Now come back as a flagged account.
    await clearAuthState(page);
    await forceFlagOnProfile(page);

    await page.goto('/#login');
    await page.locator('input#email').fill(email);
    await page.locator('input#password').fill(BOOTSTRAP_PASSWORD);
    await page.locator('button[type="submit"]').click();

    // The gate is the whole page: no sidebar, no header account menu, and no
    // way to dismiss it.
    await expect(page.getByRole('heading', { name: /set your password/i })).toBeVisible({
      timeout: 15000,
    });
    await expect(page.getByTestId('desktop-sidebar')).toHaveCount(0);
    await expect(page.getByTestId('org-switcher')).toHaveCount(0);
    await expect(page.getByRole('button', { name: /skip|later|cancel|dismiss/i })).toHaveCount(0);

    // Navigating elsewhere by hash must not get past it either.
    await page.goto('/#assets');
    await expect(page.getByRole('heading', { name: /set your password/i })).toBeVisible();
    await expect(page.getByTestId('desktop-sidebar')).toHaveCount(0);

    // Re-using the bootstrap password is refused client-side — the server would
    // accept it and clear the flag, leaving the account exactly where it started.
    await page.locator('input#forced-current-password').fill(BOOTSTRAP_PASSWORD);
    await page.locator('input#forced-new-password').fill(BOOTSTRAP_PASSWORD);
    await page.locator('input#forced-confirm-password').fill(BOOTSTRAP_PASSWORD);
    await page.getByTestId('forced-password-submit').click();
    await expect(page.getByRole('alert')).toContainText(/different from your current password/i);

    // A real rotation takes the gate down.
    await page.locator('input#forced-current-password').fill(BOOTSTRAP_PASSWORD);
    await page.locator('input#forced-new-password').fill(CHOSEN_PASSWORD);
    await page.locator('input#forced-confirm-password').fill(CHOSEN_PASSWORD);
    await page.getByTestId('forced-password-submit').click();

    await expect(page.getByTestId('desktop-sidebar')).toBeVisible({ timeout: 15000 });
    await expect(page.getByRole('heading', { name: /set your password/i })).toHaveCount(0);

    // And the new password is the one that works: signing back in with the
    // server's own (unrewritten) profile goes straight into the app.
    await page.unroute('**/api/v1/users/me');
    await clearAuthState(page);
    await page.goto('/#login');
    await page.locator('input#email').fill(email);
    await page.locator('input#password').fill(CHOSEN_PASSWORD);
    await page.locator('button[type="submit"]').click();

    await expect(page.getByTestId('org-switcher')).toBeVisible({ timeout: 15000 });
    await expect(page.getByRole('heading', { name: /set your password/i })).toHaveCount(0);
  });

  test('leaves an unflagged account alone', async ({ page }) => {
    const email = `unflagged-${uniqueId()}@example.com`;

    await page.goto('/');
    await clearAuthState(page);
    await signupTestUser(page, email, BOOTSTRAP_PASSWORD);

    await expect(page.getByTestId('org-switcher')).toBeVisible();
    await expect(page.getByRole('heading', { name: /set your password/i })).toHaveCount(0);
  });
});
