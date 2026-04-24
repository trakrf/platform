/**
 * Accept Invitation E2E Tests (TRA-174)
 *
 * Tests for accepting organization invitations - both existing and new users.
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
  createInviteViaAPI,
  getInviteToken,
  listOrgsViaAPI,
} from './fixtures/org.fixture';

test.describe('Accept Invitation - Existing User', () => {
  // Shared state - created once in beforeAll
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    // Signup admin user and create a team org
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-accept-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `Accept Test Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    // Signup creates personal org automatically
    await signupTestUser(page, adminEmail, adminPassword);

    // Create team org via API
    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;

    // Switch to the team org via API
    await switchOrgViaAPI(page, testOrgId);

    await page.close();
  });

  test('logged-in user can accept invitation via token URL', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `invitee-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup: Login as admin, create invitation
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Create invitee account
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);

    // Navigate to accept-invite URL with token
    await page.goto(`/#accept-invite?token=${token}`);

    // Click accept button
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Should show success
    await expect(page.locator(`text=Welcome to ${testOrgName}!`)).toBeVisible({
      timeout: 10000,
    });
  });

  test('user sees success message with org name', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `invitee-success-${id}@example.com`;
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

    // Create invitee and accept
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Verify success message includes org name
    await expect(page.locator(`text=Welcome to ${testOrgName}!`)).toBeVisible({
      timeout: 10000,
    });
    // Verify role is shown
    await expect(page.locator('text=viewer')).toBeVisible();
  });

  test('user can navigate to dashboard after accept', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `invitee-dash-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Create invitee and accept
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Wait for success screen
    await expect(page.locator(`text=Welcome to ${testOrgName}!`)).toBeVisible({
      timeout: 10000,
    });

    // Click "Go to Dashboard"
    await page.locator('a:has-text("Go to Dashboard")').click();

    // Should redirect to home
    await page.waitForURL(/#home/, { timeout: 10000 });
  });

  test('user added to org with correct role', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `invitee-role-${id}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation with specific role
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'manager');
    const token = await getInviteToken(page, inviteId);

    // Create invitee and accept
    await clearAuthState(page);
    await signupTestUser(page, inviteeEmail, inviteePassword);
    await page.goto(`/#accept-invite?token=${token}`);
    await page.locator('[data-testid="accept-invite-button"]').click();

    // Verify role in success message
    await expect(page.locator('text=manager')).toBeVisible({ timeout: 10000 });
  });
});

test.describe('Accept Invitation - New User', () => {
  // Shared state
  let adminEmail: string;
  let adminPassword: string;
  let testOrgId: number;
  let testOrgName: string;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    adminEmail = `test-newuser-admin-${id}@example.com`;
    adminPassword = 'TestPassword123!';
    testOrgName = `New User Invite Org ${id}`;

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    await signupTestUser(page, adminEmail, adminPassword);

    const newOrg = await createOrgViaAPI(page, testOrgName);
    testOrgId = newOrg.id;
    await switchOrgViaAPI(page, testOrgId);

    await page.close();
  });

  test('non-logged-in new user is auto-redirected to signup with token', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `new-invitee-${id}@example.com`;

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation - invitee does not exist, triggers signup redirect.
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'viewer');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then navigate (not logged in)
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // AcceptInviteScreen auto-redirects unauthenticated users based on
    // user existence; new users land on #signup with token+returnTo preserved.
    await page.waitForURL(/#signup.*returnTo=accept-invite.*token=/, { timeout: 10000 });

    const currentUrl = page.url();
    expect(currentUrl).toContain('returnTo=accept-invite');
    expect(currentUrl).toContain(`token=${encodeURIComponent(token)}`);
  });

  test('signup redirect preserves token', async ({ page }) => {
    const id = uniqueId();
    const inviteeEmail = `new-signup-${id}@example.com`;

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then navigate
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Auto-redirect to signup preserves token+returnTo in URL
    await expect(page).toHaveURL(/#signup/);
    await expect(page).toHaveURL(/token=/);
    await expect(page).toHaveURL(/returnTo=accept-invite/);
  });

  test('new user invite-signup auto-accepts and lands on home', async ({ page }) => {
    const inviteeEmail = `new-return-${uniqueId()}@example.com`;
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

    // Clear auth, reload to reset Zustand store, then navigate
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Auto-redirect to signup
    await page.waitForURL(/#signup.*token=/, { timeout: 10000 });

    // Invite-flow signup: email is pre-filled + disabled, no orgName input.
    // Fill password only, submit.
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // SignupScreen.tsx calls signup(email, password, undefined, inviteToken)
    // which accepts the invitation server-side and redirects to #home.
    await page.waitForURL(/#home/, { timeout: 10000 });
  });

  test('new user can complete full invite flow', async ({ page }) => {
    const inviteeEmail = `new-full-${uniqueId()}@example.com`;
    const inviteePassword = 'TestPassword123!';

    // Setup as admin
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, adminEmail, adminPassword);
    await switchOrgViaAPI(page, testOrgId);

    // Create invitation
    const inviteId = await createInviteViaAPI(page, testOrgId, inviteeEmail, 'operator');
    const token = await getInviteToken(page, inviteId);

    // Clear auth, reload to reset Zustand store, then start invite flow
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await page.goto(`/#accept-invite?token=${token}`);

    // Auto-redirect to signup
    await page.waitForURL(/#signup.*token=/, { timeout: 10000 });

    // Invite-flow signup: email pre-filled + disabled, no orgName input.
    await page.locator('input#password').fill(inviteePassword);
    await page.locator('button[type="submit"]').click();

    // Signup auto-accepts invitation and redirects to home
    await page.waitForURL(/#home/, { timeout: 10000 });

    // Verify membership - user is now in testOrg (confirms the invitation was accepted)
    const orgs = await listOrgsViaAPI(page);
    expect(orgs.some((o) => o.id === testOrgId)).toBe(true);
  });
});
