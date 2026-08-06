/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Connection E2E Tests
 * Tests BLE connection management with real CS108 hardware
 * Requires physical CS108 device via bridge server
 *
 * IMPORTANT: reader mode follows the active tab (DeviceManager.resolveModeForTab).
 * Scan/Kits resolve to INVENTORY, Locate to LOCATE, Assets to BARCODE, and
 * everything else to IDLE. Since TRA-1029 the app lands on Scan, so connecting
 * from '/' configures INVENTORY — there is no longer a "connect in IDLE" tab.
 *
 * Mode changes are heavyweight on real hardware (~650ms teardown + reconfigure),
 * so keep tab hops to a minimum and always wait for the mode with
 * expectReaderMode() rather than a fixed sleep (TRA-1101).
 */

import { test, expect, Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { expectConnectionState, expectBatteryPercentage, expectReaderMode } from './helpers/assertions';
import { setupConsoleMonitoring } from './helpers/console-utils';
import { getReaderState } from './helpers/device-state';
import { getE2EConfig } from './e2e.config';
import {
  simulateTriggerPress,
  simulateTriggerRelease,
  getTriggerState
} from './helpers/trigger-utils';


// SKIP: Post-refactor - re-enabling tests one by one
const config = getE2EConfig();

test.describe('Connection Operations @hardware', () => {
  // Shared page instance for all tests in this suite
  let sharedPage: Page;
  let consoleMonitor: ReturnType<typeof setupConsoleMonitoring>;

  // Connect ONCE for all tests in this group
  test.beforeAll(async ({ browser }) => {
    console.log('🔧 [beforeAll] Setting up browser and connecting to device...');
    sharedPage = await browser.newPage();

    // Navigate to the app
    await sharedPage.goto('/');

    // Set up console monitoring
    consoleMonitor = setupConsoleMonitoring(sharedPage, {
      failOnErrors: ['Connection timeout', 'Transport error'],
      warnOnErrors: ['Failed to start battery auto reporting'],
      logAllErrors: true
    });

    // Connect to device ONCE. '/' resolves to the Scan tab (TRA-1029), so the
    // reader is configured for INVENTORY as part of the connect flow.
    await connectToDevice(sharedPage);
    console.log('✅ [beforeAll] Connected to device');
  });

  test.afterAll(async () => {
    console.log('🔧 [afterAll] Disconnecting from device...');
    if (sharedPage) {
      try {
        const isConnected = await sharedPage.evaluate(() => {
          const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
          return deviceStore?.getState()?.isConnected || false;
        });

        if (isConnected) {
          await disconnectDevice(sharedPage);
        }
      } catch (error) {
        console.log('Failed to disconnect:', error);
      }

      await sharedPage.close();
      console.log('✅ [afterAll] Disconnected and closed page');
    }
  });

  test('should connect and initialize with correct state @critical', async () => {
    // Core verification: connect resolves the mode from the active tab and the
    // store receives worker updates. Landing tab is Scan, so that is INVENTORY.
    await expectReaderMode(sharedPage, 'Inventory');

    const deviceState = await sharedPage.evaluate(() => {
      const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
      return deviceStore?.getState();
    });

    // Verify connection state
    expect(deviceState.isConnected).toBe(true);
    expect(deviceState.readerState).toBe('Connected');

    // Verify battery level was received
    expect(deviceState.batteryPercentage).toBeGreaterThanOrEqual(0);
    expect(deviceState.batteryPercentage).toBeLessThanOrEqual(100);

    console.log(`Connected with battery: ${deviceState.batteryPercentage}%, mode: ${deviceState.readerMode}, state: ${deviceState.readerState}`);
  });

  test('should verify setMode calls and store updates on navigation @critical', async () => {
    // Core: the reader mode tracks the active tab in both directions, and the
    // store gets the worker's mode updates. Leaving a scanning tab must power
    // the RF stage back down — a reader left in INVENTORY while the UI sits on
    // a non-scanning tab is battery drain plus unrequested reads.

    // Scan (INVENTORY) → Settings: reader must drop to IDLE
    await sharedPage.click('button[data-testid="menu-item-settings"]');
    await expectReaderMode(sharedPage, 'Idle');
    expect(await getReaderState(sharedPage)).toBe('Connected');

    // Settings → Help: both non-scanning, so IDLE is held (no mode churn)
    await sharedPage.click('button[data-testid="menu-item-help"]');
    await expectReaderMode(sharedPage, 'Idle');

    // Help → Scan: back to a scanning tab, reader reconfigures to INVENTORY
    await sharedPage.click('button[data-testid="menu-item-scan"]');
    await expectReaderMode(sharedPage, 'Inventory');

    // Return to Settings (IDLE) so the trigger test below starts from a
    // non-scanning tab and does not fight a mode change mid-test.
    await sharedPage.click('button[data-testid="menu-item-settings"]');
    await expectReaderMode(sharedPage, 'Idle');
    expect(await getReaderState(sharedPage)).toBe('Connected');
  });

  test('should update trigger state in store on press and release @critical', async () => {
    /**
     * Core verification: Trigger press/release updates triggerState in store
     * Stay on Settings tab to avoid mode switches
     */

    // Ensure we are on Settings tab (no mode switching)
    await sharedPage.click('button[data-testid="menu-item-settings"]');
    await sharedPage.waitForTimeout(500);

    // Get initial trigger state
    const initialTriggerState = await getTriggerState(sharedPage);
    console.log(`Initial trigger state: ${initialTriggerState}`);
    expect(initialTriggerState).toBe(false);

    // Simulate trigger press
    console.log('Simulating trigger press...');
    const result = await simulateTriggerPress(sharedPage);
    expect(result.success).toBe(true);

    // Verify trigger state changed
    const pressedState = await getTriggerState(sharedPage);
    console.log(`Trigger state after press: ${pressedState}`);
    expect(pressedState).toBe(true);

    // Hold trigger for a moment
    await sharedPage.waitForTimeout(1000);

    // Simulate trigger release
    console.log('Simulating trigger release...');
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);

    // Verify trigger state returned to false
    const releasedState = await getTriggerState(sharedPage);
    console.log(`Trigger state after release: ${releasedState}`);
    expect(releasedState).toBe(false);
  });

});