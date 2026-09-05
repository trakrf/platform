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
import { HARDWARE_TEST_TIMEOUT_MS } from './e2e.config';

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

  // Wait for the reader to be CONNECTED, not for a duration.
  //
  // This slept a flat 2500ms for a tag-mask push described as "~1.3s". That is
  // not a margin, it is a guess about a quantity nobody measured — and the cost
  // of guessing low is invisible, because `reader.ts` DISCARDS a trigger press
  // unless `readerState === CONNECTED`:
  //
  //     if (this.readerState === ReaderState.CONNECTED) { startScanning() }
  //     else { logger.debug(`Trigger pressed ignored - reader state is ...`) }
  //
  // So a press arriving while the push is still BUSY is dropped silently, the
  // scan never starts, and the test reports "press should start scanning" —
  // which reads as a broken trigger rather than a press delivered too early.
  // No `.catch()` here, deliberately. This used to swallow the timeout into a
  // console.warn and press anyway — which guaranteed a failure several
  // assertions downstream instead of at the cause. Across 200 reps on
  // 2026-09-02 the warning fired ZERO times, so nothing depends on continuing
  // past it; a reader that genuinely cannot settle in 15s is wedged and this
  // should say so here (TRA-1245).
  await page.waitForFunction(
    () => window.__ZUSTAND_STORES__?.deviceStore?.getState()?.readerState === 'Connected',
    { timeout: 15000 }
  );

  // Clear the trigger debounce window. `reader.ts` sets `triggerDebounceMs =
  // 100`, so 250 is 2.5x the actual value rather than another round number
  // chosen for comfort. Without it a press issued immediately after this helper
  // can be swallowed as a repeat of whatever the previous test did.
  //
  // ⚠ This sleep is why the wait above was never sufficient on its own: the
  // reader can re-enter BUSY inside these 250ms, and a press landing there is
  // dropped. The gate that actually protects the press now lives in
  // `simulateTrigger`, immediately before the injection with nothing in
  // between. Do not "simplify" by moving it back up here.
  await page.waitForTimeout(250);
}

