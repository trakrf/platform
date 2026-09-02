/**
 * Reading a MAC register back.
 *
 * ## Why this did not exist
 *
 * `createFirmwareCommand` has had a `READ_REGISTER` branch since it was
 * written, and until TRA-1232 nothing had ever called it. Every register
 * operation we performed was a WRITE.
 *
 * ## ⚠ What the hardware said, and what it corrected
 *
 * Two things this file used to assert are wrong, and both were reasoned from
 * the spec rather than measured. The first register read this codebase ever
 * performed, 2026-09-02, settled them:
 *
 * ```
 * TX  A7 B3 0A C2 82 37 00 00 80 02 70 00 00 00 00 00 00 00   read FIRMWARE_VER
 * RX  A7 B3 03 C2 82 9E 32 F1 80 02 00                        status, on 0x8002
 * RX  A7 B3 0A C2 02 9E 97 80 81 00 70 00 00 00 29 60 00 02   REG_RESP, on 0x8100
 * ```
 *
 * 1. **"A write gets no response at all"** — the spec sentence *"There is no
 *    response if the operation is write register"* is about the REG_RESP, not
 *    about the link. Every firmware command, read or write, is acknowledged on
 *    `0x8002` with the same one-byte status.
 * 2. **A register value does not come back on `0x8002`.** It arrives on
 *    `0x8100`, the RFID processor's uplink DATA channel, alongside tag reads
 *    and discriminated by the payload's first byte — which is exactly how the
 *    vendor library dispatches it (`ClassRFID.cs:803`, `case 0x00: case 0x70:`).
 *
 * So `responseLength: 1` and `parseUint8` on `RFID_FIRMWARE_COMMAND` were right
 * all along, for reads as well as writes. What was missing was never a variable
 * payload; it was a listener on the other channel.
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
   * A register value shares the RFID uplink channel `0x8100` with tag reads,
   * and telling them apart is the whole reason this predicate exists.
   *
   * Inventory packets carry `pkt_ver` 0x03 or 0x04, the abort confirmation
   * carries 0x40, and a register response carries 0x70 in exactly eight bytes.
   * Without this check a register value reaches `InventoryParser`, which does
   * not know 0x70, byte-slides one at a time and charges eight `parseErrors`
   * for it.
   */
  it('recognises the 8-byte REG_RESP shape', () => {
    expect(isRegisterResponse(regResp(RFID_REGISTERS.FIRMWARE_VER, 0))).toBe(true);
  });

  /**
   * The bytes the bench reader actually sent, 2026-09-02, answering the first
   * register read this codebase has ever performed:
   *
   *   A7 B3 0A C2 02 9E 97 80 81 00 70 00 00 00 29 60 00 02
   *
   * Kept as a literal rather than rebuilt by `regResp` — the helper encodes the
   * same belief the parser does, so a test written only from it would agree
   * with a wrong parser.
   */
  it('recognises what the reader really sent', () => {
    const measured = new Uint8Array([0x70, 0x00, 0x00, 0x00, 0x29, 0x60, 0x00, 0x02]);
    expect(isRegisterResponse(measured)).toBe(true);
    expect(parseRegisterResponse(measured)).toEqual({
      register: RFID_REGISTERS.FIRMWARE_VER,
      value: 0x02006029,
    });
    // V2.6.41 — the image on the bench CS108. CSL publishes V2.6.46, so this
    // reader is downrev, consistently with both its board firmwares.
    expect(decodeFirmwareVersion(0x02006029).text).toBe('2.6.41');
  });

  it('does not mistake a command acknowledgement for a register value', () => {
    // The status byte the device sends for every firmware command, read or
    // write. Captured on hardware, thousands of times.
    expect(isRegisterResponse(new Uint8Array([0x00]))).toBe(false);
    // Nor a failure status.
    expect(isRegisterResponse(new Uint8Array([0xFF]))).toBe(false);
  });

  it('rejects a payload of the right length that is not low-level API', () => {
    const notLowLevel = regResp(RFID_REGISTERS.MAC_ERROR, 0);
    notLowLevel[0] = 0x01;  // a command-begin packet's pkt_ver
    expect(isRegisterResponse(notLowLevel)).toBe(false);
  });

  /**
   * The neighbours on `0x8100`. Each is a real packet version the RFID
   * processor sends on the same channel, and mistaking one for a register value
   * would file a tag read as a firmware version.
   */
  it('rejects the other packet versions that share the uplink channel', () => {
    for (const pktVer of [0x01, 0x02, 0x03, 0x04, 0x40]) {
      const other = regResp(0x0000, 0);
      other[0] = pktVer;
      expect(isRegisterResponse(other), `pkt_ver 0x${pktVer.toString(16)}`).toBe(false);
    }
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
