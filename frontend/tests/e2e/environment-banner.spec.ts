/**
 * Environment Banner E2E Tests
 *
 * The banner is runtime-driven (TRA-853): the backend injects
 * window.__APP_CONFIG__ = { environmentLabel } into index.html at serve time.
 * In E2E we simulate that injection with page.addInitScript, which runs before
 * any app code — exactly like the backend's inline <script>.
 */

import { test, expect } from '@playwright/test';

function withEnvironmentLabel(label: string) {
  return async ({ page }: { page: import('@playwright/test').Page }) => {
    await page.addInitScript((value) => {
      (window as unknown as { __APP_CONFIG__: { environmentLabel: string } }).__APP_CONFIG__ = {
        environmentLabel: value,
      };
    }, label);
  };
}

test.describe('Environment Banner', () => {
  test('shows a purple banner with title prefix for a non-prod label', async ({ page }) => {
    await withEnvironmentLabel('preview')({ page });
    await page.goto('/');

    const banner = page.locator('[data-testid="environment-banner"]');
    await expect(banner).toBeVisible();
    await expect(banner).toHaveClass(/bg-purple-600/);
    await expect(banner).toHaveText('Preview Environment');
    await expect(page).toHaveTitle(/^\[PRE\]/);
  });

  test('renders a multi-word label verbatim (GKE dry-run)', async ({ page }) => {
    await withEnvironmentLabel('GKE pre-prod')({ page });
    await page.goto('/');

    const banner = page.locator('[data-testid="environment-banner"]');
    await expect(banner).toBeVisible();
    await expect(banner).toHaveText('GKE pre-prod Environment');
    await expect(page).toHaveTitle(/^\[GKE\]/);
  });

  test('shows no banner for a prod label', async ({ page }) => {
    await withEnvironmentLabel('prod')({ page });
    await page.goto('/');

    await expect(page.locator('[data-testid="environment-banner"]')).toHaveCount(0);
  });

  test('shows no banner when no config is injected', async ({ page }) => {
    await page.goto('/');

    await expect(page.locator('[data-testid="environment-banner"]')).toHaveCount(0);
  });

  test('banner sits above the header when present', async ({ page }) => {
    await withEnvironmentLabel('preview')({ page });
    await page.goto('/');

    const banner = page.locator('[data-testid="environment-banner"]');
    await expect(banner).toBeVisible();

    const bannerBox = await banner.boundingBox();
    const headerBox = await page.locator('header').first().boundingBox();
    if (bannerBox && headerBox) {
      expect(bannerBox.y).toBeLessThanOrEqual(headerBox.y);
    }
  });
});
