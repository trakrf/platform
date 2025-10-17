# Worker Tech Debt Tracking

This document tracks technical debt in the worker implementation that needs to be addressed.

## 🚨 Priority 1: Type Safety (✅ COMPLETED)

### Current Issue
Multiple uses of `any` type throughout the packet processing pipeline, particularly in:
- ~~`CS108Packet.payload?: any` in src/worker/cs108/type.ts~~
- ~~`CS108Event.parser?: (data: Uint8Array) => any` in src/worker/cs108/type.ts~~
- ~~Parser functions returning untyped payloads~~

### Solution (✅ COMPLETED)
1. ✅ Created RFID packet intermediate types (`src/worker/cs108/rfid/packet-types.ts`)
2. ✅ Created unified payload types (`src/worker/cs108/payload-types.ts`)
3. ✅ Updated CS108Event interface to use generics
4. ✅ Updated CS108Packet to use typed payload union
5. ✅ Updated all parsers to return typed payloads
6. ✅ Updated notification handlers to use typed payloads
7. ✅ Added battery voltage constants instead of magic numbers
8. ✅ Fixed all related tests

## 📋 Priority 2: Missing Test Coverage (✅ PARTIALLY COMPLETED)

### Completed Test Coverage
- ✅ `src/worker/DeviceManager.ts` - Full lifecycle and worker management tests
- ✅ `src/worker/cs108/reader.ts` - CS108Reader state management and mode transitions
- ✅ `src/worker/cs108/command.ts` - CommandManager execution, abort, and retry logic
- ✅ `src/worker/cs108/notification/manager.ts` - NotificationManager and router tests
- ✅ `src/worker/cs108-worker.ts` - Worker API tests updated for new initialize() interface
- ✅ Integration tests fixed and passing (31 tests)
- ✅ All unit tests passing (225 tests)

### Remaining Test Gaps
- `src/worker/BaseReader.ts` - Core reader abstraction
- `src/worker/cs108/sequence.ts` - Sequence management
- All barcode handlers (`src/worker/cs108/barcode/scan-handler.ts`)
- All trigger handlers (`src/worker/cs108/system/trigger.ts`)

### Action Items
- Add unit tests for BaseReader lifecycle
- Add tests for barcode parsing and handling
- Add tests for trigger state handling

## 🔧 Priority 3: Incomplete RFID Implementation

### TODO Comments in Code
```typescript
// src/worker/cs108/reader.ts
- Line 308: TODO: Validate settings based on state/mode
- Line 309: TODO: Map RAIN RFID settings to CS108 commands
- Line 334: TODO: Implement RFID inventory start
- Line 384: TODO: Implement RFID inventory stop

// src/worker/cs108/event.ts
- Multiple TODOs for RFID register write payload builders

// src/worker/cs108/sequence.ts
- Line 57: TODO: TECH DEBT - START_TRIGGER_REPORTING not responding
```

### Action Items
- Implement RFID inventory start/stop commands
- Create RFID register write payload builders
- Investigate START_TRIGGER_REPORTING issue with vendor

## 🐛 Priority 4: Error Handling & Recovery

### Issues
1. **Mixed error handling patterns** - Some async functions don't handle errors consistently
2. **Complex emitDomainEvent error handling** - BaseReader has complex try/catch for worker vs jsdom contexts
3. **No packet stream recovery** - Missing mechanism to recover from corrupted packet streams
4. **Fragment timeout hardcoded** - 200ms timeout not configurable

### Action Items
- Standardize error handling patterns across async functions
- Simplify emitDomainEvent implementation
- Add packet stream recovery mechanism
- Make fragment timeout configurable

## 🏗️ Priority 5: Event System Migration

### Current State
- Still using old `DomainEvent` pattern in some places
- Mix of `emitDomainEvent` and direct `postMessage` calls
- Inconsistent event typing

### Action Items
- Complete migration to typed events system
- Remove DomainEvent interface
- Standardize on direct postMessage with typed events

## 🧹 Priority 6: Handler Cleanup

### Issues
- Some handlers don't implement cleanup() method
- Memory leaks possible if handlers aren't cleaned up
- Fragment timeouts not always cleared

### Action Items
- Add cleanup() to all handlers that maintain state
- Ensure all timeouts are cleared
- Add tests for cleanup behavior

## 📊 Priority 7: Packet Handler Improvements

### Current Issues
1. **Unknown event codes** - Still discovering new event codes (enhanced error now shows full packet)
2. **Fragment timeout hardcoded** - Should be configurable
3. **No retry mechanism** - Failed packets are just dropped

### Action Items
- Document all known event codes
- Make fragment timeout configurable
- Add optional retry mechanism for failed packets

## 🔍 Known Bugs

### START_TRIGGER_REPORTING Command Issue
- Command 0xA008 documented in CS108 spec v1.43+ but hardware doesn't respond
- May only work over direct Bluetooth (not WebSocket bridge)
- Needs vendor escalation

### Packet Validation Errors
- Enhanced error message now shows full packet dump for unknown event codes
- Need to catalog these unknown codes as they appear

## 📝 Documentation Needs

### Missing Documentation
- Worker lifecycle and state machine
- Packet flow from BLE to handlers
- Command sequence execution
- Error recovery strategies

### Action Items
- Create WORKER-ARCHITECTURE.md
- Document packet processing pipeline
- Document command/response flow
- Create troubleshooting guide

## ✅ Completed Items

### Recently Completed
- ✅ Enhanced packet validation error to show full packet dump
- ✅ Removed obsolete worker-old directory
- ✅ Created RFID packet type definitions
- ✅ Created unified payload type definitions
- ✅ Simplified payload types (removed unnecessary wrappers)

## 📅 Recommended Order

1. **Complete type safety improvements** (in progress)
2. **Add critical test coverage** (BaseReader, CS108Reader, CommandManager)
3. **Implement RFID functionality** (needed for full feature set)
4. **Standardize error handling**
5. **Complete event system migration**
6. **Clean up handlers**
7. **Improve packet handler**
8. **Create documentation**