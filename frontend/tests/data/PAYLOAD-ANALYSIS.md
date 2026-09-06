# CS108 Inventory Payload Analysis

## Real Payload Structures Found

### Type 0x02 Payloads (Status/Control)
- First 2 bytes: `0x02 0x00` or `0x02 0x01`
- Appear to be status or control messages
- Do not contain RFID tag data
- Example: `02 00 00 80 02 00 00 00 06 00 00 00 cb 0d 00 00`

### Type 0x70 Payloads (Keepalive/Status)
- First byte: `0x70`
- Short 8-byte payloads
- Likely keepalive or status messages
- Example: `70 00 06 07 2c 01 00 00`

### Type 0x04 Payloads (Compact Mode Tag Data)
- **Header Structure (16 bytes)**:
  - Bytes 0-1: Version `0x04 0x00` (compact mode marker)
  - Bytes 2-3: Type `0x05 0x80` (0x8005 in little-endian)
  - Bytes 4-7: Length in bytes (little-endian)
  - Bytes 8-15: Metadata (antenna port, etc.)

- **Tag Data Structure** (starting at byte 20):
  - Each tag appears to be 16 bytes:
    - RSSI/metadata (variable structure)
    - PC word (2 bytes)
    - EPC (12 bytes, often zeros in test data)
  - Tags can span packet boundaries

## Key Observations

1. **Test Environment**: The captured data shows EPCs with all zeros, suggesting:
   - Reader is in test/debug mode
   - No actual tags present during capture
   - RSSI values present but tag IDs are placeholders

2. **Packet Order Matters**: Tags can span multiple packets, so order preservation is critical

3. **Multiple Payload Types**: The 0x8100 notification contains different payload types that must be handled differently

## Implementation Notes

- Parser must handle all three payload types gracefully
- Compact mode (0x04) contains actual tag data
- Status payloads (0x02, 0x70) should be processed but yield no tags
- Buffer must accumulate partial tags across packet boundaries

## Test Data Files Created

1. `real-inventory-payloads.ts` - First 25 payloads as hex strings
2. `real-inventory-uint8.ts` - First 30 payloads as Uint8Arrays
3. `inventory-payloads.txt` - All 474 payloads as hex strings
4. `jq-magic.jq` - JQ filter to extract payloads from packet capture