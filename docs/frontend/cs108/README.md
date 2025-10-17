# CS108 Protocol Implementation

## Overview

This directory contains documentation specific to the CS108 RFID reader protocol implementation. The CS108 is a dual-band (UHF/HF) RFID reader that supports both Bluetooth and USB communication.

## Quick Reference

### Common Operations

**Connect to Device**
```typescript
const deviceManager = new DeviceManager();
await deviceManager.connect();
```

**Start RFID Inventory**
```typescript
await rfidManager.startInventory();
```

**Read Single Tag**
```typescript
const tags = await rfidManager.readSingleTag();
```

**Scan Barcode**
```typescript
await barcodeManager.startScan();
```

### Protocol Basics

- **Command Prefix**: `0xA7B3` (host to device)
- **Response Prefix**: `0xB3A7` (device to host)
- **Module IDs**: RFID = 0x01, Barcode = 0x02
- **Transport Types**: BLE = 0xB3, USB = 0xE6

## Documentation Files

### Protocol Specifications

- **[`CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md`](./CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md)**: Complete vendor spec in markdown (for AI/programmatic reference)
- **[`CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.pdf`](./CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.pdf)**: Original vendor PDF (better formatting for human reading)

### Application Documentation

- **[`inventory-parsing.md`](./inventory-parsing.md)**: Quick reference for tag parsing logic

## Key Features

### RFID Operations
- **Inventory Scanning**: Continuous tag reading with configurable parameters
- **Tag Locating**: Find specific tags with signal strength indication
- **Power Control**: Adjustable RF power levels (0-30 dBm)
- **Session Management**: EPC Gen2 session control (S0, S1, S2, S3)
- **Frequency Hopping**: Automatic frequency management

### Barcode Operations
- **Multiple Scan Modes**: One-time, continuous, and trigger-based scanning
- **Various Formats**: Support for Code 128, Code 39, QR codes, and more
- **Configuration**: Adjustable scan parameters and behavior

### Transport Support
- **Bluetooth LE**: Primary connection method using Web Bluetooth API
- **USB HID**: Future support via WebHID API (see [`../USB-TRANSPORT.md`](../../prp/archive/spec/USB-TRANSPORT.md))
- **Unified Protocol**: Identical command/response structure regardless of transport

## Implementation Notes

### Protocol Compliance
- All values use hexadecimal format to match vendor specifications
- Commands follow CS108-specific packet structure with proper headers
- Automatic packet fragmentation handling for BLE transport
- Proper error handling and timeout management

### State Management
- Zustand stores provide centralized state management
- Direct store access from UI components
- No local state shadowing to ensure consistency
- Persistent settings with localStorage integration

### Error Handling
- Transport-level error detection and recovery
- Protocol error parsing and user-friendly messages
- Automatic reconnection with exponential backoff
- Comprehensive logging for debugging

## Getting Started

1. **Review Protocol**: Read this README for protocol basics
2. **Understand Parsing**: See [`inventory-parsing.md`](./inventory-parsing.md) for tag data handling
3. **Refer to Spec**: Use the PDF specification for detailed protocol information

## Related Documentation

- [`../ARCHITECTURE.md`](../ARCHITECTURE.md): Overall system architecture
- [`../USB-TRANSPORT.md`](../../prp/archive/spec/USB-TRANSPORT.md): USB implementation guide
- [`../../CLAUDE.md`](../../CLAUDE.md): Development guidelines and conventions