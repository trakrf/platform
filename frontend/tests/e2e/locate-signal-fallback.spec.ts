/**
 * TRA-1080 regression guard against the fix itself: a gauge that shows a
 * number when there is no signal is a false POSITIVE, and on a tag finder that
 * is worse than the bug being fixed. "No signal" must never be rendered as
 * signal.
 *
 * ⚠ WHAT CHANGED, and why the original assertion here is gone.
 *
 * This file used to assert that stopping a search made the gauge fall back to
 * "No signal". TRA-1171 deliberately replaced that: the gauge now HOLDS the
 * reading the search found, because the operator released the trigger in order
 * to read it, and zeroing it is the false NEGATIVE — TRA-1123's defect from the
 * other side. Held-vs-decayed is decided as of the release.
 *
 * So the old assertion was not catching TRA-1080's defect on its merits; it was
 * catching it incidentally, through a stop-behaviour that has since changed on
 * purpose. It failed on main from the moment TRA-1171 shipped, and nothing
 * noticed because this suite never runs in CI.
 *
 * The guard is kept, aimed at the case that actually exercises it:
 *
 *   scanning, tag present   gauge shows dBm, never "No signal"
 *   stopped,  tag present   gauge HOLDS that dBm            (TRA-1171)
 *   stopped,  no tag ever   gauge stays "No signal"         (TRA-1080)
 *
 * ⚠ NOT covered here, deliberately: a search that HEARD a tag and then lost it
 * before the release must hold "No signal" rather than reviving the earlier
 * reading. Producing that on the bench needs the tag physically removed
 * mid-search — retargeting cannot stand in for it, because changing the target
 * clears the buffer and resets currentRSSI, which removes the very value that
 * would be revived. It is covered by locateStore.test.ts, which can fake the
 * clock. Do not "add" it here with a retarget and believe it is tested.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { LOCATE_TEST_TAG, NON_EXISTENT_TAG } from '@test-utils/constants';
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

test.describe('Locate gauge tells the truth when a search stops @hardware', () => {
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

  async function setTargetAndSearch(page: Page, target: string) {
    await page.goto('/#locate');
    await page.waitForSelector('[data-testid="target-epc-display"]', { timeout: 10000 });
    await page.waitForTimeout(5000);

    const epc = page.locator('[data-testid="target-epc-display"]');
    await epc.fill(target);
    await epc.blur();
    await page.waitForTimeout(2500);

    await page.locator('button', { hasText: /^Start$/ }).first().click();
    await page.waitForTimeout(4000);
  }

  test('holds the reading it found when the search stops (TRA-1171)', async () => {
    await setTargetAndSearch(page, LOCATE_TEST_TAG);

    const scanning = await readScreen(page);
    console.log('while scanning:', JSON.stringify(scanning));
    expect(scanning.status, 'Status while scanning').toBe('Searching');
    expect(scanning.gaugeSaysNoSignal, 'gauge must not say No signal while reads arrive').toBe(false);
    expect(scanning.gaugeDbm, 'gauge should render a dBm value').not.toBeNull();

    await page.locator('button', { hasText: /^Stop$/ }).first().click();

    // Well past the 1s staleness threshold and the 250ms re-render interval:
    // if the held value were going to decay, it would have by now.
    await page.waitForTimeout(4000);

    const stopped = await readScreen(page);
    console.log('after stop:', JSON.stringify(stopped));
    expect(stopped.status, 'Status after stop').toBe('Idle');
    expect(stopped.gaugeSaysNoSignal, 'gauge must HOLD the result, not zero it').toBe(false);
    expect(stopped.gaugeDbm, 'a held result is a real reading').not.toBeNull();

    // The hold is FROZEN, not merely slow to decay. Sampling twice is what
    // distinguishes the two — a value still draining would differ here.
    // Deliberately not compared against `scanning`: reads keep arriving in the
    // seconds before the Stop click, so that value has legitimately moved on.
    await page.waitForTimeout(3000);
    const later = await readScreen(page);
    console.log('later:', JSON.stringify(later));
    expect(later.gaugeDbm, 'the held value must not drift after the search ends').toBe(stopped.gaugeDbm);
  });

  test('a search that finds nothing says No signal, during and after (TRA-1080)', async () => {
    // The case the guard actually exists for. Nothing on the bench answers
    // this EPC, so every read is filtered out and the gauge has nothing to
    // show — during the search or after it.
    await setTargetAndSearch(page, NON_EXISTENT_TAG);

    const searching = await readScreen(page);
    console.log('searching for nothing:', JSON.stringify(searching));
    expect(searching.gaugeSaysNoSignal, 'gauge must say No signal while nothing answers').toBe(true);
    expect(searching.gaugeDbm, 'no dBm value may be rendered for a tag that is not there').toBeNull();

    await page.locator('button', { hasText: /^Stop$/ }).first().click();
    await page.waitForTimeout(4000);

    const stopped = await readScreen(page);
    console.log('after stop:', JSON.stringify(stopped));
    expect(stopped.status, 'Status after stop').toBe('Idle');
    expect(stopped.gaugeSaysNoSignal, 'stopping must not put a number on an empty gauge').toBe(true);
    expect(stopped.gaugeDbm, 'stopping must not invent a reading').toBeNull();
  });
});
