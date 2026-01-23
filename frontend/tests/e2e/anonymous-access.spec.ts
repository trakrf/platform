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

  test('should not redirect when tags are added to store while anonymous (TRA-305)', async ({
    page,
  }) => {
    // TRA-305: Tags scanned while anonymous should NOT trigger login redirect
    // The bug was: tagStore._queueForLookup() calls API which returns 401, triggering redirect
    // The fix: _flushLookupQueue() checks isAuthenticated before calling API

    // Monitor for 401 errors
    const errors401: string[] = [];
    page.on('response', (response) => {
      if (response.status() === 401) {
        errors401.push(response.url());
      }
    });

    // Navigate to inventory while anonymous
    await page.goto('/#inventory');
    await page.waitForTimeout(500);

    // Verify we're on inventory
    expect(page.url()).toContain('#inventory');

    // Simulate adding tags to the store (what happens when hardware scans tags)
    // This triggers the lookup queue which previously caused 401 -> redirect
    await page.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tagStore = stores?.tagStore;

      if (tagStore) {
        // Add multiple tags to trigger the lookup queue
        tagStore.getState().addTag({
          epc: 'TEST0000000000000001',
          rssi: -55,
          count: 1,
        });
        tagStore.getState().addTag({
          epc: 'TEST0000000000000002',
          rssi: -60,
          count: 1,
        });
        tagStore.getState().addTag({
          epc: 'TEST0000000000000003',
          rssi: -65,
          count: 1,
        });
      }
    });

    // Wait for the debounced lookup to potentially fire (500ms debounce + buffer)
    await page.waitForTimeout(1500);

    // Should STILL be on inventory, NOT redirected to login
    const finalUrl = page.url();
    expect(finalUrl).not.toContain('#login');
    expect(finalUrl).toContain('#inventory');

    // Verify no 401 errors occurred
    expect(errors401).toHaveLength(0);

    // Verify tags were actually added to the store
    const tagCount = await page.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return stores?.tagStore?.getState().tags.length ?? 0;
    });
    expect(tagCount).toBe(3);
  });
});
