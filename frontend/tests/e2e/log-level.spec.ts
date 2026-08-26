/**
 * E2E test for worker log level settings
 * Verifies that the worker respects the configured log level
 */

import { test, expect } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';

test.describe('Worker Log Level', () => {
  // Every test here connects to real hardware; connect + RFID bring-up alone
  // costs ~20s, and each test disconnects too (TRA-1148 item 5).
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });


  test('@hardware respects log level settings', async ({ page }) => {
    // Collect console messages
    const consoleLogs: string[] = [];
    page.on('console', msg => {
      const text = msg.text();
      consoleLogs.push(text);
      // Log to see what's happening
      if (text.includes('[Worker]')) {
        console.log(`[Console ${msg.type()}] ${text}`);
      }
    });

    // Navigate to the app
    await page.goto('/');

    // Connect to device
    await connectToDevice(page);

    // Clear existing logs since connection generates DEBUG logs initially
    consoleLogs.length = 0;

    // Navigate to settings
    await page.click('[data-testid="menu-item-settings"]');
    await page.waitForTimeout(500);

    // Expand Advanced Settings section
    await page.click('button:has-text("Advanced Settings")');
    await page.waitForTimeout(200);

    // Find and set the Worker Log Level dropdown to WARN
    const logLevelSelect = page.getByTestId('worker-log-level');
    await logLevelSelect.selectOption('warn');

    // Verify setting was saved
    await expect(logLevelSelect).toHaveValue('warn');

    // Log level should be applied immediately
    await page.waitForTimeout(500);

    // Navigate to inventory to trigger some worker activity
    await page.click('[data-testid="menu-item-scan"]');
    await page.waitForTimeout(2000);

    // Count DEBUG logs after setting WARN level
    const debugLogsAfterWarn = consoleLogs.filter(log =>
      log.includes('[Worker] DEBUG') ||
      log.includes('[Worker] TRACE') ||
      log.includes('[Worker] HEX')
    );

    console.log(`DEBUG logs after setting WARN: ${debugLogsAfterWarn.length}`);

    // Should not see DEBUG messages with WARN level
    expect(debugLogsAfterWarn.length).toBe(0);

    // But we should see INFO/WARN/ERROR messages (at least the "Log level set to WARN" message)
    const infoLogsAfterWarn = consoleLogs.filter(log =>
      log.includes('[Worker] INFO') ||
      log.includes('[Worker] WARN') ||
      log.includes('[Worker] ERROR')
    );

    console.log(`INFO/WARN/ERROR logs after setting WARN: ${infoLogsAfterWarn.length}`);
    expect(infoLogsAfterWarn.length).toBeGreaterThan(0);

    // Now switch to DEBUG level
    consoleLogs.length = 0;

    await page.click('[data-testid="menu-item-settings"]');
    await page.waitForTimeout(500);

    // Find the select again after navigating back
    const logLevelSelect2 = page.getByTestId('worker-log-level');
    await logLevelSelect2.selectOption('debug');
    await expect(logLevelSelect2).toHaveValue('debug');
    await page.waitForTimeout(500);

    // Navigate to locate to trigger worker activity
    await page.click('[data-testid="menu-item-locate"]');
    await page.waitForTimeout(2000);

    // Count DEBUG logs after setting DEBUG level
    const debugLogsAfterDebug = consoleLogs.filter(log =>
      log.includes('[Worker] DEBUG')
    );

    console.log(`DEBUG logs after setting DEBUG: ${debugLogsAfterDebug.length}`);

    // Should now see DEBUG messages
    expect(debugLogsAfterDebug.length).toBeGreaterThan(0);

    // Clean up
    await disconnectDevice(page);
  });

  test('@hardware persists log level setting across page refresh', async ({ page }) => {
    await page.goto('/');
    await connectToDevice(page);

    // Navigate to settings
    await page.click('[data-testid="menu-item-settings"]');
    await page.waitForTimeout(500);

    // Expand Advanced Settings section
    await page.click('button:has-text("Advanced Settings")');
    await page.waitForTimeout(200);

    // Find and set the Worker Log Level dropdown to ERROR
    const logLevelSelect = page.getByTestId('worker-log-level');
    await logLevelSelect.selectOption('error');
    await expect(logLevelSelect).toHaveValue('error');

    // Disconnect before refresh
    await disconnectDevice(page);

    // Refresh the page
    await page.reload();
    await page.waitForTimeout(1000);

    // Reconnect to device
    await connectToDevice(page);

    // Navigate back to settings
    await page.click('[data-testid="menu-item-settings"]');
    await page.waitForTimeout(500);

    // Expand Advanced Settings section
    await page.click('button:has-text("Advanced Settings")');
    await page.waitForTimeout(200);

    // Verify the setting persisted
    const logLevelSelectAfter = page.getByTestId('worker-log-level');
    await expect(logLevelSelectAfter).toHaveValue('error');

    // Clean up
    await disconnectDevice(page);
  });
});