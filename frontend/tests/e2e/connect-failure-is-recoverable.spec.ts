/**
 * A connect that fails part-way through must leave the operator able to try again.
 *
 * Observed on preview 2026-09-04: the CS108 refused a command during bring-up,
 * `setMode(Inventory)` threw, and every subsequent attempt died on
 * `Device already connected. Call destroy() first.` until the page was
 * reloaded. `DeviceManager.create()` assigns its singleton before the four
 * things that can still fail, and none of those failure paths cleared it.
 * TRA-1250 fixed that; this is the guard, and it asserts the thing the unit
 * tests cannot: that a REAL reader, a REAL transport and a REAL worker recover.
 *
 * ## How the failure is produced, and why not the obvious ways
 *
 * A rejection has to land while the link stays healthy. That rules out the two
 * reflexes:
 *
 *   - **Killing the transport** (power off, walk out of range, drop the bridge)
 *     trips `TRANSPORT_DISCONNECTED`, which already destroys the singleton on
 *     its own. It exercises a different recovery path and hides this one.
 *   - **Double-clicking Connect** hits the guard at the top of `create()`,
 *     which fires BEFORE the assignment, so nothing is orphaned. It produces
 *     the same error text for an entirely correct reason — worth knowing when
 *     triaging, useless as a repro.
 *
 * So: inject `0xA101` / `0x0000`, the frame the device really sent.
 * `reader.handleBleData` handles a rejection ahead of ordinary routing and
 * fails whatever command is in flight, and the socket never notices.
 *
 * Injected repeatedly across the window because `IDLE_SEQUENCE`'s
 * `RFID_POWER_OFF` carries `toleratesFailure: true` (TRA-1217) — land on that
 * one alone and the sequence absorbs it and connects fine.
 *
 * ## This spec refuses to pass without reproducing
 *
 * If the poisoning fails to break the connect, the retry assertion below would
 * pass having tested nothing — a spec satisfiable by coincidence, which is the
 * failure mode TRA-1247 spent a day on. So the first connect's failure is
 * asserted, and a run that connects anyway reports itself INCONCLUSIVE rather
 * than green.
 *
 * That guard earns its keep: `msToFirstInjection` was measured at 1.5s on one
 * run and 10.3s on the next, because arming depends on how fast the bridge
 * hands the link over. Without the guard the slow run would have injected into
 * a reader that had already settled and reported a pass.
 *
 * ## Reading the counters when it fails
 *
 * `injected` discriminates the two outcomes on its own. Against the fix the
 * app tears the transport down as soon as the connect fails, `cleanup()`
 * deletes `__TRANSPORT_MANAGER__`, and the loop finds nothing to inject into —
 * measured at 14. Against the defect the singleton is never destroyed, the
 * transport stays up, and every injection lands — measured at 195 over the
 * same window. A high count with a failing retry is the bug's signature.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice, waitForBridgeReady } from './helpers/connection';
import { getE2EConfig, HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';
import { cs108CommandRejectedPacket } from '../config/cs108.config';

const config = getE2EConfig();

/** How long to keep rejecting commands. Locate's bring-up alone is ~3.7s (TRA-1225). */
const POISON_WINDOW_MS = 5000;

/** How long to wait for the mock to accept the first injection before giving up. */
const ARMING_BUDGET_MS = 20000;

interface PoisonResult {
  injected: number;
  refused: number;
  msToFirstInjection: number | null;
}

/**
 * Reject every command the reader issues, for a while, starting as early as legal.
 *
 * Runs entirely in the page for two reasons. Bring-up issues commands roughly
 * every 100ms, so a driver-side loop would sit between them rather than inside
 * them. And the moment injection becomes legal cannot be observed from the
 * driver at all:
 *
 *   `__TRANSPORT_MANAGER__.notifyCharacteristic` is published by
 *   `cs108-ble-transport.ts` immediately after `startNotifications()`, but the
 *   mock's own subscription bookkeeping is not yet in place at that instant,
 *   and it refuses with *"it is not subscribed"*. Waiting on the global's
 *   existence therefore fires too early, every time.
 *
 * So the loop tolerates the refusal instead of trying to predict it: it keeps
 * trying, counts what was refused, and only starts the duration clock once an
 * injection actually lands. That makes it robust to wherever the boundary sits
 * rather than encoding a guess about it.
 */
