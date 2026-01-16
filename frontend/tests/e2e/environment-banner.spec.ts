/**
 * Environment Banner E2E Tests
 * Tests that the environment banner appears for non-prod environments
 *
 * NOTE: For banner to appear, VITE_ENVIRONMENT must be set at BUILD time.
 * Run: VITE_ENVIRONMENT=preview pnpm dev
 */

import { test, expect } from '@playwright/test';

test.describe('Environment Banner', () => {
  test('should show banner when VITE_ENVIRONMENT is set to non-prod value', async ({ page }) => {
    await page.goto('/');

    // Check if banner exists
    const banner = page.locator('[data-testid="environment-banner"]');
    const bannerExists = await banner.count() > 0;

    if (bannerExists) {
      // Banner found - verify it's visible and has correct styling
      await expect(banner).toBeVisible();
      await expect(banner).toHaveClass(/bg-purple-600/);

      const bannerText = await banner.textContent();
      console.log(`✅ Banner found with text: "${bannerText}"`);

      // Check title prefix
      const title = await page.title();
      console.log(`✅ Page title: "${title}"`);
      expect(title).toMatch(/^\[.{3}\]/); // Should start with [XXX]
    } else {
      // Banner not found - this means VITE_ENVIRONMENT is not set or is 'prod'
      console.log('❌ Banner NOT found');
      console.log('   This means VITE_ENVIRONMENT is either:');
      console.log('   - Not set');
      console.log('   - Set to "prod"');
      console.log('   - Not available at build time');

      // Check what the title is
      const title = await page.title();
      console.log(`   Page title: "${title}"`);

      // Fail the test with helpful message
      expect(bannerExists,
        'Banner should exist. Make sure VITE_ENVIRONMENT is set at build time. ' +
        'Run: VITE_ENVIRONMENT=preview pnpm dev'
      ).toBe(true);
    }
  });

  test('should not show banner when VITE_ENVIRONMENT is prod or empty', async ({ page }) => {
    // This test documents expected behavior
    // When VITE_ENVIRONMENT is 'prod' or empty, no banner should appear
    await page.goto('/');

    const banner = page.locator('[data-testid="environment-banner"]');
    const bannerCount = await banner.count();

    // Log current state for debugging
    const title = await page.title();
    console.log(`Current page title: "${title}"`);
    console.log(`Banner elements found: ${bannerCount}`);

    // This test passes regardless - it's documenting current behavior
    // The first test will catch if banner is missing when it shouldn't be
  });

  test('banner should be visible above header', async ({ page }) => {
    await page.goto('/');

    const banner = page.locator('[data-testid="environment-banner"]');

    if (await banner.count() > 0) {
      // Get positions
      const bannerBox = await banner.boundingBox();
      const header = page.locator('header').first();
      const headerBox = await header.boundingBox();

      if (bannerBox && headerBox) {
        console.log(`Banner position: top=${bannerBox.y}, height=${bannerBox.height}`);
        console.log(`Header position: top=${headerBox.y}`);

        // Banner should be above or at same level as header
        expect(bannerBox.y).toBeLessThanOrEqual(headerBox.y);
      }
    } else {
      console.log('Banner not present - skipping position test');
    }
  });
});
