# CS108 Worker Architecture

## System Design

### Communication Flow
```
UI → Store → DeviceManager → Comlink RPC → Worker → CS108 Hardware
↑                            ↑              ↓
UI ← Store ← Domain Events ← DeviceManager ← Worker ← CS108 Hardware
```

### Packet Processing Flow
```
BLE Data → Reader.handleBleData() → PacketHandler.processIncomingData()
           ↓
         CS108Packet[] → Route by event.type
                          ├─ Command Response → CommandManager.handleCommandResponse()
                          └─ Notification → NotificationManager.handleNotification()
                                             ↓
                                           NotificationRouter.handleNotification()
                                             ↓
                                           Handler.canHandle() → Handler.handle()
```

### Layer Responsibilities

#### Layer 1: CS108 Protocol Foundation (Worker Thread)
- **Packet parsing** - Reader owns PacketHandler for all protocol parsing
- **Command lifecycle** - CommandManager handles request/response matching
- **Packet fragmentation** - Handle 120-byte limits with 200ms timeout
- **Serial command queue** - 2-5 second timeouts per vendor spec
- **Mode configuration** - Vendor-proven command sequences
- **State management** - Idle-first transitions

#### Layer 2: Device Interface (Worker Thread)
- **IReader interface** - Clean API wrapping protocol
- **Domain events** - Pure event emission
- **Comlink exposure** - RPC for main thread

#### Layer 3: DeviceManager (Main Thread)
- **Worker lifecycle** - Exactly one worker during connection
- **Event routing** - Domain events → Zustand stores
- **Command proxy** - Forward calls via Comlink

#### Layer 4: Store Integration (Main Thread)
- **Unchanged** - Existing stores proven to work
- **Event consumption** - Via DeviceManager routing

## CS108Packet Type System

### Core Principle
**ZERO byte manipulation outside parser** - All Uint8Array operations happen in `parsePacket()`. The rest of the code works with strongly-typed objects.

### Packet Structure
```typescript
interface CS108Packet {
  // Header (bytes 0-7)
  prefix: number;          // 0xA7
  transport: number;       // 0xB3 (BT) or 0xE6 (USB)
  length: number;          // Data length (1-120)
  module: number;          // Module ID
  direction: number;       // 0x37 (down) or 0x9E (up)
  crc: number;            // CRC-16
  
  // Event (bytes 8-9)
  eventCode: number;       // Little-endian event ID
  event: CS108Event;       // Typed definition (required)
  
  // Payload (bytes 10+)
  rawPayload: Uint8Array;  // Raw bytes from packet (always present)
  payload?: any;           // Parsed data (if event.parser exists)
  
  // Computed
  totalExpected: number;   // For fragmentation
  isComplete: boolean;     // All fragments received
}
```

### Event System
```typescript
interface CS108Event {
  name: string;           // Human-readable name
  eventCode: number;      // Unique identifier  
  module: number;         // Module this belongs to
  type: 'command' | 'notification';  // Commands expect responses
  parser?: (payload: Uint8Array) => any;  // Custom parser for complex payloads
  successByte?: number;   // Expected success indicator (commands only)
}
```

The simplified approach:
- **type** replaces direction - clearer intent
- **No builder** - One generic `buildPacket(event, payload?)` function
- **Parser optional** - Only for complex payloads that need custom parsing

## Notification System Architecture

### Handler-Based Processing
The notification system uses a **handler pattern** to replace massive switch statements with extensible, testable components:

```typescript
interface NotificationHandler {
  canHandle(packet: CS108Packet, context: NotificationContext): boolean;
  handle(packet: CS108Packet, context: NotificationContext): void;
  cleanup?(): void;
}
```

### Notification Router
Central routing system with advanced features:

```typescript
class NotificationRouter {
  private handlers = new Map<number, NotificationHandler[]>();

  register(eventCode: number, handler: NotificationHandler): void
  handleNotification(packet: CS108Packet): void
}
```

