# Protocol vs Application Concepts

## Overview
This document clarifies the important distinction between **device-level protocol concepts** and **application-level abstractions** in the CS108 system. Understanding this separation is crucial for working with the notification system and avoiding confusion about how RFID operations actually work.

## Key Principle
> **The CS108 device doesn't know about our application modes - it only understands protocol commands and register settings.**

## Protocol Level (CS108 Hardware)

### What the Device Actually Does
The CS108 hardware has a limited set of actual capabilities:

#### RFID Operations
- **Inventory Command** (0x8100) - The only RFID read operation
- **Register Configuration** - Power, antenna, session, algorithm settings
- **EPC Filtering** - Can filter reads by EPC pattern/mask
- **Power Management** - RFID module on/off states

#### Barcode Operations
- **Scan Command** (0x9100) - Single barcode scan operation
- **Power Management** - Barcode module on/off states

#### System Operations
- **Battery Monitoring** - Voltage readings and autonomous reporting
- **Trigger Events** - Button press/release detection
- **Error Reporting** - Device-level error conditions

### Protocol Event Codes
These are the actual events the hardware generates:

```typescript
// System events
0xA000 - Battery voltage (both command response and notification)
0xA001 - Trigger state query response
0xA102 - Trigger pressed notification
0xA103 - Trigger released notification
0xA101 - Error notification

// RFID events
0x8100 - Inventory tag data (the ONLY RFID read event)
0x8000 - RFID power on response
0x8001 - RFID power off response
0x8002 - Configuration responses

// Barcode events
0x9100 - Barcode data
0x9101 - Barcode good read confirmation
0x9000 - Barcode power on response
0x9001 - Barcode power off response
```

### Critical Protocol Facts

#### RFID: Only One Read Operation
- **All RFID tag reading uses inventory commands**
- There is no separate "locate" command at the protocol level
- The device doesn't distinguish between "inventory" and "locate" modes
- EPC filtering is just a register setting, not a mode

#### Event Code Reuse
- **0xA000** serves dual purpose: command response + autonomous notification
- **0x8100** is used for all tag reads regardless of application intent
- Context determines meaning, not the event code itself

#### Register-Driven Behavior
Device behavior is controlled by register settings, not "modes":
- **RF Power** - Signal strength settings
- **Session** - Tag population algorithm
- **Antenna** - Which antenna to use
- **EPC Mask/Filter** - Which tags to report

## Application Level (Our Abstractions)

### What We Create for User Experience
Our application creates semantic layers on top of the protocol:

#### Reader Modes (Our Invention)
```typescript
enum ReaderMode {
  IDLE = 0,      // No operations
  INVENTORY = 1, // "Read all tags"
  LOCATE = 2,    // "Track one specific tag"
  BARCODE = 3,   // "Scan barcodes"
}
```

These modes are **application concepts** - the device only sees register changes and commands.

#### Mode Mapping to Protocol

##### IDLE Mode
**Protocol Actions:**
- RFID power off (0x8001)
- Barcode power off (0x9001)
- Start battery reporting (0xA002)

**Result:** Device is powered down but monitoring battery and trigger

##### INVENTORY Mode
**Application Intent:** "Read all RFID tags in range"
**Protocol Actions:**
- RFID power on (0x8000)
- Configure registers (power, antenna, session)
- Clear EPC filters (read all tags)
- Start inventory (0x8100 stream begins)

**Result:** Device reports all tags it can read

##### LOCATE Mode
**Application Intent:** "Track signal strength of one specific tag"
**Protocol Actions:**
- RFID power on (0x8000)
- Configure registers (possibly different power/session)
- Set EPC filter to target tag
- Start inventory (0x8100 stream begins, filtered)

**Result:** Device only reports the target tag (if present)

##### BARCODE Mode
**Application Intent:** "Scan barcodes"
**Protocol Actions:**
- Barcode power on (0x9000)
- RFID power off (0x8001)
- Configure barcode settings

**Result:** Device can scan barcodes, reports via 0x9100/0x9101

## Handler System and Protocol Abstraction

### How Handlers Bridge the Gap
The notification handler system bridges protocol and application levels:

#### Context-Driven Processing
Handlers receive `NotificationContext` which includes our application state:
```typescript
interface NotificationContext {
  currentMode: ReaderMode;    // OUR application mode
  readerState: ReaderState;   // OUR connection state
  emitDomainEvent: Function;  // OUR event system
  metadata: any;              // OUR debug/config data
}
```

