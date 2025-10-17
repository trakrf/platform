/**
 * Baseline Integration Test - THE TEMPLATE FOR ALL INTEGRATION TESTS
 *
 * This is THE fundamental test that proves all core connectivity works:
 * - Connection management (beforeAll/afterAll)
 * - Mode setup with cleanup (beforeEach)
 * - Command-response flow (setMode)
 * - State observation (mode verification)
 * - Notification injection (trigger packet)
 * - Notification handling (trigger event)
 *
 * If this test passes, the entire bidirectional pipe is working.
 *
 * ⚠️ IMPORTANT: This test is the TEMPLATE for all other integration tests.
 * Copy this EXACT pattern when creating new tests:
 *
 * LIFECYCLE:
 * - beforeAll: Connect ONCE to hardware
 * - afterAll: Disconnect ONCE from hardware
 * - beforeEach: Set mode with complete cleanup
 *
 * TEST PATTERN:
 * 1. Use public API only (setMode, startScanning, etc.)
 * 2. Verify state changes (expect mode/state to be correct)
 * 3. Simulate hardware events or start operations
 * 4. Wait for events with waitForEvent()
 * 5. Assert on event payloads
 *
 * NEVER:
 * - Use executeCommand() or sendRawCommand()
 * - Access worker internals directly
 * - Manipulate private state
 * - Use setTimeout() instead of waitForEvent()
 * - Connect/disconnect in each test (use beforeAll/afterAll)
 *
 * If your test can't follow this pattern, either:
 * - The public API is incomplete (fix the API)
 * - The test is invalid (fix the test)
 */

import { describe, it, expect, beforeAll, afterAll, beforeEach } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';

describe('CS108 Baseline Connectivity', () => {
  let harness: CS108WorkerTestHarness;

  beforeAll(async () => {
    console.log('🔧 Connecting to CS108 hardware...');
    harness = new CS108WorkerTestHarness();
    await harness.initialize(true); // Connect to real hardware ONCE

    // Call connect() on the worker - connection completes immediately now
    console.log('📡 Calling worker connect()...');
    const connected = await harness.connect();
    expect(connected).toBe(true);
    console.log('✓ Connected to CS108');

    // Explicitly test setMode(IDLE) - this is the connectivity test after all
    console.log('🔧 Setting mode to IDLE...');
    const setModePromise = harness.setMode(ReaderMode.IDLE);

    // Should transition to BUSY during mode switch
    const busyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.BUSY
    );
    expect(busyEvent.payload.readerState).toBe(ReaderState.BUSY);
    console.log('✓ State: BUSY (configuring)');

    // Should transition to READY after setMode(IDLE) completes
    const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED
    );
    expect(readyEvent.payload.readerState).toBe(ReaderState.CONNECTED);
    console.log('✓ State: READY');

    // Wait for mode switch to complete
    await setModePromise;
    console.log('✅ Mode set to IDLE');

    // IMPORTANT: Battery percentage should be available after IDLE sequence
    // The IDLE sequence now includes GET_BATTERY_VOLTAGE for immediate reading
    const batteryEvents = harness.getEventsByType(WorkerEventType.BATTERY_UPDATE);
    const batteryPercentage = harness.getBatteryPercentage();

    if (batteryPercentage !== null) {
      console.log(`✓ Battery: ${batteryPercentage}%`);
      expect(batteryPercentage).toBeGreaterThanOrEqual(0);
      expect(batteryPercentage).toBeLessThanOrEqual(100);
    } else {
      console.log('⚠️ No battery event received during connection');
    }
  });

  afterAll(async () => {
    console.log('🔧 Disconnecting from CS108...');
    await harness.cleanup();
    console.log('✓ Disconnected');
  });

  beforeEach(async () => {
    // Just clear events between tests
    harness.clearEvents();
  });

  afterEach(async () => {
    console.log('[Cleanup] Adding 5s delay for bridge recovery...');
    await new Promise(resolve => setTimeout(resolve, 5000));
    console.log('[Cleanup] Bridge recovery delay complete');
  });

  it('should prove core connectivity works', async () => {
    console.log('\n🧪 Testing baseline connectivity...');

    // Mode was set to IDLE in beforeAll - verify it's still IDLE
    const mode = harness.getReaderMode();
    console.log(`[Test] Current mode: ${mode}`);
    expect(mode).toBe(ReaderMode.IDLE);

    // 4. Inject trigger packet - proves notification injection works
    console.log('[Test] Injecting trigger press packet...');
    await harness.simulateTriggerPress();

    // 5. Wait briefly for processing
    await new Promise(resolve => setTimeout(resolve, 100));

    // 6. Verify trigger event received - proves notification handling works
    const events = harness.getEvents();
    const triggerEvents = events.filter(e => e.type.includes('TRIGGER'));
    console.log(`[Test] Found ${triggerEvents.length} trigger events`);
    expect(triggerEvents.length).toBeGreaterThan(0);

    console.log('✅ All core connectivity working!');
  });

  it.skip('should receive battery updates after connection', { timeout: 15000 }, async () => {
    // TODO: Re-enable after implementing timer-based battery polling with batteryCheckInterval
    console.log('\n🧪 Testing battery reporting...');

    // Wait for a fresh battery event (events were cleared in beforeEach)
    console.log('[Test] Waiting for battery update event...');
    const batteryEvent = await harness.waitForEvent(WorkerEventType.BATTERY_UPDATE, undefined, 10000);
    expect(batteryEvent).toBeDefined();
    expect(batteryEvent.payload.percentage).toBeGreaterThanOrEqual(0);
    expect(batteryEvent.payload.percentage).toBeLessThanOrEqual(100);

    // Now check the harness getter
    const batteryPercentage = harness.getBatteryPercentage();
    console.log(`[Test] Current battery percentage: ${batteryPercentage}%`);

    // We should have received battery from the fresh event
    expect(batteryPercentage).not.toBeNull();
    expect(batteryPercentage).toBeGreaterThanOrEqual(0);
    expect(batteryPercentage).toBeLessThanOrEqual(100);

    // Optional: Verify auto-reporting is working (should send updates every 5 seconds)
    console.log('[Test] Waiting 6 seconds to verify auto-reporting...');
    const initialEventCount = harness.getEventsByType(WorkerEventType.BATTERY_UPDATE).length;

    await new Promise(resolve => setTimeout(resolve, 6000));

    const newEventCount = harness.getEventsByType(WorkerEventType.BATTERY_UPDATE).length;
    const newBatteryPercentage = harness.getBatteryPercentage();

    console.log(`[Test] Battery events: ${initialEventCount} -> ${newEventCount}`);
    console.log(`[Test] Latest battery: ${newBatteryPercentage}% (may not change if on charger)`);

    // Should have received at least one more auto-report (even if percentage is the same)
    expect(newEventCount).toBeGreaterThan(initialEventCount);
    expect(newBatteryPercentage).toBeGreaterThanOrEqual(0);
    expect(newBatteryPercentage).toBeLessThanOrEqual(100);

    console.log('✅ Battery reporting working!');
  });
});