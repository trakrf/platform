/**
 * Trigger simulation utilities for E2E tests.
 *
 * Injects CS108 trigger packets through the ble-mcp-test testing API
 * (`navigator.bluetooth.testing.simulateNotification`) into the notify
 * characteristic the product itself subscribed to.
 *
 * The helpers deliberately do NOT call `startNotifications()`. They target
 * `window.__TRANSPORT_MANAGER__.notifyCharacteristic`, which is the same
 * characteristic instance `Cs108BleTransport` subscribed before exposing it,
 * so the subscription already exists by the time any helper runs. See
 * TRA-1179 Deliverable 1 for the evidence behind that claim, and TRA-1153 for
 * the mock-side lifecycle changes this file is written to survive.
 */

import type { Page } from '@playwright/test';
// Aliased because this module already exports a `getReaderState` of its own that
// returns a number against a store which holds strings. Importing the
// device-state one under its own name would shadow-clash with that; renaming or
// removing the local one is not this ticket's business.
import { ReaderState, getReaderState as getReaderStateName } from './device-state';
import { cs108TriggerPressPacket, cs108TriggerReleasePacket } from '../../config/cs108.config';

/**
 * How long to wait for `characteristicvaluechanged` after injecting a packet.
 *
 * Dispatch is synchronous in the current mock, so this normally resolves on the
 * first microtask. The budget exists so that a delivery path which becomes
 * asynchronous (TRA-1153) degrades into a slower pass rather than a hard fail.
 */
const NOTIFICATION_DELIVERY_TIMEOUT_MS = 1000;

/**
 * The reader state in which each trigger edge is ACTED ON rather than dropped.
 *
 * `reader.ts:229` is the whole of it:
 *
 *     if (this.readerState === ReaderState.CONNECTED) { startScanning() }
 *     else { logger.debug(`Trigger pressed ignored - reader state is ...`) }
 *
 * ...with the mirror-image branch requiring SCANNING for a release.
 *
 * ⚠ A dropped PRESS is not recoverable when the BUSY is a MODE CHANGE, and that
 * asymmetry is the reason this constant exists. A real finger survives the drop:
 * the LEVEL stays asserted and `convergeToTriggerState()` reconciles it the
 * moment the reader settles. An injected press does not, because a mode change
 * REVOKES the level it dropped:
 *
 *   `buildModeSequences()` prefixes `IDLE_SEQUENCE` to every mode, that sequence
 *   sends `GET_TRIGGER_STATE` (0xA001), the device answers in ~22ms with the
 *   real switch position, and `CommandManager` forwards that answer to the
 *   notification handler, which overwrites the latch at `reader.ts:179`. For an
 *   injected press the honest answer is "released".
 *
 * Nothing else revokes it — no timer, and no unsolicited device report (ADR
 * 0019). So an injected level holds across time, and across any BUSY that is
 * not a mode change, and is lost across every mode change. Measured in
 * `tests/e2e/trigger-level-is-reread-on-mode-change.spec.ts`; see ADR 0016.
 *
 * The gate is what removes the coin flip: 48 of 200 reps of `locate.spec.ts`
 * (24.0%) failed on 2026-09-02, every one showing `readerState: Busy` /
 * `status: Idle` for the full sample window — Busy from the LOCATE mode change,
 * whose poll revoked the press it had just dropped. The gate took that to
 * 0/101. The product was correct in all 48. See TRA-1245, and TRA-1080 for the
 * same trap in 2026-07.
 *
 * ⚠ A dropped RELEASE does not share that shape, and reading this map as though
 * it did is what #647 got wrong. See `waitForReaderToAcceptTrigger` below: a
 * release only needs honouring when there is a scan to stop, and this map says
 * which state that is — not that every release must wait for it.
 */
export const STATE_THAT_HONOURS: Record<'press' | 'release', string> = {
  press: ReaderState.CONNECTED,
  release: ReaderState.SCANNING,
};

/**
 * The states from which the reader is still on its way somewhere.
 *
 * The same pair `reader.ts:797` calls transient for its own settle wait. Every
 * other state is an answer; these two mean "ask again in a moment".
 */
