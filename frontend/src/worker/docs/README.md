# CS108 Worker Documentation

## Quick Start

This directory contains the documentation for the CS108 RFID reader worker implementation. The worker handles all communication with the CS108 hardware via Web Bluetooth, providing a clean interface to the main thread.

### Key Documents

- **[Architecture](./ARCHITECTURE.md)** - System design, layers, and data flow
- **[Implementation Guide](./IMPLEMENTATION.md)** - How to build and extend the worker
- **[Testing Guide](./TESTING.md)** - Test strategy and harness usage
- **[Progress Tracker](./PROGRESS.md)** - Current status and roadmap

## Overview

### What is this?
A complete refactor of the RFID reader web worker to replace 10k lines of spaghetti code with a clean, modular, tested implementation based on proven hardware patterns.

### Core Principles
1. **Zero byte manipulation outside parser** - Type-safe CS108Packet objects everywhere
2. **Idle-first transitions** - Always return to IDLE before mode changes
3. **Pure domain events** - No direct store coupling in worker
4. **Single source of truth** - All constants in `cs108/constants.ts`
5. **Integration-first testing** - Test with real hardware via bridge server

### Architecture Stack

```
┌─────────────────┐
│   UI Components │  (React)
├─────────────────┤
│  Zustand Stores │  (State Management)
├─────────────────┤
│  DeviceManager  │  (Main Thread Bridge)
├─────────────────┤
│   Worker RPC    │  (Comlink)
├─────────────────┤
│  CS108Reader    │  (Worker Thread)
├─────────────────┤
│  BLE Transport  │  (Web Bluetooth)
├─────────────────┤
│  CS108 Hardware │  (Physical Device)
└─────────────────┘
```

## Quick Commands

```bash
# Run integration tests with real hardware
pnpm test:integration

# Monitor BLE traffic (requires MCP setup)
mcp__ble-mcp-test__get_logs --since=30s

# Run specific test
pnpm test cs108-instantiation

# Check implementation progress
cat src/worker/docs/PROGRESS.md
```

## File Structure

```
src/worker/
├── docs/           # This documentation
├── cs108/          # Clean implementation
│   ├── reader.ts   # Main CS108Reader class
│   ├── packets.ts  # Packet parser and builder
│   ├── events.ts   # Event definitions
│   ├── constants.ts # Single source of truth
│   └── types.ts    # TypeScript types
├── types/          # Shared types
│   └── reader.ts   # Reader state and mode enums
└── BaseReader.ts   # Abstract base class
```

## Key Concepts

### CS108Packet Type
All packet data is parsed once into strongly-typed objects:
```typescript
interface CS108Packet {
  eventCode: number;      // Command/response identifier
  event: CS108Event;      // Typed event definition
  payload?: any;          // Pre-parsed data
  isComplete: boolean;    // Fragmentation status
}
```

### Domain Events
Worker emits pure events, DeviceManager routes to stores:
```typescript
// Worker emits
postMessage({ type: 'TAG_READ', payload: { epc: '...', rssi: -45 } })

// DeviceManager routes
useTagStore.getState().addTag(payload)
```

### Mode Transitions
The worker automatically handles clean transitions through IDLE:
```typescript
// Client just requests the target mode
await reader.setMode(ReaderMode.BARCODE);

// Worker internally does: INVENTORY → IDLE → BARCODE
```

## Need Help?

1. Check the [Architecture](./ARCHITECTURE.md) for system design
2. See [Implementation Guide](./IMPLEMENTATION.md) for code examples
3. Review [Testing Guide](./TESTING.md) for test patterns
4. Look at existing tests in `tests/integration/cs108/`
5. Ask questions - this is complex hardware!