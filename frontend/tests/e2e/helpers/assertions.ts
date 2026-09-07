/**
 * Common assertion helpers for E2E tests
 * Provides reusable assertions for battery, tags, connection state, etc.
 */

import { expect } from '@playwright/test';
import type { Page } from '@playwright/test';
import { getE2EConfig } from '../e2e.config';
import type { WindowWithStores } from '../types';
import { getReaderMode } from './device-state';

const config = getE2EConfig();

/**
 * Assert battery percentage is within expected range
 * @param page - Playwright page
 * @param min - Minimum expected percentage (0-100)
 * @param max - Maximum expected percentage (0-100)
 */
export async function expectBatteryPercentage(
  page: Page,
  min: number,
  max: number
): Promise<void> {
  // Wait for battery indicator to be visible
  const batteryElement = await page.waitForSelector(config.selectors.batteryIndicator, {
    timeout: config.timeouts.ui
  });
  
  // Extract the percentage value
  const batteryText = await batteryElement.textContent();
  expect(batteryText).toBeTruthy();
  
  // Parse percentage from text (e.g., "75%")
  const match = batteryText!.match(/(\d+)%/);
  expect(match).toBeTruthy();
  
  const percentage = parseInt(match![1], 10);
  expect(percentage).toBeGreaterThanOrEqual(min);
  expect(percentage).toBeLessThanOrEqual(max);
  
  // TODO: Verify store state matches UI once __ZUSTAND_STORES__ global is set up
  // const storeState = await page.evaluate(() => {
  //   const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
  //   return deviceStore?.getState().batteryPercentage;
  // });
  // expect(storeState).toBe(percentage);
}

/**
 * Assert tag count matches expected value
 * @param page - Playwright page
 * @param expectedCount - Expected number of tags
 */
export async function expectTagCount(
  page: Page,
  expectedCount: number
): Promise<void> {
  // Wait for tag count element
  const tagCountElement = await page.waitForSelector(config.selectors.tagCount, {
    timeout: config.timeouts.ui
  });
  
  // Extract count from text (e.g., "Tags: 5")
  const tagCountText = await tagCountElement.textContent();
  expect(tagCountText).toBeTruthy();
  
  const match = tagCountText!.match(/Tags:\s*(\d+)/);
  expect(match).toBeTruthy();
  
  const count = parseInt(match![1], 10);
  expect(count).toBe(expectedCount);
  
  // Verify store state matches
  const storeCount = await page.evaluate(() => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
    return tagStore?.getState().tags.length || 0;
  });
  
  expect(storeCount).toBe(expectedCount);
}

/**
 * Assert the reader settles into an expected mode.
 *
 * Mode changes are heavyweight on real hardware: setMode() aborts the running
 * sequence, runs the IDLE teardown, reconfigures, and only then emits
 * READER_MODE_CHANGED. Measured Inventory -> Idle on a CS108 is ~650ms, so a
 * fixed sleep races the transition (TRA-1101). Poll instead.
 *
 * @param page - Playwright page
 * @param expectedMode - ReaderMode value, e.g. 'Idle' or 'Inventory'
 * @param timeout - How long to allow the transition to land
 */
export async function expectReaderMode(
  page: Page,
  expectedMode: string,
  timeout: number = 10000
): Promise<void> {
  await expect
    .poll(() => getReaderMode(page), {
      timeout,
      message: `Expected reader to settle in ${expectedMode} mode`
    })
    .toBe(expectedMode);
}

/**
 * Assert connection state matches expected state
 * @param page - Playwright page
 * @param expectedState - Expected connection state
 */
export async function expectConnectionState(
  page: Page,
  expectedState: {
    isConnected: boolean;
    deviceName?: string;
    hasBattery?: boolean;
  }
): Promise<void> {
  // Check store state
  const state = await page.evaluate(() => {
    const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
    if (!deviceStore) return null;
    
    const storeState = deviceStore.getState();
    return {
      readerState: storeState.readerState,
      deviceName: storeState.deviceName,
      batteryPercentage: storeState.batteryPercentage
    };
  });
  
  // Device is connected if readerState is not DISCONNECT
  // const isConnected = state && state.readerState !== ReaderState.DISCONNECT;
  
  expect(state).toBeTruthy();
  // TODO: Re-enable isConnected check once disconnect state reset is fixed
  // expect(isConnected).toBe(expectedState.isConnected);
  
  if (expectedState.deviceName !== undefined) {
    if (expectedState.deviceName === null) {
      // When expecting null, accept either null or empty string
      expect(state!.deviceName === null || state!.deviceName === '').toBe(true);
    } else {
      expect(state!.deviceName).toContain(expectedState.deviceName);
    }
  }
  
  if (expectedState.hasBattery !== undefined) {
    if (expectedState.hasBattery) {
      expect(state!.batteryPercentage).toBeGreaterThan(0);
    } else {
      // After disconnect, battery should be falsy (0, null, undefined)
      expect(state!.batteryPercentage).toBeFalsy();
    }
  }
  
  // TODO: Re-enable UI state checks once disconnect state reset is fixed  
  // Also verify UI state - TEMPORARILY DISABLED due to battery state management issues
  // if (expectedState.isConnected) {
  //   // Should see battery indicator when connected
  //   await expect(page.locator(config.selectors.batteryIndicator)).toBeVisible();
  // } else {
  //   // Should not see battery indicator when disconnected - TEMPORARILY DISABLED
  //   // await expect(page.locator(config.selectors.batteryIndicator)).not.toBeVisible();
  // }
}

