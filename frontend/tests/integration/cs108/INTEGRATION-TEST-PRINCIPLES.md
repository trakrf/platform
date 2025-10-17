# Integration Test Architecture Principles

## 🚨 THE GOLDEN RULE
**If you need to manipulate internals to make a test pass, you've found a production bug.**

Integration tests MUST interact with the worker exactly like production code does. No special access, no test-only hooks, no internal manipulation.

## The Three Laws of Integration Testing

### 1. ✅ INTERACT - Only Through Public API
```typescript
// CORRECT - Use the public API
await harness.connect();
await harness.setMode(ReaderMode.INVENTORY);
await harness.startScanning();
await harness.stopScanning();
await harness.disconnect();
```

These are the ONLY methods tests can call. Same as production.

### 2. 👀 OBSERVE - Read State, Capture Events
```typescript
// CORRECT - Observe internal state for assertions
const state = harness.getReaderState();  // Read-only observation
const mode = harness.getReaderMode();    // Read-only observation
const events = harness.getEvents();      // Captured events

// CORRECT - Wait for events
const event = await harness.waitForEvent('TAG_READ');
expect(event.payload.tags).toHaveLength(5);
```

We can OBSERVE internals to verify behavior, but never MODIFY them.

### 3. 🚫 NEVER MANIPULATE - No Internal Access
```typescript
// WRONG - Direct internal manipulation
harness.worker.readerState = ReaderState.CONNECTED;  // FORBIDDEN
harness.commandManager.executeCommand(...);      // FORBIDDEN
harness.getWorker().somePrivateMethod();        // FORBIDDEN
```

## Input Source Agnosticism

**The worker doesn't care WHERE commands come from.**

Whether it's:
- UI button press
- Physical trigger press
- API call
- Test harness
- Remote control
- Voice command
- Telepathic mind ray control
- Quantum entangled remote trigger
- Time-traveling future command

They ALL call the same public methods:
- `startScanning()` - Start inventory/barcode/locate operation
- `stopScanning()` - Stop any active operation

This means:
- **Single implementation** - One code path for all input sources
- **Consistent behavior** - Same result regardless of trigger source
- **Simplified testing** - Test the method once, all sources work
- **Clean architecture** - Input sources are just event translators

Example:
```typescript
// UI Button
<button onClick={() => worker.startScanning()}>Start</button>

// Physical Trigger
onTriggerPressed() {
  worker.startScanning();
}

// Test Harness
await harness.startScanning();

// ALL THE SAME to the worker!
```

## Bidirectional Stream Architecture

### Transport Layer = Dumb Pipe
- **Commands**: Fire-and-forget via `sendRawBytes()`
- **Responses**: ALL through notification handler
- **No RPC**: No request-response patterns in transport

### Worker = All Intelligence
- **CommandManager**: Sends commands, waits for responses from notification stream
- **NotificationManager**: Processes ALL incoming bytes
- **Protocol handlers**: Parse bytes into events

## Test Patterns

### ❌ WRONG - RPC Pattern
```typescript
// This violates the architecture
const response = await client.sendCommandAsync(cmd);  // NO!
const result = await harness.executeCommand(cmd);     // NO!
```

### ✅ CORRECT - Stream Pattern
```typescript
// Commands through worker's public API
await harness.setMode(ReaderMode.INVENTORY);

// Events through notification stream
const event = await harness.waitForEvent('READER_MODE_CHANGED');
expect(event.payload.mode).toBe(ReaderMode.INVENTORY);
```

## Why This Matters

1. **Tests match production**: If it works in test, it works in production
2. **Find real bugs**: Can't hide problems with test-only workarounds
3. **Architecture validation**: Tests prove the architecture is sound
4. **No false positives**: Tests can't pass by cheating

## The Harness Contract

The test harness provides:
- **Public API methods**: connect, disconnect, setMode, setSettings, startScanning, stopScanning
- **Event observation**: getEvents, waitForEvent, getEventsByType
- **State observation**: getReaderState, getReaderMode (read-only)
- **Transport observation**: getTransportMessages, getOutboundCommands

The test harness NEVER provides:
- Direct access to worker internals
- Ability to call private methods
- Ability to modify state directly
- Request-response patterns at transport layer

## Summary

Integration tests prove that the worker's public API, combined with the bidirectional stream architecture, actually works in practice. If a test needs to violate these principles to pass, the production code is broken, not the test.