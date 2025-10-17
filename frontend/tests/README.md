# Test Suite Documentation

## 🚨 BEFORE RUNNING ANY TESTS

**Hardware Verification First:**
```bash
pnpm test:hardware
```

If this passes, you know with 100% certainty that:
- Hardware is working ✅
- Bridge is running ✅
- Connection is good ✅

**Only debug test failures AFTER verifying hardware works.**

## Test Structure

### Unit Tests (`src/**/*.test.ts`)
- Component tests with React Testing Library
- Store tests with Zustand
- Utility function tests
- **Run:** `pnpm test`

### Integration Tests (`tests/integration/`)
- Real hardware communication tests
- Worker integration tests
- Protocol verification tests
- **Run:** `pnpm test:integration`

### E2E Tests (`tests/e2e/`)
- Full application flow tests
- User interaction simulation
- **Run:** `pnpm test:e2e`

## The Hardware Test

The most important test in the entire suite:

```bash
pnpm test:hardware
# OR
pnpm test tests/integration/ble-mcp-test/connection.spec.ts
```

### Why It's Special

This test is unique because it:
1. **Bypasses ALL application code** - no worker, no stores, nothing
2. **Talks directly to hardware** - straight to the bridge server
3. **Proves the entire stack works** - network → bridge → BLE → CS108
4. **Takes 1 second** - instant verification

### When to Run It

- **ALWAYS** before debugging any test failures
- **ALWAYS** when integration tests fail
- **ALWAYS** when you suspect hardware issues
- **ALWAYS** means ALWAYS

### Understanding the Output

Success looks like:
```
📤 Sending test command...
[RfidReaderTestClient] Sending command: 0xa7 0xb3 0x02 0xd9 0x82 0x37 0x00 0x00 0xa0 0x01
📥 Received response: 0xa7 0xb3 0x03 0xd9 0x82 0x9e 0x74 0x37 0xa0 0x01 0x00
✅ Bridge server connection test passed!
```

This means:
- Command reached hardware ✅
- Hardware processed it ✅
- Response came back ✅
- **Hardware is fine** ✅

## Integration Test Workflow

1. **Verify hardware first**
   ```bash
   pnpm test:hardware
   ```

2. **Run integration tests**
   ```bash
   pnpm test:integration
   ```

3. **If tests fail but hardware test passed**
   - Problem is in the code, not hardware
   - Check worker implementation
   - Verify transport wiring
   - Debug with confidence

4. **Check bridge logs for details**
   ```bash
   mcp__ble-mcp-test__get_logs --since=30s
   ```

## Common Issues and Solutions

### "Integration tests are failing"
1. Run `pnpm test:hardware`
2. If it passes → Debug your code
3. If it fails → Fix hardware/bridge first

### "Commands aren't reaching the hardware"
1. Run `pnpm test:hardware`
2. If it passes → Check worker transport wiring
3. If it fails → Hardware isn't connected

### "I think the CS108 is broken"
1. Run `pnpm test:hardware`
2. If it passes → CS108 is NOT broken
3. If it fails → Check power, BLE connection

## Test Commands Reference

```bash
# The most important test
pnpm test:hardware          # Verify hardware is working

# Other test commands
pnpm test                   # Run all unit tests
pnpm test:watch            # Run tests in watch mode
pnpm test:integration      # Run integration tests
pnpm test:e2e              # Run E2E tests
pnpm validate              # Run everything (lint, typecheck, tests)
```

## The Golden Rule

**If `pnpm test:hardware` passes, stop blaming the hardware.**

The hardware is fine. The bridge is fine. The connection is fine.
The problem is in the code. Debug accordingly.

## Side-Loading Notifications - Core Testing Strategy

### The Universal Testing Pattern

Throughout our test suite, we use a critical pattern called **"side-loading notifications"** that enables automated testing of hardware interactions without requiring physical device manipulation.

### The Problem
- Testing hardware events (trigger press, battery updates, etc.) normally requires physical interaction
- This makes automated testing impossible in CI/CD environments
- We need deterministic, repeatable tests without human intervention

### The Solution - Side-Loading

We "forge" perfectly valid device notification packets and inject them into the transport layer at various levels, making each layer think the packets came from real hardware.

```
Real Hardware Flow:
[CS108 Device] --BLE--> [Transport] --bytes--> [Worker] --events--> [Store] --UI--> [User]

Side-Loading Flow:
[Test Code] --forged packet--> [Transport] --bytes--> [Worker] --events--> [Store] --UI--> [User]
                                    ↑
                        Layer thinks bytes came from device
```

### Implementation at Different Layers

#### Integration Tests (Worker Level)
```typescript
// Inject trigger press into worker transport
await harness.simulateTriggerPress();
// Worker processes it as real hardware event
```

#### E2E Tests (Application Level)
```typescript
// Inject bytes into mock Bluetooth API
await mockBluetooth.injectBytes(triggerPacket);
// Application processes it as real Bluetooth data
```

### Why This Is Essential

- ✅ **Fully Automated Testing** - No physical button pressing required
- ✅ **Real Code Paths** - Tests actual handlers, not mocks
- ✅ **Deterministic Timing** - Complete control over when events occur
- ✅ **CI/CD Compatible** - Runs in headless environments
- ✅ **Edge Case Testing** - Simulate rapid events, invalid states, etc.

### The Key Insight

**Each layer has NO IDEA the packets are fake.** They process side-loaded packets exactly as they would real hardware notifications because the bytes are identical to what the device would send.

### Visualization - The Hallway Analogy

Imagine a hallway where bytes walk from the device to the worker:
```
[Device] -----> [Hallway] -----> [Worker]
                    |
               [Side Door]
                    ↑
            [Test Packets]
```

Side-loading adds a "side door" where test packets can enter the hallway and join the normal flow. The worker sees bytes arriving and processes them - it doesn't know or care whether they entered through the front door (device) or side door (tests).

### Examples in Our Codebase

1. **Trigger Simulation**: `tests/integration/cs108/trigger-store-update.spec.ts`
   - Side-loads trigger press/release notifications
   - Worker emits `TRIGGER_STATE_CHANGED` events as if real button was pressed

2. **Battery Updates**: Side-load battery level notifications
   - Worker updates battery store without real battery changes

3. **Tag Reads**: Side-load inventory packets
   - Worker processes tags without real RFID scanning

### When to Use Side-Loading

Use side-loading when you need to:
- Test hardware event handlers without physical interaction
- Verify event processing in automated tests
- Simulate specific hardware states or sequences
- Test edge cases that are hard to reproduce with real hardware

### The Universal Truth

**Side-loading is how we achieve comprehensive automated testing of hardware-dependent functionality.** Without it, we'd need a human pressing buttons for every test run.