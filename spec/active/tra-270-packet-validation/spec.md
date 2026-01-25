# Feature: CS108 Packet Validation - CRC and Length Checks

## Origin
This specification emerged from investigating TRA-270 (barcode/QR reads occasionally drop characters). Analysis revealed our packet assembly logic lacks critical validation that the vendor's C# implementation performs.

## Outcome
Barcode reads (and all CS108 uplink packets) will be validated for completeness and integrity before processing, preventing truncated or corrupted data from reaching the application layer.

## User Story
As a warehouse operator
I want barcode scans to always return complete, verified data
So that I can trust the scanned values and avoid data entry errors

## Context

### Discovery
Comparing our `PacketHandler` implementation (`frontend/src/worker/cs108/packet.ts`) with the vendor's CSLibrary (`CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/BluetoothProtocol/BTReceive.cs`), two critical validation steps are missing:

**1. Expected Length Validation**
- CS108 packets have a length byte at position [2] indicating payload size
- Vendor checks: `data[2] != (data.Length - 8)` before accepting single packets
- Vendor checks: `_currentRecvBufferSize == (_recvBuffer[2] + 8)` for assembled packets
- Our code uses length to calculate `totalExpected` but doesn't validate final assembled size

**2. CRC Validation**
- CS108 uplink packets contain CRC at bytes [6-7] (big-endian in vendor code)
- Vendor code: `recvCRC = (UInt16)(data[6] << 8 | data[7])` then `recvCRC != Tools.Crc.ComputeChecksum(data)`
- Our code parses CRC but **never validates it**
- Spec note: "CRC is not used when value is zero. For downlink, no need to use."
- This means uplink packets (from device) SHOULD have valid CRC

### Current State (Our Implementation)
```typescript
// packet.ts:296 - We check length for completeness but not integrity
if (this.currentPacket && this.rawDataBuffer.length >= this.currentPacket.totalExpected) {
  const packet = this.finalizePacket();
  // ...
}

// protocol.ts:74 - We parse CRC but never verify it
const crc = data[6] | (data[7] << 8); // Little-endian (different from vendor!)
```

**Critical Issue**: Our CRC byte order may be wrong:
- Vendor: `data[6] << 8 | data[7]` (big-endian: high byte first)
- Us: `data[6] | (data[7] << 8)` (little-endian: low byte first)

### Vendor Implementation Pattern
From `BTReceive.cs`:
```csharp
bool CheckSingalPacket(byte[] data)
{
    // 1. Check header validity
    if (!CheckAPIHeader(data) || data[2] != (data.Length - 8))
        return false;

    // 2. Verify CRC
    UInt16 recvCRC = (UInt16)(data[6] << 8 | data[7]);
    if (recvCRC != Tools.Crc.ComputeChecksum(data))
        return false;

    ProcessAPIPacket(data);
    return true;
}
```

### Desired State
1. Validate packet length matches header before processing
2. Validate CRC on all assembled uplink packets (where CRC != 0)
3. Reject packets that fail validation with appropriate logging
4. Fix CRC byte order to match vendor implementation

## Technical Requirements

### R1: Length Validation
- Before processing any assembled packet, verify `buffer.length === header.length + 8`
- Log and discard packets where actual size doesn't match declared size
- Metric: Track discarded packets for debugging

### R2: CRC Validation
- Calculate CRC using existing `calculateCRC()` function from `protocol.ts`
- Compare against header CRC bytes (using correct byte order)
- If CRC is zero, skip validation (per spec)
- If CRC mismatch, log error with expected/actual values and discard packet

### R3: CRC Byte Order Fix
- Verify vendor uses big-endian: `(data[6] << 8) | data[7]`
- Update our parsing to match: `(data[6] << 8) | data[7]`
- Update CRC calculation to match vendor's algorithm

### R4: CRC Calculation Range
- Vendor calculates CRC over full packet minus CRC bytes themselves
- From `ClassCRC16.cs`: `for (int i = 0; i < (dataIn[2] + 8); i++) { if (i != 6 && i != 7) {...} }`
- Our `calculateCRC()` currently calculates from event code only - may need adjustment

### R5: Diagnostic Logging
- Log validation failures with context for debugging
- Include: expected length vs actual, expected CRC vs calculated
- Use debug buffer for capturing raw packet history on errors

## Code Examples

### Vendor CRC Calculation (C#)
```csharp
public static ushort ComputeChecksum(byte[] dataIn)
{
    ushort checksum = 0;

    // Calculate over entire packet EXCEPT CRC bytes (6,7)
    for (int i = 0; i < (dataIn[2] + 8); i++)
    {
        if (i != 6 && i != 7)
        {
            int index = (checksum ^ ((byte)dataIn[i] & 0x0FF)) & 0x0FF;
            checksum = (ushort)((checksum >> 8) ^ crc_lookup_table[index]);
        }
    }
    return checksum;
}
```

