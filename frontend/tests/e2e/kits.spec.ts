/**
 * E2E tests for kit scan flows (TRA-1033)
 *
 * Commission (scan + Lot#) and Verify-at-return with the red-exception →
 * Locate handoff. Requires the full stack: backend on :8080 + BLE bridge with
 * a physical CS108 and test tags in range (TEST_TAG_RANGE) — hence @hardware.
 *
 * Commission scans real tags via the trigger; the verify sessions seed
 * tagStore directly so the complete/incomplete split is deterministic while
 * still exercising the real /kits/verify backend.
 */

/* eslint-disable @typescript-eslint/no-explicit-any */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice } from './helpers/connection';
import { simulateTriggerPress, simulateTriggerRelease } from './helpers/trigger-utils';
import { signupTestUser, uniqueId } from './fixtures/org.fixture';

async function seedTags(page: Page, epcs: string[]): Promise<void> {
  await page.evaluate((values) => {
    const stores = (window as any).__ZUSTAND_STORES__;
    const tagStore = stores?.tagStore;
    tagStore.getState().clearTags();
    for (const epc of values) {
      tagStore.getState().addTag({
        epc,
        rssi: -45,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid',
      });
    }
  }, epcs);
}

test.describe('Kit Scan Flows @hardware', () => {
  // Serial — verify tests depend on the kit commissioned in the first test
  test.describe.configure({ mode: 'serial' });

  const testId = uniqueId();
  const testEmail = `test-kits-${testId}@example.com`;
  const testPassword = 'TestPassword123!';
  const testOrgName = `Kits Org ${testId}`;
  const kitLabel = `Lot-${testId}`;

  let sharedPage: Page;
  let memberEpcs: string[] = [];

  test.beforeAll(async ({ browser }) => {
    sharedPage = await browser.newPage();
    // Self-signup org (trial entitlement); creator is owner → Operator+ writes OK
    await signupTestUser(sharedPage, testEmail, testPassword, testOrgName);
    await connectToDevice(sharedPage);
  });

  test.afterAll(async () => {
    if (sharedPage) {
      await sharedPage.close();
    }
  });

  test('1. commission happy path — scan, label, save', async () => {
    await sharedPage.click('[data-testid="menu-item-kits"]');

    // Kits tab runs the reader in Inventory mode
    await sharedPage.waitForFunction(
      () => {
        const stores = (window as any).__ZUSTAND_STORES__;
        return stores?.deviceStore?.getState().readerMode === 'Inventory';
      },
      { timeout: 10000 }
    );

    await sharedPage.click('[data-testid="kits-mode-commission"]');
    await sharedPage.click('[data-testid="kit-commission-clear"]');

    // Scan real tags with the trigger
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(3000);
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(500);

    // Grab the non-location EPCs the flow will send as members
    memberEpcs = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return tags.filter((t: any) => t.type !== 'location').map((t: any) => t.epc);
    });
    console.log(`[Kits] Scanned ${memberEpcs.length} member EPCs:`, memberEpcs.slice(0, 5));
    expect(memberEpcs.length).toBeGreaterThanOrEqual(2);

    // Give the first member a role via the datalist input
    await sharedPage.fill(`[data-testid="kit-role-input-${memberEpcs[0]}"]`, 'coupon');

    await sharedPage.fill('[data-testid="kit-label-input"]', kitLabel);
    await sharedPage.click('[data-testid="kit-save"]');

    // Success toast carries label + member count, list resets
    await expect(sharedPage.getByText(`Kit ${kitLabel} created`)).toBeVisible({ timeout: 10000 });
    const remaining = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return (stores?.tagStore?.getState().tags || []).length;
    });
    expect(remaining).toBe(0);
  });

  test('2. verify complete — all members present, green result', async () => {
    await sharedPage.click('[data-testid="kits-mode-verify"]');
    await sharedPage.click('[data-testid="kit-verify-new-scan"]');

    await seedTags(sharedPage, memberEpcs);
    await sharedPage.click('[data-testid="kit-verify"]');

    const complete = sharedPage.locator('[data-testid^="kit-result-complete-"]');
    await expect(complete).toHaveCount(1, { timeout: 10000 });
    await expect(complete).toContainText(kitLabel);
  });

  test('3. verify incomplete — red banner, Locate handoff carries the EPC, way back works', async () => {
    // Scan session missing the first member
    const missingEpc = memberEpcs[0];
    await sharedPage.click('[data-testid="kit-verify-new-scan"]');
    await seedTags(sharedPage, memberEpcs.slice(1));
    await sharedPage.click('[data-testid="kit-verify"]');

    // The product moment: full-width red exception banner
    const incomplete = sharedPage.locator('[data-testid^="kit-result-incomplete-"]');
    await expect(incomplete).toHaveCount(1, { timeout: 10000 });
    await expect(incomplete).toContainText(kitLabel);

    // Tap Locate on the missing member
    await incomplete.locator('[data-testid^="locate-missing-"]').first().click();

    // Locate mode pre-armed with the missing member's EPC + return param
    await expect(sharedPage.locator('h2').first()).toContainText('Find Item');
    expect(sharedPage.url()).toContain('return=kits');
    expect(decodeURIComponent(sharedPage.url())).toContain(`epc=${missingEpc}`);

    const armedEpc = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return stores?.settingsStore?.getState().rfid.targetEPC;
    });
    // setTargetEPC normalizes on store — compare case-insensitively
    expect((armedEpc || '').toUpperCase()).toBe(missingEpc.toUpperCase());

    // The way back: results still rendered after returning
    await sharedPage.click('[data-testid="locate-back-to-results"]');
    await expect(
      sharedPage.locator('[data-testid^="kit-result-incomplete-"]')
    ).toHaveCount(1);
  });
});
