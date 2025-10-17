# Worker Component CLAUDE.md

This file provides worker-specific guidance for developing DeviceManager, CS108 protocol, and BLE transport components.

## 🔄 Architecture Context
- **Read `../../docs/WORKER-ARCHITECTURE.md`** for complete DeviceManager pattern details
- **Reference `../../CLAUDE.md`** for project-wide standards (TypeScript, ESM, git workflow)
- **This directory focuses ONLY on worker implementation** - no UI, no E2E testing

## 🎯 Worker Component Scope

This directory implements the **worker thread** and **DeviceManager lifecycle** components:

### Core Responsibilities
- **DeviceManager**: Worker lifecycle, single instance guarantees, cleanup
- **CS108Worker**: RFID protocol implementation in worker thread  
- **BLE Transport**: Web Bluetooth communication layer
- **Worker ↔ Main Thread**: Message passing via Comlink
- **State Synchronization**: Worker state → Store updates

### NOT in Scope
- UI components or React concerns
- Zustand store implementations (handled in parent)
- E2E testing (handled at project level)
- Git workflow or package management

## 🚨 Critical Worker Implementation Rules

### **DeviceManager Lifecycle (NEVER VIOLATE)**
1. **Exactly one worker during connection** - Zero at startup, zero after disconnect
2. **Fail-fast on duplicate workers** - Throw error if worker already exists
3. **Guaranteed cleanup** - Always terminate workers in finally blocks
4. **Single instance validation** - Check `this.worker` before creating new

```typescript
// ✅ CORRECT - Guaranteed single worker
async connect(): Promise<boolean> {
  if (this.worker) {
    throw new Error('Already connected - call disconnect() first');
  }
  try {
    this.worker = new Worker(workerPath, { type: 'module' });
    // ... connection logic
  } catch (error) {
    await this.cleanup(); // Always cleanup on failure
    throw error;
  }
}
```

### **CS108 Protocol Standards (NEVER VIOLATE)**
1. **Hex values ONLY** - `0xA7B3` never `42931`
2. **Constants over magic numbers** - Use `CS108_COMMANDS.INVENTORY.command` not `0xA001`
3. **Metadata-driven lookups** - `Object.values(CS108_COMMANDS).find(cmd => cmd.responseCode === command)`
4. **No hardcoded byte arrays** - Always use packet builders

```typescript
// ❌ WRONG - Hardcoded values
const inventoryCommand = [0xA7, 0xB3, 0x00, 0x03, 0xA0, 0x01, 0x00, 0x00];

// ✅ CORRECT - Constants and builders
const command = CS108_COMMANDS.INVENTORY;
const packet = buildCommand(command.command, command.payload);
```

### **Worker Communication Patterns**
1. **Pure message passing** - No direct store coupling in worker
2. **Structured events** - `{ type: 'BATTERY_UPDATE', payload: { percentage: 85 } }`
3. **DeviceManager routing** - Main thread routes messages to appropriate stores
4. **Comlink for RPC** - Complex worker interactions via Comlink proxy

```typescript
// ✅ CORRECT - Worker emits structured messages
worker.postMessage({
  type: 'TAG_READ',
  payload: { epc: 'E280...', rssi: -45, timestamp: Date.now() }
});

// ✅ CORRECT - DeviceManager routes to stores
worker.onmessage = (event) => {
  const { type, payload } = event.data;
  switch (type) {
    case 'TAG_READ':
      useTagStore.getState().addTag(payload);
      break;
  }
};
```

## 🧪 Worker Testing Strategy

### **Focus Areas**
- **Unit Tests**: DeviceManager lifecycle, CS108 protocol parsing, BLE transport
- **Integration Tests**: Worker ↔ DeviceManager ↔ Mock hardware
- **Hardware Tests**: Real CS108 device via MCP BLE tools

### **Test Commands (Run from Parent Project)**
```bash
# Worker-specific unit tests
pnpm test src/worker/

# Worker integration tests with mock hardware  
pnpm test src/worker/ --config vitest.integration.config.ts

# Real hardware testing (requires CS108 device)
pnpm test:hardware src/worker/
```

### **Testing Standards**
- **Mock external dependencies** - BLE, hardware, but NOT worker logic
- **Test real protocol implementation** - CS108 packet parsing, state machines
- **Validate state contracts** - Worker state changes trigger correct store updates
- **NO UI mocking** - Workers should have zero UI knowledge

