/**
 * The trigger LEVEL is re-read from the device on every mode change, and the
 * device's answer wins over whatever the host was holding.
 *
 * This is the mechanism ADR 0016 turns on:
 *
 *   1. `buildModeSequences()` prefixes `IDLE_SEQUENCE` to EVERY mode.
 *   2. `IDLE_SEQUENCE` sends `GET_TRIGGER_STATE` (0xA001). The device answers
 *      in ~22ms with 0=released / 1=pushed — measured on the wire 2026-09-05,
 *      and specified in the vendor byte-stream API §10.1.
 *   3. `CommandManager.handleCommandResponse()` forwards 0xA000 and 0xA001
 *      replies to the notification handler (`command.ts:374-386`), so the
 *      answer reaches `TriggerStateHandler` even though it also settles the
 *      command in flight.
 *   4. That emits `TRIGGER_STATE_CHANGED`, and `reader.ts:179` overwrites the
 *      host latch with what the device just said.
 *
 * So an INJECTED press survives time — nothing decays it — but it does not
 * survive a mode change, because no physical switch is held and the poll
 * truthfully reports "released". A real finger survives both.
 *
 * That asymmetry is the whole of ADR 0016, and it is why `simulateTrigger`
 * waits for the honouring state immediately before injecting: a press landing
 * in the BUSY of a MODE CHANGE is not merely dropped, it is revoked a moment
 * later by that mode change's own poll, so convergence has nothing to find.
 *
 * The test asserts both halves. Asserting only the revocation would pass on a
 * level that never held at all.
 */

import { test, expect, type Page } from '@playwright/test';
import { connectToDevice, disconnectDevice } from './helpers/connection';
import { expectReaderMode } from './helpers/assertions';
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';
import { simulateTriggerPress, simulateTriggerRelease, getTriggerState } from './helpers/trigger-utils';

test.describe('the trigger level is re-read on every mode change @hardware', () => {
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

  let page: Page;

  test.beforeAll(async ({ browser }) => {
    page = await browser.newPage();
    await page.goto('/');
    await connectToDevice(page);
  });

  test.afterAll(async () => {
    try {
      await simulateTriggerRelease(page).catch(() => {});
      await page.goto('/');
      await disconnectDevice(page);
    } catch (e) {
      console.error('[cleanup]', e);
    }
    await page?.close();
  });

  test('an injected level holds through time, and is revoked by a mode change', async () => {
    // Settings resolves to IDLE, so a press here starts no scan and the only
    // thing under test is the level itself.
    await page.click('button[data-testid="menu-item-settings"]');
    await expectReaderMode(page, 'Idle');
    expect(await getTriggerState(page), 'trigger must start released').toBe(false);

    const press = await simulateTriggerPress(page);
    expect(press.success, press.message).toBe(true);
    expect(await getTriggerState(page), 'the injected press must latch').toBe(true);

    // Half of the assertion: time alone does not revoke it. Well past the
    // helper's own confirmation window, so a level that merely looked latched
    // for a moment would have lapsed by now.
    await page.waitForTimeout(3000);
    expect(
      await getTriggerState(page),
      'nothing decays a latched level — no timer, no unsolicited device report'
    ).toBe(true);

    // The other half: a mode change re-reads the device, and the device says
    // released because no switch is held. No release is injected here.
    await page.click('button[data-testid="menu-item-scan"]');
    await expectReaderMode(page, 'Inventory');

    expect(
      await getTriggerState(page),
      'the mode change re-polls 0xA001 and the device answer must win'
    ).toBe(false);
  });
});
