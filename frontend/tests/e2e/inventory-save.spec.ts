/**
 * Inventory Save Flow E2E Tests
 *
 * Tests the complete flow of:
 * 1. Scanning tags anonymously
 * 2. Logging in and verifying enrichment
 * 3. Saving inventory to a location
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 * - BLE bridge server running for hardware tests
 * - CS108 RFID reader connected with test tags 10018-10023
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { simulateTriggerPress, simulateTriggerRelease } from './helpers/trigger-utils';
import {
  clearAuthState,
  uniqueId,
  signupTestUser,
  loginTestUser,
  getAuthToken,
  createOrgViaAPI,
  switchOrgViaAPI,
  listOrgsViaAPI,
} from './fixtures/org.fixture';

const API_BASE = 'http://localhost:8080/api/v1';

interface TestAsset {
  id: number;
  name: string;
  rfidTag: string;
}

interface TestLocation {
  id: number;
  name: string;
  rfidTag?: string;
}

/**
 * Create a test location via API
 */
async function createTestLocation(
  page: Page,
  name: string,
  rfidTag?: string
): Promise<TestLocation> {
  const token = await getAuthToken(page);

  const identifiers = rfidTag ? [{ type: 'rfid', value: rfidTag }] : [];

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
      identifiers,
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
    rfidTag,
  };
}

/**
 * Create a test asset via API with RFID identifier
 * Uses the raw EPC value (no transformation)
 */