const TRANSIENT_STATES: readonly string[] = [ReaderState.BUSY, ReaderState.CONNECTING];

/**
 * How long to wait for the reader to reach the state that honours the edge.
 *
 * Matches the 15s `gotoLocateWithEPC` already used for the same question, which
 * across 200 reps never came close to expiring — the reader settled every time.
 * It is a wedge detector, not a race margin.
 */
export const TRIGGER_HONOUR_TIMEOUT_MS = 15000;

/**
 * How long to wait for the device store to reflect an injected trigger packet.
 *
 * Was 500ms, and 500ms could not cover what this ticket is about: a press that
 * lands during bring-up is acted on by `convergeToTriggerState()` only once the
 * reader settles, and the Locate mask write alone measured ~3.7s (TRA-1225).
 *
 * 5000 rather than the 15s of `TRIGGER_HONOUR_TIMEOUT_MS`, deliberately, so the
 * bound carries a product requirement — Mike, on TRA-1247:
 *
 *     "if a config sequence is taking 5 seconds that's gonna frustrate users
 *      and that's something we would want to surface as a defect"
 *
 * A 15s bound would quietly absorb exactly that. Expect this to fire: at ~3.7s
 * Locate has barely a second of headroom, and the first thing it surfaces will
 * probably be the mask write. That is the defect, not a flaky new test.
 */
export const TRIGGER_CONFIRMATION_TIMEOUT_MS = 5000;

/** How often to re-read the reader state while waiting. */
const HONOUR_POLL_INTERVAL_MS = 50;

/**
 * Poll until the reader is out of a transient state, or the budget runs out.
 *
 * Resolves with whatever state ended the wait — including a transient one on
 * timeout — so the caller decides what that means. Mirrors the product's own
 * `waitForSettledState` (`reader.ts:895`), which resolves rather than throwing
 * for the same reason: "settled into Disconnected" and "still Busy when time
 * ran out" are different answers and the caller has to tell them apart.
 */
async function waitForSettledReaderState(page: Page, timeoutMs: number): Promise<string> {
  const deadline = Date.now() + timeoutMs;

  let observed = await getReaderStateName(page);
  while (TRANSIENT_STATES.includes(observed)) {
    if (Date.now() >= deadline) return observed;
    await page.waitForTimeout(HONOUR_POLL_INTERVAL_MS);
    observed = await getReaderStateName(page);
  }
  return observed;
}

/**
 * Block until injecting `action` now will either be acted on, or provably do
 * nothing at all. Throw when neither can be established.
 *
 * The two actions are NOT symmetric, and #647 shipped them as though they were.
 *
 * A PRESS has one honouring state and, in the case that bites, no recovery:
 * every other state drops the edge with a `logger.debug`, and when that state
 * is a MODE CHANGE the same mode change's `GET_TRIGGER_STATE` poll then revokes
 * the level, so `convergeToTriggerState()` finds nothing held. So the only
 * sound move is to wait for CONNECTED and refuse if it never comes. That gate
 * took `locate.spec.ts` from 24.0% to 0/101 and is deliberately untouched here.
 *
 * A RELEASE is a no-op whenever no scan is running — `reader.ts:237` drops it
 * with a `logger.debug` and nothing is left undone, because there was nothing
 * to stop. Waiting for SCANNING there waits for a state that will never arrive:
 *
 *   - `connection.spec.ts:130` and `inventory.spec.ts:111` never start a scan at
 *     all (Settings tab, mode Idle) and failed 2 of 2 reps on a 15s timeout;
 *   - `locate.spec.ts`'s `afterAll` releases against a Disconnected reader and
 *     threw into a `catch` on 101 of 101 reps, skipping `disconnectDevice()`
 *     entirely and costing 15s a rep — green the whole time.
 *
 * So a release waits only for an ANSWER to "is a scan running". BUSY and
 * CONNECTING are not answers — the reader could still resolve either way, and
 * injecting before it does is the press bug wearing a different hat. Every
 * settled state is an answer, and only a reader that never leaves a transient
 * state is still a failure, which is what the 15s budget was always for.
 *
 * Throwing rather than returning false is deliberate: injecting into a dropping
 * state produces a test failure several assertions downstream — "gauge should
 * report dBm" — which reads as a broken product rather than an edge delivered
 * into the wrong state. Failing here names the actual cause at the actual moment.
 */
