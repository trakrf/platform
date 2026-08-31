/**
 * Connection helper functions for E2E tests
 * Handles device connection, disconnection, and connection state verification
 * 
 * 🚀 ENHANCED TESTING AVAILABLE: See tests/e2e/BLE-ENHANCED-TESTING-STRATEGY.md
 * for packet-level monitoring and protocol validation capabilities
 */

import type { Page } from '@playwright/test';
import type { WindowWithStores } from '../types';
import { getE2EConfig } from '../e2e.config';
import { shouldForwardConsoleLine } from './console-forwarding';

const config = getE2EConfig();

/**
 * Connect to a CS108 device with enhanced reliability
 * Handles the full connection flow including waiting for battery indicator
 * Includes retry logic with backoff for bridge server recovery
 */
export async function connectToDevice(page: Page): Promise<void> {
  // No test-side retry. The mock owns this one now.
  //
  // There used to be one, bounded to two attempts with a 1000ms beat, and its
  // reasoning was correct when written: ble-mcp-test retried only
  // `RETRYABLE_CONNECT_CODES`, which was `['NOT_READY']`, and the command path
  // being owned by another connection came back as `DEVICE_BUSY` — which
  // upstream refused to retry ON PURPOSE, as a loud refusal no amount of waiting
  // fixes. Playwright opens a fresh page per spec, so the previous spec's
  // session could still own the path for a moment after its socket closed, and
  // that beat belonged somewhere.
  //
  // ⚠ THE REASON EXPIRED, WHICH IS WHY THIS IS GONE RATHER THAN KEPT.
  // ble-mcp-test 0.16.0 splits `DEVICE_BUSY_SELF` — our own claim still closing,
  // measured at 12-21ms — out of `DEVICE_BUSY`, and retries it inside its own
  // connect loop (250ms, x1.3, 5 attempts, ~2.4s ceiling). The previous-spec
  // handoff described above IS that case: the e2e session id is pinned per host
  // (`tests/config/ble-bridge.config.ts`), so a fresh page colliding with the
  // page before it is a self-collision by definition.
  //
  // So the wait is still paid — once, inside the path that measured it, by the
  // side that owns the policy. A second copy here would pay it twice, and the
  // two would drift.
  //
  // What this deliberately does NOT do is retry anything else. The old loop
  // caught every error and retried once, which was broader than its own
  // justification: a missing button, a dead bridge and a wedged reader all got a
  // free second attempt they had no claim to, and a 2x-slower failure reads as
  // contention when it is not. If a genuine non-busy flake shows up here, fix it
  // where it happens rather than restoring a blanket retry.
  //
  // Guarded by `tests/config/installed-mock-retryable-connect-codes.test.ts`,
  // which goes red if the installed mock ever loses `DEVICE_BUSY_SELF`. See
  // TRA-1216.
  await connectToDeviceOnce(page);
}

