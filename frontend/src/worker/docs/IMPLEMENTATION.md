# CS108 Implementation Guide

## Adding New Commands

### Step 1: Define the Event
```typescript
// In cs108/events.ts
export const GET_BATTERY_LEVEL: CS108Event = {
  name: 'GET_BATTERY_LEVEL',
  eventCode: 0xA000,
  module: CS108_MODULES.NOTIFICATION,
  type: 'command',  // Expects a response
  parser: (payload) => {
    const millivolts = payload[0] | (payload[1] << 8);
    return { millivolts, percentage: calculatePercentage(millivolts) };
  },
  successByte: 0x00
};

// Notification example
export const TRIGGER_PRESSED: CS108Event = {
  name: 'TRIGGER_PRESSED',
  eventCode: 0xA102,
  module: CS108_MODULES.NOTIFICATION,
  type: 'notification'  // No response expected
  // No parser needed - just the event itself is the data
};
```

### Step 2: Add to Command Map
```typescript
// In cs108/constants.ts
export const CS108_COMMANDS = {
  // ... existing commands
  GET_BATTERY_LEVEL: GET_BATTERY_LEVEL
};
```

### Step 3: Implement in Reader
```typescript
// In cs108/reader.ts
async getBatteryLevel(): Promise<number> {
  const response = await this.sendCommand(GET_BATTERY_LEVEL);
  if (response.payload?.percentage) {
    this.postMessage({
      type: 'BATTERY_UPDATE',
      payload: { percentage: response.payload.percentage }
    });
    return response.payload.percentage;
  }
  throw new Error('Invalid battery response');
}
```

## Mode Implementation

### Mode Configuration Sequence
```typescript
async setMode(targetMode: ReaderMode): Promise<void> {
  // Always run IDLE commands first (ensures clean state)
  await this.sendCommand(ABORT_OPERATION);
  await this.sendCommand(SET_TO_IDLE);
  
  // Then configure target mode
  switch (targetMode) {
    case ReaderMode.IDLE:
      // Already in IDLE, nothing more to do
      break;
      
    case ReaderMode.INVENTORY:
      await this.sendCommand(SET_INVENTORY_ALGORITHM);
      await this.sendCommand(SET_INVENTORY_ROUNDS);
      await this.sendCommand(SET_Q_VALUE);
      if (this.settings?.inventory?.power) {
        await this.setPower(this.settings.inventory.power);
      }
      await this.sendCommand(ENABLE_INVENTORY_MODE);
      break;
      
    case ReaderMode.BARCODE:
      await this.sendCommand(ENABLE_BARCODE_MODE);
      break;
  }
  
  this.mode = targetMode;
  this.emitModeChange(targetMode);
}
```

### Starting Operations
```typescript
async startScanning(): Promise<void> {
  // Validate state
  if (this.state !== ReaderState.CONNECTED) {
    throw new Error('Reader not ready');
  }
  
  // Mode-specific start
  switch (this.mode) {
    case ReaderMode.INVENTORY:
      await this.startInventory();
      break;
    case ReaderMode.BARCODE:
      await this.startBarcodeScan();
      break;
    default:
      throw new Error(`Cannot scan in ${this.mode} mode`);
  }
  
  this.changeState(ReaderState.SCANNING);
}

private async startInventory(): Promise<void> {
  // Send start command
  await this.sendCommand(START_INVENTORY);
  
  // Tags will arrive via notifications
  // No need to poll
}
```

## Packet Building

### Single Generic Packet Builder
```typescript
// In cs108/packets.ts
export class PacketHandler {
  // One builder for ALL packets
  buildPacket(event: CS108Event, payload?: Uint8Array): Uint8Array {
    const dataLength = 2 + (payload?.length ?? 0);
    const packet = new Uint8Array(8 + dataLength);
    
    // Header
    packet[0] = 0xA7;  // Prefix
    packet[1] = 0xB3;  // Bluetooth transport
    packet[2] = dataLength;
    packet[3] = event.module;
    packet[4] = 0x82;  // Reserved
    packet[5] = 0x37;  // Downlink (we're sending TO device)
    packet[6] = 0x00;  // CRC low
    packet[7] = 0x00;  // CRC high
    
    // Event code (little-endian)
    packet[8] = event.eventCode & 0xFF;
    packet[9] = (event.eventCode >> 8) & 0xFF;
    
    // Payload (if any)
    if (payload) {
      packet.set(payload, 10);
    }
    
    return packet;
  }
}

// Usage is clean and simple:
const packet = packetHandler.buildPacket(GET_BATTERY_LEVEL);
const packet2 = packetHandler.buildPacket(SET_POWER, powerBytes);
```

