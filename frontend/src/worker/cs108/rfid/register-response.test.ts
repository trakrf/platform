/**
 * Reading a MAC register back.
 *
 * ## Why this did not exist
 *
 * `createFirmwareCommand` has had a `READ_REGISTER` branch since it was
 * written, and nothing has ever called it. Every register operation we perform
 * is a WRITE — and per the spec, **a write gets no response at all**:
 *
 * > This response packet only comes back when the operation is Read register.
 * > There is no response if the operation is write register.
 *
 * So `RFID_FIRMWARE_COMMAND` declares `responseLength: 1` and `parseUint8`,
 * which is correct for the status byte a write acknowledgement carries and
 * cannot represent a register value. We could ask; we could not hear the answer.
 *
 * ## Why it is worth building now
 *
 * `MAC_Error` (reg `0x0005`) is where the R2000 reports its own error code. The
 * 2026-09-01 campaign characterised a state in which the device refuses
 * `0x8001` and `0xA001` for minutes to hours, and the only error we can see is
 * a `0x0000` on the Bluetooth link — the BT board's complaint, not the RFID
 * processor's. `MAC_Error` is the RFID processor's own account of what is wrong,
 * and nothing has ever read it.
 *
 * `FIRMWARE_VER` (reg `0x0000`) sits beside it on the same path and answers a
 * question every capture we hold currently cannot: which firmware was this.
 *
 * ## The one thing that makes this path safe
 *
 * ⚠ **`reg_addr` is echoed back in the response.** Every other `0x8002`
 * exchange is indistinguishable at the op-code level — the defect TRA-1154 was
 * about, where any firmware-command reply could settle any firmware command. A
 * register read is the exception: the reply names the register it answers, so a
 * reader can verify it got the value it asked for rather than assuming.
 *
 * Refs: TRA-1232, TRA-1223.
 */

import { describe, it, expect } from 'vitest';
import {
  parseRegisterResponse,
  decodeFirmwareVersion,
  isRegisterResponse,
} from './register-response';
import { RFID_REGISTERS } from './constant';

/** A REG_RESP as the device sends it: pkt_ver, reserved, addr LSB-first, data LSB-first. */
function regResp(addr: number, data: number): Uint8Array {
  return new Uint8Array([
    0x70,
    0x00,
    addr & 0xFF, (addr >> 8) & 0xFF,
    data & 0xFF, (data >> 8) & 0xFF, (data >> 16) & 0xFF, (data >> 24) & 0xFF,
  ]);
}

describe('isRegisterResponse', () => {
  /**
   * Two different payloads share op code `0x8002` on the way up, and telling
   * them apart is the whole reason this predicate exists.
   *
   * A register WRITE is acknowledged with a one-byte status. That is not
   * inference: `A7 B3 03 C2 82 9E 32 F1 80 02 00` was captured off the bench
   * reader, thousands of times, and it is what `successByte: 0x00` on
   * `RFID_FIRMWARE_COMMAND` has always been checking.
   *
   * A register READ answers with an 8-byte REG_RESP whose first byte is 0x70.
   * Run through the same `successByte` check, 0x70 !== 0x00, and a perfectly
   * good answer is reported as a failed command.
   */
  it('recognises the 8-byte REG_RESP shape', () => {
    expect(isRegisterResponse(regResp(RFID_REGISTERS.FIRMWARE_VER, 0))).toBe(true);
  });

  it('does not mistake a write acknowledgement for a register value', () => {
    // The status byte the device actually sends, captured on hardware.
    expect(isRegisterResponse(new Uint8Array([0x00]))).toBe(false);
    // Nor a failure status.
    expect(isRegisterResponse(new Uint8Array([0xFF]))).toBe(false);
  });

  it('rejects a payload of the right length that is not low-level API', () => {
    const notLowLevel = regResp(RFID_REGISTERS.MAC_ERROR, 0);
    notLowLevel[0] = 0x01;  // a command-begin packet's pkt_ver
    expect(isRegisterResponse(notLowLevel)).toBe(false);
  });

  it('rejects a truncated register response rather than guessing at it', () => {
    expect(isRegisterResponse(regResp(RFID_REGISTERS.MAC_ERROR, 0).subarray(0, 7))).toBe(false);
    expect(isRegisterResponse(new Uint8Array(0))).toBe(false);
  });
});

describe('parseRegisterResponse', () => {
  it('reads the address and value back, both byte-swapped', () => {
    // Spec A.3: "an address of 0xF000 will become 00F0 in the packet"
    expect(parseRegisterResponse(regResp(0xF000, 0x12345678))).toEqual({
      register: 0xF000,
      value: 0x12345678,
    });
  });

  it('reads FIRMWARE_VER at its own address', () => {
    expect(parseRegisterResponse(regResp(RFID_REGISTERS.FIRMWARE_VER, 0x0200062E))).toEqual({
      register: 0x0000,
      value: 0x0200062E,
    });
  });

  /**
   * The echo is the only correlation this protocol offers on 0x8002. A caller
   * that ignores it is back to TRA-1154 — accepting whatever reply arrives.
   */
  it('reports the register so a caller can verify it got what it asked for', () => {
    const parsed = parseRegisterResponse(regResp(RFID_REGISTERS.MAC_ERROR, 0x0000_0000));
    expect(parsed.register).toBe(0x0005);
  });

  it('handles a value with the high bit set without going negative', () => {
    expect(parseRegisterResponse(regResp(0x0000, 0xFFFFFFFF)).value).toBe(0xFFFFFFFF);
  });

  it('rejects a payload that is not a register response', () => {
    expect(() => parseRegisterResponse(new Uint8Array([0x00, 0x01]))).toThrow(/too short/i);
  });

  it('rejects a payload whose packet version is not the low-level 0x70', () => {
    const bad = regResp(0x0000, 0);
    bad[0] = 0x40;
    expect(() => parseRegisterResponse(bad)).toThrow(/0x70/);
  });
});

describe('decodeFirmwareVersion', () => {
  /**
   * FIRMWARE_VER, spec A.4:
   *   bits 31:24  Major   (8 bit)
   *   bits 23:12  Minor   (12 bit)
   *   bits 11:0   Patch   (12 bit)
   *
   * ⚠ Minor and Patch are TWELVE bits, not eight. A byte-wise decode reads
   * plausible-looking wrong numbers rather than failing, which is why this is
   * pinned rather than left to a shift-and-mask at the call site.
   */
  it('splits major/minor/patch on the spec bit boundaries', () => {
    // major 2, minor 6, patch 46 -> V2.6.46, the current published RFID image
    const raw = (2 << 24) | (6 << 12) | 46;
    expect(decodeFirmwareVersion(raw)).toEqual({ major: 2, minor: 6, patch: 46, text: '2.6.46' });
  });

  it('does not truncate a minor or patch above 255', () => {
    const raw = (1 << 24) | (4095 << 12) | 4095;
    expect(decodeFirmwareVersion(raw)).toEqual({
      major: 1, minor: 4095, patch: 4095, text: '1.4095.4095',
    });
  });

  it('reads all zeros as 0.0.0 rather than failing', () => {
    expect(decodeFirmwareVersion(0)).toEqual({ major: 0, minor: 0, patch: 0, text: '0.0.0' });
  });
});