async function connectToDeviceOnce(page: Page): Promise<void> {
  try {
      console.log('[Connection] Starting connection process...');
      
      // Ensure bridge is ready first
      await waitForBridgeReady(page);
      
      // Check if Web Bluetooth is available
      const hasBluetooth = await page.evaluate(() => {
        return 'bluetooth' in navigator;
      });
      console.log('[Connection] Web Bluetooth available:', hasBluetooth);
      
      // Check if mock is injected
      const isMocked = await page.evaluate(() => {
        return (window as WindowWithStores).__webBluetoothBridged === true;
      });
      console.log('[Connection] Mock injected:', isMocked);
      
      // Close hamburger menu if it's open (it might be covering the connect button)
      const hamburgerOpen = await page.locator('.fixed.inset-0.z-40').count();
      if (hamburgerOpen > 0) {
        console.log('[Connection] Hamburger menu is open, closing it...');
        // Click outside the menu to close it
        await page.keyboard.press('Escape');
        await page.waitForTimeout(500);
      }
      
      // Debug: Let's see what buttons are on the page
      const allButtons = await page.locator('button').all();
      console.log('[Connection] Total buttons on page:', allButtons.length);
      
      // Look specifically in the header for the connect button
      const headerButtons = await page.locator('header button').all();
      console.log('[Connection] Header buttons:', headerButtons.length);
      for (const button of headerButtons) {
        const text = await button.textContent();
        const testId = await button.getAttribute('data-testid');
        const ariaLabel = await button.getAttribute('aria-label');
        console.log('[Connection] Header button:', { text, testId, ariaLabel });
      }
      
      // First check if the button exists at all
      const buttonExists = await page.locator(config.selectors.connectButton).count();
      console.log('[Connection] Connect button count:', buttonExists);
      
      if (buttonExists === 0) {
        // Button not found, maybe we're already connected?
        const disconnectExists = await page.locator(config.selectors.disconnectButton).count();
        if (disconnectExists > 0) {
          console.log('[Connection] Already connected - disconnect button found');
          return;
        }
        
        // Try a more flexible selector
        const connectByText = await page.locator('button:has-text("Connect")').count();
        console.log('[Connection] Connect buttons by text:', connectByText);
        
        if (connectByText > 0) {
          console.log('[Connection] Found connect button by text, using that instead');
          const connectButton = await page.locator('button:has-text("Connect")').first();
          await connectButton.click();
          
          // Wait for connection
          await page.waitForSelector(config.selectors.disconnectButton, {
            timeout: config.timeouts.connect
          });
          console.log('[Connection] Connected successfully via text selector');
          return;
        }
        
        throw new Error('Neither connect nor disconnect button found');
      }
      
      // Wait for the connect button to be enabled (Web Bluetooth must be available)
      const connectButton = await page.waitForSelector(config.selectors.connectButton + ':not([disabled])', {
        timeout: config.timeouts.ui
      });
      
      console.log('[Connection] Connect button found, clicking...');
      
      // Set up console monitoring before clicking.
      //
      // The predicate lives in ./console-forwarding so it can be unit-tested. As
      // an inline `if` it silently dropped every `[ble-timing]` line for weeks —
      // case-sensitively, against lines the ack-latency instrument counts — and
      // nothing short of a hardware run could have caught it (TRA-1209).
      //
      // Note the two limbs are now ONE call, not two `if`s. The old pair could
      // both match and log the same line twice, which double-counts any needle
      // matching both halves.
      page.on('console', msg => {
        const type = msg.type();
        const text = msg.text();
        if (shouldForwardConsoleLine(text, type)) {
          console.log(`[Console ${type}]`, text);
        }
      });
      
      // Click the connect button
      await connectButton.click();
      
      // Add a small delay to let the connection process start
      await page.waitForTimeout(2000);
      
      // Wait for connection states: Disconnected -> Connecting -> Ready
      try {
        // First, we might see "Connecting"
        await page.waitForSelector('button:has-text("Connecting")', { timeout: 5000 });
        console.log('[Connection] Connecting state detected');
      } catch {
        console.log('[Connection] Connecting state not detected (may have been brief)');
      }
      
      // Wait for connection to complete - looking for disconnect button
      console.log('[Connection] Waiting for Connected status...');
      await page.waitForSelector(config.selectors.disconnectButton, {
        timeout: config.timeouts.connect
      });
      
      // Additional wait for connection to stabilize
      await page.waitForTimeout(1000);
      
      console.log('[Connection] Connection completed successfully');

      // Wait for battery to be available (START_BATTERY_REPORTING sends updates every 5 seconds)
      console.log('[Connection] Waiting for battery update (5-second auto-reporting)...');
      try {
        await page.waitForSelector(config.selectors.batteryIndicator, {
          timeout: 10000 // Wait up to 10 seconds for battery to appear
        });
        console.log('[Connection] Battery indicator detected');
      } catch (error) {
        console.log('[Connection] Warning: Battery indicator not detected within 10 seconds');
      }

      // Reset device state to ensure clean test environment
      console.log('[Connection] Resetting device to idle state for clean testing...');
      await page.evaluate(async () => {
        try {
          // First try to get deviceManager from stores
          const stores = window.__ZUSTAND_STORES__;
          const deviceStore = stores?.deviceStore;
          
          // Get deviceManager from different possible locations
          const deviceManager = 
            window.__TRANSPORT_MANAGER__?.deviceManager || 
            window.__DEVICE_MANAGER__ ||
            deviceStore?.getState?.()?.deviceManager;
            
          if (deviceManager && typeof deviceManager.configureForTab === 'function') {
            console.log('[Connection] Found deviceManager, configuring for settings tab (idle state)...');
            await deviceManager.configureForTab('settings');
            console.log('[Connection] Device configured for idle state');
          } else {
            console.warn('[Connection] Device manager not found or configureForIdle not available');
            console.log('[Connection] Available objects:', {
              hasTransportManager: !!window.__TRANSPORT_MANAGER__,
              hasDeviceManager: !!window.__DEVICE_MANAGER__,
              hasStores: !!stores,
              hasDeviceStore: !!deviceStore
            });
          }
        } catch (error) {
          console.warn('[Connection] Failed to reset device state:', error);
        }
      });
      
      // Wait for reset to complete
      await page.waitForTimeout(1000);
      
    } catch (error) {
      console.log('[Connection] Connection failed:', (error as Error).message);
      throw error; // Retried by connectToDevice, once, for the busy handoff.
    }
}