**Key Features:**
- **Multi-handler support** - Multiple handlers per event code (0xA000, 0x8100)
- **Error boundaries** - Handlers fail independently without breaking the system
- **Context injection** - Current mode/state passed to all handlers
- **Graceful degradation** - Unknown events are silently ignored

### Context-Aware Processing
Handlers receive rich context to make intelligent decisions:

```typescript
interface NotificationContext {
  currentMode: ReaderMode;           // IDLE, INVENTORY, LOCATE, BARCODE
  readerState: ReaderState;          // Connection and operational state
  emitDomainEvent: (event) => void;  // Domain event emission
  metadata: Record<string, any>;     // Debug flags, configuration
}
```

### Event Code Multiplexing
The CS108 protocol reuses event codes for different purposes:

- **0xA000** - Battery voltage (command response + autonomous notifications)
- **0x8100** - RFID inventory (inventory mode + locate mode)
- **0xA001** - Trigger state (command response only)
- **0xA102/0xA103** - Trigger press/release (notifications only)

Handlers use `canHandle()` to filter packets based on context rather than just event codes.

### Handler Categories

#### System Handlers (`/system/`)
- **BatteryHandler** - Voltage to percentage conversion
- **TriggerHandler** - Button press/release/state with inheritance
- **ErrorNotificationHandler** - Rate-limited error processing

#### RFID Handlers (`/rfid/`)
- **InventoryHandler** - Batched tag processing for efficiency
- **LocateHandler** - Real-time RSSI tracking for tag location

#### Barcode Handlers (`/barcode/`)
- **BarcodeDataHandler** - Scan data processing
- **BarcodeGoodReadHandler** - Scan confirmation processing

### Batching System
The `InventoryBatcher` provides configurable batching strategies:

```typescript
interface BatchingConfig {
  maxSize: number;                    // Flush after N tags
  timeWindowMs: number;               // Flush every N milliseconds
  flushOnModeChange: boolean;         // Flush when switching modes
  deduplicationWindowMs: number;      // Dedupe tags within N ms
}
```

**Operational Modes:**
- **Inventory**: 50 tags / 100ms (efficiency focused)
- **Locate**: 5 tags / 50ms (responsiveness focused)

### Factory Pattern
Handlers use factory functions to solve context binding challenges:

```typescript
export function createInventoryHandler(
  emitDomainEvent: (event: any) => void
): InventoryTagHandler {
  const batcher = new InventoryBatcher(config);

  batcher.onFlush((tags) => {
    emitDomainEvent({ type: 'TAG_BATCH', payload: { tags } });
  });

  return new InventoryTagHandler(batcher);
}
```

This ensures proper domain event emission while maintaining handler independence.

### Protocol vs Application Abstraction
The system distinguishes between device-level protocol and application-level semantics:

**Device Level:**
- All RFID operations use inventory commands (0x8100)
- Only understands register settings and filters
- No concept of "locate" vs "inventory"

**Application Level:**
- **Inventory Mode** = "read all tags" (no filtering)
- **Locate Mode** = "track one specific tag" (EPC filter + different processing)
- Context-driven handler selection

This abstraction allows the same protocol events to be processed differently based on application state.

## Domain Events

### Event Types
```typescript
// Data events
type DataReadEvent = 
  | { type: 'INVENTORY'; data: InventoryRead[] }
  | { type: 'LOCATE'; data: LocateRead[] }
  | { type: 'BARCODE'; data: BarcodeRead[] };

// System events
type SystemEvent = 
  | { type: 'READER_STATE_CHANGED'; state: ReaderState }
  | { type: 'READER_MODE_CHANGED'; mode: ReaderMode }
  | { type: 'BATTERY_UPDATE'; percentage: number }
  | { type: 'TRIGGER_PRESSED' }
  | { type: 'TRIGGER_RELEASED' };

// Error events
type ErrorEvent = 
  | { type: 'CONNECTION_ERROR'; error: string }
  | { type: 'COMMAND_ERROR'; command: string; error: string }
  | { type: 'MODE_TRANSITION_ERROR'; from: ReaderMode; to: ReaderMode };
```