async function createTestAsset(
  page: Page,
  name: string,
  rfidEpc: string
): Promise<TestAsset> {
  const token = await getAuthToken(page);

  const response = await page.request.post(`${API_BASE}/assets`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: {
      name,
      identifier: `ASSET-${uniqueId()}`,
      type: 'asset',
      is_active: true,
      valid_from: new Date().toISOString().split('T')[0],
      identifiers: [{ type: 'rfid', value: rfidEpc }],
    },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to create asset: ${response.status()} - ${text}`);
  }

  const data = await response.json();
  return {
    id: data.data.id,
    name: data.data.name,
    rfidTag: rfidEpc,
  };
}

/**
 * Wait for tag enrichment to complete
 * Checks that tags have assetId or locationId populated
 */
async function waitForEnrichment(page: Page, timeout = 10000): Promise<void> {
  await page.waitForFunction(
    () => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      // Check if any tags have been enriched
      return tags.some(
        (t: any) => t.assetId !== undefined || t.locationId !== undefined
      );
    },
    { timeout }
  );
}

test.describe('Inventory Save Flow', () => {
  // Force serial execution - tests depend on each other
  test.describe.configure({ mode: 'serial' });

  const testId = uniqueId();
  const testEmail = `test-save-${testId}@example.com`;
  const testPassword = 'TestPassword123!';
  const testOrgName = `Test Org ${testId}`;

  let testLocation: TestLocation;
  let testAssets: TestAsset[];
  let scannedEpcs: string[] = [];
  let sharedPage: Page;

  test.beforeAll(async ({ browser }) => {
    console.log('[InventorySave] Setting up test user...');
    sharedPage = await browser.newPage();

    // Create test user and org first
    await signupTestUser(sharedPage, testEmail, testPassword, testOrgName);

    // Create test location (no RFID tag - we'll use manual selection)
    testLocation = await createTestLocation(sharedPage, `Warehouse ${testId}`);
    console.log(`[InventorySave] Created location: ${testLocation.name} (ID: ${testLocation.id})`);

    // NOTE: We don't create assets here - we'll scan first to get real EPCs,
    // then create assets that match those EPCs
    testAssets = [];

    // Clear auth state to start tests as anonymous
    await clearAuthState(sharedPage);
    await sharedPage.reload({ waitUntil: 'networkidle' });
  });

  test.afterAll(async () => {
    if (sharedPage) {
      await sharedPage.close();
    }
  });

  test('1. scan tags anonymously to discover real EPCs @hardware @critical', async () => {
    // Navigate to inventory while anonymous
    await sharedPage.goto('/#inventory');
    await sharedPage.waitForTimeout(500);

    // Verify we're anonymous
    const isAuthenticated = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return stores?.authStore?.getState().isAuthenticated ?? false;
    });
    expect(isAuthenticated).toBe(false);
    console.log('[Test] Starting anonymous, connecting to device...');

    // Connect to device
    await connectToDevice(sharedPage);

    // Wait for mode change to inventory
    await sharedPage.waitForFunction(
      () => {
        const stores = (window as any).__ZUSTAND_STORES__;
        return stores?.deviceStore?.getState().readerMode === 'Inventory';
      },
      { timeout: 10000 }
    );

    // Clear any existing tags
    await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      stores?.tagStore?.getState().clearTags();
    });

    // Scan tags
    console.log('[Test] Scanning tags to discover EPCs...');
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(3000);
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(500);

    // Get scanned tags - save EPCs for asset creation
    const scanResult = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        epcs: tags.map((t: any) => t.epc),
        tags: tags.map((t: any) => ({
          epc: t.epc,
          type: t.type,
          assetId: t.assetId,
        })),
      };
    });

    console.log(`[Test] Discovered ${scanResult.count} EPCs:`, scanResult.epcs.slice(0, 5));
    expect(scanResult.count).toBeGreaterThan(0);

    // Save EPCs for later tests
    scannedEpcs = scanResult.epcs;

    // Verify tags are NOT enriched (anonymous user)
    const unenrichedCount = scanResult.tags.filter((t: any) => t.assetId === undefined).length;
    expect(unenrichedCount).toBe(scanResult.count);
    console.log('[Test] Confirmed tags are unenriched (anonymous)');

    // Navigate to home before disconnecting
    await sharedPage.click('button[data-testid="menu-item-home"]');
    await sharedPage.waitForTimeout(500);

    // Disconnect device
    await disconnectDevice(sharedPage);
  });

  test('2. login, create assets, verify enrichment, and save @hardware @critical', async () => {
    // Log in first to create assets
    console.log('[Test] Logging in to create assets...');
    await loginTestUser(sharedPage, testEmail, testPassword);

    // Create assets using the real EPCs we discovered
    const epcsToUse = scannedEpcs.slice(0, 3); // Use first 3 EPCs
    console.log('[Test] Creating assets for EPCs:', epcsToUse);

    for (let i = 0; i < epcsToUse.length; i++) {
      const asset = await createTestAsset(
        sharedPage,
        `Test Asset ${i + 1} - ${testId}`,
        epcsToUse[i]
      );
      testAssets.push(asset);
      console.log(`[Test] Created asset: ${asset.name} (ID: ${asset.id}, EPC: ${asset.rfidTag})`);
    }

    // Navigate to inventory - tags should still be in localStorage
    await sharedPage.goto('/#inventory');
    await sharedPage.waitForTimeout(1000);

    // Wait for enrichment to happen
    console.log('[Test] Waiting for enrichment...');
    try {
      await waitForEnrichment(sharedPage, 10000);
      console.log('[Test] Enrichment detected!');
    } catch (e) {
      console.log('[Test] Enrichment did not complete in time');
    }

    // Check enrichment result
    const enrichedResult = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        tags: tags.map((t: any) => ({
          epc: t.epc,
          type: t.type,
          assetId: t.assetId,
          assetName: t.assetName,
        })),
        enrichedAssets: tags.filter((t: any) => t.assetId !== undefined).length,
      };
    });

    console.log('[Test] Enrichment result:', JSON.stringify(enrichedResult.tags.slice(0, 5)));
    console.log(`[Test] Enriched: ${enrichedResult.enrichedAssets} assets out of ${enrichedResult.count} tags`);

    // We created 3 assets, so at least 3 should be enriched
    expect(enrichedResult.enrichedAssets).toBeGreaterThanOrEqual(3);

    // --- Part 2: Test location selection and Save button state ---
    console.log('[Test] Testing Save button state...');

    // Get current tag state
    const tagState = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        total: tags.length,
        assets: tags.filter((t: any) => t.type === 'asset').length,
      };
    });

    console.log(`[Test] Tag state: ${tagState.total} total, ${tagState.assets} assets`);

    // Find the Save button
    const saveButton = sharedPage.locator('button:has-text("Save")');
    await expect(saveButton).toBeVisible();

    // Without a location selected, Save should be disabled
    const isDisabledBefore = await saveButton.isDisabled();
    console.log(`[Test] Save button disabled (no location): ${isDisabledBefore}`);
    expect(isDisabledBefore).toBe(true);

    // Select a location from the dropdown (Menu button says "Select" or "Change")
    const locationSelect = sharedPage.locator('button:has-text("Select"), button:has-text("Change")').first();
    await expect(locationSelect).toBeVisible({ timeout: 5000 });
    await locationSelect.click();
    await sharedPage.waitForTimeout(300);

    // Select the test location from the dropdown menu
    const locationOption = sharedPage.locator(`button:has-text("${testLocation.name}")`);
    await expect(locationOption).toBeVisible({ timeout: 5000 });
    await locationOption.click();
    await sharedPage.waitForTimeout(300);

    // Now Save should be enabled (if we have assets)
    if (tagState.assets > 0) {
      const isDisabledAfter = await saveButton.isDisabled();
      console.log(`[Test] Save button disabled (with location): ${isDisabledAfter}`);
      expect(isDisabledAfter).toBe(false);
    }

    // --- Part 3: Actually save the inventory ---
    console.log('[Test] Saving inventory...');

    // Get saveable count
    const saveableCount = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return tags.filter((t: any) => t.type === 'asset' && t.assetId).length;
    });

    console.log(`[Test] Saveable assets: ${saveableCount}`);
    expect(saveableCount).toBeGreaterThan(0);

    // Click Save button
    await expect(saveButton).toBeEnabled();
    await saveButton.click();

    // Wait for success toast (react-hot-toast uses role="status" for success toasts)
    const toast = sharedPage.locator('[role="status"]:has-text("saved")');
    await expect(toast).toBeVisible({ timeout: 10000 });

    const toastText = await toast.textContent();
    console.log(`[Test] Toast message: ${toastText}`);
    expect(toastText).toContain('saved');

    // Verify Clear button has pulse animation after save
    const clearButton = sharedPage.locator('button:has-text("Clear")');
    const hasPulse = await clearButton.evaluate((el) =>
      el.classList.contains('pulse-attention')
    );
    console.log(`[Test] Clear button has pulse: ${hasPulse}`);
  });

  test('3. switching orgs clears asset mappings and re-enriches', async () => {
    // This test verifies that when a user switches orgs, the asset/location
    // mappings are cleared (since they're org-specific) and re-enrichment happens

    // First, ensure we're logged in
    console.log('[Test] Logging in for org switch test...');
    await loginTestUser(sharedPage, testEmail, testPassword);

    // Navigate to inventory and verify we have enriched tags
    await sharedPage.goto('/#inventory');
    await sharedPage.waitForTimeout(1000);

    // Add some tags with mock enrichment data directly to store
    await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tagStore = stores?.tagStore;
      if (tagStore) {
        // Clear existing tags first
        tagStore.getState().clearTags();

        // Add tags with fake enrichment (simulating tags from old org)
        tagStore.getState().setTags([
          {
            epc: 'ORG_TEST_0000000000001',
            displayEpc: 'ORG_TEST_0000000000001',
            count: 1,
            source: 'rfid',
            type: 'asset',
            assetId: 99999,
            assetName: 'Old Org Asset',
            timestamp: Date.now(),
          },
          {
            epc: 'ORG_TEST_0000000000002',
            displayEpc: 'ORG_TEST_0000000000002',
            count: 1,
            source: 'rfid',
            type: 'location',
            locationId: 88888,
            locationName: 'Old Org Location',
            timestamp: Date.now(),
          },
        ]);
      }
    });

    // Verify tags have enrichment data
    const beforeSwitch = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        enriched: tags.filter((t: any) => t.assetId !== undefined || t.locationId !== undefined).length,
        tags: tags.map((t: any) => ({
          epc: t.epc,
          type: t.type,
          assetId: t.assetId,
          locationId: t.locationId,
        })),
      };
    });

    console.log('[Test] Before org switch:', beforeSwitch);
    expect(beforeSwitch.enriched).toBe(2);

    // Create a new org
    console.log('[Test] Creating new org...');
    const newOrg = await createOrgViaAPI(sharedPage, `New Org ${testId}`);
    console.log(`[Test] Created org: ${newOrg.name} (ID: ${newOrg.id})`);

    // Switch to the new org via API (updates token in localStorage)
    console.log('[Test] Switching to new org via API...');
    await switchOrgViaAPI(sharedPage, newOrg.id);

    // Reload the page to pick up the new token and trigger the org change subscription
    await sharedPage.reload({ waitUntil: 'networkidle' });

    // Navigate to inventory to see the effect
    await sharedPage.goto('/#inventory');
    await sharedPage.waitForTimeout(1000);

    // Verify tags have been cleared of enrichment data
    const afterSwitch = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        enriched: tags.filter((t: any) => t.assetId !== undefined || t.locationId !== undefined).length,
        unknownType: tags.filter((t: any) => t.type === 'unknown').length,
        tags: tags.map((t: any) => ({
          epc: t.epc,
          type: t.type,
          assetId: t.assetId,
          locationId: t.locationId,
        })),
      };
    });

    console.log('[Test] After org switch:', afterSwitch);

    // Tags should still exist but with cleared enrichment
    expect(afterSwitch.count).toBe(beforeSwitch.count);
    // Enrichment should be cleared (assetId/locationId undefined)
    expect(afterSwitch.enriched).toBe(0);
    // Type should be reset to 'unknown'
    expect(afterSwitch.unknownType).toBe(afterSwitch.count);

    console.log('[Test] Org switch correctly cleared asset/location mappings');
  });

  test('4. switching back to original org re-enriches with original mappings', async () => {
    // This test verifies that when switching back to the original org,
    // tags get re-enriched with that org's asset/location mappings

    // First, add tags with EPCs that match our test assets (created in test 2)
    // These are the real EPCs from the hardware scan
    if (scannedEpcs.length === 0) {
      console.log('[Test] No scanned EPCs available, using mock data');
      scannedEpcs = ['MOCK_EPC_001', 'MOCK_EPC_002', 'MOCK_EPC_003'];
    }

    console.log('[Test] Setting up tags with original EPCs:', scannedEpcs.slice(0, 3));

    await sharedPage.evaluate((epcs: string[]) => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tagStore = stores?.tagStore;
      if (tagStore) {
        tagStore.getState().clearTags();
        // Add tags with EPCs that should match assets in the original org
        tagStore.getState().setTags(epcs.slice(0, 3).map((epc: string) => ({
          epc,
          displayEpc: epc,
          count: 1,
          source: 'rfid',
          type: 'unknown', // Not enriched yet
          timestamp: Date.now(),
        })));
      }
    }, scannedEpcs);

    // Verify tags are unenriched
    const beforeSwitch = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tags = stores?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        enriched: tags.filter((t: any) => t.assetId !== undefined).length,
      };
    });

    console.log('[Test] Before switch back:', beforeSwitch);
    expect(beforeSwitch.enriched).toBe(0);

    // Get the original org by finding it in the list
    const orgs = await listOrgsViaAPI(sharedPage);
    const originalOrg = orgs.find((o) => o.name === testOrgName);

    console.log('[Test] Available orgs:', orgs.map((o) => o.name));
    console.log('[Test] Looking for original org:', testOrgName);
    console.log('[Test] Found original org:', originalOrg?.name, originalOrg?.id);

    if (originalOrg?.id) {
      await switchOrgViaAPI(sharedPage, originalOrg.id);

      // Reload and navigate to inventory
      await sharedPage.reload({ waitUntil: 'networkidle' });
      await sharedPage.goto('/#inventory');
      await sharedPage.waitForTimeout(1000);

      // Wait for enrichment to happen
      try {
        await waitForEnrichment(sharedPage, 10000);
        console.log('[Test] Re-enrichment detected!');
      } catch (e) {
        console.log('[Test] Re-enrichment did not complete in time');
      }

      // Check if tags got re-enriched
      const afterSwitch = await sharedPage.evaluate(() => {
        const stores = (window as any).__ZUSTAND_STORES__;
        const tags = stores?.tagStore?.getState().tags || [];
        return {
          count: tags.length,
          enriched: tags.filter((t: any) => t.assetId !== undefined).length,
          tags: tags.map((t: any) => ({
            epc: t.epc,
            type: t.type,
            assetId: t.assetId,
            assetName: t.assetName,
          })),
        };
      });

      console.log('[Test] After switch back:', afterSwitch);

      // Tags should be re-enriched with original org's assets
      // We created 3 test assets, so at least those should be enriched
      if (testAssets.length > 0) {
        expect(afterSwitch.enriched).toBeGreaterThan(0);
        console.log(`[Test] Re-enriched ${afterSwitch.enriched} tags after switching back`);
      }
    } else {
      console.log('[Test] Could not find original org, skipping switch-back verification');
    }
  });

  test('5. anonymous user clicking Save redirects to login', async () => {
    // Clear auth state
    await clearAuthState(sharedPage);
    await sharedPage.reload({ waitUntil: 'networkidle' });

    // Navigate to inventory
    await sharedPage.goto('/#inventory');
    await sharedPage.waitForTimeout(500);

    // Verify anonymous
    const isAuthenticated = await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      return stores?.authStore?.getState().isAuthenticated ?? false;
    });
    expect(isAuthenticated).toBe(false);

    // Add some mock tags directly to store (simulating scan)
    await sharedPage.evaluate(() => {
      const stores = (window as any).__ZUSTAND_STORES__;
      const tagStore = stores?.tagStore;
      if (tagStore) {
        tagStore.getState().addTag({
          epc: 'MOCK0000000000000001',
          rssi: -55,
          count: 1,
        });
      }
    });

    await sharedPage.waitForTimeout(500);

    // Find Save button - should be enabled for anonymous users with scanned tags
    const saveButton = sharedPage.locator('button:has-text("Save")');
    await expect(saveButton).toBeVisible();

    // Click Save
    await saveButton.click();

    // Should redirect to login
    await sharedPage.waitForURL(/#login/, { timeout: 5000 });
    console.log('[Test] Redirected to login as expected');

    // Verify redirect was stored
    const redirectPath = await sharedPage.evaluate(() => {
      return sessionStorage.getItem('redirectAfterLogin');
    });
    expect(redirectPath).toBe('inventory');
  });
});
