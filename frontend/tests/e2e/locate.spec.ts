/**
 * Locate E2E Tests
 * Tests RFID tag location functionality with RSSI proximity feedback
 * Requires physical CS108 device via bridge server
 *
 * Pattern follows inventory.spec.ts which is 100% stable:
 * - Serial execution with shared connection
 * - Connect once in beforeAll, disconnect in afterAll
 * - Share single page across all tests
 *
 * TRA-1088: this file was rotted — every test targeted `locate-epc-input`, a
 * test id that has never existed (the input is `target-epc-display`), so all
 * five failed at the first waitForSelector. Several assertions were also
 * `if (element)`-guarded, meaning they passed whether or not the element was
 * there. Those are now real assertions against elements that exist.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { simulateTriggerPress, simulateTriggerRelease } from './helpers/trigger-utils';
import { getReaderState } from './helpers/device-state';
import { ReaderState } from './helpers/reader-state';
import { PRIMARY_TEST_TAG, NON_EXISTENT_TAG } from '@test-utils/constants';

// Test tag that should be locatable (from physical test setup)
const LOCATE_TEST_TAG = PRIMARY_TEST_TAG;

/** Read the Locate screen the way a user does. */
async function readLocateScreen(page: Page) {
  return page.evaluate(() => {
    const rowValue = (label: string) => {
      const rows = Array.from(document.querySelectorAll('div.flex.justify-between'));
      const row = rows.find((r) => r.firstElementChild?.textContent?.trim().startsWith(label));
      return row?.lastElementChild?.textContent?.trim() ?? '';
    };
    const gauge = (document.querySelector('[data-testid="proximity-display"]')?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();
    return {
      readerState: window.__ZUSTAND_STORES__?.deviceStore?.getState()?.readerState ?? '?',
      gaugeSaysNoSignal: /No signal/.test(gauge),
      gaugeDbm: (gauge.match(/(-\d+) dBm/) ?? [])[1] ?? null,
      status: rowValue('Status'),
      updateRate: rowValue('Update Rate'),
      current: rowValue('Current'),
    };
  });
}

/** Poll the screen for `ms`, returning every sample. */
async function sampleLocateScreen(page: Page, ms: number) {
  const start = Date.now();
  const out: Awaited<ReturnType<typeof readLocateScreen>>[] = [];
  while (Date.now() - start < ms) {
    out.push(await readLocateScreen(page));
    await page.waitForTimeout(250);
  }
  return out;
}

/** Navigate to Locate and let mode configuration settle. */
async function gotoLocate(page: Page) {
  await page.goto('/#locate');
  await page.waitForSelector('[data-testid="target-epc-display"]', { timeout: 10000 });

  // `page.goto` to the same hash is a same-document navigation, so the stores
  // survive between tests, and `currentRSSI` / `updateRate` are only recomputed
  // inside addRssiReading — with no new reads they hold their last value
  // forever. Clear the buffer explicitly so each test starts from silence
  // rather than inheriting the previous test's readings.
  await page.evaluate(() => {
    window.__ZUSTAND_STORES__?.locateStore?.getState()?.clearBuffer?.();
  });

  // The reader is BUSY for ~3s here and the screen is covered by the
  // ConfigurationSpinner overlay while it is.
  await page.waitForTimeout(5000);
}

async function gotoLocateWithEPC(page: Page, epc: string) {
  await gotoLocate(page);
  const epcInput = page.locator('[data-testid="target-epc-display"]');
  await epcInput.fill(epc);
  await epcInput.blur();
  // The tag-mask push keeps the reader BUSY for ~1.3s after blur.
  await page.waitForTimeout(2500);
}

// Locate mode tests - EPC filtering integration with CS108 hardware
test.describe('Locate Functionality Tests @hardware', () => {
  /**
   * CONNECTION SHARING STRATEGY (from inventory tests)
   *
   * We connect once for all locate tests because:
   * 1. Connection/disconnection is tested in connection.spec.ts
   * 2. Users perform multiple operations without reconnecting
   * 3. Tests run much faster (30s vs 2+ minutes)
   * 4. This tests real-world connection stability
   */

  // Connecting alone takes ~20s, and each test settles the reader before acting.
  test.describe.configure({ timeout: 120_000 });

  let sharedPage: Page;
  let connectionHealthy = true;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(120_000);
    console.log('[Locate] Setting up shared connection for all tests...');
    sharedPage = await browser.newPage();

    sharedPage.on('console', (msg) => {
      if (/error|warning/i.test(msg.type())) console.log('[Browser Console]', msg.type(), msg.text());
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
        await sharedPage.goto('/');
        await sharedPage.waitForTimeout(1000); // Wait for mode change
        await simulateTriggerRelease(sharedPage);
        await disconnectDevice(sharedPage);
      } catch (error) {
        console.error('[Locate] Error during disconnect:', error);
      }
      await sharedPage.close();
    }
  });

  test('basic locate: finds tag with matching EPC', async () => {
    await gotoLocateWithEPC(sharedPage, LOCATE_TEST_TAG);

    // Confirm the EPC actually reached the settings store before searching.
    const storedEPC = await sharedPage.evaluate(
      () => window.__ZUSTAND_STORES__?.settingsStore?.getState()?.rfid?.targetEPC
    );
    expect(storedEPC).toBe(LOCATE_TEST_TAG);

    console.log('[Test] Starting trigger cycle for locate...');
    const press = await simulateTriggerPress(sharedPage);
    expect(press.success).toBe(true);

    const held = await sampleLocateScreen(sharedPage, 4000);
    await simulateTriggerRelease(sharedPage);

    // The old test looked for `.rssi-value` / `rssi-display`, neither of which
    // exists, and only logged the count — it could not fail. Assert that the
    // tag was actually heard: a dBm reading on the gauge and a non-zero rate.
    console.log('[Test] samples:', JSON.stringify(held.slice(0, 4)));
    expect(held.some((s) => s.gaugeDbm !== null), 'gauge should report dBm for an in-range tag').toBe(true);
    expect(held.some((s) => /[1-9]/.test(s.updateRate)), 'update rate should be non-zero').toBe(true);

    // Verify we're back to READY after the cycle completes
    await sharedPage.waitForTimeout(2000);
    expect(await getReaderState(sharedPage)).toBe(ReaderState.CONNECTED);
  });

  test('trigger control: starts/stops locate on press/release', async () => {
    await gotoLocateWithEPC(sharedPage, LOCATE_TEST_TAG);

    console.log('[Test] Simulating trigger press...');
    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);

    // Press must actually start a scan, not merely leave the reader connected.
    const held = await sampleLocateScreen(sharedPage, 3000);
    expect(held.some((s) => s.readerState === ReaderState.SCANNING), 'press should start scanning').toBe(true);

    console.log('[Test] Simulating trigger release...');
    const releaseResult = await simulateTriggerRelease(sharedPage);
    expect(releaseResult.success).toBe(true);

    await sharedPage.waitForTimeout(2000);
    expect(await getReaderState(sharedPage)).toBe(ReaderState.CONNECTED);
  });

  test('proximity feedback: gauge reports RSSI while searching', async () => {
    await gotoLocateWithEPC(sharedPage, LOCATE_TEST_TAG);

    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);

    const held = await sampleLocateScreen(sharedPage, 3000);
    await simulateTriggerRelease(sharedPage);

    // The gauge must exist AND report a plausible value — the old test only
    // checked that one of three selectors matched something, never what it said.
    await expect(sharedPage.locator('[data-testid="proximity-display"]')).toBeVisible();
    const reading = held.find((s) => s.gaugeDbm !== null);
    expect(reading, 'gauge should show a dBm reading while searching').toBeTruthy();
    expect(Number(reading!.gaugeDbm)).toBeLessThan(0);
    expect(Number(reading!.gaugeDbm)).toBeGreaterThan(-100);

    await sharedPage.waitForTimeout(1500);
  });

  test('validation: non-hex EPC is accepted with a warning, not rejected', async () => {
    // TRA-1088: this test used to assert that 'DEADBEEF' was REJECTED as an
    // "invalid EPC format". It could never pass — and not because DEADBEEF is
    // valid hex. `validateEPC` has no `isValid: false` branch at all, so
    // `setTargetEPC` always succeeds and the screen's "Invalid EPC format"
    // message is unreachable. The relaxation is deliberate: the validator's own
    // comment says it exists "to allow any alphanumeric tag value for use with
    // the locate feature". Assert the contract that actually holds.
    await gotoLocate(sharedPage);

    const epcInput = sharedPage.locator('[data-testid="target-epc-display"]');
    await epcInput.fill('ZZZZ');
    await epcInput.blur();
    await sharedPage.waitForTimeout(1000);

    // Accepted and stored — not rejected.
    await expect(epcInput).toHaveValue('ZZZZ');
    const stored = await sharedPage.evaluate(
      () => window.__ZUSTAND_STORES__?.settingsStore?.getState()?.rfid?.targetEPC
    );
    expect(stored).toBe('ZZZZ');

    // The screen reports success, not the (unreachable) format error.
    const statusMessage = await sharedPage.evaluate(() => {
      const input = document.querySelector('[data-testid="target-epc-display"]');
      return input?.parentElement?.lastElementChild?.textContent?.trim() ?? '';
    });
    console.log('[Test] status message:', statusMessage);
    expect(statusMessage).not.toContain('Invalid EPC format');
  });

  test('edge case: reports no signal for a tag that is not present', async () => {
    await gotoLocateWithEPC(sharedPage, NON_EXISTENT_TAG);

    const pressResult = await simulateTriggerPress(sharedPage);
    expect(pressResult.success).toBe(true);

    const held = await sampleLocateScreen(sharedPage, 3000);
    await simulateTriggerRelease(sharedPage);

    // The old test looked for `.not-found` / `tag-not-found`, neither of which
    // exists, and asserted nothing. The real contract: an absent tag produces no
    // reading. This is also the false-POSITIVE guard for TRA-1080 — now that the
    // gauge follows the read stream, it must stay silent when nothing is heard.
    console.log('[Test] samples:', JSON.stringify(held.slice(0, 4)));
    expect(held.every((s) => s.gaugeDbm === null), 'gauge must not invent a reading').toBe(true);
    expect(held.every((s) => s.gaugeSaysNoSignal), 'gauge should read "No signal"').toBe(true);

    await sharedPage.waitForTimeout(1500);
  });
});
