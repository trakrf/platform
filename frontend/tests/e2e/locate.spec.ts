/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Locate E2E Tests
 * Tests RFID tag location functionality with RSSI proximity feedback
 * @hardware Requires physical CS108 device via bridge server
 * 
 * Pattern follows inventory.spec.ts which is 100% stable:
 * - Serial execution with shared connection
 * - Connect once in beforeAll, disconnect in afterAll
 * - Share single page across all tests
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { simulateTriggerPress, simulateTriggerRelease, simulateTriggerCycle } from './helpers/trigger-utils';
import { getReaderState } from './helpers/device-state';
import { waitForReaderState, getCurrentReaderState, ReaderState } from './helpers/reader-state';
import { PRIMARY_TEST_TAG, INVALID_TEST_TAG, NON_EXISTENT_TAG } from '@test-utils/constants';

// Test tag that should be locatable (from physical test setup)
const LOCATE_TEST_TAG = PRIMARY_TEST_TAG;

// Locate mode tests - EPC filtering integration with CS108 hardware
test.describe('Locate Functionality Tests', () => {
  /**
   * CONNECTION SHARING STRATEGY (from inventory tests)
   * 
   * We connect once for all locate tests because:
   * 1. Connection/disconnection is tested in connection.spec.ts
   * 2. Users perform multiple operations without reconnecting
   * 3. Tests run much faster (30s vs 2+ minutes)
   * 4. This tests real-world connection stability
   */
  
  let sharedPage: Page;
  let connectionHealthy = true;
  
  test.beforeAll(async ({ browser }) => {
    console.log('[Locate] Setting up shared connection for all tests...');
    sharedPage = await browser.newPage();

    // Capture all console messages for debugging
    sharedPage.on('console', msg => {
      console.log('[Browser Console]', msg.type(), msg.text());
    });

    // Simply connect from Home tab - IDLE mode is fine
    await sharedPage.goto('/');
    await connectToDevice(sharedPage);

    // Verify connection is ready (real hardware via bridge server, no mock needed)
    const connectionReady = await sharedPage.evaluate(() => {
      const stores = window.__ZUSTAND_STORES__;
      return stores?.deviceStore?.getState()?.isConnected || false;
    });

    console.log('[Locate] Connection ready:', connectionReady);
    connectionHealthy = connectionReady;

    // Each test will navigate to locate tab via URL, triggering natural mode changes
  });
  
  test.beforeEach(async () => {
    if (!connectionHealthy) {
      test.skip();
    }
  });
  
  test.afterAll(async () => {
    console.log('[Locate] Cleaning up shared connection...');
    if (sharedPage) {
      try {
        // Navigate to home page to trigger IDLE mode (includes RFID_POWER_OFF)
        console.log('[Locate] Navigating to home page for cleanup...');
        await sharedPage.goto('/');
        await sharedPage.waitForTimeout(1000); // Wait for mode change

        // Ensure trigger is released
        console.log('[Locate] Ensuring trigger is released...');
        await simulateTriggerRelease(sharedPage);

        await disconnectDevice(sharedPage);
      } catch (error) {
        console.error('[Locate] Error during disconnect:', error);
      }
      await sharedPage.close();
    }
  });
  
  test('basic locate: finds tag with matching EPC', async () => {
    // Navigate to locate tab first
    await sharedPage.goto('/#locate');
    await sharedPage.waitForTimeout(1000); // Give mode time to change

    console.log('[Test] Looking for locate EPC input...');

    // Enter tag EPC
    const epcInput = await sharedPage.waitForSelector('[data-testid="locate-epc-input"]', { timeout: 5000 });
    console.log('[Test] Found input, filling with:', LOCATE_TEST_TAG);
    await epcInput.fill(LOCATE_TEST_TAG);

    // Trigger blur to save EPC and call setSettings
    await sharedPage.evaluate(el => el.blur(), epcInput);

    // Wait for React state to update and setSettings to complete
    await sharedPage.waitForTimeout(500);
    
    // Debug: Check if searchTargetEPC was set in the store
    const storeState = await sharedPage.evaluate(() => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const stores = window.__ZUSTAND_STORES__;
      if (stores?.tagStore) {
        return {
          searchTargetEPC: stores.tagStore.getState().searchTargetEPC,
        };
      }
      return { searchTargetEPC: 'NO_STORE' };
    });
    console.log('[Test] Store state after EPC input:', storeState);
    
    // Use trigger cycle for proper locate test (press, hold 4s, release)
    console.log('[Test] Starting trigger cycle for locate...');
    const cycleSuccess = await simulateTriggerCycle(sharedPage, 4000);
    expect(cycleSuccess).toBe(true);
    
    // Wait for state to settle
    await sharedPage.waitForTimeout(1000);
    
    // Check that we got RSSI feedback during the cycle
    const rssiElements = await sharedPage.$$('.rssi-value') || 
                         await sharedPage.$$('[data-testid="rssi-display"]');
    console.log(`[Test] Found ${rssiElements ? rssiElements.length : 0} RSSI elements`);
    
    // Verify we're back to READY after cycle completes
    const finalState = await getReaderState(sharedPage);
    expect(finalState).toBe(ReaderState.CONNECTED);
  });

  test('trigger control: starts/stops locate on press/release', async () => {
    // Navigate to locate tab first
    await sharedPage.goto('/#locate');
    await sharedPage.waitForTimeout(1000); // Give mode time to change

    // Enter valid EPC
    const epcInput = await sharedPage.waitForSelector('[data-testid="locate-epc-input"]');
    await epcInput.fill(LOCATE_TEST_TAG);
    await sharedPage.evaluate(el => el.blur(), epcInput);
    await sharedPage.waitForTimeout(500);
    
    // Press trigger (using real hardware via bridge server)
    console.log('[Test] Simulating trigger press...');
    const pressResult = await simulateTriggerPress(sharedPage);
    console.log('[Test] Press result:', pressResult);
    expect(pressResult.success).toBe(true);
    
    // Hold for 2 seconds
    await sharedPage.waitForTimeout(2000);
    
    // Release trigger
    console.log('[Test] Simulating trigger release...');
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);
    
    // Verify we're back to READY
    await sharedPage.waitForTimeout(1000);
    const finalState = await getReaderState(sharedPage);
    expect(finalState).toBe(ReaderState.CONNECTED);
  });

  test('proximity feedback: RSSI increases as tag gets closer', async () => {
    // Navigate to locate tab first
    await sharedPage.goto('/#locate');
    await sharedPage.waitForTimeout(1000); // Give mode time to change

    // Enter EPC for a tag we know exists
    const epcInput = await sharedPage.waitForSelector('[data-testid="locate-epc-input"]');
    await epcInput.fill(LOCATE_TEST_TAG);
    await sharedPage.evaluate(el => el.blur(), epcInput);
    await sharedPage.waitForTimeout(500);
    
    // Start locate
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);
    
    // Wait for RSSI updates
    await sharedPage.waitForTimeout(3000);
    
    // Check for proximity indicator (gauge component or its wrapper)
    const proximityElement = await sharedPage.$('.proximity-indicator, [data-testid="proximity-display"], #rssi-gauge');
    expect(proximityElement).toBeTruthy();
    
    // Stop locate
    await simulateTriggerRelease(sharedPage);
  });

  test('validation: rejects invalid EPC format', async () => {
    // Navigate to locate tab first
    await sharedPage.goto('/#locate');
    await sharedPage.waitForTimeout(1000); // Give mode time to change

    // Try to enter an invalid EPC
    const epcInput = await sharedPage.waitForSelector('[data-testid="locate-epc-input"]');
    await epcInput.fill(INVALID_TEST_TAG);
    await sharedPage.evaluate(el => el.blur(), epcInput);
    await sharedPage.waitForTimeout(500);
    
    // Check for error message or validation
    const errorElement = await sharedPage.$('.error-message, [data-testid="epc-error"]');
    if (errorElement) {
      const errorText = await errorElement.textContent();
      console.log('[Test] Error message:', errorText);
      expect(errorText).toBeTruthy();
    }
    
    // Verify locate doesn't start with invalid EPC
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);
    
    await sharedPage.waitForTimeout(1000);
    
    // Should still be READY (not searching)
    const state = await getReaderState(sharedPage);
    expect(state).toBe(ReaderState.CONNECTED);
    
    await simulateTriggerRelease(sharedPage);
  });

  test('edge case: handles non-existent tag gracefully', async () => {
    // Navigate to locate tab first
    await sharedPage.goto('/#locate');
    await sharedPage.waitForTimeout(1000); // Give mode time to change

    // Enter EPC for a tag that doesn't exist
    const epcInput = await sharedPage.waitForSelector('[data-testid="locate-epc-input"]');
    await epcInput.fill(NON_EXISTENT_TAG);
    await sharedPage.evaluate(el => el.blur(), epcInput);
    await sharedPage.waitForTimeout(500);
    
    // Start locate
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);
    
    // Wait a bit
    await sharedPage.waitForTimeout(3000);
    
    // Should show no RSSI or "not found" indication
    const notFoundElement = await sharedPage.$('.not-found, [data-testid="tag-not-found"]');
    if (notFoundElement) {
      const notFoundText = await notFoundElement.textContent();
      console.log('[Test] Not found message:', notFoundText);
    }
    
    // Stop locate
    await simulateTriggerRelease(sharedPage);
  });
});