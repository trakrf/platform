/**
 * Redirect Flow E2E Tests (TRA-174, TRA-203)
 *
 * Tests for token preservation through login/signup redirects.
 * Verifies that invitation tokens survive the auth flow.
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
} from './fixtures/org.fixture';

test.describe('Redirect Flow - Token Preservation', () => {
  // Shared state
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-redirect-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Redirect Test Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    await signupTestUser(page, adminEmail, adminPassword);

    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;
    await switchOrgViaAPI(page, testOrgId);

    await page.close();
  });

  test('login redirect preserves token in URL params', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-login-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Pre-signup invitee so user_exists=true triggers the login redirect path.
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Setup as admin
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'viewer');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then navigate to accept-invite
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // AcceptInviteScreen auto-redirects to #login (existing user path) with
    // token+returnTo preserved in URL hash params.
    await page.waitForURL(/#login.*returnTo=accept-invite.*token=/, { timeout: 10000 });

    // Verify URL contains token
    const currentUrl = page.url();
    expect(currentUrl).toContain('returnTo=accept-invite');
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('signup redirect preserves token in URL params', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-signup-${id}@example.com`;

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation - invitee does not exist, triggers signup redirect path.
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then navigate to accept-invite
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // AcceptInviteScreen auto-redirects to #signup (new user path) with
    // token+returnTo preserved in URL hash params.
    await page.waitForURL(/#signup.*returnTo=accept-invite.*token=/, { timeout: 10000 });

    // Verify URL contains token
    const currentUrl = page.url();
    expect(currentUrl).toContain('returnTo=accept-invite');
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('after auth user returns to accept-invite with token intact', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-return-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Pre-signup invitee so post-auth-redirect path goes through #login,
    // which handleAuthRedirect loops back to accept-invite (signup auto-accepts
    // and lands on #home, bypassing accept-invite — not this test's subject).
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Setup as admin
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'viewer');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then start flow
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Auto-redirect lands on #login with email pre-filled from URL param
    await page.waitForURL(/#login.*returnTo=accept-invite/, { timeout: 10000 });

    // Complete login (email already populated from URL)
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // handleAuthRedirect returns to accept-invite with token preserved
    await page.waitForURL(/#accept-invite.*token=/, { timeout: 10000 });

    // Verify token is in URL
    const currentUrl = page.url();
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('token extraction works after redirect', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-extract-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Pre-signup invitee to force login-path redirect, which returns to
    // accept-invite and lets us click accept to prove token extraction works.
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Setup as admin
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'manager');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then start flow
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Auto-redirect to login, complete login
    await page.waitForURL(/#login.*returnTo=accept-invite/, { timeout: 10000 });
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // handleAuthRedirect returns to accept-invite with token
    await page.waitForURL(/#accept-invite.*token=/, { timeout: 10000 });

    // Accept the invitation - this proves the token was extracted correctly
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Verify success - this confirms the token worked
    await expect(page.locator(`text=Welcome to ${testOrgName}!`)).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator('text=manager')).toBeVisible();
  });
});
