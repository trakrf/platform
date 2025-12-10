/**
 * Organization Test Fixtures
 *
 * Reusable helpers for org-related E2E tests.
 * Establishes patterns for TRA-172 and subsequent org test issues (TRA-173, 174, 175).
 */

import type { Page } from '@playwright/test';

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
 * Sign up a new test user with organization
 * Creates user and their personal org in one flow
 */
export async function signupTestUser(
  page: Page,
  email: string,
  password: string,
  orgName: string
): Promise<void> {
  await page.goto('/#signup');
  await page.locator('input#email').fill(email);
  await page.locator('input#password').fill(password);
  await page.locator('input#organizationName').fill(orgName);
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
