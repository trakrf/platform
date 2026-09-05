/**
 * The tag-capture register writes.
 *
 * Inventory normally runs in compact mode, whose response payload the vendor
 * documents as PC + EPC + NB_RSSI — there is no field for bank data in it at
 * all. Bank data only rides the NORMAL mode inventory response. So capturing
 * TID or USER is not an extra register on top of what we already send; it is a
 * different inventory mode, and that is what this sequence switches into.
 *
 * It is spliced in AFTER INVENTORY_CONFIG_SEQUENCE, which writes the same four
 * registers to their no-capture values. Later writes win, which keeps the
 * disabled path byte-for-byte identical to what shipped before.
 */
import { describe, it, expect } from 'vitest';
import { tagCaptureSequence } from './sequences';
import { RFID_REGISTERS, TAG_MEMORY_BANK } from '../constant';
import { RFID_FIRMWARE_COMMAND } from '../../event.js';

/**
 * Pull the value written to a register out of the sequence.
 *
 * The payload is an encoded firmware command rather than a plain object, so
 * this decodes the register and value back out of the bytes. Asserting on the
 * wire form is the point: a builder that returns the right number into the
 * wrong register would pass any assertion made against its arguments.
 */
function writtenValue(sequence: ReturnType<typeof tagCaptureSequence>, register: number): number | undefined {
  for (const step of sequence) {
    if (step.event !== RFID_FIRMWARE_COMMAND) continue;
    const payload = step.payload as Uint8Array | undefined;
    if (!payload) continue;
    // createFirmwareCommand lays out: [0]=pkt_ver, [1]=write/read flag,
    // [3:2]=register, [7:4]=value — both LSB first, per spec A.3
    // ("REVERSELY POPULATED")
    const reg = payload[2] | (payload[3] << 8);
    if (reg !== register) continue;
    return (
      (payload[4] |
        (payload[5] << 8) |
        (payload[6] << 16) |
        (payload[7] << 24)) >>> 0
    );
  }
  return undefined;
}

describe('tagCaptureSequence', () => {
  it('writes nothing when capture is off', () => {
    expect(tagCaptureSequence({ captureAllTagData: false })).toEqual([]);
  });

  it('writes nothing when there are no rfid settings at all', () => {
    expect(tagCaptureSequence()).toEqual([]);
    expect(tagCaptureSequence({})).toEqual([]);
  });

  describe('with both banks (the default)', () => {
    const sequence = tagCaptureSequence({ captureAllTagData: true });

    it('reads two banks in normal mode', () => {
      const invCfg = writtenValue(sequence, RFID_REGISTERS.INV_CFG)!;

      // tag_read is bits 17:16 — 2 means "read two banks after inventory"
      expect((invCfg >> 16) & 0x03).toBe(2);
      // inv_mode is bit 26 — 0 is normal mode, and compact mode carries no
      // bank data whatsoever, so this bit IS the feature
      expect((invCfg >> 26) & 0x01).toBe(0);
      // tag_delay is bits 25:20 — vendor guidance is 30 for Bluetooth normal
      // mode, against 0-7 for compact
      expect((invCfg >> 20) & 0x3F).toBe(30);
    });

    it('names TID first and USER second', () => {
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_BANK)).toBe(0x0E);
    });

    it('defaults to 6 words of TID and 4 of USER', () => {
      // 6 words of TID covers an extended TID carrying a 48-bit serial.
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_CNT)).toBe(
        6 | (4 << 8)
      );
    });

    it('starts both banks at word zero', () => {
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_PTR)).toBe(0);
    });
  });

  describe('with userWords 0 — the TID-only escape hatch', () => {
    const sequence = tagCaptureSequence({
      captureAllTagData: true,
      userWords: 0
    });

    it('drops to a single-bank read', () => {
      const invCfg = writtenValue(sequence, RFID_REGISTERS.INV_CFG)!;
      expect((invCfg >> 16) & 0x03).toBe(1);
    });

    it('zeroes acc_bank2, as the spec requires when tag_read is not 2', () => {
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_BANK)).toBe(
        TAG_MEMORY_BANK.TID
      );
    });

    it('zeroes length2 and ptr2', () => {
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_CNT)).toBe(6);
      expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_PTR)).toBe(0);
    });
  });

  it('honours an explicit offset and lengths', () => {
    const sequence = tagCaptureSequence({
      captureAllTagData: true,
      tidWords: 2,
      userOffset: 8,
      userWords: 16
    });

    expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_CNT)).toBe(2 | (16 << 8));
    expect(writtenValue(sequence, RFID_REGISTERS.TAGACC_PTR)).toBe(8 << 16);
  });
});
