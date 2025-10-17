# CS108 Testing Guide

## Testing Philosophy

### Core Principles
1. **Integration-first** - Test with real hardware when possible
2. **No hardcoded bytes** - Use constants and builders
3. **Semantic verification** - Test behavior, not bytes
4. **Clean boundaries** - No store imports in worker tests
5. **Fast feedback** - Run specific tests during development

## Test Infrastructure

### Directory Structure
```
tests/
├── config/
│   ├── cs108-packet-builder.ts  # Test packet utilities
│   └── cs108.config.ts          # Test constants
├── integration/
│   ├── ble-mcp-test/            # Bridge connectivity
│   │   └── rfid-reader-test-client.ts
│   └── cs108/                   # CS108-specific tests
│       ├── CS108WorkerTestHarness.ts
│       └── *.spec.ts            # Test files
└── unit/
    └── cs108/                   # Unit tests (mocked)
```

## CS108WorkerTestHarness

### Purpose
Bridge real hardware to worker without Zustand stores.

### Architecture
```typescript
class CS108WorkerTestHarness {
  private rfidClient: RfidReaderTestClient;  // Hardware bridge
  private worker: CS108Reader;               // Reader instance
  private workerPort: MessagePort;           // Mock transport
  private mainPort: MessagePort;             // Main thread side
  
  async initialize(connectToHardware = true) {
    // 1. Connect to bridge server
    if (connectToHardware) {
      await this.rfidClient.connect();
    }
    
    // 2. Create MessageChannel
    const channel = new MessageChannel();
    this.workerPort = channel.port1;
    this.mainPort = channel.port2;
    
    // 3. Instantiate reader
    this.worker = new CS108Reader();
    
    // 4. Inject transport
    this.worker.setTransportPort(this.workerPort);
    
    // 5. Bridge hardware to worker
    this.rfidClient.onData((data) => {
      this.workerPort.postMessage({
        type: 'DATA',
        data: new Uint8Array(data)
      });
    });
  }
}
```

### Usage in Tests
```typescript
describe('CS108Reader', () => {
  let harness: CS108WorkerTestHarness;
  
  beforeEach(async () => {
    harness = new CS108WorkerTestHarness();
    await harness.initialize();
  });
  
  afterEach(async () => {
    await harness.cleanup();
  });
  
  it('should connect and reach READY state', async () => {
    const reader = harness.getReader();
    const connected = await reader.connect();
    
    expect(connected).toBe(true);
    expect(reader.getState()).toBe(ReaderState.CONNECTED);
  });
});
```

## Command Verification

### Never Test Raw Bytes
```typescript
// ❌ WRONG - Testing hardcoded bytes
expect(sentBytes).toEqual([0xA7, 0xB3, 0x02, ...]);

// ✅ CORRECT - Test semantic commands
expect(harness.getOutboundCommands()).toContain('RFID_POWER_ON');
```

### Transport Capture Pattern
```typescript
class CS108WorkerTestHarness {
  private transportMessages: TransportMessage[] = [];
  
  setupTransportCapture() {
    this.mainPort.onmessage = (event) => {
      if (event.data.type === 'WRITE') {
        const command = identifyCS108Command(event.data.data);
        this.transportMessages.push({
          direction: 'outbound',
          commandName: command,
          bytes: event.data.data,
          timestamp: Date.now()
        });
      }
    };
  }
  
  getOutboundCommands(): string[] {
    return this.transportMessages
      .filter(m => m.direction === 'outbound')
      .map(m => m.commandName);
  }
}
```

## Test Patterns

### Mode Transition Test
```typescript
it('should transition from INVENTORY to IDLE to BARCODE', async () => {
  const reader = harness.getReader();
  await reader.connect();
  
  // Start in INVENTORY
  await reader.setMode(ReaderMode.INVENTORY);
  expect(reader.getMode()).toBe(ReaderMode.INVENTORY);
  
  // Transition to BARCODE (should go through IDLE)
  await reader.setMode(ReaderMode.BARCODE);
  
  // Verify commands sent
  const commands = harness.getOutboundCommands();
  expect(commands).toContain('ABORT_OPERATION');  // Stop inventory
  expect(commands).toContain('SET_TO_IDLE');      // Enter IDLE
  expect(commands).toContain('ENABLE_BARCODE');   // Enter BARCODE
  
  expect(reader.getMode()).toBe(ReaderMode.BARCODE);
});
```

