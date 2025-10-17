# CS108 Worker Refactor Specification

## Executive Summary

This document specifies the complete refactoring of the RFID reader web worker components to replace the existing spaghetti implementation with a clean, tested, reliable vendor agnostic architecture based on evolved hardware understanding.

**Goals:**
- Replace 10k LOC spaghetti with clean, modular implementation
- Implement proven hardware interaction patterns (idle-first, serial commands)
- Eliminate direct store coupling in favor of pure domain events
- Build bottom-up for easy testing and validation
- Build for CS108 first since we have the hardware, but keep hardware agnostic layer for planned expansion to other hardware

**Non-Goals:**
- Changing UI or store interfaces (they work and are proven)
- Adding new hardware features (YAGNI until proven needed)
- Optimizing performance beyond current working levels

## Architecture Overview

### Communication Flow
```
UI → Store → DeviceManager → Comlink RPC → Worker → CS108 Hardware
 ↑                            ↑              ↓
UI ← Store ← Domain Events ← DeviceManager ← Worker ← CS108 Hardware
```

### Layer Responsibilities

#### Layer 1: CS108 Protocol Foundation (Worker Thread)
- **Packet fragmentation handling** with 120-byte limits and A7B3 reset logic
- **Serial command queue** with proven 2-5 second timeouts
- **Mode configuration sequences** using vendor-proven command sets
- **Hardware state management** with idle-first transitions

#### Layer 2: Device Interface (Worker Thread) 
- **Clean IHandheldDevice interface** wrapping protocol foundation
- **Domain event emission** for all state changes
- **Comlink RPC exposure** for main thread commands

#### Layer 3: DeviceManager Bridge (Main Thread)
- **Worker lifecycle management** (exactly one worker during connection)
- **Domain event routing** to appropriate Zustand stores
- **Command delegation** via Comlink RPC

#### Layer 4: Store Integration (Main Thread)
- **Unchanged** - existing stores work and are proven
- **Domain event consumption** via DeviceManager routing

## CS108Packet Type System

### Core Design Principle
**ZERO byte manipulation outside the parser** - All Uint8Array operations are constrained to the `parsePacket` function. Once parsed, all code operates on strongly-typed CS108Packet objects with pre-parsed payloads.

### CS108Packet Type Definition
```typescript
interface CS108Packet {
  // Header fields (bytes 0-7)
  prefix: number;          // Byte 0: Always 0xA7
  transport: number;       // Byte 1: 0xB3 (BT) or 0xE6 (USB)
  length: number;          // Byte 2: Data length after header (1-120)
  module: number;          // Byte 3: Module identifier
  reserve: number;         // Byte 4: Always 0x82
  direction: number;       // Byte 5: 0x37 (down) or 0x9E (up)
  crc: number;            // Bytes 6-7: CRC-16 (little-endian)
  
  // Event identification
  eventCode: number;       // Bytes 8-9: Little-endian event identifier
  event: CS108Event;       // REQUIRED: Typed event definition (fails if unknown)
  
  // Payload (bytes 10+)
  rawPayload: Uint8Array;  // Raw bytes (for unit tests/debugging only)
  payload?: any;           // Pre-parsed data (what 90% of code uses)
  
  // Computed fields
  totalExpected: number;   // 8 + length (for fragmentation)
  isComplete: boolean;     // true when all fragments received
}
```

### Single Parse Function with Auto-Parsing
```typescript
function parsePacket(data: Uint8Array): CS108Packet | null {
  // Minimum 10 bytes needed (header + event code)
  if (data.length < 10) return null;
  
  // Parse header and event code (ONLY place with byte manipulation)
  const prefix = data[0];
  const transport = data[1];
  const length = data[2];
  const module = data[3];
  const reserve = data[4];
  const direction = data[5];
  const crc = data[6] | (data[7] << 8);
  const eventCode = data[8] | (data[9] << 8);
  
  // Validate fixed bytes
  if (prefix !== 0xA7 || reserve !== 0x82) return null;
  if (length > 120) return null;
  
  // Look up event - MUST exist (fail-fast philosophy)
  const event = CS108_EVENT_MAP.get(eventCode);
  if (!event) {
    throw new Error(`Unknown event code: 0x${eventCode.toString(16).padStart(4, '0')}`);
  }
  
  // Extract raw payload
  const totalExpected = 8 + length;
  const rawPayload = data.slice(10, totalExpected);
  
  // Auto-parse payload if parser exists
  let payload = undefined;
  if (event.parser && rawPayload.length > 0) {
    try {
      payload = event.parser(rawPayload);
    } catch (error) {
      console.warn(`Parser failed for ${event.name}:`, error);
    }
  }
  
  return {
    prefix,
    transport,
    length,
    module,
    reserve,
    direction,
    crc,
    eventCode,
    event,          // Always populated - guaranteed by throw above
    rawPayload,     // Raw bytes for debugging/unit tests
    payload,        // Pre-parsed data ready to use!
    totalExpected,
    isComplete: data.length >= totalExpected
  };
}
```

### Fail-Fast Philosophy
**Unknown event codes are bugs, not runtime conditions to handle.**

If the CS108 sends an event code we don't recognize, that means:
1. We missed defining an event in our CS108_EVENT_MAP
2. The CS108 firmware has been updated with new events
3. We have a corrupted packet (but CRC should catch this)

In all cases, the correct response is to **throw an error with clear instructions** on what event code needs to be added. This ensures:
- Complete event coverage during development
- No silent packet drops in production
- Clear debugging when new firmware introduces events
- Type safety throughout the system (packet.event is never undefined)

### Fragmentation Handling
BLE MTU of 20 bytes means packets arrive in fragments:
- First fragment: 8-byte header + 2-byte event + up to 10 bytes payload
- Subsequent fragments: Up to 20 bytes of payload continuation

```typescript
class CS108PacketHandler {
  private fragmentBuffer = new Uint8Array(0);
  
  processIncomingData(data: Uint8Array): CS108Packet[] {
    // Append new data to buffer
    this.fragmentBuffer = concat(this.fragmentBuffer, data);
    
    const packets: CS108Packet[] = [];
    
    while (this.fragmentBuffer.length >= 10) {
      const packet = parsePacket(this.fragmentBuffer);
      
      if (!packet) {
        // Invalid packet, clear buffer
        this.fragmentBuffer = new Uint8Array(0);
        break;
      }
      
      if (!packet.isComplete) {
        // Need more fragments
        break;
      }
      
      // Complete packet ready
      packets.push(packet);
      this.fragmentBuffer = this.fragmentBuffer.slice(packet.totalExpected);
    }
    
    return packets;
  }
}
```

### Usage Patterns - Zero Byte Manipulation

**Production Code - Just Use `payload`:**
```typescript
// Command response handler - payload is pre-parsed
function handleResponse(packet: CS108Packet): void {
  if (packet.event === this.activeCommand?.event) {
    // Just use the pre-parsed payload!
    this.activeCommand.resolve(packet.payload ?? packet.rawPayload);
  }
}

// Notification handler - clean and simple
function handleNotification(packet: CS108Packet): void {
  if (packet.event === BATTERY_NOTIFICATION) {
    // No parsing needed - already done!
    updateBattery(packet.payload.voltage);
    updateBatteryPercentage(packet.payload.percentage);
  } else if (packet.event === TRIGGER_PRESSED_NOTIFICATION) {
    setTriggerState(true);
  }
}

// Type guards using event identity
function isInventoryPacket(packet: CS108Packet): boolean {
  return packet.event === INVENTORY_TAG_NOTIFICATION;
}
```

