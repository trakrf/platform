# CS108 Integration Test Template

## The Baseline Pattern

Every integration test should follow the pattern established in `baseline-connectivity.spec.ts`:

```typescript
describe('Feature Being Tested', () => {
  let harness: CS108WorkerTestHarness;

  beforeEach(async () => {
    harness = new CS108WorkerTestHarness();
    await harness.initialize(true);
  });

  afterEach(async () => {
    await harness.cleanup();
  });

  it('should do something specific', async () => {
    // 1. SET MODE - Use public API to configure reader
    await harness.setMode(ReaderMode.INVENTORY);

    // 2. VERIFY STATE - Confirm mode change worked
    expect(harness.getReaderMode()).toBe(ReaderMode.INVENTORY);

    // 3. PERFORM ACTION - Start operation or simulate hardware event
    await harness.startScanning();
    // OR
    await harness.simulateTriggerPress();

    // 4. WAIT FOR EVENTS - Use event system, not timeouts
    const event = await harness.waitForEvent('TAG_READ');

    // 5. ASSERT RESULTS - Verify expected behavior
    expect(event.payload.tags).toBeDefined();
  });
});
```

## Why This Pattern Works

1. **Public API Only**: Just like production code would use it
2. **State Verification**: Proves commands actually worked
3. **Event-Driven**: Uses the real notification stream
4. **No Internals**: Can't cheat by manipulating private state
5. **Production-Like**: If it works here, it works in the app

## Examples for Different Features

### Inventory Test
```typescript
it('should read inventory tags', async () => {
  // Setup mode
  await harness.setMode(ReaderMode.INVENTORY);
  expect(harness.getReaderMode()).toBe(ReaderMode.INVENTORY);

  // Start scanning
  await harness.startScanning();

  // Wait for tags
  const tagEvent = await harness.waitForEvent('TAG_READ');
  expect(tagEvent.payload.tags.length).toBeGreaterThan(0);

  // Stop scanning
  await harness.stopScanning();
});
```

### Barcode Test
```typescript
it('should read barcodes', async () => {
  // Setup mode
  await harness.setMode(ReaderMode.BARCODE);
  expect(harness.getReaderMode()).toBe(ReaderMode.BARCODE);

  // Simulate trigger
  await harness.simulateTriggerPress();

  // Simulate barcode data
  await harness.simulateBarcodeRead('123456789');

  // Wait for barcode
  const barcodeEvent = await harness.waitForEvent('BARCODE_READ');
  expect(barcodeEvent.payload.barcode).toBe('123456789');
});
```

### Battery Test
```typescript
it('should report battery level', async () => {
  // Connect starts battery reporting
  await harness.setMode(ReaderMode.IDLE);
  expect(harness.getReaderMode()).toBe(ReaderMode.IDLE);

  // Wait for battery update
  const batteryEvent = await harness.waitForEvent('BATTERY_UPDATE',
    event => event.payload.percentage !== undefined
  );

  expect(batteryEvent.payload.percentage).toBeGreaterThanOrEqual(0);
  expect(batteryEvent.payload.percentage).toBeLessThanOrEqual(100);
});
```

### Mode Transition Test
```typescript
it('should transition between modes', async () => {
  // Start in IDLE
  await harness.setMode(ReaderMode.IDLE);
  expect(harness.getReaderMode()).toBe(ReaderMode.IDLE);

  // Transition to INVENTORY
  await harness.setMode(ReaderMode.INVENTORY);

  // Wait for mode change event
  const modeEvent = await harness.waitForEvent('READER_MODE_CHANGED',
    event => event.payload.mode === ReaderMode.INVENTORY
  );

  expect(modeEvent.payload.mode).toBe(ReaderMode.INVENTORY);
  expect(harness.getReaderMode()).toBe(ReaderMode.INVENTORY);
});
```

## The Golden Rules

1. **Always verify state changes**: Don't assume `setMode()` worked
2. **Use events, not delays**: `waitForEvent()` not `setTimeout()`
3. **Test one thing**: Each test should verify one specific behavior
4. **Clean state**: Don't depend on other tests' side effects
5. **Public API only**: If you need internals, the API is incomplete

## Migration Checklist

When migrating an existing test:

- [ ] Remove all `executeCommand()` calls
- [ ] Remove all `sendRawCommand()` calls
- [ ] Replace with public API methods
- [ ] Add state verification after mode changes
- [ ] Use `waitForEvent()` instead of timeouts
- [ ] Verify it still passes
- [ ] If it doesn't pass, you found a bug!

## Summary

The baseline connectivity test proves the entire system works. Every other test should follow its pattern: use the public API, verify state changes, wait for events, assert on results. This ensures tests validate real production behavior, not test-specific workarounds.