### Never Hardcode Bytes
```typescript
// ❌ WRONG - Hardcoded bytes
const command = new Uint8Array([0xA7, 0xB3, 0x02, 0x01, ...]);

// ✅ CORRECT - Use event definitions
const command = this.packetHandler.buildPacket(RFID_POWER_ON);
```

## Response Handling

### Fragmentation with Packet Object
```typescript
private currentPacket: CS108Packet | null = null;
private fragmentTimeout?: NodeJS.Timeout;

private handleTransportMessage = (event: MessageEvent) => {
  const { type, data } = event.data;
  
  if (type === 'DATA') {
    this.handleFragment(new Uint8Array(data));
  }
};

private handleFragment(data: Uint8Array) {
  // Clear any existing fragment timeout
  if (this.fragmentTimeout) {
    clearTimeout(this.fragmentTimeout);
  }
  
  if (!this.currentPacket) {
    // First fragment - parse header to create packet object
    try {
      this.currentPacket = this.packetHandler.parseHeader(data);
      // parseHeader creates packet with header info and partial rawPayload
    } catch (e) {
      console.error('Invalid packet header:', e);
      return;
    }
  } else {
    // Continuation - append to existing packet's rawPayload
    const combined = new Uint8Array(
      this.currentPacket.rawPayload.length + data.length
    );
    combined.set(this.currentPacket.rawPayload);
    combined.set(data, this.currentPacket.rawPayload.length);
    this.currentPacket.rawPayload = combined;
  }
  
  // Check if complete
  if (this.currentPacket.rawPayload.length >= this.currentPacket.totalExpected) {
    this.processCompletePacket();
  } else {
    // Set timeout for next fragment (200ms)
    this.fragmentTimeout = setTimeout(() => {
      console.error('Fragment timeout - discarding partial packet');
      this.currentPacket = null;
    }, 200);
  }
}

private processCompletePacket() {
  if (!this.currentPacket) return;
  
  try {
    // Parse payload if event has a parser
    if (this.currentPacket.event.parser) {
      // Extract actual payload (skip success byte if present)
      const payloadStart = this.currentPacket.event.successByte !== undefined ? 1 : 0;
      const payloadBytes = this.currentPacket.rawPayload.slice(payloadStart);
      this.currentPacket.payload = this.currentPacket.event.parser(payloadBytes);
    }
    
    // Mark as complete and route
    this.currentPacket.isComplete = true;
    this.routePacket(this.currentPacket);
  } catch (e) {
    console.error('Payload parse failed:', e);
  } finally {
    // Clear for next packet
    this.currentPacket = null;
  }
}
```

### Notification Handlers

#### Handler Pattern Implementation
```typescript
class InventoryTagHandler implements NotificationHandler {
  private batcher: InventoryBatcher;

  constructor(batcher: InventoryBatcher) {
    this.batcher = batcher;
  }

  canHandle(packet: CS108Packet, context: NotificationContext): boolean {
    // Only handle in inventory mode
    if (context.currentMode !== ReaderMode.INVENTORY) {
      return false;
    }

    // Must have tag data
    const payload = packet.payload as any;
    return payload && 'epc' in payload && 'rssi' in payload;
  }

  handle(packet: CS108Packet, context: NotificationContext): void {
    // Extract tag data using pre-parsed payload
    const payload = packet.payload as any;
    const tagData: TagData = {
      epc: payload.epc,
      rssi: payload.rssi,
      pc: payload.pc,
      timestamp: packet.timestamp || Date.now(),
      phase: payload.phase,
      antenna: payload.antenna,
    };

    // Add to batcher for efficient processing
    this.batcher.add(tagData);
  }

  cleanup(): void {
    if (this.batcher.size() > 0) {
      this.batcher.flush();
    }
    this.batcher.cleanup();
  }
}
```

#### Factory Pattern for Context Binding
```typescript
export function createInventoryHandler(
  emitDomainEvent: (event: DomainEvent) => void
): InventoryTagHandler {
  const batcher = new InventoryBatcher(DEFAULT_INVENTORY_CONFIG);

  // Set up flush callback with bound context
  batcher.onFlush((tags) => {
    emitDomainEvent({
      type: 'TAG_BATCH',
      payload: { tags },
      timestamp: Date.now(),
    });
  });

  return new InventoryTagHandler(batcher);
}
```