```typescript
// ✅ CORRECT - Test real protocol logic
test('CS108 inventory command parsing', () => {
  const response = new Uint8Array([0xB3, 0xA7, 0x00, 0x05, 0xA0, 0x01, 0x00, 0x01, 0x23]);
  const parsed = parseCS108Response(response);
  expect(parsed.command).toBe(CS108_COMMANDS.INVENTORY.responseCode);
  expect(parsed.success).toBe(true);
});

// ✅ CORRECT - Test DeviceManager lifecycle
test('DeviceManager prevents duplicate workers', async () => {
  const manager = new DeviceManager();
  await manager.connect();
  
  await expect(manager.connect()).rejects.toThrow('Already connected');
  
  await manager.disconnect();
  expect(manager.getWorker()).toBeNull();
});
```

## 🔧 Hardware Debugging - ALWAYS START HERE

### **🚨 CRITICAL: Hardware Verification First**

**Before ANY debugging, before blaming hardware, before diving into code:**

```bash
pnpm test:hardware
```

This test takes 1 second and definitively proves whether hardware is working. 

**If this test passes:**
- ✅ Hardware IS working
- ✅ Bridge server IS running
- ✅ CS108 IS responding
- ✅ The problem IS in your code

**If this test fails:**
- Check bridge server is running
- Check CS108 is powered on
- Check network connectivity
- But DON'T debug your worker code - it's not the problem

### **The Golden Rule**
```bash
pnpm test:hardware  # Passes? → "Hardware is fine" → Debug your code
                    # Fails?  → Fix hardware/bridge first
```

### **Seeing Hardware Responses**
After running the hardware test, verify what happened:

```bash
# 1. Touch the hardware
pnpm test:hardware

# 2. See the hardware responding
mcp__ble-mcp-test__get_logs --since=30s
```

You'll see:
```
RX: A7 B3 02 D9 82 37 00 00 A0 01     ← Your command reached hardware
TX: A7 B3 03 D9 82 9E 74 37 A0 01 00  ← Hardware responded
```

### **Real Hardware Integration**
Use MCP BLE monitoring for debugging actual CS108 communication:

```bash
# Check connection state
mcp__ble-mcp-test__get_connection_state

# Monitor real-time packet flow
mcp__ble-mcp-test__get_logs --since=30s

# Search for specific command patterns
mcp__ble-mcp-test__search_packets --hex_pattern="A7B3"
```

### **Hardware Test Workflow**
1. **ALWAYS FIRST** - Run `pnpm test:hardware`
2. If hardware test passes, then debug your code:
   - Start worker integration test
   - Use `mcp__ble-mcp-test__get_connection_state` to verify BLE connection
   - Use `mcp__ble-mcp-test__get_logs` to monitor command/response flow
   - Compare expected vs actual CS108 packets

## 📚 Implementation Patterns

### **Shared Constants Usage**
```typescript
// ✅ Always import from shared types
import { CS108_COMMANDS, ReaderState, BLE_SERVICE_UUID } from '../types/cs108-constants';

// ✅ Use metadata for command lookups
const command = CS108_COMMANDS.SET_POWER;
const response = await sendCommand(command.command, command.payload);

// ✅ Use enums for state comparisons  
if (readerState === ReaderState.CONNECTED) {
  await startInventory();
}
```

### **Error Boundaries**
```typescript
// ✅ Separate transport vs device errors
try {
  await ble.writeCharacteristic(data);
} catch (error) {
  if (error.name === 'NotConnectedError') {
    throw new TransportError('BLE connection lost', error);
  } else {
    throw new DeviceError('CS108 command failed', error);
  }
}
```

### **State Synchronization**
```typescript
// ✅ Worker maintains authoritative state
class CS108Worker {
  private readerState: ReaderState = ReaderState.IDLE;
  private batteryLevel = 0;
  
  private notifyStateChange(changes: Partial<WorkerState>) {
    self.postMessage({
      type: 'STATE_UPDATE',
      payload: changes
    });
  }
}
```

## 🎯 Success Criteria

When working in this directory, ensure:
- ✅ **Zero workers at startup** (verified in browser dev tools)
- ✅ **Exactly one worker during connection** (no duplicates)
- ✅ **Immediate worker cleanup on disconnect** (no memory leaks)
- ✅ **All protocol values in hex format** (logging, constants, comments)
- ✅ **Constants used throughout** (no magic numbers)
- ✅ **Unit tests pass** for DeviceManager, CS108, BLE components
- ✅ **Integration tests pass** with mock hardware
- ✅ **Hardware tests pass** with real CS108 device (when available)

## 📋 Pre-commit Checklist

Before committing worker changes:
1. **Run worker tests**: `pnpm test src/worker/`
2. **Verify no hardcoded values**: Search for decimal numbers in protocol code
3. **Check constant usage**: All CS108 commands use imported constants
4. **Validate cleanup**: DeviceManager properly terminates workers
5. **Test real hardware**: Run integration tests with MCP BLE tools (if available)