#### Mode-Aware Handler Logic
```typescript
// InventoryHandler
canHandle(packet: CS108Packet, context: NotificationContext): boolean {
  // Check OUR mode, not the device's state
  if (context.currentMode !== ReaderMode.INVENTORY) {
    return false;
  }
  // The packet is always 0x8100 inventory data
  return packet.event.eventCode === 0x8100;
}

// LocateHandler
canHandle(packet: CS108Packet, context: NotificationContext): boolean {
  // Same protocol event, different application context
  if (context.currentMode !== ReaderMode.LOCATE) {
    return false;
  }
  // Also 0x8100, but we filter by target EPC
  return packet.event.eventCode === 0x8100;
}
```

### Multi-Handler Pattern for Same Protocol Events
Because the same protocol event serves different application purposes:

```typescript
// Both handlers register for the same event code
router.register(0x8100, inventoryHandler);  // For INVENTORY mode
router.register(0x8100, locateHandler);     // For LOCATE mode

// Router tries each handler until one accepts via canHandle()
```

## Event Code Multiplexing Examples

### 0xA000 - Battery Voltage
**Protocol Reality:** Single event code for battery data
**Application Uses:**
1. **Command Response** - Reply to GET_BATTERY_VOLTAGE command
2. **Autonomous Notification** - Periodic updates after START_BATTERY_REPORTING

**Handler Logic:**
- Same handler processes both cases
- Context doesn't distinguish (both are just battery updates)
- Application treats them identically

### 0x8100 - Inventory Tag Data
**Protocol Reality:** Single event code for all tag reads
**Application Uses:**
1. **Inventory Mode** - Batched processing of all tags
2. **Locate Mode** - Real-time tracking of target tag

**Handler Logic:**
- Different handlers for different modes
- `InventoryHandler` batches tags for efficiency
- `LocateHandler` processes individual readings for responsiveness
- Same protocol data, completely different processing

### 0xA001 vs 0xA102/0xA103 - Trigger Events
**Protocol Reality:** Different event codes for different trigger scenarios
**Application Use:** Single trigger state concept

**Handler Logic:**
- Multiple handlers for different event codes
- All emit same domain event type: `TRIGGER_STATE_CHANGED`
- Application sees unified trigger state, not protocol distinctions

## Configuration Translation

### Application Settings → Protocol Registers
When users change application settings, we translate to protocol commands:

#### Power Level Setting
**User sees:** "Power Level: High/Medium/Low"
**Protocol reality:** SET_RF_POWER command with numeric value (0-30)

#### Locate Target Selection
**User sees:** "Locate tag: E280123456789ABC"
**Protocol reality:** SET_EPC_FILTER with mask/pattern registers

#### Inventory Speed Setting
**User sees:** "Fast/Accurate inventory"
**Protocol reality:** Different SESSION and ALGORITHM register values

## Testing Implications

### Protocol-Level Testing
Tests that verify actual hardware communication:
- Command/response cycles work correctly
- Packet parsing handles real device data
- Register settings produce expected behavior

### Application-Level Testing
Tests that verify our abstractions work:
- Mode transitions produce correct protocol sequences
- Handlers process events according to application context
- Domain events match user expectations

### Integration Testing
Tests that verify the bridge between levels:
- Application modes produce expected device behavior
- Device events are correctly interpreted by application
- Context switching works correctly

## Common Pitfalls

### ❌ Wrong: "Device has inventory and locate modes"
The device only has inventory commands with different filter settings.

### ❌ Wrong: "0x8100 means different things"
The event always means "inventory tag data". Our handlers interpret it differently.

### ❌ Wrong: "We need separate locate commands"
We use the same inventory commands with EPC filtering.

### ✅ Correct: "We create application modes that use inventory differently"
Our modes are abstractions that configure and interpret the same protocol operations.

### ✅ Correct: "Handlers use context to process the same events differently"
Same protocol events, different application-level processing based on current mode.

### ✅ Correct: "Event codes are multiplexed based on application state"
Router + context system allows same protocol events to serve multiple application purposes.

## Architecture Benefits

### Separation of Concerns
- **Protocol layer** handles hardware communication
- **Application layer** provides user-friendly abstractions
- **Handler system** bridges between layers

### Extensibility
- New application modes don't require protocol changes
- Same protocol events can serve additional purposes
- Handler system scales to complex application needs

### Maintainability
- Protocol changes don't break application logic
- Application features don't complicate protocol code
- Clear boundaries between hardware and software concerns

### Testability
- Protocol behavior can be tested independently
- Application logic can be tested with mock protocol events
- Integration tests verify the bridge between layers

## Summary

The key insight is that **the CS108 device is a simple RFID/barcode reader with limited protocol commands**. Our application creates rich user experiences by:

1. **Abstracting protocol complexity** into user-friendly modes
2. **Multiplexing protocol events** based on application context
3. **Translating user actions** into appropriate protocol sequences
4. **Interpreting device events** according to current application state

This separation allows us to build sophisticated features while keeping the protocol layer simple and reliable.