/**
 * TRA-1080 regression guard against the fix itself.
 *
 * The staleness re-render interval is now keyed on "a signal is being
 * reported" rather than "readerState === SCANNING". If that interval stopped
 * too early, the gauge would freeze on the last dBm value after the user stops
 * searching — a false POSITIVE, which is worse than the bug being fixed.
 *
 * Scan, stop, then assert the display falls back to "No signal"/"Idle".
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { LOCATE_TEST_TAG } from '@test-utils/constants';
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';

async function readScreen(page: Page) {
  return page.evaluate(() => {
    const rowValue = (label: string) => {
      const rows = Array.from(document.querySelectorAll('div.flex.justify-between'));
      const row = rows.find((r) => r.firstElementChild?.textContent?.trim().startsWith(label));
      return row?.lastElementChild?.textContent?.trim() ?? '';
    };
    // The gauge's value label is the only element rendering formatRSSI() output.
    const gaugeText = (document.querySelector('[data-testid="proximity-display"]')?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();
    return {
      readerState: window.__ZUSTAND_STORES__?.deviceStore?.getState()?.readerState ?? '?',
      gaugeSaysNoSignal: /No signal/.test(gaugeText),
      gaugeDbm: (gaugeText.match(/(-\d+) dBm/) ?? [])[1] ?? null,
      status: rowValue('Status'),
      rate: rowValue('Update Rate'),
    };
  });
}

test.describe('TRA-1080 signal falls back after stopping @hardware', () => {
  // Real hardware: connect + RFID bring-up alone costs ~20s (TRA-1148 item 5)
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

  test.describe.configure({ timeout: 120_000 });
  let page: Page;

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(120_000);
    page = await browser.newPage();
    page.on('console', (m) => {
      if (/error|cannot start/i.test(m.text())) console.log('  !!!', m.text());
    });
    await page.goto('/');
    await connectToDevice(page);
  });

  test.afterAll(async () => {
    try {
      await page.goto('/');
      await page.waitForTimeout(1000);
      await disconnectDevice(page);
    } catch (e) {
      console.error('[cleanup]', e);
    }
    await page?.close();
  });

  test('gauge reports signal while scanning and clears once reads stop', async () => {
    await page.goto('/#locate');
    await page.waitForSelector('[data-testid="target-epc-display"]', { timeout: 10000 });
    await page.waitForTimeout(5000);

    const epc = page.locator('[data-testid="target-epc-display"]');
    await epc.fill(LOCATE_TEST_TAG);
    await epc.blur();
    await page.waitForTimeout(2500);

    await page.locator('button', { hasText: /^Start$/ }).first().click();
    await page.waitForTimeout(4000);

    const scanning = await readScreen(page);
    console.log('while scanning:', JSON.stringify(scanning));
    expect(scanning.status, 'Status while scanning').toBe('Searching');
    expect(scanning.gaugeSaysNoSignal, 'gauge must not say No signal while reads arrive').toBe(false);
    expect(scanning.gaugeDbm, 'gauge should render a dBm value').not.toBeNull();

    await page.locator('button', { hasText: /^Stop$/ }).first().click();

    // Readings go stale after 1s; the 250ms interval must survive long enough
    // to notice. Give it a generous margin, then require full fallback.
    await page.waitForTimeout(4000);

    const stopped = await readScreen(page);
    console.log('after stop:', JSON.stringify(stopped));
    expect(stopped.gaugeSaysNoSignal, 'gauge must fall back to No signal after reads stop').toBe(true);
    expect(stopped.status, 'Status after stop').toBe('Idle');
  });
});
