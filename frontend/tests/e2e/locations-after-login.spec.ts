/**
 * Locations After Login E2E Tests
 *
 * Tests the specific bug: locations not showing after logout → login → navigate to Locations
 *
 * Root cause: Login API returns token without org_id claim, causing org-scoped
 * data to not load properly until setCurrentOrg is called.
 *
 * TRA-318: Centralize org-scoped data invalidation
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 */

import { test, expect, type Page } from '@playwright/test';
import {
  clearAuthState,
  uniqueId,
  signupTestUser,
  loginTestUser,
  getAuthToken,
} from './fixtures/org.fixture';

const API_BASE = 'http://localhost:8080/api/v1';

interface TestLocation {
  id: number;
  name: string;
}

/**
 * Create a test location via API
 */
async function createTestLocation(page: Page, name: string): Promise<TestLocation> {
  const token = await getAuthToken(page);

  const response = await page.request.post(`${API_BASE}/locations`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: {
      name,
      identifier: `LOC-${uniqueId()}`,
      is_active: true,
      valid_from: new Date().toISOString().split('T')[0],
    },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to create location: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return {
    id: data.data.id,
    name: data.data.name,
  };
}

/**
 * Navigate to locations tab and wait for it to load
 */
async function navigateToLocations(page: Page): Promise<void> {
  // Click on Locations in the menu
  const locationsButton = page.locator('button[data-testid="menu-item-locations"]');
  await locationsButton.click();

  // Wait for URL to change
  await page.waitForURL(/#locations/, { timeout: 5000 });

  // Wait for the locations page content to appear
  await page.waitForTimeout(1000);
}

/**
 * Get the count of locations displayed on the page
 * Uses the stats display which shows "Total Locations: X"
 */
async function getDisplayedLocationCount(page: Page): Promise<number> {
  // Wait a bit for data to load
  await page.waitForTimeout(1000);

  // Check the "Total Locations" stat on the page
  const totalLocationsText = await page.locator('text="Total Locations"').locator('..').locator('..').textContent();

  // Extract the number from the text (e.g., "Total Locations3All registered" -> 3)
  const match = totalLocationsText?.match(/(\d+)/);
  return match ? parseInt(match[1], 10) : 0;
}

/**
 * Perform logout via UI
 */
async function logoutViaUI(page: Page): Promise<void> {
  // Click org switcher menu button
  const orgSwitcher = page.locator('[data-testid="org-switcher"]');
  await orgSwitcher.click();

  // Wait for menu to open
  await page.waitForTimeout(300);

  // Click logout button
  const logoutButton = page.locator('button:has-text("Logout")');
  await logoutButton.click();

  // Wait for logged-out state - "Log In" button appears in header
  await page.waitForSelector('button:has-text("Log In")', { timeout: 5000 });
}

test.describe('Locations After Login (TRA-318)', () => {
  // Force serial execution - tests depend on each other
  test.describe.configure({ mode: 'serial' });

  // Increase timeout for this suite (lots of navigation and API calls)
  test.setTimeout(60000);

  const testId = uniqueId();
  const testEmail = `test-locations-${testId}@example.com`;
  const testPassword = 'TestPassword123!';
  const testOrgName = `Locations Test Org ${testId}`;

  const testLocations: TestLocation[] = [];
  let sharedPage: Page;

  test.beforeAll(async ({ browser }) => {
    console.log('[LocationsAfterLogin] Setting up test user and locations...');
    sharedPage = await browser.newPage();

    // Create test user and org
    await signupTestUser(sharedPage, testEmail, testPassword, testOrgName);

    // Create test locations
    for (let i = 1; i <= 3; i++) {
      const location = await createTestLocation(sharedPage, `Test Location ${i} - ${testId}`);
      testLocations.push(location);
      console.log(`[LocationsAfterLogin] Created location: ${location.name} (ID: ${location.id})`);
    }

    console.log(`[LocationsAfterLogin] Setup complete: ${testLocations.length} locations created`);
  });

  test.afterAll(async () => {
    if (sharedPage) {
      await sharedPage.close();
    }
  });

  test('1. initial login shows locations', async () => {
    // We're already logged in from beforeAll, just navigate to locations
    await navigateToLocations(sharedPage);

    // Verify locations are displayed
    const count = await getDisplayedLocationCount(sharedPage);
    console.log(`[Test 1] Location count after initial login: ${count}`);

    expect(count).toBeGreaterThanOrEqual(testLocations.length);

    // Verify specific location names are visible (use .first() since name may appear in multiple places)
    for (const location of testLocations) {
      const locationElement = sharedPage.locator(`text="${location.name}"`).first();
      await expect(locationElement).toBeVisible({ timeout: 5000 });
    }

    console.log('[Test 1] PASS: Locations visible after initial login');
  });

  test('2. logout clears auth state', async () => {
    // Perform logout
    await logoutViaUI(sharedPage);

    // Verify logged-out state - "Log In" button visible
    await expect(sharedPage.locator('button:has-text("Log In")')).toBeVisible();

    // Verify auth state is cleared
    const isAuthenticated = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return stores?.authStore?.getState().isAuthenticated ?? false;
    });
    expect(isAuthenticated).toBe(false);

    console.log('[Test 2] PASS: Logged out successfully');
  });

  test('3. login again and verify locations still show (THE BUG TEST)', async () => {
    // Login again with same credentials
    console.log('[Test 3] Logging in again...');
    await loginTestUser(sharedPage, testEmail, testPassword);

    // Navigate to locations
    console.log('[Test 3] Navigating to locations...');
    await navigateToLocations(sharedPage);

    // THIS IS THE KEY TEST: Locations should be visible after re-login
    // The bug was: locations would NOT show because org-scoped data wasn't properly invalidated
    const count = await getDisplayedLocationCount(sharedPage);
    console.log(`[Test 3] Location count after re-login: ${count}`);

    // Should have at least our test locations
    expect(count).toBeGreaterThanOrEqual(testLocations.length);

    // Verify specific location names are visible (use .first() since name may appear in multiple places)
    for (const location of testLocations) {
      const locationElement = sharedPage.locator(`text="${location.name}"`).first();
      await expect(locationElement).toBeVisible({ timeout: 5000 });
      console.log(`[Test 3] Found location: ${location.name}`);
    }

    console.log('[Test 3] PASS: Locations visible after logout → login → navigate');
  });

  test('4. verify cache was properly invalidated', async () => {
    // Check that the locations are visible on the page
    const count = await getDisplayedLocationCount(sharedPage);
    console.log(`[Test 4] Location count from UI: ${count}`);

    expect(count).toBeGreaterThanOrEqual(testLocations.length);

    // Verify specific location is still visible (not stale data)
    const firstLocation = testLocations[0];
    const locationElement = sharedPage.locator(`text="${firstLocation.name}"`).first();
    await expect(locationElement).toBeVisible({ timeout: 5000 });

    console.log('[Test 4] PASS: Cache properly invalidated and fresh data loaded');
  });

  test('5. repeat logout/login cycle to ensure consistency', async () => {
    // Logout again
    await logoutViaUI(sharedPage);

    // Login again
    await loginTestUser(sharedPage, testEmail, testPassword);

    // Navigate to locations
    await navigateToLocations(sharedPage);

    // Verify locations are still there
    const count = await getDisplayedLocationCount(sharedPage);
    console.log(`[Test 5] Location count after second re-login: ${count}`);

    expect(count).toBeGreaterThanOrEqual(testLocations.length);

    // Verify at least one location is visible (use .first() since name may appear in multiple places)
    const firstLocation = testLocations[0];
    const locationElement = sharedPage.locator(`text="${firstLocation.name}"`).first();
    await expect(locationElement).toBeVisible({ timeout: 5000 });

    console.log('[Test 5] PASS: Locations still visible after multiple logout/login cycles');
  });
});