export async function waitForReaderToAcceptTrigger(
  page: Page,
  action: 'press' | 'release',
  timeoutMs: number = TRIGGER_HONOUR_TIMEOUT_MS
): Promise<void> {
  const wanted = STATE_THAT_HONOURS[action];

  if (action === 'release') {
    const settled = await waitForSettledReaderState(page, timeoutMs);
    if (TRANSIENT_STATES.includes(settled)) {
      throw new Error(
        `TRIGGER_NOT_HONOURABLE: waited ${timeoutMs}ms for the reader to settle ` +
          `before a release, and it was still "${settled}". A reader that never ` +
          'leaves a transient state is wedged rather than merely busy, and the ' +
          'edge would have been dropped with only a logger.debug. See TRA-1245.'
      );
    }
    if (settled !== wanted) {
      // Not a failure: with no scan running the edge is a no-op by design, and
      // the injection still carries the trigger-state transition the specs
      // assert. Said out loud because #647's version of this was a throw that
      // ran 101 times inside a catch without reaching the pass/fail record.
      console.log(
        `[Trigger] release into "${settled}" - no scan to stop, so the edge is a ` +
          'no-op; injecting for the trigger-state transition only'
      );
    }
    return;
  }

  const deadline = Date.now() + timeoutMs;

  let observed = await getReaderStateName(page);
  while (observed !== wanted) {
    if (Date.now() >= deadline) {
      throw new Error(
        `TRIGGER_NOT_HONOURABLE: waited ${timeoutMs}ms for the reader to accept a ` +
          `${action}, and it was still "${observed}" (needs "${wanted}"). ` +
          'The edge would have been dropped with only a logger.debug, leaving ' +
          'the scan to start whenever convergence next runs rather than now. ' +
          'A reader that never reaches the honouring state is wedged. ' +
          'See TRA-1245.'
      );
    }
    await page.waitForTimeout(HONOUR_POLL_INTERVAL_MS);
    observed = await getReaderStateName(page);
  }
}

/** Diagnostics returned from the in-page injection, surfaced on failure. */
interface InjectionResult {
  success: boolean;
  message: string;
  hasTestingApi?: boolean;
  hasBluetoothApi?: boolean;
  hasTransportManager?: boolean;
  hasMockFlag?: boolean;
  eventReceived?: boolean;
  eventData?: number[] | null;
  error?: string;
}

/**
 * Inject one trigger packet and confirm it reached the transport's listener.
 *
 * Shared by press and release, which previously carried this logic duplicated
 * verbatim — so a fix to one silently left the other wrong.
 */
