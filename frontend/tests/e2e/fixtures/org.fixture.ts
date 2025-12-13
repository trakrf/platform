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
 * Creates user and their org automatically
 */
export async function signupTestUser(
  page: Page,
  email: string,
  password: string,
  orgName?: string
): Promise<void> {
  await page.goto('/#signup');
  await page.locator('input#email').fill(email);
  // Organization name is required on signup
  await page.locator('input#orgName').fill(orgName || `Test Org ${uniqueId()}`);
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
 * Waits for the switcher to update to show the new org name
 */
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  // Click the org in the dropdown menu
  await page.locator(`button:has-text("${orgName}")`).click();
  // Wait for dropdown to close (menu items disappear)
  await page.waitForSelector('[role="menu"]', { state: 'hidden', timeout: 5000 });

  // Wait for UI to reflect the new org - the switcher button should show the org name
  await page.waitForSelector(`[data-testid="org-switcher"]:has-text("${orgName}")`, {
    timeout: 10000,
  });
}

// =============================================================================
// Multi-User Test Helpers (TRA-173)
// These helpers enable tests that require multiple users with different roles
// =============================================================================

/**
 * Get the auth token from localStorage
 * Token is stored in zustand's auth-storage persist key
 */
export async function getAuthToken(page: Page): Promise<string> {
  const token = await page.evaluate(() => {
    const authStorage = localStorage.getItem('auth-storage');
    if (!authStorage) return null;
    try {
      const { state } = JSON.parse(authStorage);
      return state?.token || null;
    } catch {
      return null;
    }
  });
  if (!token) {
    throw new Error('No auth token found in localStorage (auth-storage)');
  }
  return token;
}

/**
 * Get the base API URL for E2E tests
 * Always uses localhost:8080 since E2E tests require the backend to be running locally
 */
function getApiBaseUrl(_page: Page): string {
  // E2E tests always run against local backend
  return 'http://localhost:8080/api/v1';
}

/**
 * Get invitation token from test endpoint
 * Only works in non-production environments
 */
export async function getInviteToken(page: Page, inviteId: number): Promise<string> {
  // E2E tests always run against local backend
  const testEndpoint = `http://localhost:8080/test/invitations/${inviteId}/token`;

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
 * Create organization via API
 * Returns the created org with ID - avoids need to switch and query
 */
export async function createOrgViaAPI(
  page: Page,
  name: string
): Promise<{ id: number; name: string }> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.post(`${baseUrl}/orgs`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: { name },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to create org: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return { id: data.data.id, name: data.data.name };
}

/**
 * Switch to org via API (more reliable than UI for test setup)
 * Updates localStorage with new token
 */
export async function switchOrgViaAPI(page: Page, orgId: number): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const token = await getAuthToken(page);

  const response = await page.request.post(`${baseUrl}/users/me/current-org`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: { org_id: orgId },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to switch org: ${response.status()} - ${text}`);
  }

  // Update localStorage with new token
  const data = await response.json();
  await page.evaluate((newToken: string) => {
    const authStorage = localStorage.getItem('auth-storage');
    if (authStorage) {
      const parsed = JSON.parse(authStorage);
      parsed.state.token = newToken;
      localStorage.setItem('auth-storage', JSON.stringify(parsed));
    }
  }, data.token);
}

/**
 * Accept invitation via API
 * @param page Playwright page
 * @param inviteToken The invitation token
 * @param userAuthToken Optional auth token - if not provided, reads from localStorage
 */
export async function acceptInviteViaAPI(
  page: Page,
  inviteToken: string,
  userAuthToken?: string
): Promise<void> {
  const baseUrl = getApiBaseUrl(page);
  const authToken = userAuthToken || (await getAuthToken(page));

  const response = await page.request.post(`${baseUrl}/auth/accept-invite`, {
    headers: {
      Authorization: `Bearer ${authToken}`,
      'Content-Type': 'application/json',
    },
    data: { token: inviteToken },
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
  password: string,
  orgName: string
): Promise<string> {
  const baseUrl = getApiBaseUrl(page);

  const response = await page.request.post(`${baseUrl}/auth/signup`, {
    headers: {
      'Content-Type': 'application/json',
    },
    data: { email, password, org_name: orgName },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to signup via API: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return data.data.token;
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

  // Signup new user via API (creates their personal org)
  const memberOrgName = `Member Org ${id}`;
  const newUserToken = await signupViaAPI(page, email, password, memberOrgName);

  // Accept invitation as new user (pass token directly, no localStorage manipulation needed)
  await acceptInviteViaAPI(page, inviteToken, newUserToken);

  return { email, password };
}

/**
 * Navigate to members page for current org
 */
export async function goToMembersPage(page: Page): Promise<void> {
  await page.goto('/#org-members');
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
