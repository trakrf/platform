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

    // Setup as admin
    await page.goto('/');
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

    // Click "Sign In" link
    await page.locator('a:has-text("Sign In")').click();

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

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then navigate to accept-invite
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Click "Create Account" link
    await page.locator('a:has-text("Create Account")').click();

    // Verify URL contains token
    const currentUrl = page.url();
    expect(currentUrl).toContain('returnTo=accept-invite');
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('after auth user returns to accept-invite with token intact', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-return-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin
    await page.goto('/');
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

    // Go through signup
    await page.locator('a:has-text("Create Account")').click();
    await page.locator('input#email').fill(inviteeEmail);
    await page.locator('input#orgName').fill(`Personal Org ${id}`);
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // Wait for redirect back to accept-invite
    await page.waitForURL(/accept-invite/, { timeout: 10000 });

    // Verify token is in URL
    const currentUrl = page.url();
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('token extraction works after redirect', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `redirect-extract-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin
    await page.goto('/');
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

    // Go through signup
    await page.locator('a:has-text("Create Account")').click();
    await page.locator('input#email').fill(inviteeEmail);
    await page.locator('input#orgName').fill(`Personal Org ${id}`);
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // Wait for redirect back to accept-invite
    await page.waitForURL(/accept-invite/, { timeout: 10000 });

    // Accept the invitation - this proves the token was extracted correctly
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Verify success - this confirms the token worked
    await expect(page.locator(`text=Welcome to ${testOrgName}!`)).toBeVisible({
      timeout: 10000,
    });
    await expect(page.locator('text=manager')).toBeVisible();
  });
});