**Integration Tests - Use Parsed Payload:**
```typescript
// Build a test packet
const packet = TestPackets.batteryVoltage(4200);
const parsed = parsePacket(packet);

// Just use the parsed data
expect(parsed.payload.voltage).toBe(4200);
expect(parsed.payload.percentage).toBeGreaterThan(0);
```

**Unit Tests Only - Test the Parser:**
```typescript
// Only unit tests look at rawPayload
expect(packet.rawPayload).toEqual(new Uint8Array([0x68, 0x10]));
expect(packet.payload).toEqual({ voltage: 4200, percentage: 100 });
```

### Benefits
1. **Single source of truth** - All byte manipulation in parsePacket()
2. **Pre-parsed payloads** - No repeated parsing throughout codebase
3. **Type safety** - Can't access invalid offsets or misinterpret fields  
4. **Self-documenting** - `packet.payload.voltage` not `data[10] | (data[11] << 8)`
5. **Clean tests** - Integration tests use parsed data, only unit tests touch rawPayload
6. **Maintainable** - Protocol changes require updates in one location

## File Organization - Clean and Simple

### Naming Convention
Since all files live in `src/worker/cs108/`, we use short, descriptive names without redundant prefixes:

```
src/worker/cs108/
  events.ts     // All CS108Event definitions and EVENT_MAP
  packets.ts    // Packet parsing and building (PacketHandler class)
  manager.ts    // Command queue management (CommandManager class)
  reader.ts     // Main reader interface (Reader class)
  types.ts      // Shared types (CS108Packet, CS108Event interfaces)
```

**NO redundant prefixes:**
- ❌ `CS108PacketHandler.ts` → ✅ `packets.ts`
- ❌ `CS108CommandManager.ts` → ✅ `manager.ts`
- ❌ `CS108Reader.ts` → ✅ `reader.ts`

The directory already provides context - no need to repeat "CS108" everywhere!

## Phase 1: CS108 Protocol Foundation

### 1.1 Packet Structure & Fragmentation Handler

**CS108 Packet Format:**
```
Bytes 0-1:   Header (0xA7 0xB3 for commands, 0xB3 0xA7 for responses)
Byte  2:     Length (size of data after 8-byte header)
Bytes 3-5:   Additional header bytes
Bytes 6-7:   CRC (calculated per Appendix N)
Bytes 8-9:   Event code
Bytes 10+:   Payload (if any)

Total bytes = 8 (header) + length
Max total = 8 + 120 = 128 bytes
```

**Requirements:**
- Handle CS108 packets up to 128 bytes total (8 header + 120 data)
- Fragment buffer management with A7B3/B3A7 header detection
- Packet invalidation on new header when expecting fragments
- CRC verification using bytes 6:7 (per Appendix N algorithm)
- Graceful error handling with console logging

**Key Components:**
```typescript
class CS108PacketHandler {
  private fragmentBuffer: Uint8Array
  private expectedLength: number | null
  private readonly MAX_DATA_SIZE = 120  // Max size after 8-byte header
  private readonly COMMAND_HEADER = [0xA7, 0xB3]
  private readonly RESPONSE_HEADER = [0xB3, 0xA7]
  
  // Core methods
  processIncomingData(data: Uint8Array): CS108Event[]
  private hasNewPacketHeader(): boolean
  private verifyCRC(packet: Uint8Array): boolean
  private resetFragmentState(): void
}
```

**Error Handling:**
- Invalid headers → reset fragment state, log warning
- CRC mismatches → drop packet, log warning  
- Oversized packets → reset state, log warning
- New headers during fragmentation → invalidate current, start new

### 1.2 Unified Event Model

**CS108Event Interface:**
```typescript
interface CS108Event {
  // Identity
  readonly name: string;           // Human-readable for logging
  readonly eventCode: number;      // Event code (same for command and response)
  readonly module: number;         // CS108 module (0xC2 for RFID, etc.)
  
  // Type discrimination
  readonly type: 'command' | 'notification';
  
  // Request (commands only)
  readonly payloadLength?: number;     // Expected payload size
  readonly payload?: Uint8Array;       // Default payload
  readonly timeout?: number;           // Command timeout (ms)
  readonly settlingDelay?: number;     // Post-success delay (ms)
  
  // Response (commands only)
  readonly responseLength?: number;    // Expected response size
  readonly successByte?: number;       // Success indicator (usually 0x00)
  
  // Parser (both commands and notifications)
  readonly parser?: (data: Uint8Array) => any;  // Function reference
  
  // Metadata
  readonly description?: string;
}
```

**Event Lookup Strategy:**
```typescript
// Named constants (Option B - no magic numbers)
const RFID_POWER_OFF: CS108Event = {
  name: 'RFID_POWER_OFF',
  eventCode: 0x8001,
  module: 0xC2,
  type: 'command',
  payloadLength: 0,
  responseLength: 1,
  successByte: 0x00,
  parser: parseUint8,
  description: 'Power off RFID module'
};

const INVENTORY_TAG: CS108Event = {
  name: 'INVENTORY_TAG',
  eventCode: 0x8100,
  module: 0xC2,
  type: 'notification',
  parser: parseInventoryTag,  // Complex parser, deferred
  description: 'Tag read during inventory'
};

// Single lookup map for all events
const CS108_EVENT_MAP = new Map<number, CS108Event>([
  [0x8001, RFID_POWER_OFF],
  [0x8100, INVENTORY_TAG],
  // ... all events
]);
```

### 1.3 Serial Command Queue Manager

**Requirements:**
- Strict serial execution - one command at a time
- **Simplified blocking: queue.length > 0 OR state === BUSY**
- Event code matching for responses  
- Configurable timeouts (default 2.5 seconds, per command override)
- **Settling delays after command success**
- **Queue clearing on errors or mode switches**
- Safety limit: 50 queued commands (should never reach this)

**Simplified State Management:**
```typescript
enum ReaderState {
  DISCONNECTED = 0,
  CONNECTING = 1,
  READY = 2,        // Idle, ready for operations
  BUSY = 3,         // Active command, waiting for response
  SCANNING = 4      // Inventory/locate operation running
}
```

