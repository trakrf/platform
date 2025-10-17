/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Connection E2E Tests
 * Tests BLE connection management with real CS108 hardware
 * @hardware Requires physical CS108 device via bridge server
 *
 * IMPORTANT: Connection tests must NOT navigate to Inventory, Locate, or Barcode tabs
 * These tabs trigger mode changes which are heavyweight operations.
 * Only test on Home, Settings, or Help tabs to keep tests lightweight.
 */

import { test, expect, Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { expectConnectionState, expectBatteryPercentage } from './helpers/assertions';
import { setupConsoleMonitoring } from './helpers/console-utils';
import { getE2EConfig } from './e2e.config';
import {
  simulateTriggerPress,
  simulateTriggerRelease,
  getTriggerState
} from './helpers/trigger-utils';


// SKIP: Post-refactor - re-enabling tests one by one
const config = getE2EConfig();

test.describe('Connection Operations', () => {
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

    // Connect to device ONCE (from Home tab, not Inventory)
    // Stay on Home tab to avoid triggering INVENTORY mode
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
    // Core verification: connect, setMode(IDLE) is called, store receives updates
    const deviceState = await sharedPage.evaluate(() => {
      const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
      return deviceStore?.getState();
    });

    // Verify connection state
    expect(deviceState.isConnected).toBe(true);
    expect(deviceState.readerState).toBe('Connected');
    expect(deviceState.readerMode).toBe('Idle'); // setMode(IDLE) was called on connect

    // Verify battery level was received
    expect(deviceState.batteryPercentage).toBeGreaterThanOrEqual(0);
    expect(deviceState.batteryPercentage).toBeLessThanOrEqual(100);

    console.log(`Connected with battery: ${deviceState.batteryPercentage}%, mode: ${deviceState.readerMode}, state: ${deviceState.readerState}`);
  });

  test('should verify setMode calls and store updates on navigation @critical', async () => {
    // Verify Home → Settings navigation keeps IDLE mode
    // Core: setMode(IDLE) is called on tab changes, store gets proper updates

    // Navigate to Settings
    await sharedPage.click('button[data-testid="menu-item-settings"]');
    await sharedPage.waitForTimeout(500);

    const settingsState = await sharedPage.evaluate(() => {
      const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
      return {
        readerMode: deviceStore?.getState()?.readerMode,
        readerState: deviceStore?.getState()?.readerState
      };
    });

    // Should remain IDLE - Settings doesn't trigger mode change
    expect(settingsState.readerMode).toBe('Idle');
    expect(settingsState.readerState).toBe('Connected');

    // Navigate back to Home
    await sharedPage.click('button[data-testid="menu-item-home"]');
    await sharedPage.waitForTimeout(500);

    const homeState = await sharedPage.evaluate(() => {
      const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
      return {
        readerMode: deviceStore?.getState()?.readerMode,
        readerState: deviceStore?.getState()?.readerState
      };
    });

    // Still IDLE
    expect(homeState.readerMode).toBe('Idle');
    expect(homeState.readerState).toBe('Connected');
  });

  test('should update trigger state in store on press and release @hardware @critical', async () => {
    /**
     * Core verification: Trigger press/release updates triggerState in store
     * Stay on Home tab to avoid mode switches
     */

    // Ensure we're on Home tab (no mode switching)
    await sharedPage.click('button[data-testid="menu-item-home"]');
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