/**
 * Wait for a specific connection status
 * @param page - Playwright page
 * @param status - Expected status text (e.g., 'Connected', 'Connecting', 'Disconnected')
 */
export async function waitForConnectionStatus(
  page: Page, 
  status: 'Connected' | 'Connecting' | 'Disconnected' | 'Configuring'
): Promise<void> {
  // Map old status names to new button text
  const statusMap: Record<string, string> = {
    'Connected': 'Disconnect',
    'Connecting': 'Connecting...',
    'Disconnected': 'Connect',
    'Configuring': 'Cancel'
  };
  
  const buttonText = statusMap[status] || status;
  
  // Look for button text that indicates status
  await page.waitForSelector(`button:has-text("${buttonText}")`, {
    timeout: config.timeouts.ui
  });
}

/**
 * Total wall-clock this helper may spend.
 *
 * Its own internal waits sum to ~18.5s in the worst case where every one of
 * them times out (5s find button + 2s enabled + ~3s cleanup + 2s modal + 5s
 * status + 1.5s settle), so this has to sit clear of that or it would start
 * firing on slow-but-working disconnects. 30s does, and still leaves most of
 * the 90s hardware budget for the test itself.
 */
const DISCONNECT_BUDGET_MS = 30000;

/**
 * Run `work`, but give up after `budgetMs` rather than hanging forever.
 *
 * The loser of the race is left running - there is nothing useful to do with a
 * Playwright call that will not settle, and the page is closed moments later
 * anyway. Its eventual rejection is swallowed so it cannot surface as an
 * unhandled rejection after the test has moved on.
 */
async function withBudget<T>(work: Promise<T>, budgetMs: number, label: string): Promise<T | null> {
  let timer: NodeJS.Timeout | undefined;
  const expiry = new Promise<null>((resolve) => {
    timer = setTimeout(() => resolve(null), budgetMs);
  });

  work.catch(() => { /* reported by the caller, or irrelevant after expiry */ });

  try {
    const result = await Promise.race([work, expiry]);
    if (result === null) {
      console.warn(`[Connection] ${label} exceeded its ${budgetMs}ms budget - abandoning it`);
    }
    return result;
  } finally {
    if (timer) clearTimeout(timer);
  }
}

/**
 * Disconnect from the device with enhanced cleanup
 * Ensures clean disconnection and waits for UI to update
 * Prevents zombie connections by properly notifying the bridge
 *
 * TRA-1148 item 3: this runs in teardown, so it is bounded and it never throws.
 * It used to hang - `page.$()` takes no timeout and does not settle while the
 * page's main thread is busy (rendering a few hundred tags after a dense scan
 * will do it), so a run whose assertions had all passed blew the test timeout
 * and reported as a failure. A cleanup step must not be able to fail a test
 * whose subject already succeeded; every call site here is afterAll/teardown,
 * and none of them assert on disconnect behaviour.
 *
 * Failures are logged loudly rather than silently swallowed - a disconnect that
 * did not complete can leave a zombie bridge session for the next spec, so it
 * still needs to be visible in the log.
 */