**Queue Control Strategy:**
```typescript
class CS108CommandManager {
  private commandQueue: QueuedCommand[]
  private activeCommand: QueuedCommand | null
  private readerState: ReaderState = ReaderState.DISCONNECTED
  private readonly MAX_QUEUE_SIZE = 50
  
  async executeCommand(event: CS108Event, payload?: Uint8Array): Promise<any> {
    // Simplified blocking - no CONFIGURING state needed
    if (this.commandQueue.length > 0 || this.readerState === ReaderState.BUSY) {
      throw new Error('Cannot queue command - queue busy or command active');
    }
    
    if (this.commandQueue.length >= this.MAX_QUEUE_SIZE) {
      throw new Error('Command queue full - possible deadlock');
    }
    
    // Queue and process
    return new Promise((resolve, reject) => {
      this.commandQueue.push({ event, payload, resolve, reject });
      this.processQueue();
    });
  }
  
  // Process response or notification
  handlePacket(eventCode: number, payload: Uint8Array): void {
    const event = CS108_EVENT_MAP.get(eventCode);
    if (!event) return;
    
    if (event.type === 'command') {
      // Command response - check if we're waiting for it
      if (this.activeCommand?.event.eventCode === eventCode) {
        this.handleCommandResponse(event, payload);
      }
    } else {
      // Notification - process immediately
      this.emitNotification(event, payload);
    }
  }
  
  // Clear queue on errors or mode switches
  clearQueue(reason: string): void {
    console.log(`[Queue] Clearing ${this.commandQueue.length} queued commands: ${reason}`);
    this.commandQueue.forEach(cmd => {
      cmd.reject(new Error(`Queue cleared: ${reason}`));
    });
    this.commandQueue = [];
  }
}

**Timeout Configuration:**
- **Default timeout:** 2500ms (2-3 second range)
- **Per-command override:** Configurable in command definition
- **Timeout strategy:** Fail fast, clear queue, emergency recovery

**Queue Clearing Triggers:**
1. **Command timeout or error** - Clear queue, emergency idle sequence
2. **Mode switch request** - Clear queue, start with idle sequence  
3. **Connection loss** - Clear queue, reset to DISCONNECTED
4. **Emergency stop** - Clear queue, immediate power off

### 1.4 Mode Configuration (Simplified)

**Requirements:**
- **Always execute IDLE sequence first**
- Then add mode-specific commands if needed
- Progress event emission during sequences
- Error recovery via emergency idle on failure
- Inline sequences in setMode() - no SequenceManager needed

**Simplified Mode Implementation:**
```typescript
async setMode(mode: ReaderModeType): Promise<void> {
  // Always start with IDLE sequence (clean state)
  await this.executeSequence([
    RFID_POWER_OFF,
    // BARCODE_POWER_OFF,  // Only if module available
    START_BATTERY_REPORTING,
    START_TRIGGER_REPORTING
  ]);
  
  // Now in IDLE mode
  this.readerMode = ReaderMode.IDLE;
  
  // Configure target mode if not IDLE
  switch(mode) {
    case ReaderMode.IDLE:
      // Already there
      break;
      
    case ReaderMode.INVENTORY:
      await this.executeSequence([
        RFID_POWER_ON,
        // SET_SESSION, SET_TARGET, etc. - add as needed
      ]);
      this.readerMode = ReaderMode.INVENTORY;
      break;
      
    case ReaderMode.BARCODE:
      await this.executeSequence([
        BARCODE_POWER_ON,
        // Barcode configuration commands
      ]);
      this.readerMode = ReaderMode.BARCODE;
      break;
  }
}
```

**Benefits of Always-IDLE-First:**
- **Guaranteed clean state** - Every mode transition starts from known state
- **No hardware conflicts** - Modules properly powered off first
- **Predictable behavior** - Same initialization path every time
- **Simpler logic** - No need to track previous mode or conditional sequences

### 1.5 RAIN RFID Standard Settings Interface

**Requirements:**
- **Vendor-agnostic interface** using RAIN RFID standard terminology
- **Consistent naming** across all layers (Store → DeviceManager → Device → Protocol)
- Settings changes blocked during active operations
- Only transmit power needs runtime reconfiguration (YAGNI for others)

**Reader Settings Interface:**
```typescript
// Unified reader settings for RFID and Barcode modes
interface ReaderSettings {
  // RFID settings (RAIN RFID standard terminology)
  rfid?: {
    transmitPower?: number;           // dBm (e.g., 10-30)
    session?: RainSession;            // S0, S1, S2, S3
    target?: RainTarget;              // A or B inventory flags
    qValue?: number;                  // 0-15, controls inventory rounds
    blinkTimeout?: number;            // ms, time before retrying tag
    inventoryTimeout?: number;        // ms, max inventory duration
    channelMask?: number;             // Bitmask for frequency channels
    hopTable?: number[];              // Custom hop table if supported
    receiverSensitivity?: number;     // dBm, minimum RSSI threshold
  };
  
  // Barcode settings
  barcode?: {
    continuous?: boolean;             // Continuous vs single scan mode
    timeout?: number;                 // ms, scan timeout duration
    symbologies?: BarcodeSymbology[]; // Enabled symbology types
    illumination?: boolean;           // LED illumination on/off
    aimPattern?: boolean;             // Aiming pattern on/off
  };
}

// RAIN RFID standard enums
enum RainSession {
  S0 = 0,  // Volatile, resets on power cycle
  S1 = 1,  // Volatile, resets on power cycle  
  S2 = 2,  // Non-volatile, persists through power cycles
  S3 = 3   // Non-volatile, persists through power cycles
}

enum RainTarget {
  A = 'A',  // Inventory flag A
  B = 'B'   // Inventory flag B  
}

enum BarcodeSymbology {
  CODE128 = 'Code128',
  CODE39 = 'Code39', 
  CODABAR = 'Codabar',
  UPC_A = 'UPC-A',
  UPC_E = 'UPC-E',
  EAN13 = 'EAN-13',
  EAN8 = 'EAN-8',
  QR_CODE = 'QR Code',
  DATA_MATRIX = 'Data Matrix'
}
```

**Consistent Layer Interface:**
```typescript
// Same method signature across all layers
Store:         setSettings(settings: ReaderSettings): Promise<void>
DeviceManager: setSettings(settings: ReaderSettings): Promise<void>
Device:        setSettings(settings: ReaderSettings): Promise<void>
Protocol:      executeCommand(command: CS108Command): Promise<CS108Response>
```

**Vendor Implementation Mapping:**
```typescript
// CS108 maps RAIN RFID standards to vendor-specific registers
class CS108Device {
  async setSettings(settings: ReaderSettings): Promise<void> {
    // RFID settings
    if (settings.rfid?.transmitPower !== undefined) {
      // Map standard dBm to CS108 register 0x1234
      const command = this.buildRegisterCommand(0x1234, settings.rfid.transmitPower);
      await this.commandManager.executeCommand(command);
    }
    
    if (settings.rfid?.session !== undefined) {
      // Map RAIN session to CS108 register 0x5678
      const command = this.buildRegisterCommand(0x5678, settings.rfid.session);
      await this.commandManager.executeCommand(command);
    }
    
    // Barcode settings
    if (settings.barcode?.continuous !== undefined) {
      // Map barcode mode to CS108 barcode configuration
      const command = this.buildBarcodeCommand('SET_CONTINUOUS_MODE', settings.barcode.continuous);
      await this.commandManager.executeCommand(command);
    }
    
    // All vendor complexity hidden behind standard interface
  }
}
```

**Validation Rules:**
- Cannot change settings when `readerState === ReaderState.BUSY` (command running)
- Cannot change settings when `readerState === ReaderState.SCANNING` (operation active)
- Cannot set RFID power when `currentMode === ReaderMode.IDLE`
- Cannot set RFID power when `currentMode === ReaderMode.BARCODE`
- Settings changes allowed only during `readerState === ReaderState.CONNECTED`

## Phase 2: Inventory Stop Challenge

### 2.1 Problem Analysis

**Root Cause:** CS108 firmware continues sending inventory notifications for 10-20 packets after STOP command acknowledgment.

**Current Issues:**
- Trigger release not properly detected
- Stop command response vs actual stop confusion
- UI thread blocking during stop sequences

### 2.2 Solution Design

**Two-Phase Stop Process:**
1. **Command Phase:** Send STOP_INVENTORY_COMMAND, wait for device acknowledgment (2s timeout)
2. **Effect Phase:** Continue processing 10-20 more notifications until silence detected

**Implementation Approach:**
```typescript
// Simplified state tracking in CS108 Protocol Foundation
let stopRequested: boolean = false;
let stopCommandSent: boolean = false;
let postStopNotificationCount: number = 0;