async function rejectCommandsDuringBringUp(
  page: Page,
  durationMs: number,
  armingBudgetMs: number
): Promise<PoisonResult> {
  return page.evaluate(
    async ({ packet, durationMs, armingBudgetMs }): Promise<PoisonResult> => {
      const data = new Uint8Array(packet);
      const started = Date.now();
      const armingDeadline = started + armingBudgetMs;

      let injected = 0;
      let refused = 0;
      let firstInjectionAt: number | null = null;

      for (;;) {
        const characteristic = window.__TRANSPORT_MANAGER__?.notifyCharacteristic;
        if (characteristic) {
          try {
            await navigator.bluetooth.testing.simulateNotification({ characteristic, data });
            injected++;
            if (firstInjectionAt === null) firstInjectionAt = Date.now();
          } catch {
            // Not subscribed yet, or the transport went away. Either way, ask again.
            refused++;
          }
        }

        if (firstInjectionAt !== null && Date.now() - firstInjectionAt >= durationMs) break;
        if (firstInjectionAt === null && Date.now() >= armingDeadline) break;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }

      return {
        injected,
        refused,
        msToFirstInjection: firstInjectionAt === null ? null : firstInjectionAt - started
      };
    },
    { packet: Array.from(cs108CommandRejectedPacket), durationMs, armingBudgetMs }
  );
}

test.describe('a failed connect can be retried @hardware', () => {
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

  test('a rejection during bring-up does not strand the app', async ({ browser }) => {
    const page = await browser.newPage();
    const failures: string[] = [];
    page.on('console', (m) => {
      if (/Connection failed:/i.test(m.text())) failures.push(m.text());
    });

    try {
      await page.goto('/');
      await waitForBridgeReady(page);

      // Click Connect, then start poisoning. The click only dispatches — the
      // connect runs on, and the poisoner arms itself as soon as the mock will
      // accept an injection.
      await page.locator(config.selectors.connectButton + ':not([disabled])').click();
      const poison = await rejectCommandsDuringBringUp(page, POISON_WINDOW_MS, ARMING_BUDGET_MS);
      console.log('[connect-failure]', JSON.stringify(poison));

      expect(
        poison.injected,
        `never landed a single rejection in ${ARMING_BUDGET_MS}ms ` +
          `(${poison.refused} refused). Nothing was tested.`
      ).toBeGreaterThan(0);

      // The connect must actually have failed. Without this the retry below
      // proves nothing, because a connect that never broke does not need
      // recovering.
      expect(
        failures.join(' | '),
        `INCONCLUSIVE: ${poison.injected} rejections landed and the connect survived them, ` +
          'so the recovery path was never entered. Either the reader tolerated them, or ' +
          `bring-up was already finished — first injection landed ` +
          `${poison.msToFirstInjection}ms in. Widen POISON_WINDOW_MS or arm sooner. ` +
          'This is not a pass.'
      ).toMatch(/Connection failed/i);
      await expect(
        page.locator(config.selectors.disconnectButton),
        'a failed connect must not present as connected'
      ).toHaveCount(0);

      // THE assertion. Before TRA-1250 this died on
      // `Device already connected. Call destroy() first.` and only a page
      // reload could clear it.
      await connectToDevice(page);
      await expect(
        page.locator(config.selectors.disconnectButton),
        'the retry after a failed connect must succeed without a page reload'
      ).toHaveCount(1);
    } finally {
      try {
        await disconnectDevice(page);
      } catch (e) {
        console.error('[connect-failure] cleanup', e);
      }
      await page.close();
    }
  });
});
