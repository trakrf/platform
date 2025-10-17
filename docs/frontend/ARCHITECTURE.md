# TrakRF Handheld System Architecture

## Overview

TrakRF Handheld is a standalone React application for CS108 RFID handheld readers, extracted from a Next.js monolith. The architecture emphasizes modularity, type safety, and clean separation of concerns.

## Core Design Principles

1. **Module-Based Architecture**: Clear separation between transport, device management, and UI layers
2. **State Ownership Model**: Zustand stores as single source of truth, no local state shadowing
3. **Metadata-Driven Design**: Use constants and metadata over hardcoded values
4. **Protocol Abstraction**: Transport-agnostic command/response handling
5. **Type Safety**: Comprehensive TypeScript types with shared enums and constants

## System Layers

### 1. UI Layer (Main Thread)
- **React Components**: Pure UI components with no device knowledge
- **Complete Decoupling**: All device interaction through Zustand store actions
- **Command Pattern**: UI triggers store actions, never direct device calls

### 2. State Management Layer (Main Thread)
- **Pure UI State Stores**: Zustand stores containing ONLY UI-relevant state
  - `DeviceStore`: Connection status, battery level, reader state
  - `TagStore`: RFID tag collection and inventory status  
  - `SettingsStore`: User preferences and configuration
- **Command Actions**: Store methods that call DeviceManager for device operations
- **Event Subscriptions**: Stores receive updates from DeviceManager subscriptions

### 3. Worker Lifecycle Management (Main Thread)
- **DeviceManager**: Single responsibility for worker lifecycle
  - Creates workers only during `connect()`
  - Destroys workers immediately during `disconnect()`
  - Manages transport and event subscriptions
- **Guaranteed Single Worker**: Prevents multiple worker instances
- **Resource Efficiency**: Zero workers at startup, exact lifecycle control

### 4. Transport Layer (Main Thread)
- **BLE Transport**: Web Bluetooth API implementation with packet fragmentation
- **Bridge Transport**: WebSocket to ble-mcp-test for E2E testing
- **Mock Transport**: Simulated responses for unit testing
- **MessagePort Communication**: Transfers data channel to worker

### 5. Device Protocol Layer (Worker Thread)
- **CS108 Worker**: Handles all hardware protocol communication
- **Module Handlers**: RFID (0xC2), Barcode (0x6A), System (0xD9) protocol handlers
- **Packet Processing**: Command sequencing, response parsing, state management
- **Event Generation**: Battery updates, tag reads, trigger events sent to main thread

## Data Flow Patterns

### Command Flow (UI → Device)
```
UI Component → Store Action → DeviceManager → Worker → Transport → Hardware
```

### Event Flow (Device → UI)  
```
Hardware → Transport → Worker → Event Subscription → Store Update → UI Re-render
```

### Worker Lifecycle Flow
```
User Connect → DeviceManager.connect() → Creates Worker + Transport → Device Connected
User Disconnect → DeviceManager.disconnect() → Destroys Worker + Transport → Clean State
```

### State Synchronization
- Worker maintains authoritative device state
- DeviceManager subscriptions push events to stores
- UI components observe store changes via hooks
- No direct worker-to-UI communication

## Key Components

### DeviceManager
Central hub for device communication:
- Maintains transport connection
- Routes packets by module ID
- Manages device lifecycle events
- Coordinates module initialization

### Module Handlers
Protocol-specific implementations:
- **RFIDManager**: Inventory, tag processing, reader configuration
- **BarcodeManager**: Scanning modes, barcode data processing
- Each handler manages its own command creation and response parsing

### Transport Abstraction
- Uniform interface for different transport types (BLE, USB)
- Automatic packet fragmentation and reassembly
- Connection state management
- Error handling and reconnection logic

## Protocol Implementation

### CS108 Command Structure
- Commands: `0xA7B3` prefix
- Responses: `0xB3A7` prefix
- Module-based routing (RFID: 0x01, Barcode: 0x02, etc.)
- See `docs/cs108/` for detailed protocol documentation

### Packet Processing
1. Raw bytes received from transport
2. Packet assembly (handling BLE fragmentation)
3. Header parsing and validation
4. Module ID extraction and routing
5. Module-specific processing
6. Store updates and UI refresh

## Error Handling

### Transport Errors
- Connection failures
- Packet corruption
- Timeout handling
- Automatic reconnection with exponential backoff

### Protocol Errors
- Invalid command responses
- Malformed packets
- Module-specific error codes
- User-friendly error messages

## Performance Considerations

- **Debounced Tag Processing**: Prevent UI flooding during rapid inventory
- **React.memo**: Optimize expensive component re-renders
- **Selective Store Subscriptions**: Components subscribe only to needed state slices
- **Virtual Scrolling**: Handle large tag lists efficiently

## Testing Strategy

### Unit Tests
- Individual function and component testing
- Store action and selector tests
- Protocol parsing validation

### Integration Tests
- Cross-module communication
- Command/response flows
- State synchronization

### E2E Tests
- Full user workflows
- Device connection scenarios
- Data export functionality

## Future Enhancements

1. **USB Transport**: Direct USB connection support via WebHID
2. **Offline Mode**: Local storage and sync capabilities
3. **Multi-Device Support**: Connect to multiple readers simultaneously
4. **Advanced Filtering**: Complex tag search and filtering options

## Related Documentation

- `/docs/cs108/`: CS108 protocol-specific documentation
- `/docs/USB-TRANSPORT.md`: USB implementation guide
- `/docs/README.md`: Documentation index
- `/CLAUDE.md`: Development guidelines and conventions