// Derived state instead of managed state
const isScanning = (state: ReaderState) => state === ReaderState.SCANNING;
const inventoryRunning = (mode: ReaderMode, state: ReaderState) => 
  mode === ReaderMode.INVENTORY && isScanning(state);
const locateRunning = (mode: ReaderMode, state: ReaderState) => 
  mode === ReaderMode.LOCATE && isScanning(state);

// Example usage in stop detection:
if (isScanning(this.readerState)) {
  // Send stop command, continue processing notifications
  // State transition SCANNING → READY handles the "running" flag automatically
}

// No separate manager class needed - integrate into existing packet handlers
```

**Stop Detection Strategy:**
- Send stop command through serial queue (gets acknowledgment)
- Continue processing inventory notifications normally
- Count post-stop notifications (for debugging/monitoring)
- Detect actual stop via notification silence (separate mechanism)
- Change ReaderStatus from SCANNING to READY

**Trigger Integration:**
- Trigger release immediately calls `stopInventory()` (non-blocking)
- Stop process handles delayed effect asynchronously
- UI gets immediate feedback, actual stop happens when hardware ready

## Phase 3: Domain Events & Clean Interface

### 3.1 Domain Event Specification

**Core Principle:** Workers emit standardized domain events using ReaderMode enum for data reads + discrete events for state/config changes.

**Data Read Events (ReaderMode-based):**
```typescript
// Raw data emissions from CS108 → Core processor
type DataReadEvent = 
  | { type: ReaderMode.INVENTORY; data: InventoryRead[] }
  | { type: ReaderMode.LOCATE; data: LocateRead[] }
  | { type: ReaderMode.BARCODE; data: BarcodeRead[] }

// Core processor → Store routing uses same enum
comlink.publish(ReaderMode.INVENTORY, processedInventoryBatch)
comlink.publish(ReaderMode.LOCATE, processedLocateReads)
comlink.publish(ReaderMode.BARCODE, processedBarcodeRead)
```

**State & Configuration Events:**
```typescript
type SystemEvent = 
  | { type: 'READER_STATE_CHANGED'; payload: { readerState: ReaderState } }
  | { type: 'READER_MODE_CHANGED'; payload: { mode: ReaderMode } }
  | { type: 'BATTERY_LEVEL_CHANGED'; payload: { percentage: number; voltage?: number } }
  | { type: 'TRIGGER_STATE_CHANGED'; payload: { pressed: boolean } }
  | { type: 'SETTINGS_UPDATED'; payload: { settings: Partial<ReaderSettings> } }
  | { type: 'CONFIGURATION_COMPLETE'; payload: { mode: ReaderMode; duration: number } }
  | { type: 'CONFIGURATION_FAILED'; payload: { mode: ReaderMode; error: string } }
```

**Event Emission Patterns:**
```typescript
// CS108 Protocol Foundation → Core Processor (data reads)
self.postMessage({
  type: ReaderMode.INVENTORY,
  data: [{ epc: 'E280...', rssi: -45, timestamp: Date.now() }]
});

// CS108 Protocol Foundation → DeviceManager (state changes)  
self.postMessage({
  type: 'READER_STATE_CHANGED',
  payload: { readerState: ReaderState.SCANNING }
});

// Core Processor → Stores (batched, processed data)
const batchedData = await processor.batchAndFilter(rawInventoryReads);
comlink.publish(ReaderMode.INVENTORY, batchedData);
```

**Store Routing Benefits:**
- **Perfect naming consistency** - ReaderMode flows through entire pipeline
- **Type safety** - Enum prevents routing errors
- **Clean separation** - Data reads vs system events handled differently
- **Scalable** - Adding new modes automatically creates new event types

### 3.2 Clean High-Level Reader Interface

**IReader Contract (Vendor-Agnostic):**
```typescript
interface IReader {
  // Connection lifecycle
  connect(): Promise<boolean>
  disconnect(): Promise<void>
  
  // Simplified scanning-focused operations
  setMode(mode: ReaderMode): Promise<void>              // → vendor command sequences
  setSettings(settings: ReaderSettings): Promise<void>  // → vendor register/command mapping
  startScanning(): Promise<void>                        // → vendor start commands
  stopScanning(): Promise<void>                         // → vendor stop commands
  
  // State queries (simple returns)
  getMode(): ReaderMode                                 // → simple state return
  getState(): ReaderState                               // → simple state return
  getSettings(): ReaderSettings                         // → json object of configurable RAIN RFID settings
}
```

**External Interface Benefits:**
- **Ultra-minimal API surface** - Only 7 core methods for maximum simplicity
- **No CS108 commands exposed** - DeviceManager never sees protocol details
- **Scanning-focused operations** - Clear start/stop semantics for all modes
- **RAIN RFID standard terminology** - Works with any vendor hardware
- **Universal reader support** - Same interface works for handheld and fixed readers
- **Business-aligned naming** - Matches terminology used in business conversations
- **Simple state queries** - Direct returns without complex parameters
- **Graceful module handling** - Missing barcode modules handled via clear error messages

**Implementation Strategy:**
- Interface methods delegate to protocol foundation classes
- All state changes emit domain events via `self.postMessage()`
- Comlink RPC exposure for main thread communication
- No direct store knowledge in worker

**CS108 Implementation Mapping:**

| Interface Method | Current CS108 Implementation | Required Changes |
|------------------|------------------------------|------------------|
| `connect()` | Direct BLE transport connection | ✅ Keep existing - no changes needed |
| `disconnect()` | Direct BLE transport disconnection | ✅ Keep existing - no changes needed |
| `setMode(mode)` | `SET_OPERATION_MODE` message → triggers `startBaselineSequence()` or `startInventoryConfigSequence()` | 🔄 Wrap existing sequences in mode setter |
| `setSettings(settings)` | No centralized settings API | 🆕 New - map RAIN RFID settings to CS108 register writes |
| `startScanning()` | `START_INVENTORY_OPERATION` message → `startInventorySequence()` | 🔄 Generalize for all modes (inventory/locate/barcode) |
| `stopScanning()` | `STOP_INVENTORY_OPERATION` message → stop command packet | 🔄 Generalize for all modes with delayed-effect handling |
| `getMode()` | `operationMode` variable | ✅ Direct return - no changes needed |
| `getState()` | `readerState` variable | ✅ Direct return - no changes needed |
| `getSettings()` | No centralized settings API | 🆕 New - return JSON object of current RAIN RFID settings |

**Key CS108 Patterns to Preserve:**
- **Serial command queue** - All commands execute sequentially with 2-3s timeouts
- **Idle-first transitions** - Always transition to IDLE before changing modes
- **Delayed-effect inventory stop** - Stop commands don't immediately halt; requires state tracking
- **Fragmentation handling** - 120-byte packets with A7B3/B3A7 headers
- **Command sequence patterns** - baseline, initialization, inventory config sequences

**Modular Hardware Handling:**
```typescript
// RFID is core business value - always present
// Barcode is secondary - field-removable module, ultra-rare removal

