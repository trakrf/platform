/**
 * CS108 Barcode Integration Tests
 *
 * Tests barcode scanning functionality following the baseline pattern:
 * - Use public API only (setMode, startScanning, stopScanning)
 * - Verify state changes
 * - Wait for events from notification stream
 * - Never use internal access or RPC patterns
 */

import { describe, it, expect, beforeAll, afterAll, beforeEach } from 'vitest';
import { CS108WorkerTestHarness } from './CS108WorkerTestHarness';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';
import { BARCODE_TEST_TAG } from '@test-utils/constants';

describe('CS108 Barcode Integration', () => {
  let harness: CS108WorkerTestHarness;

  beforeAll(async () => {
    console.log('\n🔧 Connecting to CS108 hardware for barcode tests...');
    harness = new CS108WorkerTestHarness();
    await harness.initialize(true); // Connect ONCE for all barcode tests

    // Call connect() on the worker and verify state transitions
    console.log('📡 Calling worker connect()...');
    const connected = await harness.connect();
    expect(connected).toBe(true);
    console.log('✓ Connected to CS108');

    // Now set mode to BARCODE for all tests
    console.log('🔧 Setting mode to BARCODE...');
    const setModePromise = harness.setMode(ReaderMode.BARCODE);

    // Should transition to BUSY during mode switch
    const busyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.BUSY
    );
    expect(busyEvent.payload.readerState).toBe(ReaderState.BUSY);
    console.log('✓ State: BUSY (configuring)');

    // Should transition to READY after setMode(BARCODE) completes
    const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === ReaderState.CONNECTED
    );
    expect(readyEvent.payload.readerState).toBe(ReaderState.CONNECTED);
    console.log('✓ State: READY');

    // Wait for mode switch to complete
    await setModePromise;
    console.log('✓ Mode set to BARCODE');
  });

  afterAll(async () => {
    console.log('🔧 Disconnecting from CS108...');
    await harness.cleanup();
    console.log('✓ Disconnected');
  });

  beforeEach(async () => {
    console.log('\n[Setup] Setting mode to BARCODE...');

    // Clear any events from previous test
    harness.clearEvents();

    // Check if we're already in BARCODE mode
    const currentMode = harness.getReaderMode();
    console.log(`[Setup] Current mode: ${currentMode}`);

    if (currentMode === ReaderMode.BARCODE) {
      console.log('[Setup] Already in BARCODE mode, no mode change needed');
    } else {
      // Set mode to BARCODE for each test
      const setModePromise = harness.setMode(ReaderMode.BARCODE);

      // Should transition to BUSY state during configuration
      const busyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
        event => event.payload.readerState === ReaderState.BUSY
      );
      expect(busyEvent.payload.readerState).toBe(ReaderState.BUSY);
      console.log('[Setup] State: BUSY (configuring)');

      // Wait for mode change to complete
      const modeEvent = await harness.waitForEvent(WorkerEventType.READER_MODE_CHANGED,
        event => event.payload.mode === ReaderMode.BARCODE
      );
      expect(modeEvent.payload.mode).toBe(ReaderMode.BARCODE);

      // Should transition to READY state after configuration
      const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
        event => event.payload.readerState === ReaderState.CONNECTED
      );
      expect(readyEvent.payload.readerState).toBe(ReaderState.CONNECTED);
      console.log('[Setup] State: READY');

      await setModePromise;
    }

    console.log('[Setup] BARCODE mode ready');
  });

  afterEach(async () => {
    console.log('[Cleanup] Adding 5s delay for bridge recovery...');
    await new Promise(resolve => setTimeout(resolve, 5000));
    console.log('[Cleanup] Bridge recovery delay complete');
  });

  it('should verify BARCODE mode is active', async () => {
    console.log('\n🧪 Verifying barcode mode...');

    // Mode was set in beforeEach, just verify it's correct
    const mode = harness.getReaderMode();
    console.log(`[Test] Current mode: ${mode}`);
    expect(mode).toBe(ReaderMode.BARCODE);

    console.log('✅ Barcode mode confirmed!');
  });

  it.skip('should start and stop barcode scanning', async () => {
    console.log('\n🧪 Testing barcode scanning control...');

    // Mode already set in beforeEach
    harness.clearEvents();

    // 3. PERFORM ACTION - Start scanning
    console.log('[Test] Starting barcode scanning...');
    await harness.startScanning();

    // 4. WAIT FOR EVENTS - Wait for scanning state
    const scanningEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === 'SCANNING'
    );
    expect(scanningEvent).toBeDefined();

    // Stop scanning
    console.log('[Test] Stopping barcode scanning...');
    await harness.stopScanning();

    // Wait for ready state
    const readyEvent = await harness.waitForEvent(WorkerEventType.READER_STATE_CHANGED,
      event => event.payload.readerState === 'READY'
    );
    expect(readyEvent).toBeDefined();

    console.log('✅ Barcode scanning control working!');
  });

  it('should handle trigger press for barcode scanning', async () => {
    console.log('\n🧪 Testing trigger-based barcode scanning...');

    // Mode already set in beforeEach
    harness.clearEvents();

    // Simulate trigger press - should start scanning
    console.log('[Test] Simulating trigger press...');
    await harness.simulateTriggerPress();

    // Wait for the trigger event to be processed
    const triggerPressEvent = await harness.waitForEvent(
      WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === true,
      2000
    );
    expect(triggerPressEvent).toBeDefined();
    expect(triggerPressEvent.payload.pressed).toBe(true);
    console.log('[Test] Trigger press event received');

    // Simulate trigger release - should stop scanning
    console.log('[Test] Simulating trigger release...');
    await harness.simulateTriggerRelease();

    // Wait for the trigger release event
    const triggerReleaseEvent = await harness.waitForEvent(
      WorkerEventType.TRIGGER_STATE_CHANGED,
      event => event.payload.pressed === false,
      2000
    );
    expect(triggerReleaseEvent).toBeDefined();
    expect(triggerReleaseEvent.payload.pressed).toBe(false);
    console.log('[Test] Trigger release event received');

    console.log('✅ Trigger-based barcode scanning working!');
  });

  // This test requires physical barcode to be positioned in front of scanner
  it.skip('should read physical barcode when positioned correctly', async () => {
    console.log('\n🧪 Testing physical barcode read...');
    console.log(`⚠️  Position barcode "${BARCODE_TEST_TAG}" in front of scanner`);

    // Mode already set in beforeEach

    // Start scanning
    await harness.startScanning();

    // Give hardware time to read physical barcode
    console.log('[Test] Waiting for barcode read (10 second timeout)...');
    const testBarcode = BARCODE_TEST_TAG;

    const barcodeEvent = await harness.waitForEvent(WorkerEventType.BARCODE_READ,
      event => event.payload.barcode === testBarcode,
      10000  // 10 second timeout for physical barcode scanning
    );

    expect(barcodeEvent.payload.barcode).toBe(testBarcode);
    console.log(`✅ Successfully read barcode: ${testBarcode}`);

    // Stop scanning
    await harness.stopScanning();
  });
});