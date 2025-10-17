# CS108 Protocol Quirks and Implementation Knowledge

## Critical Implementation Patterns from Working Code

### 1. BLE Packet Fragmentation
- CS108 packets can fragment across multiple BLE notifications
- Must use RingBuffer (16MB like vendor implementation) to accumulate data
- Packets can arrive with continuation fragments that don't start with 0xA7
- Multiple complete packets can arrive in a single BLE notification
- CS108 headers are in the first eight bytes of of the packet
- Header byte 2 is the packet length excluding the header. So the full number of bytes from the device is length + 8
- Header bytes 6 and 7 contain the CRC which is computed for the payload, i.e. bytes 8:n

### 2. Command/Response Patterns

#### Working Command Sequence (from deviceManager.ts)
```typescript
// Baseline initialization sequence that works
1. GET_BATTERY_VOLTAGE (0xA000) - timeout: 2000ms
2. START_BATTERY_REPORTING (0xA001) - autonomous updates every 5s
3. GET_TRIGGER_STATE (0xA100) - timeout: 1000ms  
4. SET_TRIGGER_ABORT_MODE (0xA004) - enable: true
5. START_TRIGGER_REPORTING (0xA101) - interval: 1s
```

#### Inventory Preparation (from rfidManager.ts)
```typescript
// Register configuration sequence for inventory
1. ANT_PORT_SEL = PORT_DEFAULT
2. INV_SEL = SELECT_ALL
3. INV_CFG = COMPACT_MODE | SESSION_S1  
4. INV_ALGORITHM_SELECT = FIXED_Q
5. INV_Q_VALUE = Q_VALUE_4
6. TAG_FOCUS_ENABLE = false
7. HST_ANT_CYCLES = ANT_CYCLE_CONTINUOUS
8. HST_CMD = CMD_INVENTORY
```

### 3. Known Firmware Bugs

#### STOP_INVENTORY No Response
- STOP_INVENTORY (0x4004) command may not send a response
- Must handle timeout gracefully and continue
- Device stops inventory even without response

#### Battery Value 65535
- Battery value 0xFFFF (65535) indicates fault condition
- Should be handled as null/unknown battery level

#### START_TRIGGER_REPORTING No Response
- START_TRIGGER_REPORTING (0xA008) documented in spec v1.43+ but doesn't respond
- May only work over direct Bluetooth, not WebSocket bridge
- **TECH DEBT**: Escalate to vendor for clarification

#### Overloaded Event Codes
- Event code 0xA000 is used for both GET_BATTERY_VOLTAGE responses AND autonomous battery notifications
- Must handle both cases in routing logic
- Use isCommand and isNotification boolean flags to indicate dual-purpose events

#### Trigger/Battery Stop Commands
- STOP_BATTERY_REPORTING causes 5000ms timeout
- STOP_TRIGGER_REPORTING has firmware bug
- Both disabled in production code

### 4. Module Routing

#### Module IDs (byte[3] in packet)
```
0xC2 - RFID module
0x6A - Barcode module  
0xD9 - Notification module (battery, trigger)
0xE8 - Silicon Lab module
0x5F - Bluetooth module
```

### 5. Packet Structure Validation

#### CS108 Header Pattern
```
Bytes 0-1: 0xA7B3 (command) or 0xB3A7 (response)
Byte 2: Payload length (after 8-byte header)
Byte 3: Module ID
Byte 4: Sequence number (for inventory packets)
```

#### Abort Signature
- Firmware abort: [0x40, 0x03, 0xbf, 0xfc, 0xbf, 0xfc, 0xbf, 0xfc]
- Can appear at end of payload
- Packet is still valid but operation was aborted

### 6. Inventory Tag Parsing

#### Compact Mode (0x04 version)
- PC word (2 bytes) -> EPC (variable) -> RSSI (1 byte)
- EPC length from PC: ((pc & 0xf800) >> 11) * 2
- RSSI conversion: raw * -0.8 for dBm

#### Normal Mode (0x03 version)  
- More complex structure with metadata
- Tag data starts at offset 20
- Length calculation: ((pktLenWords - 3) * 4) - ((flags >> 6) & 3)

### 7. State Management

#### Reader States (from constants.ts)
```typescript
enum ReaderState {
  DISCONNECT = 0,
  CONNECTING = 1,
  CONFIGURING = 2,  
  IDLE = 4,
  SCANNING = 5,
  BUSY = 8,
  READYFORDISCONNECT = 10
}
```

#### Critical State Transitions
- Must be IDLE before starting inventory
- Set CONFIGURING during hardware init
- SCANNING during active inventory
- Minimum 1000ms between stop and next start

### 8. Error Handling

#### Exponential Backoff (from transportManager)
- Retry delays: [500ms, 1500ms, 5000ms]
- Max retries: 3
- Handle "GATT operation already in progress"

#### Command Queue
- Max queue length: 5 commands
- Process sequentially to avoid conflicts
- Clear queue on disconnect

### 9. Performance Optimizations

#### Tag Processing
- Batch updates with 100ms debounce
- Handle 100+ tags/second during inventory
- Use 16MB RingBuffer for high-volume data

#### Packet Processing
- Max 10ms per batch to avoid blocking
- Max 50 packets per batch
- Schedule remaining with setTimeout(0)

### 10. Transport Abstraction Requirements

#### MessagePort Pattern
```typescript
// Main thread creates channel
const channel = new MessageChannel();
// Port2 transferred to worker
worker.postMessage({ port: channel.port2 }, [channel.port2]);
// Port1 stays in transport for bidirectional communication
```

### 11. Special Command Handling

#### Commands Without Module Handler
- Battery voltage (0xA000) - handled directly in deviceManager
- Battery reporting (0xA001/0xA002) - autonomous updates
- Trigger state (0xA100) - updates Zustand store directly
- Trigger events (0xA102/0xA103) - press/release notifications

### 12. Browser Compatibility

#### Web Bluetooth Requirements
- Chrome 89+ or Edge 89+
- User permission required
- Handle "Device selection cancelled" gracefully

#### Characteristic Cleanup
- Must remove event listeners before disconnect
- Stop notifications before GATT disconnect
- Clear subscriptions to prevent memory leaks

## Implementation Checklist

These patterns MUST be preserved in the new architecture:

- [ ] RingBuffer with 16MB capacity for packet assembly
- [ ] Exponential backoff for BLE retries
- [ ] Command queue with sequential processing
- [ ] Baseline initialization sequence on connect
- [ ] Module-based packet routing
- [ ] Abort signature detection
- [ ] Compact/normal mode inventory parsing
- [ ] State machine with proper transitions
- [ ] Battery fault value handling (65535)
- [ ] STOP_INVENTORY timeout handling
- [ ] MessagePort for worker-transport communication
- [ ] Proper BLE cleanup on disconnect