// Graceful failure approach preserves 7-method interface
try {
  await reader.setMode(ReaderMode.BARCODE);
} catch (error) {
  if (error.code === 'BARCODE_MODULE_NOT_AVAILABLE') {
    // One-time notification, disable barcode functionality
    showNotification("This reader doesn't support barcode scanning");
    disableBarcodeTab();
  }
}

// CS108 implementation: Optimistic barcode detection during IDLE initialization
class CS108Reader {
  private barcodeAvailable: boolean = true;  // Optimistic default (99% case)
  
  async setMode(mode: ReaderMode): Promise<void> {
    if (mode === ReaderMode.IDLE) {
      // IDLE sequence powers off all modules
      await this.commandManager.executeCommand(RFID_POWER_OFF);
      
      if (this.barcodeAvailable) {  // Only try if we think it's available
        try {
          await this.commandManager.executeCommand(BARCODE_POWER_OFF);
          // Success - barcode module confirmed present
        } catch (error) {
          if (error.isHardwareUnavailable()) {  // 0xFF or timeout response
            this.barcodeAvailable = false;
            this.emitDomainEvent({
              type: 'BARCODE_MODULE_UNAVAILABLE',
              payload: {}
            });
          } else {
            throw error;  // Real error, not missing module
          }
        }
      }
      // Continue with rest of IDLE sequence...
    }
    
    if (mode === ReaderMode.BARCODE && !this.barcodeAvailable) {
      throw new ReaderError('BARCODE_MODULE_NOT_AVAILABLE');
    }
    
    // Continue with normal mode configuration...
  }
}
```

**Module Detection Strategy:**
- **Optimistic default** - Assumes barcode available (99% case), discovers reality during first IDLE
- **One-time discovery** - First `setMode(IDLE)` after connect tests barcode module presence
- **Performance optimization** - Future IDLE calls skip barcode command if module unavailable
- **Immediate UI feedback** - Domain event allows UI to disable barcode tab right after connect
- **Clear error messages** - Subsequent barcode mode attempts fail fast with specific error
- **Session persistence** - Reader remembers module status for entire connection session

### 3.3 Data Emission Architecture

**Pipeline Flow:** CS108 Worker → Core Processor → Zustand Stores

**Data Structures:**
```typescript
// Inventory - batched for volume handling
interface InventoryRead {
  epc: string;
  rssi: number;        // NB_RSSI (narrow band, direct path)
  timestamp: number;   // ms since epoch on packet receipt
}

// Location - real-time for hotter/colder feedback  
interface LocateRead {
  nbRssi: number;      // Primary - narrow band for direct path signal
  wbRssi?: number;     // Optional - wide band including multipath/environment
  phase?: number;      // Optional - for precision ranging applications
  timestamp: number;   // Required for real-time feedback
}

// Barcode - single reads, immediate processing
interface BarcodeRead {
  symbology: string;
  data: string;
  timestamp: number;
}
```

**Harmonized Event Naming (ReaderMode Enum):**
```typescript
// CS108 → Core (raw parsed data)
{ type: ReaderMode.INVENTORY, data: InventoryRead[] }
{ type: ReaderMode.LOCATE, data: LocateRead[] }
{ type: ReaderMode.BARCODE, data: BarcodeRead[] }

// Core → Stores (batched, filtered, processed)
comlink.publish(ReaderMode.INVENTORY, processedInventoryBatch)
comlink.publish(ReaderMode.LOCATE, processedLocateReads)
comlink.publish(ReaderMode.BARCODE, processedBarcodeRead)
```

**Batching Strategy by Mode:**
```typescript
const BATCH_CONFIG = {
  [ReaderMode.INVENTORY]: { timeWindow: 100, maxSize: 10 }, // Volume efficiency
  [ReaderMode.LOCATE]: { timeWindow: 50, maxSize: 3 },      // Real-time feedback
  [ReaderMode.BARCODE]: { timeWindow: 0, maxSize: 1 }       // Immediate single reads
}

// Core Processor knows current mode and applies appropriate batching
class CoreProcessor {
  private currentMode: ReaderMode = ReaderMode.IDLE;
  
  onDataRead(event: DataReadEvent) {
    const config = BATCH_CONFIG[this.currentMode];
    // Apply batching rules based on current mode
    this.batchReads(event.data, config);
  }
  
  onModeChanged(newMode: ReaderMode) {
    this.currentMode = newMode;
    // Flush any pending batches when mode changes
    this.flushPendingBatches();
  }
}
```

**RSSI Architecture (RAIN RFID Standard):**
- **NB_RSSI** - Narrow band signal strength (direct path, less multipath)
- **WB_RSSI** - Wide band signal strength (total environment including reflections)
- **Usage**: NB_RSSI primary for locate mode, WB_RSSI optional for environmental analysis

### 3.3 DeviceManager Bridge

**Single Responsibility:** Worker lifecycle + domain event routing

**Key Components:**
```typescript
class DeviceManager {
  private worker: Worker | null
  private device: WorkerProxy<IHandheldDevice> | null
  
  // Lifecycle management
  async connect(): Promise<boolean>
  async disconnect(): Promise<void>
  private async cleanup(): Promise<void>
  
  // Domain event routing
  private routeDomainEvent(event: DomainEvent): void
  
  // Command delegation
  async setMode(mode: ReaderMode): Promise<void>
  async startScanning(): Promise<void>
  async stopScanning(): Promise<void>
  async setSettings(settings: ReaderSettings): Promise<void>
}
```

**Worker Lifecycle Rules:**
- Zero workers at startup
- Exactly one worker during device connection
- Zero workers after disconnect (guaranteed cleanup)
- Fail-fast on duplicate connection attempts
- Emergency cleanup on any errors

## Object Model Architecture

### Overview

The refactored architecture uses inheritance + composition patterns with clear separation between vendor-agnostic core and vendor-specific implementations.

### Architecture Layers

**1. Core/Common Layer (`src/worker/core/`)**
- `IReader` - Vendor-agnostic interface all readers must implement
- `BaseReader` - Abstract base class with common functionality:
  - Transport management (MessagePort communication)
  - State management (ReaderState tracking)
  - Subscriber pattern for domain events
  - Message queuing and lifecycle management
- Module interfaces: `IReaderRFID`, `IReaderBarcode`, `IReaderSystem`

**2. CS108 Implementation Layer (`src/worker/cs108/`)**
- `CS108Reader extends BaseReader implements IReader`
- Concrete implementations of modules:
  - `CS108RFID implements IReaderRFID`
  - `CS108Barcode implements IReaderBarcode`  
  - `CS108System implements IReaderSystem`
- CS108-specific protocol implementation with serial command queue

### Key Design Patterns

**1. Inheritance + Interface Implementation**
```typescript
class CS108Reader extends BaseReader implements IReader {
  // Inherits common transport/state logic from BaseReader
  // Must implement reader-specific methods
  getModel(): string { return 'CS108'; }
  getFirmwareVersion(): Promise<string> { /* CS108 implementation */ }
  