async function injectTriggerPacket(
  page: Page,
  packet: number[],
  action: 'press' | 'release'
): Promise<InjectionResult> {
  return page.evaluate(
    async ({ packet, action, timeoutMs }): Promise<InjectionResult> => {
      if (!navigator.bluetooth?.testing?.simulateNotification) {
        return {
          success: false,
          message:
            'TESTING_API_NOT_FOUND: navigator.bluetooth.testing.simulateNotification not available. Check the ble-mcp-test mock is loaded.',
          hasTestingApi: false,
          hasBluetoothApi: !!navigator.bluetooth,
          hasMockFlag: !!window?.__webBluetoothBridged
        };
      }

      // The characteristic the product subscribed to, taken by reference rather
      // than re-resolved via getCharacteristic().
      //
      // The original reason was that the mock minted a fresh characteristic per
      // call, so a re-resolved one was never subscribed. TRA-1153 item 1 fixed
      // that in 0.8.0 — the mock now caches per canonical UUID — so this is no
      // longer a workaround. It stays because it is still the more direct
      // statement of intent: this helper wants THE instance the product
      // subscribed, not an equivalent one.
      const tm = window.__TRANSPORT_MANAGER__;
      if (!tm?.notifyCharacteristic) {
        return {
          success: false,
          message:
            'NOTIFY_CHAR_NOT_FOUND: No notify characteristic found in transport manager. Ensure device is connected.',
          hasTestingApi: true,
          hasTransportManager: !!tm
        };
      }
      const characteristic = tm.notifyCharacteristic;

      let eventData: number[] | null = null;
      let signalReceived: () => void = () => {};
      const received = new Promise<void>((resolve) => {
        signalReceived = resolve;
      });

      const eventHandler = (event: Event) => {
        const value = (event as Event & { target?: { value?: DataView } }).target?.value;
        if (!value) return;
        // Honour the view window. A real DataView (TRA-1153) may be a slice of a
        // larger ArrayBuffer; reading .buffer alone takes the whole backing
        // store and yields wrong bytes in a plausible-looking shape.
        eventData = Array.from(
          new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
        );
        console.log(
          `[Trigger] characteristicvaluechanged received:`,
          eventData.map((b) => '0x' + b.toString(16).padStart(2, '0')).join(' ')
        );
        signalReceived();
      };

      characteristic.addEventListener('characteristicvaluechanged', eventHandler);

      try {
        await navigator.bluetooth.testing.simulateNotification({
          characteristic,
          data: new Uint8Array(packet)
        });

        // Await delivery instead of busy-waiting on it. The previous spin loop
        // blocked the event loop for up to 50ms, so it could only ever observe a
        // synchronous dispatch — an async one would be starved out and fail
        // 100% of the time while reading as a mock defect.
        let timer: ReturnType<typeof setTimeout> | undefined;
        const eventReceived = await Promise.race([
          received.then(() => true),
          new Promise<boolean>((resolve) => {
            timer = setTimeout(() => resolve(false), timeoutMs);
          })
        ]);
        if (timer !== undefined) clearTimeout(timer);

        return {
          success: eventReceived,
          message: eventReceived
            ? `NOTIFICATION_SENT: Trigger ${action} packet injected successfully`
            : `NOTIFICATION_FAILED: Event dispatched but not received within ${timeoutMs}ms`,
          hasTestingApi: true,
          hasTransportManager: true,
          eventReceived,
          eventData
        };
      } catch (e) {
        return {
          success: false,
          message: `NOTIFICATION_ERROR: Failed to inject packet - ${e}`,
          hasTestingApi: true,
          hasTransportManager: true,
          error: String(e)
        };
      } finally {
        characteristic.removeEventListener('characteristicvaluechanged', eventHandler);
      }
    },
    { packet, action, timeoutMs: NOTIFICATION_DELIVERY_TIMEOUT_MS }
  );
}

/**
 * Poll the device store until the trigger level matches, or the budget expires.
 *
 * A condition rather than a duration: the old form ran a fixed 500ms wall and
 * then read the state once more, so a level that landed at 501ms was reported
 * as never having landed at all.
 */
async function waitForTriggerLevel(
  page: Page,
  desiredState: boolean,
  timeoutMs: number
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if ((await getTriggerState(page)) === desiredState) return true;
    if (Date.now() >= deadline) return false;
    await page.waitForTimeout(HONOUR_POLL_INTERVAL_MS);
  }
}

/**
 * Say what actually ran out, given where the reader was when it did.
 *
 * A timeout that misreports what it measured is how TRA-1247's whole detour
 * started: `STATE_NOT_UPDATED` named the store, when the thing that had not
 * finished was the reader's bring-up.
 */
function describeConfirmationTimeout(
  action: 'press' | 'release',
  readerState: string,
  timeoutMs: number,
  initialState: boolean,
  desiredState: boolean
): string {
  if (TRANSIENT_STATES.includes(readerState)) {
    return (
      `BRING_UP_INCOMPLETE: bring-up did not complete within ${timeoutMs}ms — ` +
      `the reader was still "${readerState}" after the ${action} was delivered. ` +
      'What exceeded the budget is the config sequence the reader is still ' +
      'running, and a sequence this long is a product defect worth surfacing, ' +
      'not a flaky test. Note that if that sequence is a MODE CHANGE, its ' +
      'GET_TRIGGER_STATE poll will also have revoked the injected level. ' +
      'See TRA-1247 and TRA-1225.'
    );
  }
  return (
    `STATE_NOT_UPDATED: the store did not move from ${initialState} to ` +
    `${desiredState} within ${timeoutMs}ms, with the reader settled at ` +
    `"${readerState}". The packet reached the transport, so the break is ` +
    'between the worker notification handler and deviceStore.'
  );
}

