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
import { ReaderState } from './device-state';
import { cs108TriggerPressPacket, cs108TriggerReleasePacket } from '../../config/cs108.config';

/**
 * How long to wait for `characteristicvaluechanged` after injecting a packet.
 *
 * Dispatch is synchronous in the current mock, so this normally resolves on the
 * first microtask. The budget exists so that a delivery path which becomes
 * asynchronous (TRA-1153) degrades into a slower pass rather than a hard fail.
 */
const NOTIFICATION_DELIVERY_TIMEOUT_MS = 1000;

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
 * Drive one trigger transition to completion: inject, confirm the packet
 * reached the transport, then wait for the device store to reflect it.
 *
 * @param page - Playwright page
 * @param action - Which transition to simulate
 * @param maxRetries - Maximum number of retry attempts (default 3)
 */
async function simulateTrigger(
  page: Page,
  action: 'press' | 'release',
  maxRetries: number = 3
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  const packet = action === 'press' ? cs108TriggerPressPacket : cs108TriggerReleasePacket;
  const desiredState = action === 'press';
  const initialState = await getTriggerState(page);

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    console.log(`[Trigger] ${action} attempt ${attempt}/${maxRetries}`);

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

    // Wait up to 500ms for the store to catch up with the injected packet.
    const startTime = Date.now();
    while (Date.now() - startTime < 500) {
      const triggerState = await getTriggerState(page);
      if (triggerState === desiredState) {
        console.log(`[Trigger] ${action} confirmed - state changed from`, initialState, 'to', triggerState);
        return {
          success: true,
          message: `STATE_UPDATED: Trigger ${action} successful and state confirmed`,
          triggerState: desiredState
        };
      }
      await page.waitForTimeout(50);
    }

    const finalState = await getTriggerState(page);
    console.warn(`[Trigger] State did not update on attempt ${attempt}/${maxRetries}`);
    console.warn('[Trigger] Initial state:', initialState, '| Final state:', finalState);

    if (attempt < maxRetries) {
      console.log(`[Trigger] Retrying ${action} simulation...`);
      await page.waitForTimeout(300);
    }
  }

  const finalState = await getTriggerState(page);
  return {
    success: false,
    message: `STATE_NOT_UPDATED: All ${maxRetries} attempts failed. Trigger state did not change from ${initialState} to ${desiredState}. Check deviceManager notification handler.`,
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
  maxRetries: number = 3
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  return simulateTrigger(page, 'press', maxRetries);
}

/**
 * Simulate trigger release with retries
 * @param page - Playwright page
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @returns Success status, message, and trigger state
 */
export async function simulateTriggerRelease(
  page: Page,
  maxRetries: number = 3
): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  return simulateTrigger(page, 'release', maxRetries);
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
    
    // Check if trigger is fully reset and inventory is fully stopped
    if (!states.triggerState && 
        !states.inventoryRunning && 
        states.readerState === ReaderState.IDLE) {
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