  // Simplified 7-method interface
  setMode(mode: ReaderMode): Promise<void> { /* CS108 idle-first sequences */ }
  setSettings(settings: ReaderSettings): Promise<void> { /* CS108 register mapping */ }
  startScanning(): Promise<void> { /* CS108 start commands */ }
  stopScanning(): Promise<void> { /* CS108 stop with delayed-effect handling */ }
  
  getMode(): ReaderMode | null { return this.currentMode; }  // null until initialized
  getState(): ReaderState { return this.readerState; }
  getSettings(): ReaderSettings { return this.currentSettings; }
}
```

**2. Composition for Modules**
```typescript
class CS108Reader {
  // Module composition for hardware abstraction
  rfid: CS108RFID;      // RAIN RFID standard operations
  barcode: CS108Barcode; // Barcode scanning operations
  system: CS108System;   // Battery, trigger, system info
  
  constructor() {
    // Modules delegate to protocol foundation classes
    this.rfid = new CS108RFID(this.commandManager, this.sequenceManager);
    this.barcode = new CS108Barcode(this.commandManager);
    this.system = new CS108System(this.commandManager);
  }
}
```

**3. Worker/Main Thread Communication**
```typescript
// DeviceManager creates worker and Comlink proxy
class DeviceManager {
  private reader: WorkerProxy<IReader> | null = null;
  
  async connect(): Promise<boolean> {
    // Create worker with CS108Reader
    this.worker = new Worker('./cs108-worker.js');
    
    // Wrap with Comlink RPC proxy
    this.reader = wrap<IReader>(this.worker);
    
    // Commands go through proxy methods
    return await this.reader.connect();
  }
  
  async setMode(mode: ReaderMode): Promise<void> {
    return await this.reader?.setMode(mode);
  }
}
```

### Data Flow Patterns

**1. Main Thread → Worker**
- DeviceManager calls method on Comlink proxy
- Comlink serializes call to worker thread
- CS108Reader method executes with actual command builders
- Protocol foundation handles CS108-specific packet construction

**2. Worker → Transport**
- CS108Reader sends commands through BaseReader's `sendToTransport()`
- Uses MessagePort to communicate with BLE transport layer
- Serial command queue ensures proper CS108 timing

**3. Transport → Worker**
- BLE data comes through MessagePort
- BaseReader's `handleTransportMessage()` receives it
- CS108Reader's `handleIncomingData()` parses CS108 packets
- Domain events emitted for state changes and data reads

### Integration with Refined Architecture

**Idle-First Mode Implementation:**
```typescript
class CS108Reader extends BaseReader {
  private currentMode: ReaderMode | null = null;  // null until initialized
  
  async setMode(mode: ReaderMode): Promise<void> {
    // Always start with IDLE - powers off all modules, clean state
    await this.sendCommandSequence(IDLE_SEQUENCE);
    this.currentMode = ReaderMode.IDLE;
    
    // Then configure target mode if not IDLE
    switch (mode) {
      case ReaderMode.IDLE:
        // Already done - stay in IDLE
        break;
        
      case ReaderMode.INVENTORY:
        await this.sendCommandSequence(INVENTORY_SEQUENCE);
        this.currentMode = ReaderMode.INVENTORY;
        break;
        
      case ReaderMode.LOCATE:
        await this.sendCommandSequence(LOCATE_SEQUENCE);
        this.currentMode = ReaderMode.LOCATE;
        break;
        
      case ReaderMode.BARCODE:
        await this.sendCommandSequence(BARCODE_SEQUENCE);
        this.currentMode = ReaderMode.BARCODE;
        break;
    }
  }
}
```

**Idle-First Benefits:**
- **Guaranteed clean state** - Every mode transition starts from IDLE
- **No hardware conflicts** - RFID/Barcode modules properly powered off first
- **Predictable behavior** - Same initialization path every time
- **Honest semantics** - Mode is `null` until actually initialized
- **Client control** - DeviceManager explicitly calls `setMode(IDLE)` on connect

**Protocol Foundation Integration:**
```typescript
class CS108Reader extends BaseReader {
  private packetHandler: CS108PacketHandler;
  private commandManager: CS108CommandManager;
  private sequenceManager: CS108SequenceManager;
  
  constructor() {
    super();
    // Protocol foundation classes handle CS108 complexity
    this.packetHandler = new CS108PacketHandler();
    this.commandManager = new CS108CommandManager();
    this.sequenceManager = new CS108SequenceManager(this.commandManager);
  }
  
  // Simplified interface delegates to protocol foundation
  async setMode(mode: ReaderMode): Promise<void> {
    return this.sequenceManager.configureMode(mode);
  }
  
  async startScanning(): Promise<void> {
    return this.commandManager.startOperation(this.currentMode);
  }
}
```

**Domain Event Emission:**
```typescript
class BaseReader {
  protected emitDomainEvent(event: SystemEvent | DataReadEvent): void {
    // Emit to DeviceManager for store routing
    self.postMessage(event);
  }
}

class CS108Reader extends BaseReader {
  private onTagRead(tags: InventoryRead[]): void {
    // Emit data read event using ReaderMode enum
    this.emitDomainEvent({
      type: ReaderMode.INVENTORY,
      data: tags
    });
  }
  