export async function disconnectDevice(page: Page): Promise<void> {
  const outcome = await withBudget(
    disconnectDeviceUnbounded(page),
    DISCONNECT_BUDGET_MS,
    'disconnectDevice'
  ).catch((error) => {
    console.warn(`[Connection] Disconnect cleanup failed (continuing teardown): ${error}`);
    return null;
  });

  if (outcome === null) {
    console.warn('[Connection] Disconnect did not complete cleanly - the bridge may need to recover');
  }
}

async function disconnectDeviceUnbounded(page: Page): Promise<'disconnected' | 'already-disconnected'> {
  if (page.isClosed()) {
    console.log('[Connection] Page already closed, nothing to disconnect');
    return 'already-disconnected';
  }

  // Is there a disconnect button? (There is not, if we are already disconnected.)
  //
  // waitForSelector, NOT page.$: page.$ has no timeout and will not settle while
  // the page is busy, which is exactly the teardown hang this replaces.
  const disconnectButton = await page
    .waitForSelector(config.selectors.disconnectButton, {
      state: 'attached',
      timeout: config.timeouts.ui
    })
    .catch(() => null);

  if (disconnectButton) {
    console.log('[Connection] Initiating clean disconnect...');

    // Wait for the button to be enabled (in case it's debounced)
    // Check current reader state before disconnect
    const readerState = await page.evaluate(() => {
      const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
      return deviceStore?.getState().readerState;
    });
    console.log('[Connection] Current reader state before disconnect:', readerState);

    try {
      await page.waitForSelector(config.selectors.disconnectButton + ':not([disabled])', {
        timeout: 2000
      });
    } catch (error) {
      console.log('[Connection] Warning: Disconnect button remained disabled, attempting click anyway');
      // Check what state we're in that's causing the button to be disabled
      const state = await page.evaluate(() => {
        const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
        return {
          readerState: deviceStore?.getState().readerState,
          isConnected: deviceStore?.getState().isConnected
        };
      });
      console.log('[Connection] Device state when button disabled:', state);
    }
    
    // Stop any ongoing operations first
    await cleanupOngoingOperations(page);
    
    // Click disconnect button
    await disconnectButton.click();
    
    // Handle disconnect confirmation dialog if it appears
    try {
      // Look for the confirmation dialog using the data-testid
      const confirmButton = await page.waitForSelector('[data-testid="modal-confirm-button"]', {
        timeout: 2000 // Short timeout - dialog might not always appear
      });
      
      if (confirmButton) {
        console.log('[Connection] Confirming disconnect in dialog');
        await confirmButton.click();
      }
    } catch {
      // Dialog might not appear or might auto-confirm
      console.log('[Connection] No disconnect confirmation dialog detected');
    }
    
    // Wait for disconnection to complete
    await waitForConnectionStatus(page, 'Disconnected');
    
    // TODO: Re-enable battery indicator check once disconnect state reset is fixed
    // Currently causing 10s timeout on every disconnect
    // await page.waitForSelector(config.selectors.batteryIndicator, {
    //   state: 'hidden',
    //   timeout: config.timeouts.ui
    // });
    
    // Wait for the bridge to actually release the command path.
    //
    // This used to be `waitForTimeout(1500)` justified as "0.4.3 has
    // postDisconnectDelay of 1.1s". That constant is from a version we are eight
    // releases past — 0.12.0's postDisconnectDelay is 250ms, measured over 997
    // cycles — so the number was neither current nor derived from anything this
    // suite can observe.
    //
    // It is also the wrong SHAPE. The command-path release completes when the
    // bridge processes the socket close, which is not a fixed duration: under
    // contention it is longer than any sleep anyone would write, and idle it is
    // far shorter. A fixed sleep is therefore simultaneously too slow for the
    // common case and too short for the case that actually fails — which is what
    // `inventory-save` hit, timing out in the NEXT spec's connect while this
    // teardown had already reported success.
    //
    // Poll the observable condition instead: the page reports disconnected and
    // the connect button has come back enabled, which is the UI's own statement
    // that a fresh connect is possible.
    await page
      .waitForSelector(config.selectors.connectButton + ':not([disabled])', {
        timeout: config.timeouts.ui
      })
      .catch(() => {
        // Not fatal in teardown: report it rather than hang. A connect that then
        // fails is a better signal than a teardown that silently passed.
        console.warn('[Connection] Connect button did not re-enable after disconnect');
      });
    
    // Check final state after disconnect
    const finalState = await page.evaluate(() => {
      const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
      if (deviceStore) {
        return deviceStore.getState().readerState;
      }
      return null;
    });
    console.log(`[Connection] Final reader state after disconnect: ${finalState}`);

    console.log('[Connection] Clean disconnect completed');
    return 'disconnected';
  }

  console.log('[Connection] Already disconnected, no action needed');
  return 'already-disconnected';
}

