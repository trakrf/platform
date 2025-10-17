# CS108 Notification Handlers Reference

## Overview
This document provides detailed reference information for all notification handlers in the CS108 worker system. Each handler implements the `NotificationHandler` interface to process specific types of autonomous notifications from the CS108 hardware.

## Handler Interface Contract

### NotificationHandler Interface
```typescript
interface NotificationHandler {
  canHandle(packet: CS108Packet, context: NotificationContext): boolean;
  handle(packet: CS108Packet, context: NotificationContext): void;
  cleanup?(): void;
}
```

### Implementation Requirements
- **canHandle()** - Must check context and packet contents to determine handling capability
- **handle()** - Must process the packet and emit appropriate domain events
- **cleanup()** - Optional cleanup for stateful handlers (timers, batchers, etc.)

### Context Usage
All handlers receive `NotificationContext` containing:
- **currentMode** - Current reader operational mode
- **readerState** - Current connection/operational state
- **emitDomainEvent** - Function to emit domain events
- **metadata** - Debug flags and configuration

## System Handlers

### BatteryHandler
**File**: `src/worker/cs108/system/battery.ts`
**Event Code**: 0xA000 (GET_BATTERY_VOLTAGE)

#### Purpose
Converts raw battery voltage readings to percentage values for UI display.

#### Packet Requirements
```typescript
canHandle(packet: CS108Packet, context: NotificationContext): boolean {
  // Must have voltage data in payload
  return packet.payload?.voltage !== undefined;
}
```

#### Processing Logic
1. Extract voltage from packet payload (millivolts)
2. Calculate percentage using linear mapping: 3000-4200mV → 0-100%
3. Emit `BATTERY_UPDATE` domain event with percentage and voltage

#### Domain Events
- `BATTERY_UPDATE` - Battery status update
  ```typescript
  {
    type: 'BATTERY_UPDATE',
    payload: {
      percentage: number;  // 0-100
      voltage: number;     // Raw millivolts
    }
  }
  ```

#### Testing
- Unit tests verify voltage-to-percentage conversion accuracy
- Edge cases tested: minimum (3000mV), maximum (4200mV), out-of-range values
- Debug logging available when metadata.debug = true

### TriggerHandler Family
**File**: `src/worker/cs108/system/trigger.ts`
**Event Codes**: 0xA102 (TRIGGER_PRESSED), 0xA103 (TRIGGER_RELEASED), 0xA001 (TRIGGER_STATE)

#### Base Class Pattern
Uses inheritance to share common logic:

```typescript
abstract class BaseTriggerHandler implements NotificationHandler {
  protected abstract getPressed(): boolean;

  handle(packet: CS108Packet, context: NotificationContext): void {
    const pressed = this.getPressed();

    context.emitDomainEvent({
      type: 'TRIGGER_STATE_CHANGED',
      payload: { pressed }
    });
  }
}
```

#### Derived Handlers
- **TriggerPressedHandler** (0xA102) - Always returns `pressed: true`
- **TriggerReleasedHandler** (0xA103) - Always returns `pressed: false`
- **TriggerStateHandler** (0xA001) - Extracts state from payload

#### Domain Events
- `TRIGGER_STATE_CHANGED` - Trigger button state change
  ```typescript
  {
    type: 'TRIGGER_STATE_CHANGED',
    payload: {
      pressed: boolean;
    }
  }
  ```
- `COMMAND_RESPONSE` - For state query responses (0xA001 only)

### ErrorNotificationHandler
**File**: `src/worker/cs108/system/error.ts`
**Event Code**: 0xA101 (ERROR_NOTIFICATION)

#### Purpose
Processes device error notifications with rate limiting to prevent log spam.

#### Error Code Mapping
Maps numeric error codes to human-readable messages:
- 0x0001 - "Invalid command"
- 0x0002 - "Invalid parameter"
- 0x0004 - "Unknown event code"
- etc.

#### Rate Limiting
- Tracks last error time per error code
- Minimum 5-second interval between same error emissions
- Prevents console spam from repeated hardware errors

#### Domain Events
- `DEVICE_ERROR` - Device error notification
  ```typescript
  {
    type: 'DEVICE_ERROR',
    payload: {
      code: number;     // Raw error code
      message: string;  // Human-readable message
    }
  }
  ```

## RFID Handlers