  private onStateChange(newState: ReaderState): void {
    // Emit system event
    this.emitDomainEvent({
      type: 'READER_STATE_CHANGED',
      payload: { readerState: newState }
    });
  }
}
```

### Benefits of This Design

**Extensibility:**
- Adding new readers (Zebra, Nordic) by extending BaseReader
- Shared transport/state logic in BaseReader
- Reader-specific protocol in concrete classes

**Modularity:**
- Clean module separation (RFID, Barcode, System)
- Protocol foundation classes encapsulate CS108 complexity
- Vendor-agnostic interfaces enable future hardware support

**Maintainability:**
- Clear inheritance hierarchy
- Composition for hardware modules
- Domain events decouple worker from stores
- Ultra-minimal 7-method external interface

**Performance:**
- Comlink RPC handles worker communication efficiently
- Serial command queue prevents CS108 timing issues
- Batched domain events reduce main thread overhead

## Testing Strategy & Bootstrap Plan

### Integration-Only TDD Approach

**Rationale:** Hardware-responsive (10-20% overhead) vs hardcoded byte sequence drift pain and double test maintenance burden.

**Benefits:**
- **Single source of truth** - Real CS108 hardware responses, no drift possible
- **Zero maintenance overhead** - One test suite instead of unit + integration
- **Immediate hardware validation** - Tests against actual protocol behavior
- **Fast development velocity** - Extract proven patterns from working code

### Test Infrastructure

**Directory Structure:**
```
tests/
├── e2e/                              # Existing Playwright tests
├── integration/
│   └── worker/
│       ├── core/                     # Future: BaseReader, shared worker tests
│       └── cs108/
│           ├── helpers/
│           │   ├── CS108TestClient.ts    # Shared NodeBleClient wrapper
│           │   ├── test-config.ts        # Config loading (.env.local)
│           │   └── smoke-test.spec.ts    # Client integration validation
│           └── cs108-reader.spec.ts      # Main TDD integration tests
└── data/                             # Existing test data
```

**Test Infrastructure Notes:**
- Bridge server runs on separate VM (192.168.50.73:8080) - not managed by this project
- Run `pnpm dev` in separate terminal for visual monitoring
- Use `pnpm test:integration` for all integration tests
- Add specific test scripts only if needed (e.g., `test:worker`, `test:cs108`)

### Generic Test Client Architecture

**Generic client supporting multiple hardware vendors:**

```typescript
// Generic client for any RFID reader hardware
export class RfidReaderTestClient {
  private client: NodeBleClient;
  
  constructor(private config: RfidReaderConfig) {
    this.client = new NodeBleClient({
      bridgeUrl: config.wsUrl,
      service: config.service,
      write: config.write,
      notify: config.notify
    });
  }
  
  // Generic raw command interface
  async sendRawCommand(bytes: number[]): Promise<Uint8Array>
  async connect(): Promise<void>
  async disconnect(): Promise<void>
}

// Hardware-specific configuration and test data
export class CS108TestHelpers {
  static readonly CONFIG = {
    service: '9800',
    write: '9900', 
    notify: '9901'
  };
  
  static readonly SMOKE_TEST = {
    // GET_TRIGGER_STATE (0xA001) - deterministic response (0=released, 1=pressed)
    command: [0xA7, 0xB3, 0x02, 0xD9, 0x82, 0x37, 0x00, 0x00, 0xA0, 0x01],
    expectedResponse: [0xA7, 0xB3, 0x03, 0xD9, 0x82, 0x9E, 0x74, 0x37, 0xA0, 0x01, 0x00]  // 0x00=released
  };
}

// Future hardware support
export class ZebraTestHelpers {
  static readonly CONFIG = {
    service: 'zebra-service-uuid',
    write: 'zebra-write-uuid',
    notify: 'zebra-notify-uuid'
  };
  
  static readonly SMOKE_TEST = {
    command: [/* Zebra command bytes */],
    expectedResponse: [/* Zebra response bytes */]
  };
}
```

### Bootstrap Implementation Plan

**Step 1: Adapt NodeBleClient**
- Extract connection config from parent's `.env.local` (URL, UUIDs)
- Build thin CS108TestClient wrapper around NodeBleClient
- Get basic WebSocket → Bridge Server → CS108 hardware connection working

**Step 2: Smoke Test (ONE-TIME Hardcoded Bytes Exception)**
```typescript
// CS108-specific smoke test using generic client
const client = new RfidReaderTestClient({
  wsUrl: getWsUrl(),
  ...CS108TestHelpers.CONFIG
});

await client.connect();
const response = await client.sendRawCommand(CS108TestHelpers.SMOKE_TEST.command);
expect(response).toEqual(new Uint8Array(CS108TestHelpers.SMOKE_TEST.expectedResponse));
```

**Step 3: First TDD Integration Test**
```typescript
test('setMode(IDLE) completes successfully', async () => {
  const client = new CS108TestClient(getTestConfig());
  await client.connect();
  await client.setMode(ReaderMode.IDLE);
  expect(await client.getMode()).toBe(ReaderMode.IDLE);
  expect(await client.getState()).toBe(ReaderState.CONNECTED);
});
```

**Step 4: Extract & Refactor CS108 Code**
- Copy patterns from existing `cs108-worker.ts`
- Build clean CS108PacketHandler, CS108CommandManager, CS108SequenceManager classes
- Make integration test pass using extracted code

**Step 5: Rinse & Repeat**
- Add integration tests for remaining functionality
- Extract and refactor corresponding implementation code
- Build complete CS108Reader with IReader interface

### Usage Patterns

**Daily TDD Development:**
```bash
# Ensure bridge server is running on 192.168.50.73:8080 (separate VM)
# Run in separate terminal: pnpm dev:mock (for visual monitoring)

# Run integration tests
pnpm test:integration                           # Run all integration tests
pnpm test:integration tests/integration/worker  # Run worker tests specifically

# Watch mode for TDD
pnpm vitest tests/integration/worker/cs108/ --watch  # TDD with hardware feedback
```

**Test Organization:**
```bash
# Current simplified structure
pnpm test:integration  # Main integration test command

# Can add specific scripts if needed:
# pnpm test:worker     # Worker-specific tests
# pnpm test:cs108      # CS108-specific tests
```

## Implementation Phases

### Phase 1: Protocol Foundation
**Deliverables:**
- CS108PacketHandler with fragmentation
- CS108CommandManager with serial queue  
- CS108SequenceManager with mode configurations
- Comprehensive unit tests against real hardware via MCP BLE tools

**Success Criteria:**
- All packet fragmentation edge cases handled
- Command sequences execute reliably
- Mode transitions work with idle-first pattern
- Hardware integration tests pass

### Phase 2: Inventory Stop & Device Interface  
**Deliverables:**
- CS108InventoryManager with delayed-effect stop handling
- IReader implementation wrapping Phase 1 components
- Domain event emission for all state changes
- Integration tests with Phase 1 components

**Success Criteria:**
- Inventory stop works reliably (trigger release → actual stop)
- Clean interface abstracts protocol complexity
- Domain events match store requirements
- No store coupling in worker code

### Phase 3: DeviceManager Bridge
**Deliverables:**
- DeviceManager with worker lifecycle management
- Domain event routing to existing stores
- Comlink RPC integration
- End-to-end integration with existing UI

**Success Criteria:**
- Worker lifecycle guarantees maintained
- Domain events correctly routed to stores
- UI functions identically to current implementation
- No performance regressions

### Phase 4: Migration & Cleanup
**Deliverables:**
- Remove old spaghetti implementation
- Update existing DeviceManager to use new worker
- Documentation updates
- Performance validation

**Success Criteria:**
- Old implementation completely removed
- New implementation integrated seamlessly
- No user-visible changes or regressions
- Improved debuggability and maintainability

## Testing Strategy

### 🚨 CRITICAL: No Hardcoded Bytes Rule

**NEVER write raw byte arrays in tests. ALWAYS use packet builders and metadata.**

```typescript
// ❌ FORBIDDEN - Never do this in tests
expect(sentCommand).toEqual([0xA7, 0xB3, 0x00, 0x03, 0xA0, 0x01, 0x00, 0x00]);

