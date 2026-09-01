/**
 * The Bluetooth board's own identity: firmware versions and serial number.
 *
 * ⚠ **These are NOT byte-swapped, and the RFID registers next door ARE.**
 *
 * Spec A.3's "REVERSELY POPULATED" note is scoped to *"RFID firmware commands
 * and replies"* — the `0x8002` register path. The BT-board commands here
 * (`0xB000`, `0xC000`, `0xB004`) are plain, MSB-first:
 *
 * ```
 * 0xB000 / 0xC000   Byte 0: Major   Byte 1: Minor   Byte 2: Build
 * ```
 *
 * Two adjacent decode conventions in one protocol is exactly the shape that
 * produces a plausible wrong number rather than a failure, so both directions
 * are pinned by tests rather than left to a comment.
 *
 * Refs: TRA-1232.
 */

import { describe, it, expect } from 'vitest';
import {
  GET_SILICON_LAB_VERSION,
  GET_BLUETOOTH_VERSION,
  GET_SERIAL_NUMBER,
  parseBoardVersion,
  parseSerialNumber,
} from './device-info';

describe('board firmware version commands', () => {
  it('uses the op codes the spec assigns', () => {
    expect(GET_SILICON_LAB_VERSION.eventCode).toBe(0xB000);
    expect(GET_BLUETOOTH_VERSION.eventCode).toBe(0xC000);
    expect(GET_SERIAL_NUMBER.eventCode).toBe(0xB004);
  });

  it('are commands that expect a response, not fire-and-forget notifications', () => {
    for (const e of [GET_SILICON_LAB_VERSION, GET_BLUETOOTH_VERSION, GET_SERIAL_NUMBER]) {
      expect(e.isCommand).toBe(true);
      expect(e.isNotification).toBe(false);
    }
  });
});

describe('parseBoardVersion', () => {
  /**
   * The published Silicon Labs image is V1.0.17 and the Bluetooth image is
   * V1.0.20; the vendor library gates behaviour on `>= 0x00010010` (V1.0.16).
   * So real values live in a range where a byte-swapped read would produce
   * something that still looks like a version number.
   */
  it('reads major, minor, build in that order — NOT byte-swapped', () => {
    // toMatchObject, not toEqual: `packed` is also returned and is asserted
    // separately below. Pinning the whole shape here would couple this test to
    // a field it is not about.
    expect(parseBoardVersion(new Uint8Array([1, 0, 17]))).toMatchObject({
      major: 1, minor: 0, patch: 17, text: '1.0.17',
    });
  });

  it('would report a different version if read in reverse, so the order is load-bearing', () => {
    const forward = parseBoardVersion(new Uint8Array([1, 0, 20]));
    expect(forward.text).toBe('1.0.20');
    expect(forward.text).not.toBe('20.0.1');
  });

  /**
   * The vendor treats a failed read as version 0 rather than throwing
   * (`_firmwareVersion = 0` in ClassSiliconLabIC). A short payload here means
   * we did not get a version, and callers must be able to tell that from a
   * genuine 0.0.0.
   */
  it('rejects a payload too short to be a version', () => {
    expect(() => parseBoardVersion(new Uint8Array([1, 0]))).toThrow(/3 bytes/);
  });

  it('exposes a comparable numeric form for version gating', () => {
    // The vendor's own gate is `>= 0x00010010`, i.e. V1.0.16 packed as bytes.
    expect(parseBoardVersion(new Uint8Array([1, 0, 16])).packed).toBe(0x00010010);
    expect(parseBoardVersion(new Uint8Array([1, 0, 17])).packed).toBeGreaterThan(0x00010010);
    expect(parseBoardVersion(new Uint8Array([1, 0, 8])).packed).toBeLessThan(0x00010010);
  });
});

describe('parseSerialNumber', () => {
  it('reads the 13-byte UTF-8 serial the vendor library reads', () => {
    const serial = 'CS108ABC12345';
    expect(parseSerialNumber(new TextEncoder().encode(serial))).toBe(serial);
  });

  it('trims trailing padding rather than returning it', () => {
    const padded = new Uint8Array(13);
    padded.set(new TextEncoder().encode('CS108X'));
    expect(parseSerialNumber(padded)).toBe('CS108X');
  });

  it('returns empty for an unreadable payload rather than throwing', () => {
    // The vendor wraps this in try/catch and yields "" — a missing serial is
    // not worth failing a connect over.
    expect(parseSerialNumber(new Uint8Array(0))).toBe('');
  });
});
