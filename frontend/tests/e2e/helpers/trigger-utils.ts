/**
 * Trigger simulation utilities for E2E tests
 * Uses ble-mcp-test v0.7.0 testing API with testSimulateNotification
 */

import type { Page } from '@playwright/test';
import { ReaderState } from './device-state';
import { cs108TriggerPressPacket, cs108TriggerReleasePacket } from '../../config/cs108.config';

/**
 * Simulate trigger press using ble-mcp-test v0.7.0 testing API with retries
 * @param page - Playwright page
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @returns Success status, message, and trigger state
 */
export async function simulateTriggerPress(page: Page, maxRetries: number = 3): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  // First check the initial trigger state
  const initialState = await getTriggerState(page);
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    console.log(`[Trigger] Press attempt ${attempt}/${maxRetries}`);
    
    // Hook into characteristicvaluechanged event to verify notification dispatch
    const result = await page.evaluate(async (packet) => {
      // Check if the v0.7.0 testing API exists
      if (!navigator.bluetooth?.testing?.simulateNotification) {
        console.log('[DEBUG] Testing API not found, checking structure:');
        console.dir(navigator.bluetooth);
        console.dir(navigator.bluetooth?.testing);
        return { 
          success: false, 
          message: 'TESTING_API_NOT_FOUND: navigator.bluetooth.testing.simulateNotification not available. Check ble-mcp-test v0.7.0+ mock is loaded.',
          hasTestingApi: false,
          hasBluetoothApi: !!navigator.bluetooth,
          hasMockFlag: !!window?.__webBluetoothMocked
        };
      }
      
      // Get the current notify characteristic from transport manager
      const tm = window.__TRANSPORT_MANAGER__;
      if (!tm?.notifyCharacteristic) {
        return { 
          success: false, 
          message: 'NOTIFY_CHAR_NOT_FOUND: No notify characteristic found in transport manager. Ensure device is connected.',
          hasTestingApi: true,
          hasTransportManager: !!tm
        };
      }
      
      // Set up event listener to verify the notification reaches the transport
      let eventReceived = false;
      let eventData: Uint8Array | null = null;
      
      const eventHandler = (event: Event) => {
        const bleEvent = event as Event & { target?: { value?: DataView } };
        if (bleEvent?.target?.value) {
          eventReceived = true;
          eventData = new Uint8Array(bleEvent.target.value.buffer);
          console.log('[Trigger] CharacteristicValueChanged event received:', Array.from(eventData).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));
        }
      };
      
      tm.notifyCharacteristic.addEventListener('characteristicvaluechanged', eventHandler);
      
      try {
        // Use the testing API for simulation (ble-mcp-test v0.7+)
        if (navigator.bluetooth?.testing?.simulateNotification) {
          const { simulateNotification } = navigator.bluetooth.testing;
          await simulateNotification({
            characteristic: tm.notifyCharacteristic,
            data: new Uint8Array(packet)
          });
        } else {
          // Fallback: Create and dispatch the event manually
          const dataView = new DataView(new Uint8Array(packet).buffer);
          const event = new Event('characteristicvaluechanged');
          Object.defineProperty(event, 'target', {
            value: {
              value: dataView
            },
            writable: false
          });
          tm.notifyCharacteristic.dispatchEvent(event);
        }

        // Give a brief moment for synchronous event dispatch
        // The event should fire immediately
        let waited = 0;
        while (!eventReceived && waited < 50) {
          // Busy wait for a very short time to catch immediate events
          const start = Date.now();
          while (Date.now() - start < 2) { /* busy wait */ }
          waited += 2;
        }

        tm.notifyCharacteristic.removeEventListener('characteristicvaluechanged', eventHandler);

        return {
          success: eventReceived,
          message: eventReceived ?
            'NOTIFICATION_SENT: Trigger press packet injected successfully' :
            'NOTIFICATION_FAILED: Event was dispatched but not received',
          hasTestingApi: !!navigator.bluetooth?.testing?.simulateNotification,
          hasTransportManager: true,
          eventReceived,
          eventData: eventData ? Array.from(eventData) : null
        };
      } catch (e) {
        tm.notifyCharacteristic.removeEventListener('characteristicvaluechanged', eventHandler);
        return { 
          success: false, 
          message: `NOTIFICATION_ERROR: Failed to inject packet - ${e}`,
          hasTestingApi: true,
          hasTransportManager: true,
          error: String(e)
        };
      }
    }, Array.from(cs108TriggerPressPacket));

    if (!result.success) {
      console.error(`[Trigger] Press simulation failed on attempt ${attempt}:`, result.message);
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
      
      // Wait before retry
      await page.waitForTimeout(200);
      continue;
    }

    // Log event dispatch verification
    if (result.eventReceived && result.eventData) {
      console.log('[Trigger] Event dispatch verified - data reached transport layer');
    } else {
      console.warn('[Trigger] Event dispatch not verified - notification may not have reached transport');
    }

    // Wait up to 500ms for the state to update
    const startTime = Date.now();
    while (Date.now() - startTime < 500) {
      const triggerState = await getTriggerState(page);
      if (triggerState === true) {
        console.log('[Trigger] Press confirmed - state changed from', initialState, 'to', triggerState);
        return { 
          success: true, 
          message: 'STATE_UPDATED: Trigger press successful and state confirmed',
          triggerState: true 
        };
      }
      await page.waitForTimeout(50);
    }

    // State didn't update on this attempt
    const finalState = await getTriggerState(page);
    console.warn(`[Trigger] State did not update on attempt ${attempt}/${maxRetries}`);
    console.warn('[Trigger] Initial state:', initialState, '| Final state:', finalState);
    
    if (attempt < maxRetries) {
      console.log('[Trigger] Retrying press simulation...');
      await page.waitForTimeout(300); // Longer wait between retries
    }
  }

  // All retries exhausted
  const finalState = await getTriggerState(page);
  return { 
    success: false, 
    message: `STATE_NOT_UPDATED: All ${maxRetries} attempts failed. Trigger state did not change from ${initialState} to true. Check deviceManager notification handler.`,
    triggerState: finalState 
  };
}