/**
 * Drive one trigger transition to completion: inject, confirm the packet
 * reached the transport, then wait for the device store to reflect it.
 *
 * @param page - Playwright page
 * @param action - Which transition to simulate
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @param honourTimeoutMs - How long to wait for a state that will act on the edge
 * @param confirmTimeoutMs - How long to wait for the store to reflect the packet
 */
async function simulateTrigger(
  page: Page,
  action: 'press' | 'release',
  maxRetries: number = 3,
  honourTimeoutMs: number = TRIGGER_HONOUR_TIMEOUT_MS,
  confirmTimeoutMs: number = TRIGGER_CONFIRMATION_TIMEOUT_MS
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  const packet = action === 'press' ? cs108TriggerPressPacket : cs108TriggerReleasePacket;
  const desiredState = action === 'press';
  const initialState = await getTriggerState(page);
  let lastTimeoutReason = '';

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    console.log(`[Trigger] ${action} attempt ${attempt}/${maxRetries}`);

    // Immediately before the injection, with nothing between the two.
    //
    // A caller that checks the state and THEN sleeps has not gated anything:
    // `gotoLocateWithEPC` waited for CONNECTED and then slept 250ms for the
    // trigger debounce, and the reader re-entered BUSY inside that gap in 24%
    // of reps. The gate has to be the last thing that happens before the
    // packet goes in, which is why it lives here rather than at the call sites.
    await waitForReaderToAcceptTrigger(page, action, honourTimeoutMs);

    const result = await injectTriggerPacket(page, Array.from(packet), action);

    if (!result.success) {
      console.error(`[Trigger] ${action} simulation failed on attempt ${attempt}:`, result.message);
      console.error('[Trigger] Diagnostics:', {
        hasTestingApi: result.hasTestingApi,
        hasBluetoothApi: result.hasBluetoothApi,
        hasTransportManager: result.hasTransportManager,
        hasMockFlag: result.hasMockFlag,
        error: result.error
      });

      if (attempt === maxRetries) {
        return { ...result, triggerState: initialState };
      }

      await page.waitForTimeout(200);
      continue;
    }

    console.log('[Trigger] Event dispatch verified - data reached transport layer');

    if (await waitForTriggerLevel(page, desiredState, confirmTimeoutMs)) {
      console.log(`[Trigger] ${action} confirmed - state changed from`, initialState, 'to', desiredState);
      return {
        success: true,
        message: `STATE_UPDATED: Trigger ${action} successful and state confirmed`,
        triggerState: desiredState
      };
    }

    lastTimeoutReason = describeConfirmationTimeout(
      action,
      await getReaderStateName(page),
      confirmTimeoutMs,
      initialState,
      desiredState
    );
    console.warn(`[Trigger] attempt ${attempt}/${maxRetries}: ${lastTimeoutReason}`);

    if (attempt < maxRetries) {
      console.log(`[Trigger] Retrying ${action} simulation...`);
      await page.waitForTimeout(300);
    }
  }

  const finalState = await getTriggerState(page);
  return {
    success: false,
    message: `All ${maxRetries} attempts failed. ${lastTimeoutReason}`,
    triggerState: finalState
  };
}

/**
 * Simulate trigger press with retries
 * @param page - Playwright page
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @returns Success status, message, and trigger state
 */
export async function simulateTriggerPress(
  page: Page,
  maxRetries: number = 3,
  honourTimeoutMs: number = TRIGGER_HONOUR_TIMEOUT_MS,
  confirmTimeoutMs: number = TRIGGER_CONFIRMATION_TIMEOUT_MS
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  return simulateTrigger(page, 'press', maxRetries, honourTimeoutMs, confirmTimeoutMs);
}