/**
 * Get current connection state from the device store
 */
export async function getConnectionState(page: Page): Promise<{
  isConnected: boolean;
  deviceName: string | null;
  batteryPercentage: number;
}> {
  return await page.evaluate(() => {
    const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
    if (!deviceStore) {
      throw new Error('Device store not found');
    }
    
    const state = deviceStore.getState();
    return {
      isConnected: state.isConnected,
      deviceName: state.deviceName,
      batteryPercentage: state.batteryPercentage
    };
  });
}

/**
 * Wait for auto-reconnection to occur
 * Used in connection loss scenarios
 */
export async function waitForReconnection(page: Page): Promise<void> {
  // Wait for the connection to be lost first
  await page.waitForSelector(config.selectors.batteryIndicator, {
    state: 'hidden',
    timeout: config.timeouts.ui
  });
  
  // Then wait for reconnection
  await page.waitForSelector(config.selectors.batteryIndicator, {
    state: 'visible',
    timeout: config.timeouts.connect
  });
  
  // Verify we're back in ready state
  await waitForConnectionStatus(page, 'Connected');
}

/**
 * Clean up any ongoing operations before disconnect
 * Prevents operations from continuing after disconnect
 */
export async function cleanupOngoingOperations(page: Page): Promise<void> {
  try {
    // Stop inventory if running by releasing trigger
    const isInventoryRunning = await page.evaluate(() => {
      const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
      return tagStore?.getState().isInventoryRunning || false;
    });
    
    if (isInventoryRunning) {
      // Release trigger to stop inventory
      await page.evaluate(() => {
        const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
        if (deviceStore) {
          deviceStore.getState().setTriggerState(false);
        }
      });
      console.log('[Connection] Released trigger to stop inventory');
      await page.waitForTimeout(500);
    }
    
    // Stop locate/search if running
    // Bounded for the same reason as disconnectDevice: page.$ takes no timeout
    // and this runs on the teardown path (TRA-1148 item 3).
    const stopLocateButton = await page
      .waitForSelector('button:has-text("Stop")', { state: 'attached', timeout: 2000 })
      .catch(() => null);
    if (stopLocateButton) {
      await stopLocateButton.click();
      console.log('[Connection] Stopped locate operation');
      await page.waitForTimeout(500);
    }
    
    // Ensure RFID is powered off (via store)
    // TODO: RFID power off is disabled due to firmware issue - device does not send proper response
    // This causes a 5000ms timeout. The command may still execute on the device side.
    // await page.evaluate(() => {
    //   try {
    //     // Try to power off RFID through the store/manager
    //     if ((window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore) {
    //       const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__.deviceStore;
    //       const state = deviceStore.getState();
    //       
    //       // If there's an rfidPowerOff method, call it
    //       if (typeof state.rfidPowerOff === 'function') {
    //         state.rfidPowerOff();
    //       }
    //     }
    //   } catch (error) {
    //     console.warn('[Connection] Could not power off RFID:', error);
    //   }
    // });
    
  } catch (error) {
    console.warn('[Connection] Error during operation cleanup:', error);
  }
}

/**
 * Enhanced disconnect with retry logic
 * Ensures zombie connections are avoided
 */
