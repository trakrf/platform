/**
 * TRA-824: Inventory location selector serves stale cache after in-app create.
 *
 * Regression: LocationFormModal called locationsApi.create directly (bypassing
 * useLocationMutations) which left TanStack's ['locations'] query cache un-
 * invalidated. Combined with refetchOnMount: false on useLocations, the
 * Inventory screen rendered an empty selector and could not resolve detected
 * location tags until a hard refresh.
 *
 * This test exercises the in-app create path end-to-end (no hardware needed):
 *   1. signup → fresh org (empty locations list cached on first Inventory mount)
 *   2. visit Inventory → useLocations caches []
 *   3. visit Locations → create a location via the FAB modal
 *   4. visit Inventory → open the manual location selector
 *   5. expect the newly-created location to appear in the dropdown
 */

import { test, expect } from '@playwright/test';
import { signupTestUser, uniqueId } from './fixtures/org.fixture';

test.describe('Inventory location cache after in-app create (TRA-824)', () => {
  test.setTimeout(60000);

  test('newly-created location appears in Inventory selector without refresh', async ({ page }) => {
    const id = uniqueId();
    const email = `test-tra824-${id}@example.com`;
    const password = 'TestPassword123!';
    const orgName = `TRA-824 Org ${id}`;
    const locExternalKey = `whs-${id}`;
    const locName = `Warehouse ${id}`;

    // 1. Fresh signup creates a clean org with zero locations.
    await signupTestUser(page, email, password, orgName);

    // 2. Visit Inventory first so useLocations caches an empty list under
    //    queryKey ['locations', orgId]. This is the pre-condition for the
    //    bug: subsequent in-app creates must invalidate this cache, otherwise
    //    a return visit to Inventory keeps showing stale data.
    await page.locator('button[data-testid="menu-item-inventory"]').click();
    await page.waitForURL(/#inventory/, { timeout: 5000 });
    // Wait for the location bar to settle into its empty state.
    await expect(page.getByText('No location tag detected')).toBeVisible({ timeout: 5000 });

    // 3. Visit Locations and create a location via the FAB modal.
    await page.locator('button[data-testid="menu-item-locations"]').click();
    await page.waitForURL(/#locations/, { timeout: 5000 });
    await page.getByRole('button', { name: 'Create new location' }).click();
    await page.locator('input#external_key').fill(locExternalKey);
    await page.locator('input#name').fill(locName);
    await page.getByRole('button', { name: 'Create Location' }).click();
    // Toast confirms the create round-tripped.
    await expect(page.getByText(`Location "${locExternalKey}" created successfully`)).toBeVisible({
      timeout: 5000,
    });

    // 4. Return to Inventory. With the bug, useLocations would serve the
    //    stale empty cache because TanStack was never invalidated. With the
    //    fix, the cache is marked stale on create and refetchOnMount: true
    //    triggers a refetch on this mount.
    await page.locator('button[data-testid="menu-item-inventory"]').click();
    await page.waitForURL(/#inventory/, { timeout: 5000 });

    // 5. Open the manual location selector and expect the new location to be
    //    listed. With the bug the dropdown showed "No locations available".
    await page.getByRole('button', { name: 'Select' }).click();
    await expect(page.getByRole('menuitem', { name: locName })).toBeVisible({ timeout: 5000 });
  });
});
