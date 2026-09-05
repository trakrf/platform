/**
 * The FIRST Start click after each connect must start a search, even though
 * the reader is still finishing bring-up when the button becomes clickable.
 *
 * ⚠ This guard is worthless unless it lands inside the bring-up window, and it
 * can go green without ever getting there. That is why it records the reader
 * state at the instant of the click and ASSERTS that at least one cycle caught
 * the reader BUSY. A spec satisfiable by coincidence is the same defect it
 * exists to catch, one level up.
 *
 * Why nothing else covers this: `locate-signal-fallback.spec.ts` drives the
 * same button but waits ~7.5s first (5000ms after navigation, 2500ms after the
 * EPC blur), by which time the reader has long settled to CONNECTED. Settling
 * is precisely what makes the window unreachable, so a green run there is a
 * true answer to a different question.
 *
 * What it guards: the single `setTimeout` backstop at `reader.ts:132-139` that
 * schedules `convergeToTriggerState()` on the CONNECTED transition. Both halves
 * of TRA-1080's deferral — the button and the trigger — pass because of that
 * one line, and a refactor could remove it without any other test noticing.
 *
 * History. Ran as a throwaway probe on hardware 2026-09-04 against main
 * @ `ceccd767`: 3/3 cycles caught the reader BUSY (112ms, 109ms, 134ms after
 * connect) and the search started every time. TRA-1080 deferred this, TRA-1247
 * settled it and promoted the probe.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { ReaderState } from './helpers/device-state';
import { LOCATE_TEST_TAG } from '@test-utils/constants';

const CYCLES = 3;

type CycleResult = {
  cycle: number;
  stateAtClick: string;
  msFromConnectToClick: number;
  statusAfter: string;
  busyErrors: string[];
};

test.describe('Locate Start survives being clicked during bring-up @hardware', () => {
  test.describe.configure({ timeout: 300_000 });

  test('a Start click taken as soon as the button is enabled starts the search', async ({ browser }) => {
    test.setTimeout(300_000);
    const results: CycleResult[] = [];

    for (let cycle = 1; cycle <= CYCLES; cycle++) {
      const page: Page = await browser.newPage();

      // Diagnostics only, never an assertion. The original report's line —
      // `[DeviceManager] Failed to sync scanning state: Error: Cannot start
      // scanning from state Busy` — is main-thread and would be caught here,
      // but worker-side `logger` output was never confirmed to reach
      // `page.on('console')` at all. Asserting an empty array would be
      // asserting on a channel that may be unable to deliver: green because
      // nothing can arrive. The load-bearing assertion is `statusAfter`.
      const busyErrors: string[] = [];
      page.on('console', (m) => {
        if (/Cannot start scanning from state/i.test(m.text())) busyErrors.push(m.text());
      });

      await page.goto('/');
      await connectToDevice(page);
      const connectedAt = Date.now();

      // Straight to Locate. No settle — the settle is the thing that hides it.
      await page.goto('/#locate');
      await page.waitForSelector('[data-testid="target-epc-display"]', { timeout: 10000 });

      const epc = page.locator('[data-testid="target-epc-display"]');
      await epc.fill(LOCATE_TEST_TAG);
      await epc.blur();

      const startButton = page.locator('button', { hasText: /^Start$/ }).first();
      await startButton.waitFor({ state: 'visible', timeout: 10000 });

      // The datum the verdict turns on: what state was the reader actually in
      // at the moment of the click? Read immediately before it, not after.
      const stateAtClick = await page.evaluate(
        () => window.__ZUSTAND_STORES__?.deviceStore?.getState()?.readerState ?? '?'
      );
      const msFromConnectToClick = Date.now() - connectedAt;
      await startButton.click();

      await page.waitForTimeout(4000);
      const statusAfter = await page.evaluate(() => {
        const rows = Array.from(document.querySelectorAll('div.flex.justify-between'));
        const row = rows.find((r) => r.firstElementChild?.textContent?.trim().startsWith('Status'));
        return row?.lastElementChild?.textContent?.trim() ?? '';
      });

      const result = { cycle, stateAtClick, msFromConnectToClick, statusAfter, busyErrors };
      results.push(result);
      console.log('[bringup]', JSON.stringify(result));

      try {
        await disconnectDevice(page);
      } catch (e) {
        console.error('[bringup] cleanup', e);
      }
      await page.close();
    }

    console.log('[bringup] SUMMARY', JSON.stringify(results, null, 2));

    // Whatever the state at click, a click the UI offered must start a search.
    for (const r of results) {
      expect(
        r.statusAfter,
        `cycle ${r.cycle} status after Start (state at click: ${r.stateAtClick}, ` +
          `${r.msFromConnectToClick}ms after connect)`
      ).toBe('Searching');
    }

    // The assertion that keeps this honest. Without it a run that never caught
    // the reader BUSY passes vacuously, reporting a result for a window it
    // never entered — which is exactly the failure the spec is guarding.
    const exercised = results.filter((r) => r.stateAtClick === ReaderState.BUSY);
    expect(
      exercised.length,
      `no cycle caught the reader BUSY at click, so the bring-up window was ` +
        `never exercised. States at click: ${results.map((r) => r.stateAtClick).join(', ')}. ` +
        'This is INCONCLUSIVE rather than a pass: the reader is settling faster ' +
        'than the click can be taken, and the spec needs to click sooner.'
    ).toBeGreaterThan(0);
  });
});
