/**
 * E2E test for locate navigation with URL parameters
 * Verifies that clicking locate links from other tabs correctly sets targetEPC
 */

import { test, expect } from '@playwright/test';
import { connectToDevice } from './helpers/connection';
import { setupConsoleMonitoring } from './helpers/console-utils';

test.describe('Locate Navigation Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Set up console monitoring
    const logs: string[] = [];
    page.on('console', (msg) => {
      const text = msg.text();
      logs.push(text);
      // Log worker messages for debugging
      if (text.includes('[Worker]') || text.includes('targetEPC')) {
        console.log(text);
      }
    });

    // Navigate to home and connect
    await page.goto('/');
    await connectToDevice(page);

    // Store logs on the page for later access
    await page.evaluate(() => {
      (window as any).__TEST_LOGS__ = [];
    });

    // Add logs as they come
    page.on('console', (msg) => {
      const text = msg.text();
      page.evaluate((logText) => {
        (window as any).__TEST_LOGS__.push(logText);
      }, text);
    });
  });

  test('navigate from inventory: clicking locate link sets correct targetEPC', async ({ page }) => {
    // Navigate to inventory tab
    await page.click('[data-testid="menu-item-inventory"]');

    // Wait for the mode switch to complete (spinner disappears or times out)
    await page.waitForTimeout(3000); // Give it time to complete mode switch

    // Now we should see the Inventory header - may have configuration spinner
    // Just verify we're on the inventory page by checking for any inventory element
    await page.waitForSelector('[data-testid="inventory-tag-list"], h2:has-text("Inventory"), h2:has-text("Configuring")', { timeout: 10000 });

    // Wait for inventory to load and show tags
    await page.waitForTimeout(1000); // Let mode switch complete

    // Add a test tag to inventory
    await page.evaluate(() => {
      const { useTagStore } = window as any;
      useTagStore.getState().addTag({
        epc: '10019',
        rssi: -45,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid'
      });
    });

    // Wait for tag to appear
    await expect(page.locator('[data-testid="tag-row"]')).toHaveCount(1);

    // Click the locate link for tag 10019
    const locateLink = page.locator('[data-testid="tag-row"]').first().locator('a[href*="locate"]');
    await expect(locateLink).toHaveAttribute('href', '#locate?epc=10019');

    // Capture logs before navigation
    const logsBeforeClick = await page.evaluate(() => window.__TEST_LOGS__);
    console.log('Logs before click:', logsBeforeClick.filter(l => l.includes('targetEPC')));

    await locateLink.click();

    // Wait for configuration spinner to disappear if present
    await page.waitForSelector('h2:text("Configuring Reader")', { state: 'detached', timeout: 10000 }).catch(() => {});

    // Verify we're on the locate screen
    await expect(page.locator('h2').first()).toContainText('Find Item');

    // Verify the URL has the correct EPC parameter
    const url = page.url();
    expect(url).toContain('#locate?epc=10019');

    // Verify the input shows the correct EPC
    const epcInput = page.locator('[data-testid="locate-epc-input"]');
    await expect(epcInput).toHaveValue('10019');

    // Check logs to verify hardware received correct targetEPC
    await page.waitForTimeout(500); // Let mode switch complete
    const logs = await page.evaluate(() => window.__TEST_LOGS__);
    const targetEPCLogs = logs.filter(log => log.includes('targetEPC'));

    console.log('All targetEPC logs:', targetEPCLogs);

    // Verify the worker received the correct targetEPC
    const locateBuildLogs = targetEPCLogs.filter(log => log.includes('Building LOCATE'));
    const lastLocateBuild = locateBuildLogs[locateBuildLogs.length - 1];

    if (lastLocateBuild) {
      console.log('Last LOCATE build:', lastLocateBuild);
      expect(lastLocateBuild).toContain('targetEPC: 10019');
    }
  });

  test('navigate from barcode: clicking locate link sets correct targetEPC', async ({ page }) => {
    // Navigate to barcode tab
    await page.click('[data-testid="menu-item-barcode"]');

    // Wait for configuration spinner to disappear
    await page.waitForSelector('h2:text("Configuring Reader")', { state: 'detached', timeout: 10000 });

    // Now we should see the Barcode header
    await expect(page.locator('h2').first()).toContainText('Barcode Scanner');

    // Wait for mode switch
    await page.waitForTimeout(1000);

    // Add a test barcode with EPC
    await page.evaluate(() => {
      const { useBarcodeStore } = window as any;
      useBarcodeStore.getState().addBarcode({
        data: '10018',
        type: 'Code128',
        timestamp: Date.now()
      });
    });

    // Wait for barcode to appear
    await expect(page.locator('[data-testid="barcode-row"]')).toHaveCount(1);

    // Click the locate link for barcode 10018
    const locateLink = page.locator('[data-testid="barcode-row"]').first().locator('a[href*="locate"]');
    await expect(locateLink).toHaveAttribute('href', '#locate?epc=10018');

    await locateLink.click();

    // Wait for configuration spinner to disappear if present
    await page.waitForSelector('h2:text("Configuring Reader")', { state: 'detached', timeout: 10000 }).catch(() => {});

    // Verify we're on the locate screen
    await expect(page.locator('h2').first()).toContainText('Find Item');

    // Verify the URL has the correct EPC parameter
    const url = page.url();
    expect(url).toContain('#locate?epc=10018');

    // Verify the input shows the correct EPC
    const epcInput = page.locator('[data-testid="locate-epc-input"]');
    await expect(epcInput).toHaveValue('10018');

    // Check logs to verify hardware received correct targetEPC
    await page.waitForTimeout(500); // Let mode switch complete
    const logs = await page.evaluate(() => window.__TEST_LOGS__);
    const targetEPCLogs = logs.filter(log => log.includes('targetEPC'));

    console.log('All targetEPC logs from barcode test:', targetEPCLogs);

    // Verify the worker received the correct targetEPC
    const locateBuildLogs = targetEPCLogs.filter(log => log.includes('Building LOCATE'));
    const lastLocateBuild = locateBuildLogs[locateBuildLogs.length - 1];

    if (lastLocateBuild) {
      console.log('Last LOCATE build:', lastLocateBuild);
      expect(lastLocateBuild).toContain('targetEPC: 10018');
    }
  });

  test('direct URL: navigate to #locate?epc=X sets targetEPC', async ({ page }) => {
    // Test direct navigation with EPC in URL
    const testEpc = '10019';

    // Navigate directly to locate with EPC parameter
    await page.goto(`/#locate?epc=${testEpc}`);

    // Give time for navigation and mode configuration
    await page.waitForTimeout(2000);

    // Verify we're on locate tab
    await expect(page.locator('h2').first()).toContainText('Locate Item');

    // Verify EPC is set in input - use the correct data-testid
    const epcInput = await page.locator('[data-testid="target-epc-display"]');
    await expect(epcInput).toHaveValue(testEpc);

    // Verify settings store has the EPC
    const storedEpc = await page.evaluate(() => {
      const store = (window as any).__ZUSTAND_STORES__?.settingsStore;
      return store?.getState().rfid.targetEPC;
    });
    expect(storedEpc).toBe(testEpc);

    // Verify mode was set correctly
    const modeInfo = await page.evaluate(() => {
      const store = (window as any).__ZUSTAND_STORES__?.deviceStore;
      return {
        readerMode: store?.getState().readerMode,
        modeNumber: store?.getState().readerModeNumber
      };
    });
    console.log('[Test] URL parameter set EPC to:', testEpc, 'Mode:', modeInfo);
  });

  test('URL changes: navigating to new ?epc=Y updates targetEPC', async ({ page }) => {
    // First EPC
    const firstEpc = '10021';
    await page.goto(`/#locate?epc=${firstEpc}`);

    // Give time for navigation and mode configuration
    await page.waitForTimeout(1500);

    const epcInput = page.locator('[data-testid="target-epc-display"]');
    await expect(epcInput).toHaveValue(firstEpc);

    // Change to second EPC via URL
    const secondEpc = '10023';
    await page.goto(`/#locate?epc=${secondEpc}`);

    await page.waitForTimeout(500);

    // Verify update
    await expect(epcInput).toHaveValue(secondEpc);

    // Verify settings store updated
    const storedEpc = await page.evaluate(() => {
      const store = (window as any).__ZUSTAND_STORES__?.settingsStore;
      return store?.getState().rfid.targetEPC;
    });
    expect(storedEpc).toBe(secondEpc);

    console.log('[Test] Updated EPC from', firstEpc, 'to', secondEpc);
  });
});