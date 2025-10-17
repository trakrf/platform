# Type Safety Implementation Plan

This document outlines the plan for eliminating `any` types from the packet processing pipeline.

## 🎯 Goal

Replace all `any` types in the packet processing pipeline with proper TypeScript types, creating type safety from hardware packets through to UI events.

## 📐 Architecture Overview

```
Hardware → BLE → CS108Packet → Parser → TypedPayload → Handler → Event → UI
                  ↑             ↑         ↑
                  rawPayload    typed     reused types
                  (Uint8Array)  parser    (same as events)
```

## ✅ Completed Steps

### 1. Created RFID Packet Types (`src/worker/cs108/rfid/packet-types.ts`)
- Defined multi-layer RFID packet structure
- PacketVersion and PacketType enums
- Inventory packet interfaces (Compact, Normal modes)
- PC Word structure for EPC parsing
- Command packets (Begin, End, Cycle)
- Type guards for packet discrimination

### 2. Created Unified Payload Types (`src/worker/cs108/payload-types.ts`)
- ScalarPayload for simple numeric values
- Complex types only where needed:
  - ErrorPayload (code + message)
  - BarcodeDataPayload (symbology + data)
  - TagDataPayload (simplified RFID tag)
  - RFIDInventoryPayload (full packet structure)
  - CommandResponsePayload
- Type guards for runtime validation
- Parser type signatures

### 3. Design Decisions Made
- **YAGNI principle applied**: No unnecessary wrapper types
- **Battery voltage → percentage only**: Calculation at parse time
- **Trigger state → scalar**: Just 0 or 1, context from event code
- **Reuse event payload types**: Same types from packet to UI

## 🚧 Next Steps

### Step 1: Update CS108Event Interface
```typescript
// Current (with any)
interface CS108Event {
  parser?: (data: Uint8Array) => any;
}

// Target (with generics)
interface CS108Event<T = CS108PayloadType> {
  parser?: PayloadParser<T>;
}
```

### Step 2: Update CS108Packet Interface
```typescript
// Current (with any)
interface CS108Packet {
  rawPayload: Uint8Array;
  payload?: any;
}

// Target (with typed union)
interface CS108Packet {
  rawPayload: Uint8Array;
  payload?: CS108PayloadType;  // Typed union
}
```

### Step 3: Update Event Definitions
```typescript
// Example: Battery event
export const BATTERY_VOLTAGE: CS108Event<ScalarPayload> = {
  name: 'BATTERY_VOLTAGE',
  eventCode: 0xA000,
  parser: parseBatteryPercentage,  // Returns number (0-100)
  // ...
};
```

### Step 4: Update Parser Implementations
```typescript
// Current
export const parseBatteryVoltage = (data: Uint8Array): { voltage: number; percentage: number } => {
  const voltage = parseUint16LE(data);
  const percentage = Math.max(0, Math.min(100, ((voltage - 3000) / 1200) * 100));
  return { voltage, percentage };
};

// Target (return just percentage)
export const parseBatteryPercentage: ScalarParser = (data: Uint8Array): number => {
  const voltage = parseUint16LE(data);
  return Math.max(0, Math.min(100, ((voltage - 3000) / 1200) * 100));
};
```

### Step 5: Update Notification Handlers
```typescript
// Current (casting to any)
const payload = packet.payload as ParsedBarcodePayload;

// Target (type-safe with guards)
if (isBarcodeDataPayload(packet.payload)) {
  const payload = packet.payload;  // TypeScript knows the type
  // ...
}
```

### Step 6: Update Handler Context
```typescript
// Remove DomainEvent, use typed events
interface NotificationContext {
  currentMode: ReaderModeType;
  readerState: ReaderStateType;
  reader: IReader;  // Direct reader reference instead of emitDomainEvent
  metadata?: Record<string, unknown>;
}
```

## 📊 Type Mapping

| Event Code | Event Name | Current Type | New Type |
|------------|------------|--------------|----------|
| 0xA000 | BATTERY_VOLTAGE | `{ voltage, percentage }` | `ScalarPayload` (percentage only) |
| 0xA001 | TRIGGER_STATE | `number` | `ScalarPayload` (0 or 1) |
| 0xA002 | TRIGGER_PRESSED | `undefined` | `null` |
| 0xA003 | TRIGGER_RELEASED | `undefined` | `null` |
| 0xA0FF | ERROR_NOTIFICATION | `number` | `ErrorPayload` |
| 0x9100 | BARCODE_DATA | `any` | `BarcodeDataPayload` |
| 0x8100 | INVENTORY_TAG | `any` | `RFIDInventoryPayload` → `TagDataPayload` |

## 🔍 Testing Strategy

1. **Type checking**: `pnpm typecheck` should pass with no `any` types
2. **Runtime validation**: Type guards ensure data integrity
3. **Unit tests**: Each parser tested with known inputs/outputs
4. **Integration tests**: Full packet flow with typed payloads
5. **E2E tests**: Verify events reach UI with correct types

## 📈 Success Metrics

- [ ] Zero `any` types in packet processing pipeline
- [ ] All parsers return typed payloads
- [ ] All handlers use type guards
- [ ] TypeScript catches payload mismatches at compile time
- [ ] No runtime type errors in production

## 🚀 Benefits

1. **Compile-time safety**: TypeScript catches errors before runtime
2. **Better IntelliSense**: IDE knows exact payload shapes
3. **Self-documenting**: Types serve as documentation
4. **Easier refactoring**: Change types once, TypeScript finds all usages
5. **Runtime validation**: Type guards ensure data integrity

## 📝 Notes

- Keep `rawPayload` for debugging and future needs
- Event code provides semantic context for scalar values
- RFID packets have complex multi-layer structure (CS108 → Inventory → Tag data)
- Reuse same payload types for worker→main thread events where possible