/**
 * Simulate trigger release using ble-mcp-test v0.7.0 testing API with retries
 * @param page - Playwright page
 * @param maxRetries - Maximum number of retry attempts (default 3)
 * @returns Success status, message, and trigger state
 */
export async function simulateTriggerRelease(page: Page, maxRetries: number = 3): Promise<{ success: boolean; message: string; triggerState: boolean }> {
  // First check the initial trigger state
  const initialState = await getTriggerState(page);
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    console.log(`[Trigger] Release attempt ${attempt}/${maxRetries}`);
    
    // Hook into characteristicvaluechanged event to verify notification dispatch
    const result = await page.evaluate(async (packet) => {
      // Check if the v0.7.0 testing API exists
      if (!navigator.bluetooth?.testing?.simulateNotification) {
        return { 
          success: false, 
          message: 'TESTING_API_NOT_FOUND: navigator.bluetooth.testing.simulateNotification not available. Check ble-mcp-test v0.7.0+ mock is loaded.',
          hasTestingApi: false,
          hasBluetoothApi: !!navigator.bluetooth,
          hasMockFlag: !!window?.__webBluetoothMocked
        };
      }
      
      // Get the current notify characteristic from transport manager
      const tm = window.__TRANSPORT_MANAGER__;
      if (!tm?.notifyCharacteristic) {
        return { 
          success: false, 
          message: 'NOTIFY_CHAR_NOT_FOUND: No notify characteristic found in transport manager. Ensure device is connected.',
          hasTestingApi: true,
          hasTransportManager: !!tm
        };
      }
      
      // Set up event listener to verify the notification reaches the transport
      let eventReceived = false;
      let eventData: Uint8Array | null = null;
      
      const eventHandler = (event: Event) => {
        const bleEvent = event as Event & { target?: { value?: DataView } };
        if (bleEvent?.target?.value) {
          eventReceived = true;
          eventData = new Uint8Array(bleEvent.target.value.buffer);
          console.log('[Trigger] CharacteristicValueChanged event received:', Array.from(eventData).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));
        }
      };
      
      tm.notifyCharacteristic.addEventListener('characteristicvaluechanged', eventHandler);
      
      try {
        // Use the testing API for simulation (ble-mcp-test v0.7+)
        if (navigator.bluetooth?.testing?.simulateNotification) {
          const { simulateNotification } = navigator.bluetooth.testing;
          await simulateNotification({
            characteristic: tm.notifyCharacteristic,
            data: new Uint8Array(packet)
          });
        } else {
          // Fallback: Create and dispatch the event manually
          const dataView = new DataView(new Uint8Array(packet).buffer);
          const event = new Event('characteristicvaluechanged');
          Object.defineProperty(event, 'target', {
            value: {
              value: dataView
            },
            writable: false
          });
          tm.notifyCharacteristic.dispatchEvent(event);
        }

        // Give a brief moment for synchronous event dispatch
        // The event should fire immediately
        let waited = 0;
        while (!eventReceived && waited < 50) {
          // Busy wait for a very short time to catch immediate events
          const start = Date.now();
          while (Date.now() - start < 2) { /* busy wait */ }
          waited += 2;
        }

        tm.notifyCharacteristic.removeEventListener('characteristicvaluechanged', eventHandler);

        return {
          success: eventReceived,
          message: eventReceived ?
            'NOTIFICATION_SENT: Trigger release packet injected successfully' :
            'NOTIFICATION_FAILED: Event was dispatched but not received',
          hasTestingApi: !!navigator.bluetooth?.testing?.simulateNotification,
          hasTransportManager: true,
          eventReceived,
          eventData: eventData ? Array.from(eventData) : null
        };
      } catch (e) {
        tm.notifyCharacteristic.removeEventListener('characteristicvaluechanged', eventHandler);
        return { 
          success: false, 
          message: `NOTIFICATION_ERROR: Failed to inject packet - ${e}`,
          hasTestingApi: true,
          hasTransportManager: true,
          error: String(e)
        };
      }
    }, Array.from(cs108TriggerReleasePacket));

    if (!result.success) {
      console.error(`[Trigger] Release simulation failed on attempt ${attempt}:`, result.message);
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
      
      // Wait before retry
      await page.waitForTimeout(200);
      continue;
    }

    // Log event dispatch verification
    if (result.eventReceived && result.eventData) {
      console.log('[Trigger] Event dispatch verified - data reached transport layer');
    } else {
      console.warn('[Trigger] Event dispatch not verified - notification may not have reached transport');
    }

    // Wait up to 500ms for the state to update
    const startTime = Date.now();
    while (Date.now() - startTime < 500) {
      const triggerState = await getTriggerState(page);
      if (triggerState === false) {
        console.log('[Trigger] Release confirmed - state changed from', initialState, 'to', triggerState);
        return { 
          success: true, 
          message: 'STATE_UPDATED: Trigger release successful and state confirmed',
          triggerState: false 
        };
      }
      await page.waitForTimeout(50);
    }

    // State didn't update on this attempt
    const finalState = await getTriggerState(page);
    console.warn(`[Trigger] State did not update on attempt ${attempt}/${maxRetries}`);
    console.warn('[Trigger] Initial state:', initialState, '| Final state:', finalState);
    
    if (attempt < maxRetries) {
      console.log('[Trigger] Retrying release simulation...');
      await page.waitForTimeout(300); // Longer wait between retries
    }
  }

  // All retries exhausted
  const finalState = await getTriggerState(page);
  return { 
    success: false, 
    message: `STATE_NOT_UPDATED: All ${maxRetries} attempts failed. Trigger state did not change from ${initialState} to false. Check deviceManager notification handler.`,
    triggerState: finalState 
  };
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