// Locate mode tests - EPC filtering integration with CS108 hardware
test.describe('Locate Functionality Tests @hardware', () => {
  // Real hardware: connect + RFID bring-up alone costs ~20s (TRA-1148 item 5)
  test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS });

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

    // '/' lands on the Scan tab, so connect configures INVENTORY (TRA-1029);
    // the Locate navigation below switches the reader to LOCATE.
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

  /**
   * Hand the reader back between tests, whatever happened during one.
   *
   * These tests share one page and one reader, and a trigger press is STATE:
   * a test that fails between press and release leaves the reader SCANNING.
   * The Locate screen disables its EPC input while scanning
   * (`disabled={readerState === ReaderState.SCANNING}`), so the very next test's
   * `fill()` silently does not commit and it asserts against the PREVIOUS test's
   * target.
   *
   * That is exactly how the validation test came to expect 'ZZZZ' and receive
   * '10018' — which is this file's own LOCATE_TEST_TAG, not a stray value. The
   * reported failure was in the wrong test: one test failed, and a different one
   * was blamed.
   *
   * Best-effort by design: teardown must not convert one red test into two.
   */
  test.afterEach(async () => {
    if (!sharedPage || sharedPage.isClosed()) return;

    // Only attempt a release if the injection point is actually there. A test
    // that navigated has torn the transport down, which deletes
    // `__TRANSPORT_MANAGER__` (see cs108-ble-transport cleanup), and calling the
    // helper anyway spends three retries producing NOTIFY_CHAR_NOT_FOUND on
    // every single test — noise that looks like a hardware fault in the log.
    const canRelease = await sharedPage
      .evaluate(() => !!window.__TRANSPORT_MANAGER__?.notifyCharacteristic)
      .catch(() => false);
    if (!canRelease) return;

    // Best-effort, but never silent. This catch was empty — `() => {}` — and
    // that is how #647's release gate threw here roughly six times a rep for
    // 101 straight reps without producing so much as a log line. A swallowed
    // error is invisible to a pass/fail instrument by construction, so the
    // record read 0/101 while the defect ran on every one. TRA-1245.
    await simulateTriggerRelease(sharedPage).catch((error) => {
      console.warn('[Locate] teardown release did not complete:', error);
    });
    await sharedPage
      .waitForFunction(
        // ⚠ Passed in rather than written as a literal. This compared against
        // 'SCANNING' while the store holds 'Scanning', so the predicate was
        // true on its first evaluation no matter what the reader was doing:
        // the wait never waited and the warning below could never fire.
        (scanning) =>
          window.__ZUSTAND_STORES__?.deviceStore?.getState()?.readerState !== scanning,
        ReaderState.SCANNING,
        { timeout: 5000 }
      )
      .catch(() => {
        console.warn('[Locate] reader still SCANNING after release; next test may inherit it');
      });
  });

  test.afterAll(async () => {
    console.log('[Locate] Cleaning up shared connection...');
    if (sharedPage) {
      try {
        await sharedPage.goto('/');
        await sharedPage.waitForTimeout(1000); // Wait for mode change
        // No trigger release here. `goto('/')` tears the transport down and
        // leaves the reader Disconnected, so there is no scan to stop and
        // nothing to release into — and the per-test afterEach above has
        // already released while the transport still existed.
        //
        // It used to release here, and under #647's gate that spent 15s and
        // threw on every rep. The throw landed in the catch below, which meant
        // `disconnectDevice()` NEVER RAN: the reader was freed only as a side
        // effect of `sharedPage.close()`, and the clean disconnect this block
        // exists for had not executed once since #647 merged. TRA-1245.
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
    // ⚠ This comment used to read "the flake is the product's, not this test's".
    // That was measured and is WRONG — corrected under TRA-1245.
    //
    // `reader.ts:229` discards a trigger press unless `readerState ===
    // CONNECTED`, and that is CORRECT: the edge is dropped and the trigger
    // LEVEL is reconciled by `convergeToTriggerState()` once the reader
    // settles. A real finger is still holding the trigger at that point, so the
    // scan starts. An INJECTED press is not: the LOCATE mode change's own
    // GET_TRIGGER_STATE poll re-reads the switch and the device truthfully says
    // "released", so the level is revoked and there is nothing to converge to.
    // A dropped edge is unrecoverable here and only here. See ADR 0016.
    //
    // A 200-rep arm on 2026-09-02 put that at 48/200 (24.0%), every failure
    // showing `readerState: Busy` / `status: Idle` for the whole window, with
    // the transport and the device clean underneath. The window is now closed
    // in `simulateTrigger`, which waits for the honouring state immediately
    // before injecting.
    //
    // Still deliberately NOT retried here. The earlier reasoning for that —
    // preserving a measurement of "how often a real press lands on a
    // non-CONNECTED state" — does not survive: an injected press is not a real
    // press, so what it measured was this harness's own timing, and that
    // number has now been taken properly. The assertion stays because it is a
    // real assertion; it should simply no longer be intermittent.
    expect(
      held.some((s) => s.readerState === ReaderState.SCANNING),
      'press should start scanning — if this is red, the press was dropped on a ' +
        'non-CONNECTED reader state (TRA-1171), not a broken trigger simulation'
    ).toBe(true);

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

  test('validation: non-hex EPC is refused and leaves the target unchanged', async () => {
    // CORRECTED 2026-08-28, and the previous correction is the interesting part.
    //
    // This test was rewritten under TRA-1088 to assert that a non-hex EPC is
    // ACCEPTED, on the reasoning that `validateEPC` has no `isValid: false`
    // branch, so `setTargetEPC` always succeeds and "Invalid EPC format" is
    // unreachable. Every clause of that is true about the STORE's validator, and
    // it is the wrong layer: `LocateScreen.commitTarget` has its own guard,
    //
    //     if (!/^[0-9A-F]+$/.test(value)) { setStatusMessage('Invalid EPC
    //     format...'); return; }
    //
    // which returns BEFORE `setTargetEPC` is ever called. The message is not
    // unreachable; it is produced right there. So the screen does refuse non-hex
    // and the previous target survives — which is why this test asserted 'ZZZZ'
    // and received '10018', this file's own LOCATE_TEST_TAG left by the test
    // before it.
    //
    // A dead branch in one layer was read as a dead behaviour in the system.
    // Assert what the screen actually does.
    await gotoLocate(sharedPage);

    const targetBefore = await sharedPage.evaluate(
      () => window.__ZUSTAND_STORES__?.settingsStore?.getState()?.rfid?.targetEPC
    );

    const epcInput = sharedPage.locator('[data-testid="target-epc-display"]');
    await epcInput.fill('ZZZZ');
    await epcInput.blur();
    await sharedPage.waitForTimeout(500);

    // The typed text stays visible so the operator can correct it...
    await expect(epcInput).toHaveValue('ZZZZ');

    // ...but it must NOT become the target. Masking on a value the reader cannot
    // hunt reports "no signal", which on a tag finder reads as "the item is not
    // here" — the most harmful wrong answer this screen can give.
    //
    // Asserted against the value held BEFORE this test typed anything, rather
    // than a hardcoded constant: what matters is that the target did not move,
    // not which target it happened to be.
    const stored = await sharedPage.evaluate(
      () => window.__ZUSTAND_STORES__?.settingsStore?.getState()?.rfid?.targetEPC
    );
    expect(stored).toBe(targetBefore);
    expect(stored).not.toBe('ZZZZ');

    // The screen reports success, not the (unreachable) format error.
    // Read the status the same way locate-barcode-target.spec.ts does, from the
    // container two levels up. The previous reader walked
    // `input.parentElement.lastElementChild`, which returned '' — a DOM shape
    // this screen no longer has. An empty string from a broken reader is
    // indistinguishable from a screen that said nothing, so it could only ever
    // satisfy a `not.toContain` assertion. That is what made the inverted
    // assertion above look correct for as long as it did.
    const statusMessage =
      (await sharedPage
        .locator('[data-testid="target-epc-display"]')
        .locator('xpath=../..')
        .textContent()) ?? '';
    console.log('[Test] status message:', statusMessage);
    // The screen says why it refused. This is the assertion that was inverted:
    // it previously required the message to be ABSENT, on the belief that it
    // could never be produced.
    expect(statusMessage).toContain('Invalid EPC format');
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