### Event Flow
1. **Worker detects change** (e.g., tag read, battery update)
2. **Worker posts message** with typed event
3. **DeviceManager receives** and identifies event type
4. **DeviceManager routes** to appropriate store
5. **Store updates** trigger React re-renders

## State Management

### Reader States
```typescript
enum ReaderState {
  DISCONNECTED = 'DISCONNECTED',
  CONNECTING = 'CONNECTING',
  READY = 'READY',
  BUSY = 'BUSY',           // Processing command, can't accept new ones
  SCANNING = 'SCANNING',
  ERROR = 'ERROR'
}
```

### Reader Modes
```typescript
enum ReaderMode {
  IDLE = 'IDLE',
  INVENTORY = 'INVENTORY',
  BARCODE = 'BARCODE',
  LOCATE = 'LOCATE'
}
```

### Mode Transition Rules
1. **Always through IDLE** - No direct mode-to-mode transitions
2. **Stop operations first** - Must stop scanning before mode change
3. **Verify success** - Check response before updating state
4. **Handle delayed stops** - Inventory may continue briefly

## Transport Layer

### BLE Characteristics
- **Service UUID**: `00009800-0000-1000-8000-00805f9b34fb`
- **Write UUID**: `00009900-0000-1000-8000-00805f9b34fb`
- **Notify UUID**: `00009901-0000-1000-8000-00805f9b34fb`

### MessagePort Bridge
```typescript
// Worker receives port from main thread
onmessage = (e: MessageEvent) => {
  if (e.data.type === 'SET_TRANSPORT_PORT') {
    this.transportPort = e.data.port;
    this.transportPort.onmessage = this.handleTransportMessage;
  }
};

// Send commands through port
this.transportPort.postMessage({
  type: 'WRITE',
  data: commandBytes
});
```

### Fragmentation
- **CS108 limit**: 8 bytes header + 120 bytes data max per packet
- **BLE MTU**: 20 bytes per characteristic write
- **Reassembly logic** (works with raw bytes):
  1. Empty buffer = new packet: read header, get `length` field (byte 2)
  2. Total expected = `length + 8` (header size)
  3. Start fragment timeout (200ms)
  4. Append raw fragments until `bytesReceived >= totalExpected`
  5. Parse complete buffer: extract `rawPayload` from bytes 10+
  6. If event has parser, convert `rawPayload` → `payload`
  7. Clear buffer and cancel timeout
- **Fragment timeout**: 200ms between fragments (industry standard: 150-250ms)
  - Transport should be near-instant (~10-50ms typical)
  - If timeout fires → clear buffer, log error
  - NOT the same as command timeout (2-5 seconds)
- **No header scanning**: CS108 is single-threaded, won't send new packet mid-stream
- **Error recovery**: Parse failures and fragment timeouts clear buffer automatically

## Error Handling

### Command Timeouts
```typescript
// 2-5 second timeout per vendor spec
const response = await this.sendCommand(command, { 
  timeout: 5000 
});
```

### NAK Responses
```typescript
// Check success byte in response
if (packet.event.successByte && 
    packet.payload?.success !== packet.event.successByte) {
  throw new CommandError(`Command failed: ${packet.event.name}`);
}
```

### Connection Loss
```typescript
// BLE disconnect triggers cleanup
this.transport.ondisconnect = () => {
  this.changeState(ReaderState.DISCONNECTED);
  this.cleanup();
};
```

## Performance Considerations

### Command Queue
- Serial execution (one command at a time)
- 50ms minimum spacing between commands
- Priority queue for stop commands

### Event Throttling
- Tag reads: Max 10/second to UI
- Battery updates: Max 1/second
- RSSI updates: Debounced 100ms

### Memory Management
- Clear tag buffer on mode change
- Limit history to 1000 tags
- Release worker on disconnect