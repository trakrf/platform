# CS108 Notification System

## Overview
The CS108 notification system uses a **handler-based architecture** to process autonomous notifications from CS108 hardware. This replaces simple switch statements with an extensible, testable system that handles complex notification scenarios like batching, rate limiting, and mode-aware processing.

## Architecture

### Handler Pattern
Each notification type has a dedicated handler implementing the `NotificationHandler` interface:

```typescript
interface NotificationHandler {
  canHandle(packet: CS108Packet, context: NotificationContext): boolean;
  handle(packet: CS108Packet, context: NotificationContext): void;
  cleanup?(): void;
}
```

### NotificationRouter
Central routing system with key features:
- **Multi-handler support** - Multiple handlers can register for the same event code
- **Context-aware processing** - Handlers receive current mode/state information
- **Error boundaries** - Graceful failure handling with custom error callbacks
- **Mode-based filtering** - Handlers use `canHandle()` to determine packet relevance

### NotificationManager
Simplified manager that replaces the old massive switch statement with a clean 5-line delegation:

```typescript
handleNotification(packet: CS108Packet): void {
  this.router.handleNotification(packet);
}
```

## Event Code Handling

### Dual-Purpose Event Codes
The CS108 protocol reuses event codes for multiple purposes:

- **0xA000** - Battery voltage (command responses + autonomous notifications)
- **0x8100** - RFID inventory (inventory mode + locate mode)
- **0xA001** - Trigger state (command responses only)
- **0xA102/0xA103** - Trigger press/release (autonomous notifications)

### Protocol vs Application Concepts
- **Device Level**: All RFID operations use inventory functionality (0x8100)
- **Application Level**: We create "inventory" vs "locate" modes as abstractions
- **Context-Driven Logic**: Handlers use reader mode to determine processing behavior

## Notification Handlers

### System Handlers (`src/worker/cs108/system/`)

#### BatteryHandler
- **Event**: 0xA000 (GET_BATTERY_VOLTAGE)
- **Purpose**: Convert voltage to percentage and emit BATTERY_UPDATE events
- **Features**: Linear voltage mapping (3000-4200mV → 0-100%)

#### TriggerHandler (Base + Derived Classes)
- **Events**: 0xA102 (TRIGGER_PRESSED), 0xA103 (TRIGGER_RELEASED), 0xA001 (TRIGGER_STATE)
- **Purpose**: Handle trigger button events and state queries
- **Pattern**: Inheritance pattern with shared base logic

#### ErrorNotificationHandler
- **Event**: 0xA101 (ERROR_NOTIFICATION)
- **Purpose**: Process device errors with rate limiting
- **Features**: Prevents log spam for repeated errors

### RFID Handlers (`src/worker/cs108/rfid/`)

#### InventoryHandler + InventoryBatcher
- **Event**: 0x8100 (INVENTORY_TAG)
- **Mode**: INVENTORY
- **Purpose**: Batch tag reads for efficient UI updates
- **Features**:
  - Configurable time/size triggers (default: 50 tags or 100ms)
  - Tag deduplication with best RSSI tracking
  - Factory pattern for proper callback binding

#### LocateHandler
- **Event**: 0x8100 (INVENTORY_TAG)
- **Mode**: LOCATE
- **Purpose**: Real-time RSSI tracking for tag location
- **Features**:
  - Target EPC filtering
  - Ring buffer RSSI smoothing
  - Immediate (no batching) updates

### Barcode Handlers (`src/worker/cs108/barcode/`)

#### BarcodeDataHandler + BarcodeGoodReadHandler
- **Events**: 0x9100 (BARCODE_DATA), 0x9101 (BARCODE_GOOD_READ)
- **Purpose**: Process barcode scan data and confirmations
- **Features**: Symbology mapping, duplicate detection, mode filtering

## Batching System

### InventoryBatcher
Configurable batching with multiple triggers:

```typescript
interface BatchingConfig {
  maxSize: number;                    // Flush after N tags
  timeWindowMs: number;               // Flush every N milliseconds
  flushOnModeChange: boolean;         // Flush when switching modes
  deduplicationWindowMs: number;      // Dedupe tags within N ms
}
```

**Presets**:
- **Inventory Mode**: 50 tags / 100ms (efficiency focused)
- **Locate Mode**: 5 tags / 50ms (responsiveness focused)

### Tag Deduplication
- Groups by EPC identifier
- Tracks read count per tag
- Keeps best (highest) RSSI value
- Updates timestamp on each read

## Context System

### NotificationContext
Provides handlers with current system state:

```typescript
interface NotificationContext {
  currentMode: ReaderMode;           // IDLE, INVENTORY, LOCATE, BARCODE
  readerState: ReaderState;          // DISCONNECTED, CONNECTING, READY, etc.
  emitDomainEvent: (event) => void;  // Domain event emission
  metadata: Record<string, any>;     // Debug flags, etc.
}
```

### Mode-Aware Processing
Handlers check context to determine behavior:
- **Inventory tags**: Only process in INVENTORY mode
- **Locate tags**: Only process in LOCATE mode for target EPC
- **Barcode data**: Only process in BARCODE mode

## Testing

### Comprehensive Test Coverage
- **65 tests total** across the notification system
- **Unit tests**: Individual handler logic and batching
- **Integration tests**: End-to-end notification flow
- **Router tests**: Multi-handler registration and error handling

### Test Structure
```
src/worker/cs108/
├── notification/
│   ├── router.test.ts           # Router functionality
│   └── integration.test.ts      # Complete system tests
├── system/
│   └── battery.test.ts          # Battery handler tests
└── rfid/
    └── inventory-batcher.test.ts # Batching logic tests
```

### Hardware Integration
All tests pass with real CS108 hardware via the bridge server, ensuring the system works with actual device firmware behavior.

## Factory Pattern

### Context Binding Solution
The factory pattern solves the challenge of binding domain event emitters to flush callbacks:

```typescript
export function createInventoryHandler(
  emitDomainEvent: (event: any) => void
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

## Migration Benefits

### From Switch Statement to Handlers
- **Eliminated** repetitive switch case code
- **Added** extensibility for new notification types
- **Improved** testability with isolated handler logic
- **Enhanced** error handling with boundaries per handler
- **Enabled** complex processing (batching, filtering, smoothing)

### Architecture Improvements
- **Separation of concerns** between protocol and business logic
- **Context-driven processing** instead of hardcoded conditions
- **Configurable strategies** for different operational modes
- **Comprehensive error handling** with graceful degradation
- **Factory patterns** for proper dependency injection