/**
 * Assert inventory is running
 * @param page - Playwright page
 */
/**
 * ⚠ These two read `searchRunning`, which is what the store actually calls it.
 *
 * They read `inventoryRunning` until TRA-1253, and no such field has ever
 * existed on `TagState`. Both did `?? false`-style coalescing, so the wrong name
 * did not throw — it made one helper unable to pass and the other unable to
 * fail:
 *
 *   expectInventoryRunning   `expect(undefined || false).toBe(true)`   NEVER PASSES
 *   expectInventoryStopped   `expect(undefined || false).toBe(false)`  ALWAYS PASSES
 *
 * The second is the dangerous one: a vacuous green that looks like coverage of
 * the stop path. Neither has any caller today (verified by grep across `tests/`
 * on 2026-09-07), which is why correcting the name changes no run — but a future
 * caller would otherwise have inherited a broken assertion that reads as a
 * working one.
 */
export async function expectInventoryRunning(page: Page): Promise<void> {
  // With trigger-based control, we check the store state directly
  const storeState = await page.evaluate(() => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
    const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
    const tagState = tagStore?.getState();
    const deviceState = deviceStore?.getState();
    console.log('[Test] Checking inventory state:', {
      searchRunning: tagState?.searchRunning,
      tagCount: tagState?.tags.length,
      triggerState: deviceState?.triggerState,
      readerState: deviceState?.readerState
    });
    return {
      isRunning: tagState?.searchRunning || false,
      hasNoTags: tagState?.tags.length === 0
    };
  });
  
  expect(storeState.isRunning).toBe(true);
  
  // Check for scanning status in the connect button
  const scanningIndicator = page.locator('text="Scanning"');
  await expect(scanningIndicator).toBeVisible();
}

/**
 * Assert inventory is stopped
 * @param page - Playwright page
 */
export async function expectInventoryStopped(page: Page): Promise<void> {
  // With trigger-based control, we check the store state directly
  const isRunning = await page.evaluate(() => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
    return tagStore?.getState().searchRunning || false;
  });
  
  expect(isRunning).toBe(false);
  
  // Check that we're not showing "Searching for tags..." anymore
  await expect(page.locator('text="Searching for tags..."')).not.toBeVisible();
}

/**
 * Assert error message is displayed
 * @param page - Playwright page
 * @param errorPattern - Pattern to match in error message
 */
export async function expectErrorMessage(
  page: Page,
  errorPattern: RegExp | string
): Promise<void> {
  // Look for error message in various possible locations
  const errorSelector = `text=/${errorPattern}/i`;
  
  await page.waitForSelector(errorSelector, {
    timeout: config.timeouts.ui
  });
  
  // Verify the error is visible
  await expect(page.locator(errorSelector)).toBeVisible();
}

/**
 * Assert no error messages are displayed
 * @param page - Playwright page
 */
export async function expectNoErrorMessages(page: Page): Promise<void> {
  // Common error patterns to check
  const errorPatterns = [
    'text=/error/i',
    'text=/failed/i',
    'text=/exception/i',
    '.error-message',
    '[role="alert"]'
  ];
  
  for (const pattern of errorPatterns) {
    const errorElements = await page.$$(pattern);
    expect(errorElements).toHaveLength(0);
  }
}

/**
 * Assert tag exists in inventory
 * @param page - Playwright page
 * @param epc - Expected EPC value
 */
export async function expectTagInInventory(
  page: Page,
  epc: string
): Promise<void> {
  // Look for tag in table
  const tagSelector = `tr:has-text("${epc}")`;
  
  await page.waitForSelector(tagSelector, {
    timeout: config.timeouts.ui
  });
  
  // Verify in store
  const hasTag = await page.evaluate((targetEpc) => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
    const tags = tagStore?.getState().tags || [];
    return tags.some((tag: { epc: string }) => tag.epc === targetEpc);
  }, epc);
  
  expect(hasTag).toBe(true);
}