/**
 * Simulate trigger release with retries
 * @param page - Playwright page
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @returns Success status, message, and trigger state
 */
export async function simulateTriggerRelease(
  page: Page,
  maxRetries: number = 3,
  honourTimeoutMs: number = TRIGGER_HONOUR_TIMEOUT_MS,
  confirmTimeoutMs: number = TRIGGER_CONFIRMATION_TIMEOUT_MS
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  return simulateTrigger(page, 'release', maxRetries, honourTimeoutMs, confirmTimeoutMs);
}

/**
 * Simulate a complete trigger cycle (press, hold, release)
 * @param page - Playwright page
 * @param holdDuration - How long to hold trigger in ms (default 3000)
 * @returns Success status
 */
export async function simulateTriggerCycle(
  page: Page, 
  holdDuration: number = 3000
): Promise<boolean> {
  console.log('[Trigger] Starting trigger cycle with', holdDuration, 'ms hold duration');
  
  const pressResult = await simulateTriggerPress(page);
  if (!pressResult.success) {
    console.error('[Trigger] Cycle aborted - press failed');
    console.error('[Trigger] Failure reason:', pressResult.message);
    return false;
  }
  
  console.log('[Trigger] Press successful, holding for', holdDuration, 'ms');
  await page.waitForTimeout(holdDuration);
  
  const releaseResult = await simulateTriggerRelease(page);
  if (!releaseResult.success) {
    console.error('[Trigger] Cycle incomplete - release failed');
    console.error('[Trigger] Failure reason:', releaseResult.message);
    // Even though release failed, press was successful
    console.warn('[Trigger] WARNING: Trigger may be stuck in pressed state');
    return false;
  }
  
  console.log('[Trigger] Cycle completed successfully');
  return true;
}

/**
 * Get current trigger state from device store
 * @param page - Playwright page
 * @returns Current trigger state
 */
export async function getTriggerState(page: Page): Promise<boolean> {
  return await page.evaluate(() => {
    const state = (window as unknown as { __ZUSTAND_STORES__?: { deviceStore?: { getState: () => { triggerState?: boolean } } } }).__ZUSTAND_STORES__?.deviceStore?.getState();
    return state?.triggerState || false;
  });
}

/**
 * Get current inventory running state
 * @param page - Playwright page  
 * @returns Whether inventory is running
 */
export async function getInventoryRunning(page: Page): Promise<boolean> {
  const states = await page.evaluate(() => {
    const stores = (window as unknown as { __ZUSTAND_STORES__?: { deviceStore?: { getState: () => { triggerState?: boolean } } } }).__ZUSTAND_STORES__;
    const deviceStore = stores?.deviceStore?.getState();
    const tagStore = stores?.tagStore?.getState();
    const uiStore = stores?.uiStore?.getState();
    
    return {
      readerState: deviceStore?.readerState || 0,
      inventoryRunning: tagStore?.inventoryRunning || false,
      activeTab: uiStore?.activeTab,
      triggerState: deviceStore?.triggerState
    };
  });
  
  // Check both reader state and inventory flag
  const readerInInventory = states.readerState === ReaderState.SCANNING;
  const inventoryFlag = states.inventoryRunning;
  
  console.log('[Trigger] Inventory check:', {
    readerState: states.readerState,
    readerInInventory,
    inventoryFlag,
    activeTab: states.activeTab,
    triggerState: states.triggerState
  });
  
  return readerInInventory || inventoryFlag;
}

/**
 * Get current reader state
 * @param page - Playwright page
 * @returns Reader state number (4=READY, 5=INVENTORY, etc)
 */
export async function getReaderState(page: Page): Promise<number> {
  return await page.evaluate(() => {
    const deviceStore = (window as unknown as { __ZUSTAND_STORES__?: { deviceStore?: { getState: () => unknown } } }).__ZUSTAND_STORES__?.deviceStore?.getState();
    return deviceStore?.readerState || 0;
  });
}

/**
 * Wait for inventory to start after trigger press
 * @param page - Playwright page
 * @param timeout - Max time to wait in ms
 * @returns Whether inventory started
 */
