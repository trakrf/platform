# CS108 Integration Test Migration Status

## Architecture: Bidirectional Stream Pattern

All tests follow the baseline connectivity pattern:
- Commands sent via public API (setMode, startScanning, stopScanning)
- ALL responses received through notification handler
- NO request-response patterns at transport layer
- Worker methods handle protocol intelligence

## Test Organization

Tests map directly to ReaderMode enums:
- `connectivity.spec.ts` - Baseline connectivity and template
- `inventory.spec.ts` - INVENTORY mode (RFID tag reading)
- `barcode.spec.ts` - BARCODE mode (barcode scanning)
- `locate.spec.ts` - LOCATE mode (find specific tag by signal strength)

## Test Status

### ✅ COMPLETE - Following Baseline Pattern
- `connectivity.spec.ts` - THE baseline test and template for all others
- `inventory.spec.ts` - RFID inventory scanning with real tags
- `barcode.spec.ts` - Barcode scanning with trigger control
- `locate.spec.ts` - Locate mode for finding specific tags

### 🗑️ DELETED - Redundant/Unnecessary
- `trigger.spec.ts` - Functionality covered by mode-specific tests
- `trigger-store-update.spec.ts` - Store updates tested implicitly
- `trigger-unit.spec.ts` - Unit-level testing not needed
- `instantiation.spec.ts` - Basic setup covered by connectivity test
- `mode-transitions.spec.ts` - Mode changes tested in each mode test
- `notification-integration.spec.ts` - Notifications tested throughout
- `battery.spec.ts` - Battery updates tested implicitly
- `sequences.spec.ts` - Command sequences tested in mode tests
- `trigger-simple.spec.ts` - Replaced by connectivity test
- `barcode-hardware.spec.ts` - Merged into barcode.spec.ts

## Key Testing Patterns

### ✅ CORRECT - Connect Once, Set Mode Per Test
```typescript
// Connect ONCE per test file (realistic usage pattern)
beforeAll(async () => {
  harness = new CS108WorkerTestHarness();
  await harness.initialize(true); // Single WebSocket connection
});

afterAll(async () => {
  await harness.cleanup(); // Clean disconnect
});

// Set mode before EACH test (guarantees clean state)
beforeEach(async () => {
  harness.clearEvents();
  await harness.setMode(ReaderMode.INVENTORY);
  await harness.waitForEvent(WorkerEventType.READER_MODE_CHANGED,
    event => event.payload.mode === ReaderMode.INVENTORY
  );
});
```

This pattern mimics real-world usage:
- Users connect once and use the reader all day
- Mode changes happen between operations (like switching tabs)
- Each setMode() call performs complete cleanup of previous mode
- Notification handlers are cleared and re-initialized
- Reduces WebSocket/bridge resource pressure
- Faster test execution

**Mode Cleanup Behavior:**
When `setMode()` is called, it:
1. Aborts any running command sequences
2. Clears all notification handlers (complete state reset)
3. Powers off RFID/Barcode modules (IDLE sequence)
4. Configures hardware for new mode
5. Re-initializes handlers for new mode

This ensures 100% cleanup when switching modes, just like navigating between tabs in the app.

### ✅ CORRECT - Bidirectional Stream Pattern
```typescript
// Use worker methods that handle protocol properly
await harness.startScanning();

// Wait for events from the notification stream
const event = await harness.waitForEvent(WorkerEventType.TAG_READ);
```

### ❌ WRONG - Request/Response Pattern
```typescript
// NEVER use these patterns
const response = await harness.executeCommand(cmd);
const response = await client.sendRawCommand(cmd);
```

## Running Tests

```bash
# Run all integration tests
pnpm test tests/integration/cs108

# Run specific mode test
pnpm test connectivity.spec.ts
pnpm test inventory.spec.ts
pnpm test barcode.spec.ts
pnpm test locate.spec.ts

# Hardware smoke test (verify connection)
pnpm test:hardware
```

## Test Coverage

Each mode test covers:
1. Mode transition verification
2. Start/stop scanning via public API
3. Trigger press/release simulation
4. Event verification from notification stream
5. State observation (read-only)

The tests prove that:
- The bidirectional byte pipe works end-to-end
- Public API is sufficient for all operations
- Input source agnosticism (trigger vs API) works
- Notification stream delivers all events correctly