### Our Current CRC (TypeScript)
```typescript
// protocol.ts - currently only calculates over provided data
export function calculateCRC(data: Uint8Array): number {
  let crc: number = 0;
  for (let i = 0; i < data.length; i++) {
    const index: number = (crc ^ data[i]) & 0xff;
    crc = ((crc >> 8) ^ crcLookupTable[index]) & 0xffff;
  }
  return crc;
}
```

## Validation Criteria
- [ ] Single complete packets pass validation (correct length, matching CRC)
- [ ] Fragmented packets pass validation after assembly
- [ ] Packets with wrong declared length are rejected with log
- [ ] Packets with CRC mismatch are rejected with log
- [ ] Packets with CRC=0x0000 skip CRC validation (per spec)
- [ ] Existing unit tests continue to pass
- [ ] Barcode truncation rate decreases (verify via real hardware testing)

## Test Update Required
The barcode e2e test currently expects `10020` (5 chars) which fits in a single BLE packet. This doesn't exercise the fragmentation path.

**Change**: Update `BARCODE_TEST_TAG` in `tests/e2e/barcode.spec.ts`:
```typescript
// Old (fits in single packet - doesn't test fragmentation)
const BARCODE_TEST_TAG = '10020';

// New (24 chars + AIM prefix - forces fragmentation)
const BARCODE_TEST_TAG = 'Q]Q1E20034120000000000001234';
```

This forces packet fragmentation across multiple BLE MTU chunks, which is the scenario where CRC/length validation failures cause truncation.

**Stress Test**: Add a separate test or temporarily increase scan count to get statistical confidence:
```typescript
// Run multiple scan cycles to measure failure rate
// Before fix: expect ~30-90% empty reads
// After fix: expect 0% empty reads (all valid or cleanly rejected)
test('should reliably read fragmented barcodes under stress', async () => {
  const SCAN_CYCLES = 20;
  const results = { valid: 0, empty: 0, rejected: 0 };
  // ... run multiple trigger press/release cycles, collect stats
  expect(results.empty).toBe(0); // No silent failures
});
```

## Files to Modify
1. `frontend/src/worker/cs108/packet.ts` - Add validation in `finalizePacket()`
2. `frontend/src/worker/cs108/protocol.ts` - Fix CRC byte order, possibly adjust calculation range
3. `frontend/src/worker/cs108/packet.test.ts` - Add validation test cases
4. `frontend/tests/e2e/barcode.spec.ts` - Update `BARCODE_TEST_TAG` to 24-char QR value

## Conversation References
- Linear Issue: [TRA-270](https://linear.app/trakrf/issue/TRA-270) - Barcode reads occasionally truncated
- Vendor Reference: `CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/BluetoothProtocol/BTReceive.cs`
- Vendor CRC: `CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/Tools/ClassCRC16.cs`
- API Spec: `docs/frontend/cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md`

## Testing Resources

**ble-mcp-test is active** - Real CS108 hardware is connected via the bridge server.

### Live Bug Reproduction (2026-01-22)
Running `pnpm test:e2e tests/e2e/barcode.spec.ts` with single QR code in read field:
- `E20034120000000000001234` (24-char hex EPC)

**Results (single QR, aligned):**
| Time | Result |
|------|--------|
| 3:33:45 PM | `Q]Q1E20034120000000000001234` ✅ |
| 3:33:42 PM | `Q]Q1E20034120000000000001234` ✅ |
| 3:33:42 PM | `""` (empty) ❌ |

- `Q]Q1` is the AIM ID prefix for QR codes (per spec 9.2.1) - this is correct
- **3 scans total: 2 successful, 1 empty = ~33% failure rate**

This is TRA-270 - intermittent empty reads. Without CRC/length validation, we process invalid/corrupted packets and return empty data to the UI.

Use MCP tools to capture and analyze live packet data:

```bash
# Check connection status
mcp__ble-mcp-test__get_connection_state

# Capture recent packets for analysis
mcp__ble-mcp-test__get_logs --since=30s

# Search for specific patterns (e.g., barcode packets)
mcp__ble-mcp-test__search_packets --hex_pattern="6A"  # Module byte for barcode
```

This enables:
- Verifying CRC byte order against real uplink packets
- Capturing truncated barcode scenarios for reproduction
- Validating fix effectiveness with live hardware

## Open Questions

1. ~~**CRC byte order confirmation**~~: **VERIFIED** - CRC is big-endian. Test packet `A7 B3 03 D9 82 9E 74 37 A0 01 00` has CRC bytes `[6]=0x74, [7]=0x37`. Calculated CRC = `0x7437`, which matches big-endian interpretation `(data[6] << 8) | data[7]`. **Our code currently uses little-endian and must be fixed.**

2. **CRC calculation scope**: Vendor calculates over entire packet excluding CRC bytes. Our current implementation calculates from event code offset. Need to align.

3. **Barcode-specific handling**: The vendor's barcode parser (`ClassBarCode.cs`) accumulates partial barcode strings across multiple packets using prefix/suffix markers. Should we consider this pattern for long barcodes that span multiple BLE fragments?
