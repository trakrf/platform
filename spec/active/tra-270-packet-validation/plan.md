# Implementation Plan: CS108 Packet Validation - CRC and Length Checks

Generated: 2026-01-22
Specification: spec.md
Linear Issue: TRA-270

## Understanding

This plan addresses intermittent barcode read failures (~33% failure rate) caused by missing packet validation. The CS108 protocol includes CRC and length fields that we parse but never validate. When fragmented packets arrive corrupted or incomplete, we process them anyway, resulting in empty/truncated barcode data.

**Root causes identified:**
1. CRC byte order is wrong (little-endian vs vendor's big-endian)
2. CRC is parsed but never validated against calculated value
3. Length is used for assembly but final size isn't verified
4. Vendor's barcode prefix/suffix accumulation pattern not implemented

**Fix approach:**
Follow vendor's C# implementation exactly - validate length, validate CRC (big-endian), reject invalid packets with Sentry reporting.

## Relevant Files

**Reference Patterns** (existing code to follow):
- `CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/BluetoothProtocol/BTReceive.cs` (lines 67-131) - Vendor packet validation pattern
- `CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/Tools/ClassCRC16.cs` - Vendor CRC algorithm (skip bytes 6-7)
- `CS108-Mobile-CSharp-DotNetStd-App-v4/Library/CSLibrary/BarcodeReader/ClassBarCode.cs` (lines 66-131) - Vendor barcode accumulation
- `frontend/src/components/ErrorBoundary.tsx` (line 31) - Sentry error capture pattern

**Files to Modify**:
- `frontend/src/worker/cs108/protocol.ts` - Fix CRC byte order, add validation functions
- `frontend/src/worker/cs108/packet.ts` - Fix CRC byte order, call validation in finalizePacket()
- `frontend/src/worker/cs108/barcode/parser.ts` - Add stateful prefix/suffix accumulation
- `frontend/test-utils/constants.ts` - Update BARCODE_TEST_TAG to 24-char QR value
- `frontend/tests/e2e/barcode.spec.ts` - Add stress test for fragmented barcodes

**Files to Create**:
- `frontend/src/worker/cs108/packet.test.ts` - Unit tests for CRC/length validation

## Architecture Impact
- **Subsystems affected**: CS108 protocol layer, barcode parser, test infrastructure
- **New dependencies**: None (Sentry already available)
- **Breaking changes**: None - packets that previously passed (with bad data) will now be rejected (with logging)

## Task Breakdown

### Task 1: Fix CRC Byte Order in protocol.ts
**File**: `frontend/src/worker/cs108/protocol.ts`
**Action**: MODIFY
**Pattern**: Match vendor's big-endian: `(data[6] << 8) | data[7]`

**Implementation**:
```typescript
// Line 74: Change from little-endian to big-endian
// OLD: const crc = data[6] | (data[7] << 8);
// NEW:
const crc = (data[6] << 8) | data[7]; // Big-endian per vendor spec
```

Also update the comment at line 16-17 to reflect correct byte order.

**Validation**: `just frontend typecheck`

---

### Task 2: Fix CRC Byte Order in packet.ts
**File**: `frontend/src/worker/cs108/packet.ts`
**Action**: MODIFY
**Pattern**: Match vendor's big-endian in parseHeader()

**Implementation**:
```typescript
// Line 172: Change CRC parsing to big-endian
// OLD: crc: data[6] | (data[7] << 8),
// NEW:
crc: (data[6] << 8) | data[7], // Big-endian per vendor spec
```

Also fix line 89-90 in buildPacket() for outgoing CRC:
```typescript
// OLD:
// packet[PACKET_CONSTANTS.CRC_OFFSET] = crc & 0xFF;        // Low byte at position 6
// packet[PACKET_CONSTANTS.CRC_OFFSET + 1] = (crc >> 8) & 0xFF;  // High byte at position 7
// NEW:
packet[PACKET_CONSTANTS.CRC_OFFSET] = (crc >> 8) & 0xFF;     // High byte at position 6
packet[PACKET_CONSTANTS.CRC_OFFSET + 1] = crc & 0xFF;        // Low byte at position 7
```

**Validation**: `just frontend typecheck`

---

### Task 3: Add CRC Validation Function to protocol.ts
**File**: `frontend/src/worker/cs108/protocol.ts`
**Action**: MODIFY
**Pattern**: Vendor's `ComputeChecksum` from ClassCRC16.cs

**Implementation**:
```typescript
/**
 * Calculate CRC-16 for CS108 packet validation
 * Matches vendor algorithm: calculates over entire packet EXCEPT CRC bytes (6-7)
 * @param packet - Complete packet including header
 * @returns Calculated CRC value
 */
export function calculatePacketCRC(packet: Uint8Array): number {
  const crcLookupTable: number[] = [...]; // existing table

  let crc: number = 0;
  const packetLength = packet[2] + 8; // Length from header + 8 byte header

  for (let i = 0; i < packetLength; i++) {
    // Skip CRC bytes at positions 6 and 7
    if (i !== 6 && i !== 7) {
      const index: number = (crc ^ packet[i]) & 0xff;
      crc = ((crc >> 8) ^ crcLookupTable[index]) & 0xffff;
    }
  }

  return crc;
}

/**
 * Validate packet CRC
 * @returns true if CRC is valid or zero (per spec), false if mismatch
 */
export function validatePacketCRC(packet: Uint8Array): { valid: boolean; expected: number; actual: number } {
  const headerCRC = (packet[6] << 8) | packet[7]; // Big-endian

  // CRC of zero means skip validation (per CS108 spec)
  if (headerCRC === 0) {
    return { valid: true, expected: 0, actual: 0 };
  }

  const calculatedCRC = calculatePacketCRC(packet);
  return {
    valid: headerCRC === calculatedCRC,
    expected: headerCRC,
    actual: calculatedCRC
  };
}

/**
 * Validate packet length matches header declaration
 */
export function validatePacketLength(packet: Uint8Array): { valid: boolean; expected: number; actual: number } {
  const declaredLength = packet[2];
  const expectedTotal = declaredLength + 8; // Header is 8 bytes
  return {
    valid: packet.length === expectedTotal,
    expected: expectedTotal,
    actual: packet.length
  };
}
```

**Validation**: `just frontend typecheck && just frontend test`

---

### Task 4: Integrate Validation in finalizePacket()
**File**: `frontend/src/worker/cs108/packet.ts`
**Action**: MODIFY
**Pattern**: Vendor's CheckSingalPacket pattern

**Implementation**:
```typescript
import * as Sentry from '@sentry/react';
import { validatePacketCRC, validatePacketLength } from './protocol.js';

// In finalizePacket(), after extracting packetData (around line 330):
private finalizePacket(): CS108Packet | null {
  if (!this.currentPacket || this.rawDataBuffer.length < this.currentPacket.totalExpected) {
    return null;
  }

  try {
    const packetData = this.rawDataBuffer.slice(0, this.currentPacket.totalExpected);

    // === NEW: Validate length ===
    const lengthResult = validatePacketLength(packetData);
    if (!lengthResult.valid) {
      logger.warn(
        `[PacketHandler] Length validation failed: expected ${lengthResult.expected}, got ${lengthResult.actual}`
      );
      Sentry.captureMessage('CS108 packet length validation failed', {
        level: 'warning',
        extra: {
          expected: lengthResult.expected,
          actual: lengthResult.actual,
          packetHex: Array.from(packetData.slice(0, 20)).map(b => b.toString(16).padStart(2, '0')).join(' ')
        }
      });
      // Discard and reset
      this.currentPacket = null;
      this.rawDataBuffer = new Uint8Array(0);
      return null;
    }

    // === NEW: Validate CRC ===
    const crcResult = validatePacketCRC(packetData);
    if (!crcResult.valid) {
      logger.warn(
        `[PacketHandler] CRC validation failed: expected 0x${crcResult.expected.toString(16)}, ` +
        `calculated 0x${crcResult.actual.toString(16)}`
      );
      Sentry.captureMessage('CS108 packet CRC validation failed', {
        level: 'warning',
        extra: {
          expectedCRC: `0x${crcResult.expected.toString(16).padStart(4, '0')}`,
          calculatedCRC: `0x${crcResult.actual.toString(16).padStart(4, '0')}`,
          packetHex: Array.from(packetData.slice(0, 20)).map(b => b.toString(16).padStart(2, '0')).join(' ')
        }
      });
      // Discard and reset
      this.currentPacket = null;
      this.rawDataBuffer = new Uint8Array(0);
      return null;
    }

    // ... rest of existing finalization logic ...
```

**Validation**: `just frontend typecheck && just frontend test`

---

### Task 5: Add Barcode Prefix/Suffix Accumulation
**File**: `frontend/src/worker/cs108/barcode/parser.ts`
**Action**: MODIFY
**Pattern**: Vendor's `DeviceRecvData` from ClassBarCode.cs

**Implementation**:
Create stateful barcode accumulator class:
```typescript
/**
 * Stateful barcode accumulator following vendor pattern
 * Accumulates partial barcode strings across multiple packets
 * using prefix/suffix markers to detect complete barcodes
 */
export class BarcodeAccumulator {
  private barcodeStr: string = '';

  private static readonly PREFIX = '\u0002\u0000\u0007\u0010\u0017\u0013';
  private static readonly SUFFIX = '\u0005\u0001\u0011\u0016\u0003\u0004';

  /**
   * Process incoming barcode data
   * @returns Complete barcode if found, null if still accumulating
   */
  processData(data: Uint8Array): string | null {
    // Append new data as UTF-8 string (vendor: line 87)
    this.barcodeStr += new TextDecoder().decode(data);

    // Need minimum length to detect prefix/suffix (vendor: line 89)
    if (this.barcodeStr.length < 12) {
      return null;
    }

    // Look for complete barcode with prefix and suffix
    const prefixAt = this.barcodeStr.indexOf(BarcodeAccumulator.PREFIX);
    const suffixAt = this.barcodeStr.indexOf(BarcodeAccumulator.SUFFIX);

    if (prefixAt !== -1 && suffixAt !== -1 && prefixAt < suffixAt) {
      // Extract barcode between prefix+10 and suffix (vendor: line 123)
      // +10 accounts for prefix(6) + CodeID(1) + AIM ID(3)
      const barcode = this.barcodeStr.substring(prefixAt + 10, suffixAt);

      // Remove processed portion (vendor: line 129)
      this.barcodeStr = this.barcodeStr.substring(suffixAt + 6);

      return barcode;
    }

    // Cleanup: if no prefix but have suffix, discard up to suffix
    if (prefixAt === -1 && suffixAt !== -1) {
      this.barcodeStr = this.barcodeStr.substring(suffixAt + 6);
    }

    // Cleanup: if buffer too long without markers, keep last 5 chars
    if (prefixAt === -1 && suffixAt === -1 && this.barcodeStr.length > 5) {
      this.barcodeStr = this.barcodeStr.substring(this.barcodeStr.length - 5);
    }

    return null;
  }

  /**
   * Reset accumulator state (call on scan stop)
   */
  reset(): void {
    this.barcodeStr = '';
  }
}
```

**Validation**: `just frontend typecheck`

---

### Task 6: Update Test Constants
**File**: `frontend/test-utils/constants.ts`
**Action**: MODIFY

**Implementation**:
```typescript
// Line 43: Update to 24-char QR value that forces fragmentation
// OLD: export const BARCODE_TEST_TAG = TEST_TAGS.TAG_3; // '10020'
// NEW:
export const BARCODE_TEST_TAG = 'Q]Q1E20034120000000000001234'; // 24-char QR with AIM prefix - forces BLE fragmentation

// Also add a constant for the raw value without AIM prefix
export const BARCODE_TEST_TAG_RAW = 'E20034120000000000001234'; // Without AIM prefix
```

**Validation**: `just frontend typecheck`

---

### Task 7: Add Barcode Stress Test
**File**: `frontend/tests/e2e/barcode.spec.ts`
**Action**: MODIFY

**Implementation**:
Add new test after existing tests:
```typescript
test('should reliably read fragmented barcodes under stress', async () => {
  const SCAN_CYCLES = parseInt(process.env.BARCODE_STRESS_CYCLES || '50');
  const results = { valid: 0, empty: 0, total: 0 };

  console.log(`[Stress Test] Running ${SCAN_CYCLES} scan cycles...`);

  for (let i = 0; i < SCAN_CYCLES; i++) {
    // Clear previous barcodes
    await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      barcodeStore?.getState().clearBarcodes();
    });

    // Scan
    await simulateTriggerPress(sharedPage);
    await sharedPage.waitForTimeout(500);
    await simulateTriggerRelease(sharedPage);
    await sharedPage.waitForTimeout(200);

    // Check result
    const barcodes = await sharedPage.evaluate(() => {
      const barcodeStore = (window as WindowWithStores).__ZUSTAND_STORES__?.barcodeStore;
      return barcodeStore?.getState().barcodes || [];
    });

    results.total++;
    if (barcodes.length > 0 && barcodes[0].data && barcodes[0].data.length > 0) {
      results.valid++;
    } else {
      results.empty++;
    }

    if ((i + 1) % 10 === 0) {
      console.log(`[Stress Test] Progress: ${i + 1}/${SCAN_CYCLES} - Valid: ${results.valid}, Empty: ${results.empty}`);
    }
  }

  console.log(`[Stress Test] Final: ${results.valid}/${results.total} valid (${(results.valid/results.total*100).toFixed(1)}%)`);

  // After fix: should have 0 empty reads (packets either valid or cleanly rejected)
  expect(results.empty).toBe(0);
  expect(results.valid).toBeGreaterThan(0);
});
```

**Validation**: Manual test with hardware - run `BARCODE_STRESS_CYCLES=20 pnpm test:e2e tests/e2e/barcode.spec.ts`

---

### Task 8: Add Unit Tests for Packet Validation
**File**: `frontend/src/worker/cs108/packet.test.ts`
**Action**: CREATE
**Pattern**: Existing Vitest patterns in the codebase

**Implementation**:
```typescript
import { describe, it, expect } from 'vitest';
import { calculatePacketCRC, validatePacketCRC, validatePacketLength } from './protocol';
import { PacketHandler } from './packet';

describe('CS108 Packet Validation', () => {
  describe('CRC Calculation', () => {
    it('should calculate CRC matching vendor algorithm', () => {
      // Real packet from hardware test: A7 B3 03 D9 82 9E 74 37 A0 01 00
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x74, 0x37, 0xa0, 0x01, 0x00]);
      const crc = calculatePacketCRC(packet);
      expect(crc).toBe(0x7437); // Verified against hardware
    });

    it('should parse CRC as big-endian', () => {
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x74, 0x37, 0xa0, 0x01, 0x00]);
      const result = validatePacketCRC(packet);
      expect(result.expected).toBe(0x7437); // bytes[6]=0x74, bytes[7]=0x37 -> 0x7437
      expect(result.valid).toBe(true);
    });

    it('should skip validation when CRC is zero', () => {
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x00, 0x00, 0xa0, 0x01, 0x00]);
      const result = validatePacketCRC(packet);
      expect(result.valid).toBe(true);
    });

    it('should detect CRC mismatch', () => {
      // Corrupt CRC bytes
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0xFF, 0xFF, 0xa0, 0x01, 0x00]);
      const result = validatePacketCRC(packet);
      expect(result.valid).toBe(false);
    });
  });

  describe('Length Validation', () => {
    it('should validate correct length', () => {
      // Length byte [2] = 0x03, total should be 3 + 8 = 11 bytes
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x74, 0x37, 0xa0, 0x01, 0x00]);
      const result = validatePacketLength(packet);
      expect(result.valid).toBe(true);
      expect(result.expected).toBe(11);
      expect(result.actual).toBe(11);
    });

    it('should detect length mismatch - too short', () => {
      const packet = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x74, 0x37, 0xa0, 0x01]); // Missing 1 byte
      const result = validatePacketLength(packet);
      expect(result.valid).toBe(false);
      expect(result.expected).toBe(11);
      expect(result.actual).toBe(10);
    });
  });

  describe('PacketHandler Integration', () => {
    it('should reject packets with invalid CRC', () => {
      const handler = new PacketHandler();
      // Packet with corrupted CRC
      const corruptPacket = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0xFF, 0xFF, 0xa0, 0x01, 0x00]);
      const packets = handler.processIncomingData(corruptPacket);
      expect(packets.length).toBe(0); // Should be rejected
    });

    it('should accept packets with valid CRC', () => {
      const handler = new PacketHandler();
      // Valid packet from hardware
      const validPacket = new Uint8Array([0xa7, 0xb3, 0x03, 0xd9, 0x82, 0x9e, 0x74, 0x37, 0xa0, 0x01, 0x00]);
      const packets = handler.processIncomingData(validPacket);
      expect(packets.length).toBe(1);
    });
  });
});
```

**Validation**: `just frontend test src/worker/cs108/packet.test.ts`

---

## Risk Assessment

- **Risk**: CRC byte order change might break working packets
  **Mitigation**: Hardware-verified test case proves big-endian is correct. Unit tests lock in expected behavior.

- **Risk**: Rejecting too many packets could worsen UX
  **Mitigation**: Log rejections to Sentry for monitoring. If rejection rate is high, we have a different problem to investigate.

- **Risk**: Barcode accumulator state could leak between scans
  **Mitigation**: Reset accumulator on scan stop (trigger release). Add cleanup in test beforeEach.

## Integration Points
- **Sentry**: Add `import * as Sentry from '@sentry/react'` to packet.ts
- **Logger**: Use existing `logger.warn()` for validation failures
- **Stores**: No store changes needed - validation happens at protocol layer

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
# Gate 1: Syntax & Style
just frontend lint

# Gate 2: Type Safety
just frontend typecheck

# Gate 3: Unit Tests
just frontend test

# Final validation (after all tasks):
just frontend validate
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Loop until ALL gates pass
- After 3 failed attempts → Stop and ask for help

## Validation Sequence

After each task: `just frontend lint && just frontend typecheck && just frontend test`

Final validation with hardware:
```bash
# Unit tests
just frontend test

# E2E with stress test
BARCODE_STRESS_CYCLES=50 pnpm test:e2e tests/e2e/barcode.spec.ts
```

## Plan Quality Assessment

**Complexity Score**: 4/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
- CRC byte order verified against real hardware packet ✅
- Vendor source code available as reference ✅
- Existing test infrastructure for E2E ✅
- Sentry already integrated ✅
- Clear validation criteria (0% empty reads under stress) ✅

**Assessment**: High confidence - we have verified the root cause (CRC byte order) against real hardware, have vendor code to follow exactly, and have reproducible test cases.

**Estimated one-pass success probability**: 85%

**Reasoning**: The only uncertainty is potential edge cases in the barcode accumulator pattern, but we're following vendor code closely and have hardware available to validate.