#### Registration Pattern
```typescript
class NotificationManager {
  private router: NotificationRouter;

  constructor(emitDomainEvent: (event: DomainEvent) => void, config: NotificationManagerConfig) {
    this.router = new NotificationRouter(emitDomainEvent, routerConfig);
    this.registerHandlers();
  }

  private registerHandlers(): void {
    // System handlers
    this.router.register(EventCodes.BATTERY_VOLTAGE, new BatteryHandler());
    this.router.register(EventCodes.TRIGGER_PRESSED, new TriggerPressedHandler());
    this.router.register(EventCodes.ERROR_NOTIFICATION, new ErrorNotificationHandler());

    // RFID handlers with factory pattern
    const inventoryHandler = createInventoryHandler(this.emitDomainEvent);
    this.router.register(EventCodes.INVENTORY_TAG, inventoryHandler);

    // Multiple handlers for same event code
    this.router.register(EventCodes.INVENTORY_TAG, new LocateTagHandler());

    // Barcode handlers
    this.router.register(EventCodes.BARCODE_DATA, new BarcodeDataHandler());
  }

  handleNotification(packet: CS108Packet): void {
    // Simply delegate to router - no switch statement needed!
    this.router.handleNotification(packet);
  }
}
```

## Error Handling

### Command Timeout Pattern
```typescript
async sendCommand(event: CS108Event, options = {}): Promise<CS108Packet> {
  const timeout = options.timeout ?? 5000;
  
  return new Promise((resolve, reject) => {
    // Set pending command
    this.pendingCommand = { event, resolve, reject };
    
    // Send command bytes
    const bytes = this.packetHandler.buildPacket(event);
    this.transportPort.postMessage({ type: 'WRITE', data: bytes });
    
    // Timeout handler
    const timer = setTimeout(() => {
      this.pendingCommand = null;
      reject(new CommandTimeout(`${event.name} timed out after ${timeout}ms`));
    }, timeout);
    
    // Update resolve to clear timer
    const originalResolve = resolve;
    this.pendingCommand.resolve = (packet) => {
      clearTimeout(timer);
      this.pendingCommand = null;
      originalResolve(packet);
    };
  });
}
```

### Recovery Strategies
```typescript
async recover(error: Error): Promise<void> {
  console.error('Recovery triggered:', error);
  
  try {
    // Stop any active operations
    if (this.state === ReaderState.SCANNING) {
      await this.stopScanning().catch(() => {});
    }
    
    // Return to IDLE
    await this.setMode(ReaderMode.IDLE);
    
    // Clear buffers
    this.fragmentBuffer = new Uint8Array(0);
    this.pendingCommand = null;
    
    // Restore to READY
    this.changeState(ReaderState.CONNECTED);
  } catch (recoveryError) {
    // Recovery failed, disconnect
    console.error('Recovery failed:', recoveryError);
    await this.disconnect();
  }
}
```

## Testing Patterns

### Mock Transport for Unit Tests
```typescript
// Create mock transport
const mockTransport = {
  postMessage: vi.fn(),
  onmessage: null
};

// Inject into reader
reader.setTransportPort(mockTransport);

// Simulate response
mockTransport.onmessage({
  data: {
    type: 'DATA',
    data: buildResponse(RFID_POWER_ON, { success: true })
  }
});
```

### Integration Test Pattern
```typescript
// Use real hardware via bridge
const harness = new CS108WorkerTestHarness();
await harness.initialize();

// Send real command
const reader = harness.getReader();
await reader.connect();

// Verify with real response
expect(reader.getState()).toBe(ReaderState.CONNECTED);
```

## Common Patterns

### Settings Application
```typescript
async applySettings(settings: ReaderSettings): Promise<void> {
  // Store for later use
  this.settings = settings;
  
  // Apply if connected
  if (this.state === ReaderState.CONNECTED) {
    // Stop scanning if active
    if (this.state === ReaderState.SCANNING) {
      await this.stopScanning();
    }
    
    // Apply mode-specific settings
    switch (this.mode) {
      case ReaderMode.INVENTORY:
        await this.configureInventoryMode();
        break;
      // ... other modes
    }
  }
}
```

### State Change Pattern
```typescript
private changeState(newState: ReaderState) {
  const oldState = this.state;
  this.state = newState;
  
  // Emit event
  this.postMessage({
    type: 'READER_STATE_CHANGED',
    payload: { 
      from: oldState, 
      to: newState 
    }
  });
  
  // Log for debugging
  console.log(`State: ${oldState} → ${newState}`);
}