/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Consolidated Inventory E2E Tests
 * Minimal set of tests covering all critical inventory functionality
 * Based on previously working tests from phase 7
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import {
  simulateTriggerPress,
  simulateTriggerRelease,
  getTriggerState
} from './helpers/trigger-utils';
import type { WindowWithStores } from './types';
import { TEST_TAG_ARRAY, TEST_TAG_RANGE, TEST_EXPECTATIONS } from '@test-utils/constants';
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';


// Post-refactor - re-enabling tests one by one
test.describe('Consolidated Inventory Tests', () => {
  // Wall-clock here scales with how many tags are in front of the reader: test 3
  // took 36.6s against a dense field and blew the 30s default with every
  // assertion passing and the data healthy. Read volume must not decide
  // pass/fail (TRA-1148 item 5).
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

  /**
   * CONNECTION SHARING STRATEGY
   * 
   * We connect once for all inventory tests because:
   * 1. Connection/disconnection is tested in connection.spec.ts
   * 2. Users perform multiple operations without reconnecting
   * 3. Tests run much faster (30s vs 2+ minutes)
   * 4. This tests real-world connection stability
   */
  
  let sharedPage: Page;
  let connectionHealthy = true;
  
  test.beforeAll(async ({ browser }) => {
    console.log('[Inventory] Setting up shared connection for all tests...');
    sharedPage = await browser.newPage();

    // '/' lands on the Scan tab, so connect configures INVENTORY (TRA-1029)
    await sharedPage.goto('/');
    await connectToDevice(sharedPage);

    // Now navigate to inventory tab after connection established
    console.log('[Inventory] Navigating to inventory tab (will trigger mode change)...');
    await sharedPage.click('[data-testid="menu-item-scan"]');

    // Wait for tab to change
    await sharedPage.waitForTimeout(500);

    // Wait for mode change to complete - poll until mode changes or timeout
    console.log('[Inventory] Waiting for mode change to INVENTORY...');
    await sharedPage.waitForFunction(
      () => {
        const stores = window.__ZUSTAND_STORES__;
        return stores?.deviceStore?.getState().readerMode === 'Inventory';
      },
      { timeout: 10000 }
    );

    // Verify the mode change happened correctly
    const modeChangeResult = await sharedPage.evaluate(() => {
      const stores = window.__ZUSTAND_STORES__;
      return {
        activeTab: stores?.uiStore?.getState().activeTab,
        readerMode: stores?.deviceStore?.getState().readerMode,
        readerState: stores?.deviceStore?.getState().readerState
      };
    });

    console.log('[Inventory] Mode change result:', modeChangeResult);
    expect(modeChangeResult.activeTab).toBe('scan');
    expect(modeChangeResult.readerMode).toBe('Inventory');

    // Wait for state to settle to Ready
    await sharedPage.waitForFunction(
      () => {
        const stores = window.__ZUSTAND_STORES__;
        return stores?.deviceStore?.getState().readerState === 'Connected';
      },
      { timeout: 10000 }
    );

    // Check final state
    const finalState = await sharedPage.evaluate(() => {
      const stores = window.__ZUSTAND_STORES__;
      return stores?.deviceStore?.getState().readerState;
    });
    expect(finalState).toBe('Connected');
    
    // Wait for configuration to complete
    console.log('[Inventory] Waiting for RFID configuration...');
    await sharedPage.waitForTimeout(3000);
  });

  test.beforeEach(async () => {
    if (!connectionHealthy) {
      test.skip();
      return;
    }
    
    // Clean state between tests
    console.log('[Inventory] Cleaning state before test...');
    
    // Ensure trigger is released
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(500);
    
    // Clear any existing tags
    await sharedPage.evaluate(() => {
      const tagStore = window.__ZUSTAND_STORES__?.tagStore;
      tagStore?.getState().clearTags();
    });
  });
  
  test.afterEach(async ({ page }, testInfo) => {
    if (testInfo.status === 'failed') {
      console.log('[Inventory] Test failed, capturing state...');
      const state = await sharedPage.evaluate(() => {
        const stores = window.__ZUSTAND_STORES__;
        return {
          triggerState: stores?.deviceStore?.getState().triggerState,
          readerState: stores?.deviceStore?.getState().readerState,
          inventoryRunning: stores?.tagStore?.getState().inventoryRunning,
          tagCount: stores?.tagStore?.getState().tags?.length || 0
        };
      });
      console.log('[Inventory] Failed test state:', state);
      connectionHealthy = false; // Mark connection as potentially corrupted
    }
  });
  
  test.afterAll(async () => {
    if (sharedPage && connectionHealthy) {
      console.log('[Inventory] Cleaning up - switching to settings tab to power off RFID...');

      // Click settings tab to trigger IDLE mode
      await sharedPage.click('button[data-testid="menu-item-settings"]');
      await sharedPage.waitForTimeout(1000); // Give time for IDLE mode to activate

      // Verify RFID is powered off
      const finalMode = await sharedPage.evaluate(() => {
        const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
        return deviceStore?.getState().readerMode;
      });
      console.log(`[Inventory] Final reader mode: ${finalMode}`);

      console.log('[Inventory] RFID powered off, disconnecting...');
      await disconnectDevice(sharedPage);
      await sharedPage.close();
    } else if (sharedPage) {
      console.log('[Inventory] Connection unhealthy, forcing cleanup...');
      await sharedPage.close();
    }
  });

  test('1. trigger press/release changes trigger state @hardware @critical', async () => {
    // Press trigger
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);
    await sharedPage.waitForTimeout(500);
    
    let triggerState = await getTriggerState(sharedPage);
    expect(triggerState).toBe(true);
    
    // Release trigger
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);
    await sharedPage.waitForTimeout(500);
    
    triggerState = await getTriggerState(sharedPage);
    expect(triggerState).toBe(false);
  });

  test.skip('2. multiple trigger cycles work correctly @hardware @critical', async () => {
    // This test verifies the system can handle multiple trigger operations
    // in a high tag density environment without becoming unresponsive
    
    console.log('[Test] Testing trigger responsiveness in high tag density environment');
    
    // Clear tags to start clean
    await sharedPage.evaluate(() => {
      const tagStore = window.__ZUSTAND_STORES__?.tagStore;
      tagStore?.getState().clearTags();
    });
    
    let totalInventoryDataReceived = 0;
    
    // Monitor for inventory data being generated to prove trigger → inventory works
    sharedPage.on('console', msg => {
      const text = msg.text();
      if (text.includes('BLE] Received continuation fragment')) {
        totalInventoryDataReceived++;
      }
    });
    
    // Cycle 1: Press and hold for data generation
    console.log('[Test] Cycle 1: Trigger press/hold/release');
    const press1Result = await simulateTriggerPress(sharedPage);
    expect(press1Result.success).toBe(true);
    
    // Hold to generate inventory data
    await sharedPage.waitForTimeout(2000);
    
    const release1Result = await simulateTriggerRelease(sharedPage);
    expect(release1Result.success).toBe(true);
    
    // Allow system to settle
    await sharedPage.waitForTimeout(3000);
    
    const cycle1Data = totalInventoryDataReceived;
    console.log(`[Test] Cycle 1 generated ${cycle1Data} data fragments`);
    
    // Reset counter for cycle 2
    totalInventoryDataReceived = 0;
    
    // Cycle 2: Verify system is still responsive  
    console.log('[Test] Cycle 2: Verify continued responsiveness');
    const press2Result = await simulateTriggerPress(sharedPage);
    expect(press2Result.success).toBe(true);
    
    // Hold to generate more data
    await sharedPage.waitForTimeout(2000);
    
    const release2Result = await simulateTriggerRelease(sharedPage);
    expect(release2Result.success).toBe(true);
    
    // Allow system to settle
    await sharedPage.waitForTimeout(3000);
    
    const cycle2Data = totalInventoryDataReceived;
    console.log(`[Test] Cycle 2 generated ${cycle2Data} data fragments`);
    
    // The key test: system should continue generating inventory data in both cycles
    // This proves trigger → inventory control is working across multiple cycles
    expect(cycle1Data).toBeGreaterThan(0);
    expect(cycle2Data).toBeGreaterThan(0);
    
    console.log('[Test] SUCCESS: Multiple trigger cycles generated inventory data - trigger control working');
  });

  test('3. tag accumulation over multiple reads @hardware', async () => {
    // Clear tags
    await sharedPage.evaluate(() => {
      const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
      tagStore?.getState().clearTags();
    });
    
    // First read cycle
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(2000);
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(500);
    
    const firstResult = await sharedPage.evaluate(() => {
      const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
      const tags = tagStore?.getState().tags || [];
      const epcs = tags.map(t => t.epc);
      return {
        count: tags.reduce((sum, t) => sum + (t.count || 1), 0),
        epcs: epcs,
        uniqueCount: epcs.length
      };
    });
    console.log(`[Test] First read: ${firstResult.count} reads, ${firstResult.uniqueCount} unique tags`);
    console.log(`[Test] EPCs found:`, firstResult.epcs);
    
    // Second read cycle
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(2000);
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(500);
    
    const secondResult = await sharedPage.evaluate(() => {
      const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
      const tags = tagStore?.getState().tags || [];
      const epcs = tags.map(t => t.epc);
      return {
        count: tags.reduce((sum, t) => sum + (t.count || 1), 0),
        epcs: epcs,
        uniqueCount: epcs.length
      };
    });
    console.log(`[Test] Second read: ${secondResult.count} reads, ${secondResult.uniqueCount} unique tags`);
    console.log(`[Test] EPCs found:`, secondResult.epcs);

    // Check for test tags
    const testTags = TEST_TAG_ARRAY;
    const foundTestTags = testTags.filter(tag =>
      secondResult.epcs.some((epc: string) => epc.includes(tag))
    );
    console.log(`[Test] Found test tags (${TEST_TAG_RANGE}):`, foundTestTags);

    // --- Assertions (TRA-1148 item 4) -------------------------------------
    //
    // This test used to assert only `second.count >= first.count`, which is
    // satisfied by 0 >= 0 and by 375 >= 375. Both were observed passing green:
    // once against a completely dead link, and once with a second cycle that
    // contributed no reads at all. A green run here has to mean the scan path
    // works, so each of those holes is now closed by its own assertion.

    // 1. The first cycle actually read something. Catches the dead-link run
    //    that "passed" with 0 reads / 0 unique tags.
    expect(firstResult.count,
      'first read cycle should return reads - 0 means the scan path is dead'
    ).toBeGreaterThanOrEqual(TEST_EXPECTATIONS.MIN_READS_PER_CYCLE);
    expect(firstResult.uniqueCount,
      'first read cycle should decode at least one unique tag'
    ).toBeGreaterThanOrEqual(TEST_EXPECTATIONS.MIN_UNIQUE_TAGS_PER_CYCLE);

    // 2. The second cycle contributed reads of its own. Strict, not >=:
    //    the tag store accumulates, so a working second cycle can only push the
    //    total up. Equality means the trigger/scan did nothing the second time,
    //    which is the 375 -> 375 run that used to pass.
    expect(secondResult.count,
      'second read cycle should add reads - equality means it contributed nothing'
    ).toBeGreaterThan(firstResult.count);

    // 3. Known EPCs came back. Without this the test cannot tell a working
    //    decoder from one returning garbage: counts alone are just byte volume.
    //    Requires the staged test tags (TEST_TAG_RANGE) to be in the field.
    expect(foundTestTags.length,
      `expected at least ${TEST_EXPECTATIONS.MIN_NAMED_TEST_TAGS} of the staged test tags ` +
      `(${TEST_TAG_RANGE}) to decode; found [${foundTestTags.join(', ')}] ` +
      `among ${secondResult.uniqueCount} unique tags`
    ).toBeGreaterThanOrEqual(TEST_EXPECTATIONS.MIN_NAMED_TEST_TAGS);

    console.log(`[Test] Tag reads: first=${firstResult.count}, second=${secondResult.count}`);
  });

  test.skip('4. power setting affects RSSI @hardware', async () => {
    // This test navigates between tabs so handle that carefully
    await sharedPage.goto('/?tab=settings');
    await sharedPage.waitForTimeout(1000);
    
    const powerSlider = await sharedPage.locator('input[type="range"][min="0"][max="30"]');
    const sliderExists = await powerSlider.count() > 0;
    
    if (sliderExists) {
      // Set high power
      await powerSlider.fill('30');
      await sharedPage.waitForTimeout(500);
      
      await sharedPage.goto('/?tab=scan');
      await sharedPage.waitForTimeout(1000);
      
      // Clear and read
      await sharedPage.evaluate(() => {
        const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
        tagStore?.getState().clearTags();
      });
      
      await simulateTriggerPress(sharedPage);
      await sharedPage.waitForTimeout(3000);
      await simulateTriggerRelease(sharedPage);
      
      const highRSSI = await sharedPage.evaluate(() => {
        const tags = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore?.getState().tags || [];
        if (tags.length === 0) return null;
        return tags.reduce((sum, t) => sum + (t.rssi || 0), 0) / tags.length;
      });
      
      // Set low power
      await sharedPage.goto('/?tab=settings');
      await sharedPage.waitForTimeout(1000);
      await powerSlider.fill('10');
      await sharedPage.waitForTimeout(500);
      
      await sharedPage.goto('/?tab=scan');
      await sharedPage.waitForTimeout(1000);
      
      // Clear and read
      await sharedPage.evaluate(() => {
        const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
        tagStore?.getState().clearTags();
      });
      
      await simulateTriggerPress(sharedPage);
      await sharedPage.waitForTimeout(3000);
      await simulateTriggerRelease(sharedPage);
      
      const lowRSSI = await sharedPage.evaluate(() => {
        const tags = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore?.getState().tags || [];
        if (tags.length === 0) return null;
        return tags.reduce((sum, t) => sum + (t.rssi || 0), 0) / tags.length;
      });
      
      console.log(`[Test] RSSI: high=${highRSSI}, low=${lowRSSI}`);
      
      if (highRSSI !== null && lowRSSI !== null) {
        expect(highRSSI).toBeGreaterThan(lowRSSI);
      }
      
      // Reset to default
      await sharedPage.goto('/?tab=settings');
      await powerSlider.fill('25');
    } else {
      console.warn('[Test] Power slider not found');
    }
  });

  test.skip('5. packet parsing handles extended inventory @hardware @critical', async () => {
    // Monitor for parsing errors
    let parsingErrors = 0;
    sharedPage.on('console', msg => {
      const text = msg.text();
      if (text.includes('parse') && text.includes('error')) {
        parsingErrors++;
      }
    });
    
    // Clear tags
    await sharedPage.evaluate(() => {
      const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
      tagStore?.getState().clearTags();
    });
    
    // Run extended inventory
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(8000); // Long run
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(1000);
    
    // Check data integrity
    const result = await sharedPage.evaluate(() => {
      const tags = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore?.getState().tags || [];
      return {
        count: tags.length,
        totalReads: tags.reduce((sum, t) => sum + (t.count || 1), 0),
        validEPCs: tags.filter(t => t.epc && /^[0-9A-Fa-f]+$/.test(t.epc)).length,
        validRSSI: tags.filter(t => typeof t.rssi === 'number' && t.rssi < 0 && t.rssi > -100).length
      };
    });
    
    console.log(`[Test] Extended inventory: ${result.count} tags, ${result.totalReads} reads`);
    
    if (result.count > 0) {
      expect(result.validEPCs).toBe(result.count);
      expect(result.validRSSI).toBe(result.count);
    }
    
    expect(parsingErrors).toBe(0);
  });
});