export async function safeDisconnectDevice(page: Page, maxRetries: number = 2): Promise<void> {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      console.log(`[Connection] Disconnect attempt ${attempt}/${maxRetries}`);
      
      await disconnectDevice(page);
      
      // Verify disconnect succeeded
      const connectButton = await page.waitForSelector(config.selectors.connectButton, {
        timeout: 5000
      });
      
      if (connectButton) {
        console.log('[Connection] Disconnect verified successfully');
        return;
      }
      
    } catch (error) {
      console.warn(`[Connection] Disconnect attempt ${attempt} failed:`, (error as Error).message);
      
      if (attempt === maxRetries) {
        console.error('[Connection] All disconnect attempts failed');
        throw error;
      }
      
      // Wait before retry
      await page.waitForTimeout(2000);
    }
  }
}

/**
 * Simulate connection loss by evaluating in page context
 * Useful for testing reconnection scenarios
 */
export async function simulateConnectionLoss(page: Page): Promise<void> {
  await page.evaluate(() => {
    // Access the transport manager and simulate disconnect
    const transportManager = (window as WindowWithStores).__TRANSPORT_MANAGER__;
    if (transportManager && transportManager.device) {
      // Dispatch gatt disconnect event
      transportManager.device.gatt.disconnect();
    }
  });
}

/**
 * Wait for the page to be able to reach a device.
 *
 * The condition checked here is `navigator.bluetooth` existing in the page,
 * which is true only when the dev server was started in bridge mode — that is
 * what injects the mock. It says nothing whatsoever about the bridge process.
 *
 * That distinction cost an afternoon (TRA-1190). Started as `pnpm vite` instead
 * of `pnpm dev:bridge`, 13 @hardware specs failed with "Bridge server not ready
 * within timeout" while the bridge was up, fresh, verified and idle on its port
 * the entire time. The message named a healthy component, so debugging went to
 * the bridge, then the reader, then the radio — all fine, none of them the
 * fault. The unmet precondition was never mentioned by the error describing it.
 *
 * So the two faults are reported apart, because they have different fixes:
 *
 *   no `navigator.bluetooth`  → the dev server is not in bridge mode. Restart
 *                               it as `pnpm dev:bridge`. The bridge is not
 *                               involved and restarting it changes nothing.
 *   mock present, not usable  → the injection happened but the object is not
 *                               functional; this one really is about the bridge.
 */
export async function waitForBridgeReady(page: Page, timeout: number = 5000): Promise<void> {
  const startTime = Date.now();
  let last = { hasBluetooth: false, hasRequestDevice: false, isMocked: false, hasWebBleMock: false };

  while (Date.now() - startTime < timeout) {
    try {
      // DO NOT manually inject - the dev:bridge server already did this.
      // Just check if it's available.
      last = await page.evaluate(() => ({
        hasBluetooth: !!navigator.bluetooth,
        hasRequestDevice: !!(navigator.bluetooth && typeof navigator.bluetooth.requestDevice === 'function'),
        isMocked: (window as WindowWithStores).__webBluetoothBridged === true,
        hasWebBleMock: typeof (window as WindowWithStores).WebBleMock !== 'undefined'
      }));

      console.log('[Connection] Bridge check:', last);

      if (last.hasBluetooth && last.hasRequestDevice) {
        console.log('[Connection] Bridge server ready');
        return;
      }

    } catch (error) {
      // Continue waiting
    }

    await page.waitForTimeout(100);
  }

  // Nothing was injected at all: this is the dev server, not the bridge.
  if (!last.hasBluetooth && !last.hasWebBleMock) {
    throw new Error(
      'The dev server is not in bridge mode — `navigator.bluetooth` was never ' +
      'injected into the page, so no device can be reached.\n' +
      '  Fix: restart the dev server as `pnpm dev:bridge` (not `pnpm vite`).\n' +
      '  NOTE: this is NOT a bridge fault. A running, healthy bridge produces ' +
      'this exact failure when the page has no mock to talk to it with, so ' +
      'restarting the bridge or power-cycling the reader will not help.'
    );
  }

  // The mock is present but not usable — now the bridge is a fair suspect.
  throw new Error(
    'Web Bluetooth mock is present but not usable within timeout ' +
    `(bluetooth=${last.hasBluetooth} requestDevice=${last.hasRequestDevice} ` +
    `bridged=${last.isMocked} webBleMock=${last.hasWebBleMock}).\n` +
    '  The dev server IS in bridge mode, so check the bridge itself: that it is ' +
    'running, and that no other client already holds the connection.'
  );
}

