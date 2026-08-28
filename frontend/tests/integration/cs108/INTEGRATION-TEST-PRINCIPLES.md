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

Plus the rest of the harness's public surface, which this list omitted:

```typescript
await harness.initialize();          // install the mock, connect the transport
await harness.setSettings({ ... });  // and harness.getSettings()
await harness.waitForState('Ready');
await harness.simulateTriggerPress();
await harness.simulateTriggerRelease();
await harness.cleanup();             // quiesce, disconnect, uninstall
```

**These are the only methods tests may call.** The list above used to stop at
`disconnect()`, which made it wrong rather than strict: `simulateTriggerPress`
is used by five of the six specs, so every one of them violated this document's
own rule. A rule that the codebase universally breaks is not a rule.

`simulateTrigger*` earns its place because it is not internal access — it
injects a notification through `navigator.bluetooth.testing.simulateNotification`
onto the characteristic the transport subscribed, so it enters by the same door
a real trigger packet does.

### 2. 👀 OBSERVE - Read State, Capture Events
```typescript
// CORRECT - Observe internal state for assertions
const state = harness.getReaderState();  // Read-only observation
const mode = harness.getReaderMode();    // Read-only observation
const events = harness.getEvents();      // Captured events

// CORRECT - Wait for events
const event = await harness.waitForEvent('TRIGGER_STATE_CHANGED');
expect(event.payload.pressed).toBe(true);
```

We can OBSERVE internals to verify behavior, but never MODIFY them.

### 3. 🚫 NEVER MANIPULATE - No Internal Access
```typescript
// WRONG - Direct internal manipulation
harness.worker.readerState = ReaderState.CONNECTED;  // FORBIDDEN
harness.getWorker().somePrivateMethod();            // FORBIDDEN
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
- **Commands**: the worker posts `{ type: 'ble:write' }` to the transport's `MessagePort`
- **Responses**: ALL through notification handler, as `{ type: 'ble:data' }`
- **No RPC**: no request-response pattern anywhere in the transport

This used to read *"fire-and-forget via `sendRawBytes()`"*. That method lived on
`RfidReaderTestClient`, which TRA-1187 item 3 deleted along with the flat
`NodeBleClient` API it wrapped. The principle is unchanged; only the mechanism
was renamed by reality.

### Worker = All Intelligence
- **CommandManager**: Sends commands, waits for responses from notification stream
- **NotificationManager**: Processes ALL incoming bytes
- **Protocol handlers**: Parse bytes into events

## Test Patterns

### ❌ WRONG - RPC Pattern
```typescript
// This violates the architecture
const response = await transportClient.sendCommand(cmd);  // NO! -- not from a spec
```

Both previous examples here named methods that no longer exist:
`client.sendCommandAsync()` went with `src/node/` in TRA-1187 item 4, and
`harness.executeCommand()` never existed at all. **A warning against a method
nobody can call teaches nothing** -- worse, it implies the harness has a surface
it does not have.

The live version of the hazard is `TransportCommandClient.sendCommand()`, which
does correlate a write with the next inbound frame. It exists for
`connection.spec.ts`, the byte-level smoke test that deliberately bypasses the
worker. **Never reach for it from a worker-level spec**: correlation is the
worker's `CommandManager`'s job, and doing it in a spec couples the test to
command/response timing the CS108 does not guarantee.

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