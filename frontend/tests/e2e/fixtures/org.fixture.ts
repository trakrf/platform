/**
 * Organization Test Fixtures
 *
 * Reusable helpers for org-related E2E tests.
 * Establishes patterns for TRA-172 and subsequent org test issues (TRA-173, 174, 175).
 */

import type { Page } from '@playwright/test';

/** Role types matching backend */
export type OrgRole = 'owner' | 'admin' | 'manager' | 'operator' | 'viewer';

/** Credentials for test user */
export interface TestUserCredentials {
  email: string;
  password: string;
}

/**
 * Generate unique test identifier
 * Uses timestamp + random suffix to avoid collisions between test runs
 */
export function uniqueId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}

/**
 * Clear auth state from browser storage
 * Pattern from auth.spec.ts - ensures clean state before tests
 */
export async function clearAuthState(page: Page): Promise<void> {
  await page.evaluate(() => {
    localStorage.clear();
    sessionStorage.clear();
  });
}

/**
 * Sign up a new test user
 * Creates user and their personal org automatically (backend creates personal org on signup)
 */
export async function signupTestUser(
  page: Page,
  email: string,
  password: string
): Promise<void> {
  await page.goto('/#signup');
  await page.locator('input#email').fill(email);
  await page.locator('input#password').fill(password);
  await page.locator('button[type="submit"]').click();
  // Wait for redirect to home after successful signup
  await page.waitForURL(/#home/, { timeout: 10000 });
}

/**
 * Log in an existing test user
 */
export async function loginTestUser(
  page: Page,
  email: string,
  password: string
): Promise<void> {
  await page.goto('/#login');
  await page.locator('input#email').fill(email);
  await page.locator('input#password').fill(password);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/#home/, { timeout: 10000 });
  // Wait for profile to load (org-switcher appears when profile is fetched)
  await page.waitForSelector('[data-testid="org-switcher"]', { timeout: 10000 });
}

/**
 * Create org via UI (for testing the create flow)
 */
export async function createOrgViaUI(page: Page, name: string): Promise<void> {
  await page.goto('/#create-org');
  await page.locator('input#name').fill(name);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL(/#home/, { timeout: 10000 });
}

/**
 * Navigate to org settings page
 */
export async function goToOrgSettings(page: Page): Promise<void> {
  await page.goto('/#org-settings');
}

/**
 * Open the org switcher dropdown
 * Uses data-testid for reliable selection
 */
export async function openOrgSwitcher(page: Page): Promise<void> {
  await page.locator('[data-testid="org-switcher"]').click();
}

/**
 * Switch to a specific org via the org switcher dropdown
 */
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  await page.getByRole('menuitem', { name: orgName }).click();
  // Wait for the org switcher to update with the new org name
  await page.waitForSelector(`[data-testid="org-switcher"]:has-text("${orgName}")`, { timeout: 5000 });
}

// =============================================================================
// Multi-User Test Helpers (TRA-173)
// These helpers enable tests that require multiple users with different roles
// =============================================================================

/**
 * Get the auth token from localStorage
 */
export async function getAuthToken(page: Page): Promise<string> {
  const token = await page.evaluate(() => localStorage.getItem('token'));
  if (!token) {
    throw new Error('No auth token found in localStorage');
  }
  return token;
}

/**
 * Get the base API URL from the page
 */
function getApiBaseUrl(page: Page): string {
  const url = new URL(page.url());
  return `${url.protocol}//${url.host}/api/v1`;
}

/**
 * Get invitation token from test endpoint
 * Only works in non-production environments
 */
export async function getInviteToken(page: Page, inviteId: number): Promise<string> {
  const url = new URL(page.url());
  const testEndpoint = `${url.protocol}//${url.host}/test/invitations/${inviteId}/token`;

  const response = await page.request.get(testEndpoint);
  if (!response.ok()) {
    throw new Error(`Failed to get invite token: ${response.status()}`);
  }

  const data = await response.json();
  return data.token;
}

/**
 * Create invitation via API
 * Returns the invitation ID
 */
export async function createInviteViaAPI(
  page: Page,
  orgId: number,
  email: string,
  role: OrgRole
): Promise<number> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.post(`${baseUrl}/orgs/${orgId}/invitations`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: { email, role },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to create invitation: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return data.data.id;
}

/**
 * Accept invitation via API
 * Must be called when logged in as the invited user
 */
export async function acceptInviteViaAPI(page: Page, token: string): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const authToken = await getAuthToken(page);

  const response = await page.request.post(`${baseUrl}/auth/accept-invite`, {
    headers: {
      Authorization: `Bearer ${authToken}`,
      'Content-Type': 'application/json',
    },
    data: { token },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to accept invitation: ${response.status()} - ${text}`);
  }
}

/**
 * Sign up a new test user via API (faster than UI)
 * Returns auth token
 */
async function signupViaAPI(
  page: Page,
  email: string,
  password: string
): Promise<string> {
  const baseUrl = getApiBaseUrl(page);

  const response = await page.request.post(`${baseUrl}/auth/signup`, {
    headers: {
      'Content-Type': 'application/json',
    },
    data: { email, password },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to signup via API: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return data.token;
}

/**
 * Add a test member to an org
 *
 * Full flow:
 * 1. Generate unique email
 * 2. Create invitation via API
 * 3. Get token from test endpoint
 * 4. Create new user account via API signup
 * 5. Accept invitation via API
 *
 * Returns credentials for the new member
 */
export async function addTestMemberToOrg(
  page: Page,
  orgId: number,
  role: OrgRole
): Promise<TestUserCredentials> {
  const id = uniqueId();
  const email = `test-member-${id}@example.com`;
  const password = 'TestPassword123!';

  // Create invitation as current user (admin)
  const inviteId = await createInviteViaAPI(page, orgId, email, role);

  // Get the token from test endpoint
  const inviteToken = await getInviteToken(page, inviteId);

  // Save current auth state
  const currentToken = await getAuthToken(page);

  // Signup new user via API
  const newUserToken = await signupViaAPI(page, email, password);

  // Set new user's token temporarily
  await page.evaluate((t) => localStorage.setItem('token', t), newUserToken);

  // Accept invitation as new user
  await acceptInviteViaAPI(page, inviteToken);

  // Restore original user's token
  await page.evaluate((t) => localStorage.setItem('token', t), currentToken);

  return { email, password };
}

/**
 * Navigate to members page for current org
 */
export async function goToMembersPage(page: Page): Promise<void> {
  await page.goto('/#members');
  // Wait for members table to be visible
  await page.waitForSelector('th:has-text("Name")', { timeout: 10000 });
}

/**
 * Get org ID from the current user's profile
 * Returns the current org ID
 */
export async function getCurrentOrgId(page: Page): Promise<number> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.get(`${baseUrl}/users/me`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok()) {
    throw new Error(`Failed to get profile: ${response.status()}`);
  }

  const data = await response.json();
  if (!data.data.current_org) {
    throw new Error('No current org set');
  }
  return data.data.current_org.id;
}
