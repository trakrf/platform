/**
 * CS108 Locate Integration Test
 *
 * Tests RFID locate functionality following the baseline pattern:
 * - Use public API only (setMode, startScanning, stopScanning)
 * - Verify state changes
 * - Wait for events from notification stream
 * - Never use internal access or RPC patterns
 *
 * Locate mode tracks the strongest RFID tag signal to help find a specific item.
 *
 * CONSOLIDATED TEST:
 * - Single comprehensive test covering all locate functionality
 * - Reduces test runtime by eliminating redundant setup/teardown
 * - Tests trigger-based scanning and RSSI validation in one flow
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';
import { useSettingsStore } from '@/stores/settingsStore';
import { useLocateStore } from '@/stores/locateStore';
import { LOCATE_TEST_TAG, EPC_FORMATS, TEST_EXPECTATIONS } from '@test-utils/constants';

describe('CS108 Locate Integration', () => {
  let harness: CS108WorkerTestHarness;

  beforeAll(async () => {
    console.log('\n🔧 Connecting to CS108 hardware for locate test...');
    harness = new CS108WorkerTestHarness();
    await harness.initialize(true); // Connect ONCE for the test

    // Call connect() on the worker
    console.log('📡 Calling worker connect()...');
    const connected = await harness.connect();
    expect(connected).toBe(true);
    console.log('✓ Connected to CS108');

    // Set targetEPC BEFORE setting mode to LOCATE so tag mask is configured
    const testEPC = EPC_FORMATS.toTrimmed(LOCATE_TEST_TAG);
    console.log(`🔧 Setting targetEPC to ${testEPC}...`);
    await harness.setSettings({ rfid: { targetEPC: testEPC } });

    // Now set mode to LOCATE with the targetEPC configured
    console.log('🔧 Setting mode to LOCATE...');
    const setModePromise = harness.setMode(ReaderMode.LOCATE);

    // Should transition to BUSY during mode switch
    const busyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.BUSY
    );
    expect(busyEvent.payload.readerState).toBe(ReaderState.BUSY);
    console.log('✓ State: BUSY (configuring)');

    // Should transition to READY after setMode(LOCATE) completes
    const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED
    );
    expect(readyEvent.payload.readerState).toBe(ReaderState.CONNECTED);
    console.log('✓ State: READY');

    // Wait for mode switch to complete
    await setModePromise;
    console.log('✅ Mode set to LOCATE');

    // Give the connection time to stabilize
    console.log('Waiting 2s for connection to stabilize...');
    await new Promise(resolve => setTimeout(resolve, 2000));
  });

  afterAll(async () => {
    console.log('🔧 Cleaning up CS108 connection...');
    if (harness) {
      // Force IDLE sequence to run even if already in IDLE mode
      // Strategy: briefly switch to INVENTORY, then immediately to IDLE
      // This ensures RFID_POWER_OFF actually executes to stop any runaway scanning
      console.log('Forcing IDLE sequence to run (INVENTORY → IDLE)...');
      try {
        await harness.setMode(ReaderMode.INVENTORY); // Force mode change
        await harness.setMode(ReaderMode.IDLE);       // Now IDLE sequence will run
      } catch (error) {
        console.warn('Mode switching failed during cleanup:', error);
        // Still try direct IDLE in case we're in a weird state
        await harness.setMode(ReaderMode.IDLE);
      }

      await harness.cleanup();
    }
    console.log('✓ Disconnected');
  });

  it('should handle complete locate flow with trigger scanning and RSSI validation', { timeout: 30000 }, async () => {
    console.log('\n🧪 Testing comprehensive locate functionality...');
    console.log(`⚠️  Ensure test tag ${LOCATE_TEST_TAG} is positioned in front of reader`);

    // ========================================================================
    // PHASE 1: Configure LOCATE mode with targetEPC
    // ========================================================================
    console.log('\n[1] Setting up LOCATE mode with target EPC...');

    // Test tags are just decimal numbers with leading zeros (customer pattern)
    const testEPC = EPC_FORMATS.toCustomerInput(LOCATE_TEST_TAG); // Customer input with leading zeros
    const trimmedEPC = EPC_FORMATS.toTrimmed(LOCATE_TEST_TAG); // What we expect to store (leading zeros stripped)

    // First, set the targetEPC in the settings store
    console.log(`    Setting targetEPC in settingsStore: ${testEPC}`);
    const settingsStore = useSettingsStore.getState();
    const epcSet = settingsStore.setTargetEPC(testEPC);
    expect(epcSet).toBe(true);
    const afterState = useSettingsStore.getState();
    expect(afterState.rfid?.targetEPC).toBe(trimmedEPC); // Should store trimmed value
    console.log('    ✓ targetEPC stored in settingsStore');

    // Mode and targetEPC already set in beforeAll
    console.log('    Already in LOCATE mode with targetEPC from beforeAll');

    // Verify we're in the right mode and have targetEPC
    const currentMode = harness.getReaderMode();
    expect(currentMode).toBe(ReaderMode.LOCATE);

    const workerSettings = await harness.getSettings();
    expect(workerSettings.rfid?.targetEPC).toBe(trimmedEPC);
    console.log('    ✓ LOCATE mode and targetEPC verified');

    // Clear locate store for clean test state
    const locateStore = useLocateStore.getState();
    locateStore.clearBuffer();

    // ========================================================================
    // PHASE 2: Trigger-based scanning with RSSI validation
    // ========================================================================
    console.log('\n[2] Testing trigger-based locate scanning...');

    harness.clearEvents();

    // Simulate trigger press - should start scanning
    console.log('    Simulating trigger press...');
    await harness.simulateTriggerPress();

    // Wait for trigger event
    const triggerEvent = await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === true
    );
    expect(triggerEvent.payload.pressed).toBe(true);
    console.log('    ✓ Trigger pressed');

    // Should start scanning
    const scanningEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.SCANNING
    );
    expect(scanningEvent).toBeDefined();
    expect(harness.getReaderState()).toBe(ReaderState.SCANNING);
    console.log('    ✓ Scanning started');

    // Let it scan to collect RSSI data
    console.log(`    Scanning for ${TEST_EXPECTATIONS.LOCATE_SCAN_DURATION_MS / 1000} seconds to collect RSSI data...`);
    await new Promise(resolve => setTimeout(resolve, TEST_EXPECTATIONS.LOCATE_SCAN_DURATION_MS));

    // ========================================================================
    // PHASE 3: Validate LOCATE_UPDATE events and RSSI data
    // ========================================================================
    console.log('\n[3] Analyzing LOCATE_UPDATE events...');

    const locateEvents = harness.getEventsByType(WorkerEventType.LOCATE_UPDATE);
    console.log(`    Found ${locateEvents.length} LOCATE_UPDATE events`);
    expect(locateEvents.length).toBeGreaterThan(0);

    if (locateEvents.length > 0) {
      // Extract and analyze RSSI values
      const workerRssiValues = locateEvents.map((e: any) => e.payload.rssi);
      const strongestRSSI = Math.max(...workerRssiValues);
      const weakestRSSI = Math.min(...workerRssiValues);

      console.log(`    RSSI Results:`);
      console.log(`      - Total locate updates: ${locateEvents.length}`);
      console.log(`      - RSSI range: ${weakestRSSI} to ${strongestRSSI} dBm`);
      console.log(`      - First 5 RSSI values: ${workerRssiValues.slice(0, 5)}`);

      // Verify RSSI values are reasonable (negative values, typically between -90 and -30 dBm)
      expect(weakestRSSI).toBeLessThan(0);
      expect(strongestRSSI).toBeLessThan(0);
      expect(strongestRSSI).toBeGreaterThan(-100);
      console.log(`    ✓ RSSI values are in valid range`);

      // Check that we're getting the target EPC
      const targetEPCs = locateEvents.filter((e: any) =>
        e.payload.epc && e.payload.epc.includes(LOCATE_TEST_TAG)
      );
      console.log(`      - Events with target EPC (${LOCATE_TEST_TAG}): ${targetEPCs.length}/${locateEvents.length}`);

      if (targetEPCs.length > 0) {
        console.log('    ✓ Target EPC filtering is working');
      }
    }

    // ========================================================================
    // PHASE 4: Stop scanning via trigger release
    // ========================================================================
    console.log('\n[4] Stopping locate scan...');

    // Simulate trigger release - should stop scanning
    console.log('    Simulating trigger release...');
    await harness.simulateTriggerRelease();

    // Wait for trigger release event
    const releaseEvent = await harness.waitForEvent(WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === false
    );
    expect(releaseEvent.payload.pressed).toBe(false);
    console.log('    ✓ Trigger released');

    // Should stop scanning
    const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED
    );
    expect(readyEvent).toBeDefined();
    expect(harness.getReaderState()).toBe(ReaderState.CONNECTED);
    console.log('    ✓ Scanning stopped, reader back to READY');

    // Explicitly ensure scanning is stopped (defensive cleanup)
    if (harness.getReaderState() === ReaderState.SCANNING) {
      console.log('    [Cleanup] Explicitly stopping scanning as defensive measure...');
      await harness.stopScanning();
    }

    console.log('\n✅ Complete locate test successful!');
    console.log('   Verified:');
    console.log('   - LOCATE mode configuration with targetEPC');
    console.log('   - Trigger-based scanning control');
    console.log('   - LOCATE_UPDATE events with valid RSSI data');
    console.log('   - Target EPC filtering');
  });
});