### InventoryHandler + InventoryBatcher
**File**: `src/worker/cs108/rfid/inventory-handler.ts`, `inventory-batcher.ts`
**Event Code**: 0x8100 (INVENTORY_TAG)
**Required Mode**: INVENTORY

#### Purpose
Efficiently batches RFID tag reads for optimal UI performance during inventory operations.

#### Context Filtering
```typescript
canHandle(packet: CS108Packet, context: NotificationContext): boolean {
  // Must be in inventory mode
  if (context.currentMode !== ReaderMode.INVENTORY) {
    return false;
  }

  // Must have tag data
  const payload = packet.payload as any;
  return payload && 'epc' in payload && 'rssi' in payload;
}
```

#### Batching Strategy
The `InventoryBatcher` provides configurable batching:

```typescript
interface BatchingConfig {
  maxSize: number;                    // Flush after N tags
  timeWindowMs: number;               // Flush every N milliseconds
  flushOnModeChange: boolean;         // Flush when switching modes
  deduplicationWindowMs: number;      // Dedupe tags within N ms
}
```

**Default Config**: 50 tags or 100ms (efficiency focused)

#### Tag Deduplication
- Groups tags by EPC identifier
- Tracks read count per unique tag
- Keeps best (highest) RSSI value
- Updates timestamp on each read

#### Factory Pattern
Uses factory function to solve callback binding:

```typescript
export function createInventoryHandler(
  emitDomainEvent: (event: DomainEvent) => void
): InventoryTagHandler {
  const batcher = new InventoryBatcher(DEFAULT_INVENTORY_CONFIG);

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

#### Domain Events
- `TAG_BATCH` - Batched tag data
  ```typescript
  {
    type: 'TAG_BATCH',
    payload: {
      tags: TagData[];  // Array of batched tags
    }
  }
  ```

#### Testing
- Comprehensive batching tests (size-based, time-based)
- Deduplication logic verification
- Configuration update testing
- Edge case handling (rapid additions, empty batches)

### LocateHandler
**File**: `src/worker/cs108/rfid/locate-handler.ts`
**Event Code**: 0x8100 (INVENTORY_TAG)
**Required Mode**: LOCATE

#### Purpose
Provides real-time RSSI tracking for tag location functionality with signal smoothing.

#### Target EPC Filtering
- Only processes tags matching the configured target EPC
- Target EPC set via `setTargetEpc(epc: string)` method
- Ignores all other tags to focus on location tracking

#### RSSI Smoothing
Uses ring buffer for weighted RSSI averaging:
- Configurable buffer size (default: 5 readings)
- More recent readings have higher weight
- Reduces signal noise for smoother location tracking

#### Real-time Processing
- No batching - immediate event emission
- Optimized for responsiveness over efficiency
- Each valid reading triggers immediate domain event

#### Domain Events
- `LOCATE_UPDATE` - Real-time locate data
  ```typescript
  {
    type: 'LOCATE_UPDATE',
    payload: {
      epc: string;        // Target EPC being located
      rssi: number;       // Smoothed RSSI value
      rawRssi: number;    // Original RSSI reading
      timestamp: number;  // Reading timestamp
    }
  }
  ```

## Barcode Handlers

### BarcodeDataHandler
**File**: `src/worker/cs108/barcode/scan-handler.ts`
**Event Code**: 0x9100 (BARCODE_DATA)
**Required Mode**: BARCODE

#### Purpose
Processes barcode scan data with symbology identification and duplicate detection.

#### Symbology Mapping
Maps numeric symbology codes to human-readable names:
- 0x08 - "UPC-A"
- 0x09 - "UPC-E"
- 0x0C - "Code 39"
- 0x15 - "QR Code"
- etc. (46 total symbology types supported)

#### Duplicate Detection
- Tracks recent scans within configurable time window
- Prevents duplicate emissions for rapid re-scans
- Configurable window (default: 1 second)

#### Domain Events
- `BARCODE_SCAN` - Barcode scan data
  ```typescript
  {
    type: 'BARCODE_SCAN',
    payload: {
      data: string;       // Barcode content
      symbology: string;  // Human-readable symbology name
      timestamp: number;  // Scan timestamp
    }
  }
  ```

### BarcodeGoodReadHandler
**File**: `src/worker/cs108/barcode/scan-handler.ts`
**Event Code**: 0x9101 (BARCODE_GOOD_READ)
**Required Mode**: BARCODE

#### Purpose
Handles barcode scan confirmation signals for UI feedback.

#### Confirmation Tracking
- Increments confirmation counter for each good read
- Provides feedback for successful scan operations
- Used for audio/visual scan confirmations

#### Domain Events
- `BARCODE_GOOD_READ` - Scan confirmation
  ```typescript
  {
    type: 'BARCODE_GOOD_READ',
    payload: {
      confirmationNumber: number;  // Incremental confirmation ID
    }
  }
  ```

## Handler Registration Patterns

### Single Handler Registration
```typescript
this.router.register(EventCodes.BATTERY_VOLTAGE, new BatteryHandler());
```

### Factory-Based Registration
```typescript
const inventoryHandler = createInventoryHandler(this.emitDomainEvent);
this.router.register(EventCodes.INVENTORY_TAG, inventoryHandler);
```

### Multi-Handler Registration
```typescript
// Both inventory and locate handlers listen to same event code
this.router.register(EventCodes.INVENTORY_TAG, inventoryHandler);
this.router.register(EventCodes.INVENTORY_TAG, locateHandler);
```

The router will try each handler until one accepts the packet via `canHandle()`.

## Testing Patterns

### Handler Unit Testing
```typescript
describe('BatteryHandler', () => {
  let handler: BatteryHandler;
  let context: NotificationContext;
  let emitDomainEvent: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    handler = new BatteryHandler();
    emitDomainEvent = vi.fn();
    context = {
      currentMode: ReaderMode.IDLE,
      readerState: ReaderState.CONNECTED,
      emitDomainEvent,
      metadata: {},
    };
  });

  it('should emit battery update', () => {
    const packet = createBatteryPacket(3700); // Helper function

    handler.handle(packet, context);

    expect(emitDomainEvent).toHaveBeenCalledWith({
      type: 'BATTERY_UPDATE',
      payload: { percentage: 58, voltage: 3700 },
      timestamp: expect.any(Number),
    });
  });
});
```

### Integration Testing
```typescript
describe('Notification System Integration', () => {
  let manager: NotificationManager;
  let emittedEvents: DomainEvent[] = [];

  beforeEach(() => {
    manager = new NotificationManager(
      (event) => emittedEvents.push(event),
      config
    );
  });

  it('should process complete notification flow', () => {
    const packet = createInventoryPacket();

    manager.handleNotification(packet);

    expect(emittedEvents).toHaveLength(1);
    expect(emittedEvents[0].type).toBe('TAG_BATCH');
  });
});
```

## Error Handling

### Handler Error Boundaries
The router provides error boundaries for each handler:
- Errors in `canHandle()` are caught and logged
- Errors in `handle()` are caught and passed to error callback
- Failed handlers don't prevent other handlers from processing

### Cleanup Error Handling
```typescript
unregister(eventCode: number): void {
  const handlers = this.handlers.get(eventCode);
  if (handlers) {
    for (const handler of handlers) {
      if (handler.cleanup) {
        try {
          handler.cleanup();
        } catch (error) {
          console.error('Error during handler cleanup:', error);
        }
      }
    }
  }
}
```

## Performance Considerations

### Handler Selection Efficiency
- Handlers are grouped by event code for O(1) lookup
- `canHandle()` methods should return quickly
- Context checks should be first (fastest to evaluate)

### Memory Management
- Handlers with state (batchers, buffers) must implement cleanup
- Factory pattern prevents memory leaks in callback bindings
- Router automatically calls cleanup when handlers are unregistered

### Batching Strategy Tuning
Different batching configs optimize for different scenarios:
- **Inventory**: Larger batches (50 tags/100ms) for efficiency
- **Locate**: Smaller batches (5 tags/50ms) for responsiveness
- **Debug**: Immediate flush for real-time debugging

## Extension Guidelines

### Adding New Handlers
1. Implement `NotificationHandler` interface
2. Add comprehensive unit tests
3. Register in `NotificationManager.registerHandlers()`
4. Update this documentation
5. Consider batching needs for high-volume events

### Handler Best Practices
- Keep `canHandle()` fast and deterministic
- Use context to make smart filtering decisions
- Implement proper cleanup for stateful handlers
- Emit structured domain events with consistent payloads
- Add debug logging when `context.metadata.debug` is true