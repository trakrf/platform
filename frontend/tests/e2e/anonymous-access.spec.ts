/**
 * Anonymous Access E2E Tests
 *
 * Verifies that Inventory, Locate, and Barcode screens are accessible
 * without logging in. Regression test for TRA-177.
 */

import { test, expect } from '@playwright/test';
import { clearAuthState } from './fixtures/org.fixture';

test.describe('Anonymous Access', () => {
  test.beforeEach(async ({ page }) => {
    // Ensure no auth state
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('should access inventory screen without login redirect', async ({ page }) => {
    await page.goto('/#inventory');

    // Wait for any potential redirects
    await page.waitForTimeout(1000);

    // Should NOT have been redirected to login
    const url = page.url();
    expect(url).not.toContain('#login');
    expect(url).toContain('#inventory');
  });

  test('should access locate screen without login redirect', async ({ page }) => {
    await page.goto('/#locate');

    // Wait for any potential redirects
    await page.waitForTimeout(1000);

    // Should NOT have been redirected to login
    const url = page.url();
    expect(url).not.toContain('#login');
    expect(url).toContain('#locate');

    // Locate screen has a testid we can verify
    await expect(page.locator('[data-testid="target-epc-display"]')).toBeVisible({ timeout: 5000 });
  });

  test('should access barcode screen without login redirect', async ({ page }) => {
    await page.goto('/#barcode');

    // Wait for any potential redirects
    await page.waitForTimeout(1000);

    // Should NOT have been redirected to login
    const url = page.url();
    expect(url).not.toContain('#login');
    expect(url).toContain('#barcode');
  });
});
