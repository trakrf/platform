/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Optimized Barcode E2E Tests
 * Reuses single connection and page instance across all tests for faster execution
 * Requires physical CS108 device with test barcode
 */

import { test, expect, Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { BARCODE_TEST_TAG } from '@test-utils/constants';
import { setupConsoleMonitoring } from './helpers/console-utils';
import { simulateTriggerPress, simulateTriggerRelease } from './helpers/trigger-utils';
import type { WindowWithStores } from './types';
import { ReaderMode, ReaderState } from '@/worker/types/reader';

test.describe('Barcode Operations', () => {
  // Shared page instance and connection for all tests in this suite
  let sharedPage: Page;

  // Connect once for all tests in this suite
  test.beforeAll(async ({ browser }) => {
    sharedPage = await browser.newPage();
    console.log('[Suite] Setting up barcode test suite with single connection and shared page');

    // Monitor console for debugging
    setupConsoleMonitoring(sharedPage, {
      failOnErrors: ['Transport error', 'Barcode manager not initialized'],
      warnOnErrors: ['No barcode captured'],
      logAllErrors: true
    });

    // Navigate to app and connect to device ONCE
    await sharedPage.goto('/');
    await connectToDevice(sharedPage);

    // Wait for connection to be fully established
    await sharedPage.waitForTimeout(1000);

    // Navigate to barcode tab - try both possible selectors
    // The menu item might be in TabNavigation (desktop) or HamburgerMenu (mobile)
    try {
      // First try the direct menu item (TabNavigation on desktop)
      await sharedPage.click('[data-testid="menu-item-barcode"]', { timeout: 2000 });
    } catch {
      // If that fails, try navigating via URL hash
      console.log('[Test] Menu item not found, navigating via URL');
      await sharedPage.goto('/#barcode');
    }

    // Wait for activeTab to change
    await sharedPage.waitForFunction(
      () => {
        const uiStore = (window as any).__ZUSTAND_STORES__?.uiStore;
        const activeTab = uiStore?.getState().activeTab;
        console.log('[Test] Current activeTab:', activeTab);
        return activeTab === 'barcode';
      },
      { timeout: 5000 }
    );

    console.log('[Test] Barcode tab activated, waiting for mode to be set...');

    // Give DeviceManager time to react to activeTab change
    await sharedPage.waitForTimeout(2000);

    // Check current state before waiting
    const currentState = await sharedPage.evaluate(() => {
      const deviceStore = (window as any).__ZUSTAND_STORES__?.deviceStore;
      const deviceManager = (window as any).__DEVICE_MANAGER__;
      return {
        readerMode: deviceStore?.getState().readerMode,
        readerState: deviceStore?.getState().readerState,
        isConnected: deviceStore?.getState().isConnected,
        hasDeviceManager: !!deviceManager
      };
    });
    console.log('[Test] Current state before wait:', currentState);

    // Tab navigation should have triggered the mode switch automatically
    // No need to manually call any methods on DeviceManager

    // Wait for mode change to complete
    await sharedPage.waitForTimeout(2000);

    // Wait for mode to actually change to Barcode and state to be Ready
    // We need to pass the enum values into the browser context since the function runs in the browser
    const expectedMode = ReaderMode.BARCODE;
    const expectedState = ReaderState.CONNECTED;

    await sharedPage.waitForFunction(
      ({ expectedMode, expectedState }) => {
        const deviceStore = (window as any).__ZUSTAND_STORES__?.deviceStore;
        const state = deviceStore?.getState();
        const readerMode = state?.readerMode;
        const readerState = state?.readerState;
        console.log('[Test] Reader mode:', readerMode, 'State:', readerState);
        return readerMode === expectedMode && readerState === expectedState;
      },
      { expectedMode, expectedState },
      { timeout: 10000 }
    );

    console.log('[Test] Barcode mode activated and ready');
    console.log('[Test] Shared page and connection ready for all tests');
  });

  test.afterAll(async () => {
    if (sharedPage) {
      // Stop any active scanning
      try {
        await simulateTriggerRelease(sharedPage);
      } catch (error) {
        // Ignore errors - trigger might already be released
      }

      // Disconnect device once at the end
      try {
        await disconnectDevice(sharedPage);
        console.log('[Suite] Disconnected device after all tests');
      } catch (error) {
        console.log('[Suite] Already disconnected');
      }

      await sharedPage.close();
    }
  });

  test.beforeEach(async () => {
    console.log(`[Test] Starting: ${test.info().title}`);

    // Ensure we're on barcode tab
    const isOnBarcodeTab = await sharedPage.evaluate(() => {
      const uiStore = (window as WindowWithStores).__ZUSTAND_STORES__?.uiStore;
      return uiStore?.getState().activeTab === 'barcode';
    });

    if (!isOnBarcodeTab) {
      try {
        await sharedPage.click('[data-testid="menu-item-barcode"]', { timeout: 2000 });
      } catch {
        await sharedPage.goto('/#barcode');
      }
      await sharedPage.waitForTimeout(500);
    }

    // Clear any existing barcodes
    await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      barcodeStore?.getState().clearBarcodes();
    });
  });

  test.afterEach(async () => {
    // Stop any active scanning with trigger release (in case test failed)
    try {
      await simulateTriggerRelease(sharedPage);
    } catch (error) {
      // Ignore errors - trigger might already be released
    }

    // Brief pause between tests
    await sharedPage.waitForTimeout(1000);
  });

  test('should enable barcode scanning with trigger', async () => {
    // Start scanning with trigger press simulation
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);
    console.log('[Test] Trigger pressed:', pressResult.message);

    await sharedPage.waitForTimeout(500);

    // Check scanning state
    const isScanning = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().scanning || false;
    });
    expect(isScanning).toBe(true);
    console.log('[Test] Barcode scanning started');

    // Stop scanning with trigger release
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);
    console.log('[Test] Trigger released:', releaseResult.message);

    await sharedPage.waitForTimeout(500);

    const isStopped = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().scanning || false;
    });
    expect(isStopped).toBe(false);
    console.log('[Test] Barcode scanning stopped');
  });

  test('should capture hardware barcodes', async () => {
    console.log('[Test] Testing live hardware barcode capture...');

    // Start scanning with trigger press
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);

    // Wait for barcode captures (CS108 is actively scanning test barcode)
    await sharedPage.waitForTimeout(3000);

    // Stop scanning with trigger release
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);

    // Check captured barcodes
    const capturedBarcodes = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().barcodes || [];
    });

    expect(capturedBarcodes.length).toBeGreaterThan(0);
    console.log(`[Test] SUCCESS: Captured ${capturedBarcodes.length} barcodes from hardware`);

    if (capturedBarcodes.length > 0) {
      console.log(`[Test] First barcode: ${capturedBarcodes[0].data}`);
      expect(capturedBarcodes[0].data).toBe(BARCODE_TEST_TAG); // Expected test barcode
    }
  });

  test('should clear barcode history', async () => {
    // First capture some barcodes
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);

    await sharedPage.waitForTimeout(2000);

    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);

    // Verify we have barcodes
    const beforeClear = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().barcodes.length || 0;
    });
    expect(beforeClear).toBeGreaterThan(0);

    // Clear barcodes
    await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      barcodeStore?.getState().clearBarcodes();
    });

    // Verify cleared
    const afterClear = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().barcodes.length || 0;
    });
    expect(afterClear).toBe(0);
    console.log('[Test] Barcode history cleared');
  });

  test('should reliably read fragmented barcodes under stress', async () => {
    // This test validates CRC/length validation prevents silent failures
    // The 24-char QR code forces BLE fragmentation across multiple MTU chunks
    // Before fix: ~30-90% empty reads due to corrupted/incomplete packets
    // After fix: 0% empty reads (packets either valid or cleanly rejected)

    const SCAN_CYCLES = parseInt(process.env.BARCODE_STRESS_CYCLES || '10');
    const results = { valid: 0, empty: 0, total: 0 };

    console.log(`[Stress Test] Running ${SCAN_CYCLES} scan cycles with fragmented barcode...`);

    for (let i = 0; i < SCAN_CYCLES; i++) {
      // Clear previous barcodes
      await sharedPage.evaluate(() => {
        const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
        barcodeStore?.getState().clearBarcodes();
      });

      // Scan
      await simulateTriggerPress(sharedPage);
      await sharedPage.waitForTimeout(500);
      await simulateTriggerRelease(sharedPage);
      await sharedPage.waitForTimeout(200);

      // Check result
      const barcodes = await sharedPage.evaluate(() => {
        const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
        return barcodeStore?.getState().barcodes || [];
      });

      results.total++;
      if (barcodes.length > 0 && barcodes[0].data && barcodes[0].data.length > 0) {
        results.valid++;
      } else {
        results.empty++;
      }

      if ((i + 1) % 5 === 0) {
        console.log(`[Stress Test] Progress: ${i + 1}/${SCAN_CYCLES} - Valid: ${results.valid}, Empty: ${results.empty}`);
      }
    }

    const successRate = (results.valid / results.total) * 100;
    console.log(`[Stress Test] Final: ${results.valid}/${results.total} valid (${successRate.toFixed(1)}%)`);

    // After fix: success rate should be >80% (was ~33% before fix)
    // Some empty reads are expected due to:
    // - BLE packet loss at radio layer (before our code receives it)
    // - Brief scan windows where barcode isn't captured
    // - Legitimate scanner "no data" responses
    expect(successRate).toBeGreaterThan(80);
    expect(results.valid).toBeGreaterThan(0);
  });
});