### Tag Read Test
```typescript
it('should emit TAG_READ events during inventory', async () => {
  const reader = harness.getReader();
  const events = harness.captureEvents();
  
  await reader.connect();
  await reader.setMode(ReaderMode.INVENTORY);
  await reader.startScanning();
  
  // Wait for tags
  await harness.waitForEvents('TAG_READ', { count: 5 });
  
  // Verify events
  const tagReads = events.filter(e => e.type === 'TAG_READ');
  expect(tagReads).toHaveLength(5);
  expect(tagReads[0].payload).toHaveProperty('epc');
  expect(tagReads[0].payload).toHaveProperty('rssi');
});
```

### Error Recovery Test
```typescript
it('should recover from command timeout', async () => {
  const reader = harness.getReader();
  
  // Simulate timeout by not forwarding response
  harness.blockNextResponse();
  
  // Command should timeout
  await expect(reader.getBatteryLevel()).rejects.toThrow('timed out');
  
  // Reader should recover
  expect(reader.getState()).toBe(ReaderState.CONNECTED);
  
  // Next command should work
  harness.unblock();
  const battery = await reader.getBatteryLevel();
  expect(battery).toBeGreaterThan(0);
});
```

## Unit Testing

### Mock Transport
```typescript
// tests/unit/cs108/reader.test.ts
import { MockTransport } from '../mocks/MockTransport';

describe('CS108Reader Unit Tests', () => {
  let reader: CS108Reader;
  let transport: MockTransport;
  
  beforeEach(() => {
    reader = new CS108Reader();
    transport = new MockTransport();
    reader.setTransportPort(transport);
  });
  
  it('should parse battery response', async () => {
    // Queue mock response
    transport.queueResponse(
      buildResponse(GET_BATTERY_LEVEL, {
        payload: new Uint8Array([0xE8, 0x0C]) // 3304mV
      })
    );
    
    const battery = await reader.getBatteryLevel();
    expect(battery).toBe(75); // Calculated percentage
  });
});
```

## MCP BLE Monitoring

### Setup
```bash
# Ensure MCP server is configured in .claude/settings.json
# Bridge server should be running on 192.168.50.73:8080
```

### Real-time Monitoring
```typescript
// During test development, monitor actual packets
it('should handle fragmented packets', async () => {
  // In another terminal: mcp__ble-mcp-test__get_logs --since=now
  
  const reader = harness.getReader();
  await reader.connect();
  
  // Send command that triggers large response
  await reader.getInventoryRounds();
  
  // Check MCP logs to see fragmentation
  // Logs will show:
  // RX: A7 B3 78 01 82 9E ... (first fragment)
  // RX: [continuation bytes]
  // RX: A7 B3 03 01 82 9E ... (next packet starts)
});
```

### Debug Helpers
```bash
# Get current connection state
mcp__ble-mcp-test__get_connection_state

# Search for specific packets
mcp__ble-mcp-test__search_packets --hex_pattern="A7B3"

# Get detailed metrics
mcp__ble-mcp-test__get_metrics
```

## Running Tests

### Quick Commands
```bash
# Run all CS108 tests
pnpm test tests/integration/cs108

# Run specific test file
pnpm test cs108-instantiation

# Run with coverage
pnpm test:coverage tests/integration/cs108

# Watch mode for TDD
pnpm test --watch cs108-instantiation
```

### Test Organization
- **instantiation.spec.ts** - Object creation and initial state
- **lifecycle.spec.ts** - Connect/disconnect sequences
- **mode-transitions.spec.ts** - Mode switching logic
- **inventory.spec.ts** - Tag reading operations
- **error-recovery.spec.ts** - Failure handling

## Common Issues

### Issue: Tests timeout waiting for hardware
```typescript
// Solution: Increase timeout for hardware tests
it('should connect', async () => {
  // ...
}, 10000); // 10 second timeout
```

### Issue: Fragmented packets not reassembled
```typescript
// Solution: Ensure fragment buffer handles A7B3 resets
if (data[0] === 0xA7 && data[1] === 0xB3) {
  // New packet starts, clear buffer
  this.fragmentBuffer = new Uint8Array(0);
}
```

### Issue: Commands sent too quickly
```typescript
// Solution: Add delay between commands
await this.sendCommand(COMMAND_1);
await new Promise(r => setTimeout(r, 50)); // 50ms delay
await this.sendCommand(COMMAND_2);
```

## Best Practices

1. **Test behavior, not implementation**
   - Test that inventory starts, not which commands are sent
   - Test that tags are read, not packet parsing

2. **Use test builders**
   - `TestPackets.triggerPress()` not hardcoded bytes
   - `buildResponse()` helper for mock responses

3. **Clean up properly**
   - Always disconnect in afterEach
   - Clear event listeners
   - Reset harness state

4. **Test incrementally**
   - Start with instantiation
   - Add connection
   - Add mode changes
   - Add operations

5. **Monitor real hardware**
   - Use MCP tools during development
   - Verify assumptions with actual packets
   - Document hardware quirks