// ✅ CORRECT - Use semantic verification
expect(commandLog.last()).toEqual({ command: 'SET_MODE', mode: 'IDLE' });
expect(reader.getCommandHistory()).toContain('INVENTORY_START');
```

### Command Verification Strategies

**1. Semantic Command Log (Primary)**
```typescript
class CS108Reader {
  private commandLog: CommandLogEntry[] = [];
  
  async setMode(mode: ReaderMode): Promise<void> {
    this.logCommand('SET_MODE', { mode });
    // ... actual implementation
  }
  
  getCommandHistory(): string[] {
    return this.commandLog.map(e => e.command);
  }
}

// Test uses semantic names only
test('should execute idle sequence', async () => {
  await reader.setMode('IDLE');
  expect(reader.getCommandHistory()).toEqual([
    'RFID_POWER_OFF',
    'BARCODE_POWER_OFF', 
    'SET_BATTERY_REPORTING',
    'SET_TRIGGER_REPORTING'
  ]);
});
```

**2. Response-Based Verification (Black Box)**
```typescript
// Don't test what was sent, test the outcome
test('should enter IDLE mode', async () => {
  await reader.setMode('IDLE');
  expect(reader.getMode()).toBe('IDLE');
  expect(reader.getState()).toBe('READY');
  // Trust that if mode changed, correct commands were sent
});
```

**3. Metadata-Driven Command Matching**
```typescript
// If we must verify actual bytes, parse them back to names
const identifyCommand = (bytes: Uint8Array): string => {
  const command = Object.values(CS108_COMMANDS).find(
    cmd => cmd.command === bytes[8] && bytes[9] === bytes[9]
  );
  return command?.name || 'UNKNOWN';
};

// Never compare bytes directly
expect(identifyCommand(capturedBytes)).toBe('INVENTORY_START');
```

### Unit Testing (Per Phase)
- Pure protocol functions tested in isolation
- Real hardware integration via MCP BLE tools
- Error injection and recovery testing
- Performance and timing validation

### Integration Testing (Cross-Phase)  
- Command sequences with real hardware responses
- Mode transitions end-to-end
- Domain event flow validation
- Store integration verification

### Migration Testing (Final Phase)
- Side-by-side comparison with old implementation
- User workflow validation
- Performance regression testing
- Error handling parity verification

## Risk Mitigation

### High Risk: Inventory Stop Complexity
- **Mitigation:** Implement Phase 2 first, extensive testing with real hardware
- **Fallback:** Emergency power-off recovery if stop fails

### Medium Risk: Command Timing Issues  
- **Mitigation:** Use proven timeout values, extensive sequential testing
- **Fallback:** Configurable timeouts per command type

### Low Risk: Domain Event Mapping
- **Mitigation:** Careful analysis of existing store contracts
- **Fallback:** Gradual migration with compatibility layer

## Implementation Insights (December 2024)

### Key Decisions Made

**1. Test Architecture**
- **NO Zustand stores in worker tests** - Workers emit pure domain events
- **CS108WorkerTestHarness** bridges real hardware to worker without stores
- **Transport boundary verification** - Capture postMessage to prove commands sent
- **Single source of truth** - `cs108/constants.ts` for all event codes

**2. Directory Structure**
```
tests/integration/
├── ble-mcp-test/          # Bridge server connectivity
│   ├── connection.spec.ts # Generic bridge test
│   └── rfid-reader-test-client.ts
└── cs108/                  # CS108-specific tests
    ├── CS108WorkerTestHarness.ts
    └── *.spec.ts          # TDD test files
```

**3. Clean Workspace Strategy**
- Moved old implementation to `cs108-old/` and `core-old/`
- Clean `cs108/` directory with only:
  - `CS108Reader.ts` - TDD implementation
  - `constants.ts` - Single source of truth
- Pull patterns from old code as needed

**4. Command Verification Approach**
```typescript
// Transport capture at MessagePort boundary
class CS108WorkerTestHarness {
  private transportMessages: TransportMessage[] = [];
  
  // Capture actual bytes sent to hardware
  setupTransportCapture() {
    workerPort.postMessage = (msg) => {
      this.transportMessages.push({
        bytes: msg.data,
        commandName: identifyCS108Command(msg.data)
      });
    };
  }
  
  // Tests verify semantic names, not bytes
  getOutboundCommands(): string[] {
    return this.transportMessages
      .filter(m => m.direction === 'outbound')
      .map(m => m.commandName); // 'RFID_POWER_OFF', etc.
  }
}
```

**5. Constants Architecture**
```typescript
// Event codes same for commands and responses
export const CS108_COMMANDS = {
  RFID_POWER_OFF: { code: 0x8001, name: 'RFID_POWER_OFF' },
  // ...
};

// Event code at bytes 8-9 after 8-byte header
export function identifyCS108Command(bytes: Uint8Array): string {
  const eventCode = (bytes[8] << 8) | bytes[9];
  return CS108_COMMAND_MAP[eventCode] || 'UNKNOWN';
}
```

**6. Development Workflow**
- **Bridge server** runs on separate VM (192.168.50.73:8080)
- **Run `pnpm dev:mock`** in separate terminal for monitoring
- **Simplified scripts** - just `pnpm test:integration`
- **Tight collaboration** - Every change reviewed, no vibe coding

### TDD Implementation Progress

**Phase 1: Foundation ✅**
- ✅ Test framework with transport verification
- ✅ Single source of truth for constants
- ✅ Clean workspace (old code moved to `-old` dirs)
- ✅ Basic CS108Reader skeleton

**Phase 2: Architecture & State Management ✅**
- ✅ BaseReader/CS108Reader inheritance model
- ✅ MessagePort transport layer setup
- ✅ State machine (DISCONNECTED → CONNECTING → READY)
- ✅ Mode management (null → IDLE on connect)
- ✅ Domain event emission
- ✅ Test harness with real hardware bridge
- ✅ All TypeScript/lint issues resolved

**Phase 3: Command Implementation (Next)**
- ⏳ Packet builder with CS108 constants
- ⏳ Command sending via MessagePort
- ⏳ Response parsing and routing
- ⏳ Error handling for timeouts/NAKs
- ⏳ First real command (GET_VERSION or RFID_POWER_OFF)

**Phase 4: Mode Sequences**
- ⏳ IDLE mode command sequences
- ⏳ INVENTORY mode transitions
- ⏳ BARCODE mode (if module available)
- ⏳ Mode-specific settings application

**Phase 5: Operations**
- ⏳ Start/stop scanning with proper sequencing
- ⏳ Inventory stop with delayed effect handling
- ⏳ Settings persistence and validation
- ⏳ Complete domain event coverage

## Success Metrics

### Technical Metrics
- Codebase size reduction (10k LOC → target: 2-3k LOC)
- Test coverage > 90% for core protocol components
- Zero direct store imports in worker code
- Command response time < 100ms 95th percentile

### Functional Metrics  
- Inventory stop success rate > 99%
- Mode transition reliability > 99%
- Zero user-visible regressions
- Improved error messages and debugging

### Maintainability Metrics
- Clear component boundaries and responsibilities
- Self-documenting code with minimal comments needed
- Easy to add new device features (extensibility)
- Simplified debugging and troubleshooting