test.describe('Locations After Fresh Browser Session (TRA-318)', () => {
  const testId = uniqueId();
  const testEmail = `test-fresh-${testId}@example.com`;
  const testPassword = 'TestPassword123!';
  const testOrgName = `Fresh Session Org ${testId}`;

  test('login from fresh session loads locations correctly', async ({ page }) => {
    // Navigate first, then clear state (can't access localStorage on about:blank)
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });

    // Signup new user
    await signupTestUser(page, testEmail, testPassword, testOrgName);

    // Create a location
    const token = await getAuthToken(page);
    const response = await page.request.post(`${API_BASE}/locations`, {
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      data: {
        name: `Fresh Location ${testId}`,
        identifier: `LOC-FRESH-${testId}`,
        is_active: true,
        valid_from: new Date().toISOString().split('T')[0],
      },
    });
    expect(response.ok()).toBe(true);

    // Navigate to locations - should show the location
    await page.locator('button[data-testid="menu-item-locations"]').click();
    await page.waitForURL(/#locations/, { timeout: 5000 });
    await page.waitForTimeout(1000);

    // Verify location is visible (use .first() since it may appear in table and stats)
    const locationElement = page.locator(`text="Fresh Location ${testId}"`).first();
    await expect(locationElement).toBeVisible({ timeout: 5000 });

    // Now clear state and login fresh (simulate closing browser and reopening)
    await clearAuthState(page);
    await page.goto('/#login');
    await page.waitForTimeout(500);

    // Login directly (don't use loginTestUser since redirect behavior may vary)
    await page.locator('input#email').fill(testEmail);
    await page.locator('input#password').fill(testPassword);
    await page.locator('button[type="submit"]').click();

    // Wait for login to complete (could redirect to home or locations)
    await page.waitForTimeout(2000);

    // Navigate to locations
    await page.locator('button[data-testid="menu-item-locations"]').click();
    await page.waitForURL(/#locations/, { timeout: 5000 });
    await page.waitForTimeout(1000);

    // Location should still be visible (use .first() since it may appear in table and stats)
    const locationAfterFreshLogin = page.locator(`text="Fresh Location ${testId}"`).first();
    await expect(locationAfterFreshLogin).toBeVisible({ timeout: 5000 });

    console.log('[Fresh Session] PASS: Location visible after fresh login');
  });
});
