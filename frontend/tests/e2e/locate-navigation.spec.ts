/**
 * E2E test for locate navigation with URL parameters
 * Verifies that clicking locate links from other tabs correctly sets targetEPC
 */

import { test, expect } from '@playwright/test';
import { connectToDevice } from './helpers/connection';
import { setupConsoleMonitoring } from './helpers/console-utils';
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';

test.describe('Locate Navigation Tests @hardware', () => {
  // Real hardware: connect + RFID bring-up alone costs ~20s (TRA-1148 item 5)
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

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
    await page.click('[data-testid="menu-item-scan"]');

    // Wait for the mode switch to complete (spinner disappears or times out)
    await page.waitForTimeout(3000); // Give it time to complete mode switch

    // The tab is titled by PAGE_TITLES in the header and is named "Scan", not
    // "Inventory" (TRA-1029). The old selector waited on an `h2:has-text("Inventory")`
    // and a `inventory-tag-list` test id, neither of which exists.
    await expect(page.getByTestId('page-title')).toContainText('Scan', { timeout: 10000 });

    // Wait for inventory to load and show tags
    await page.waitForTimeout(1000); // Let mode switch complete

    // Add a test tag to inventory. Stores are exposed as
    // `window.__ZUSTAND_STORES__.tagStore` (main.tsx) — there is no bare
    // `window.useTagStore`, so the old form threw on `.getState()` (TRA-1088).
    await page.evaluate(() => {
      const tagStore = window.__ZUSTAND_STORES__!.tagStore;
      tagStore.getState().addTag({
        epc: '10019',
        rssi: -45,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid'
      });
    });

    // Wait for the row to render. `tag-row` never existed on the desktop table;
    // the row's locate control is the stable handle. Both the mobile card and
    // the desktop row are in the DOM at once (Tailwind hides one), so match on
    // `:visible` — an unqualified test id resolves to 2 elements.
    const locateButton = page.locator('[data-testid="locate-button"]:visible');
    await expect(locateButton).toHaveCount(1);

    // The locate affordance is a <button> that assigns window.location.hash, not
    // an <a href>. The old test asserted toHaveAttribute('href', …), which no
    // element here can satisfy — assert the resulting navigation instead (below).

    // Capture logs before navigation
    const logsBeforeClick = await page.evaluate(() => window.__TEST_LOGS__);
    console.log('Logs before click:', logsBeforeClick.filter(l => l.includes('targetEPC')));

    await locateButton.click();

    // Wait for configuration spinner to disappear if present
    await page.waitForSelector('h2:text("Configuring Reader")', { state: 'detached', timeout: 10000 }).catch(() => {});

    // Verify we're on the locate screen. The screen no longer prints its own
    // heading (TRA-1071) — the header title is the page identity now.
    await expect(page.getByTestId('page-title')).toContainText('Locate');

    // Verify the URL has the correct EPC parameter
    const url = page.url();
    expect(url).toContain('#locate?epc=10019');

    // Verify the input shows the correct EPC. The test id is `target-epc-display`
    // — `locate-epc-input` has never existed (TRA-1088).
    const epcInput = page.locator('[data-testid="target-epc-display"]');
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

  test('navigate from inventory: a 128-bit tag deep-links its full-width EPC (TRA-1108)', async ({ page }) => {
    // The Scan tab renders `displayEpc` (leading zeros stripped), but the
    // Locate link has to carry `tag.epc`. The mask builder pads a deep-linked
    // value back out, and at 128 bits that padding cannot be the inverse of
    // the stripping — it lands on entirely the wrong 96 bits.
    const EPC_128 = '00000000000000000000533034313633';

    await page.click('[data-testid="menu-item-scan"]');
    await page.waitForTimeout(3000);
    await expect(page.getByTestId('page-title')).toContainText('Scan', { timeout: 10000 });
    await page.waitForTimeout(1000);

    await page.evaluate((epc) => {
      window.__ZUSTAND_STORES__!.tagStore.getState().addTag({
        epc,
        rssi: -45,
        count: 1,
        antenna: 1,
        timestamp: Date.now(),
        source: 'rfid'
      });
    }, EPC_128);

    const locateButton = page.locator('[data-testid="locate-button"]:visible');
    await expect(locateButton).toHaveCount(1);

    // The row itself still shows the operator the trimmed form.
    await expect(page.getByText('533034313633')).toBeVisible();

    await locateButton.click();
    await page.waitForSelector('h2:text("Configuring Reader")', { state: 'detached', timeout: 10000 }).catch(() => {});

    expect(page.url()).toContain(`#locate?epc=${EPC_128}`);
    await expect(page.locator('[data-testid="target-epc-display"]')).toHaveValue(EPC_128);
  });

  test('direct URL: navigate to #locate?epc=X sets targetEPC', async ({ page }) => {
    const testEpc = '10019';

    // Navigate directly to locate with EPC parameter
    await page.goto(`/#locate?epc=${testEpc}`);

    // Give time for navigation and mode configuration
    await page.waitForTimeout(2000);

    // Verify we're on locate tab. The screen no longer prints its own heading
    // (TRA-1071) — `h2` now resolves to the "Configuring Reader" spinner, so the
    // old 'Locate Item' assertion could never pass. Header title is the identity.
    await expect(page.getByTestId('page-title')).toContainText('Locate');

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