export async function waitForInventoryStart(page: Page, timeout: number = 5000): Promise<boolean> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeout) {
    const running = await getInventoryRunning(page);
    if (running) {
      console.log('[Trigger] Inventory started');
      return true;
    }
    await page.waitForTimeout(100);
  }
  
  console.log('[Trigger] Inventory did not start within timeout');
  return false;
}

/**
 * Wait for inventory to stop after trigger release
 * @param page - Playwright page
 * @param timeout - Max time to wait in ms
 * @returns Whether inventory stopped
 */
export async function waitForInventoryStop(page: Page, timeout: number = 5000): Promise<boolean> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeout) {
    const running = await getInventoryRunning(page);
    if (!running) {
      console.log('[Trigger] Inventory stopped');
      return true;
    }
    await page.waitForTimeout(100);
  }
  
  console.log('[Trigger] Inventory did not stop within timeout');
  return false;
}

/**
 * Wait for trigger state to reset and inventory to fully stop
 * @param page - Playwright page
 * @param timeout - Max time to wait in ms
 * @returns Whether trigger is properly reset
 */
export async function waitForTriggerReset(page: Page, timeout: number = 10000): Promise<boolean> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeout) {
    const states = await page.evaluate(() => {
      const stores = (window as unknown as { __ZUSTAND_STORES__?: { deviceStore?: { getState: () => { triggerState?: boolean } } } }).__ZUSTAND_STORES__;
      const deviceStore = stores?.deviceStore?.getState();
      const tagStore = stores?.tagStore?.getState();
      return {
        triggerState: deviceStore?.triggerState,
        readerState: deviceStore?.readerState,
        inventoryRunning: tagStore?.inventoryRunning
      };
    });
    
    // Check if trigger is fully reset and inventory is fully stopped.
    //
    // ⚠ This read `ReaderState.IDLE`, and there is no IDLE in ReaderState —
    // the members are Disconnected/Connecting/Configuring/Connected/Busy/
    // Scanning/Error. IDLE belongs to ReaderMode, a different enum off a
    // different store field. So the comparison was `readerState === undefined`,
    // never true for a connected reader, and this helper could only ever burn
    // its full timeout and return false however completely the trigger reset.
    //
    // CONNECTED is the resting state that was meant: reader.ts documents it as
    // "Connected and idle, ready for operations" — the "idle" being reached for.
    //
    // Nothing called this when it was found, so nothing was failing; it was a
    // trap armed for the next caller. Same shape as locate.spec.ts comparing
    // against 'SCANNING' while the store holds 'Scanning'. TRA-1245.
    if (!states.triggerState &&
        !states.inventoryRunning &&
        states.readerState === ReaderState.CONNECTED) {
      console.log('[Trigger] Fully reset and ready');
      return true;
    }
    
    await page.waitForTimeout(200);
  }
  
  console.log('[Trigger] Did not fully reset within timeout');
  return false;
}

// === v0.7.0 Simplified API ===

/**
 * Simple trigger press using v0.7.0 testing API - throws on failure
 * @param page - Playwright page
 */
export async function pressTrigger(page: Page): Promise<void> {
  const result = await simulateTriggerPress(page);
  if (!result.success) {
    throw new Error(`Trigger press failed: ${result.message}`);
  }
}

/**
 * Simple trigger release using v0.7.0 testing API - throws on failure
 * @param page - Playwright page
 */
export async function releaseTrigger(page: Page): Promise<void> {
  const result = await simulateTriggerRelease(page);
  if (!result.success) {
    throw new Error(`Trigger release failed: ${result.message}`);
  }
}

/**
 * Execute a trigger sequence with multiple actions
 * @param page - Playwright page
 * @param sequence - Array of trigger actions with durations
 */
export async function triggerSequence(
  page: Page, 
  sequence: Array<{ action: 'press' | 'release'; duration: number }>
): Promise<void> {
  for (const step of sequence) {
    if (step.action === 'press') {
      await pressTrigger(page);
    } else {
      await releaseTrigger(page);
    }
    
    if (step.duration > 0) {
      await page.waitForTimeout(step.duration);
    }
  }
}
