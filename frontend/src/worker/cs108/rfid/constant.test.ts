/**
 * TAGACC register packing.
 *
 * These three registers change shape depending on INV_CFG's `tag_read` field.
 * With `tag_read` at 0 — everything this codebase did until now — TAGACC_PTR is
 * a flat 32-bit offset and the second-bank fields are meaningless. With
 * `tag_read` at 1 or 2 the registers split in half, and the vendor spec is
 * explicit that the second-bank fields "must be set to 0" when they are not in
 * use. Packing them by hand at each call site is how that requirement gets
 * quietly violated, so it lives here with tests.
 */
import { describe, it, expect } from 'vitest';
import {
  buildTagaccBank,
  buildTagaccPtr,
  buildTagaccCnt,
  TAG_MEMORY_BANK
} from './constant';

describe('buildTagaccBank', () => {
  it('packs TID as first bank and USER as second', () => {
    // acc_bank in bits 1:0, acc_bank2 in bits 3:2.
    // TID(2) | USER(3) << 2 = 0b1110
    expect(buildTagaccBank({
      bank: TAG_MEMORY_BANK.TID,
      bank2: TAG_MEMORY_BANK.USER
    })).toBe(0x0E);
  });

  it('leaves acc_bank2 zero when only one bank is read', () => {
    // The spec requires acc_bank2 to be 0 whenever tag_read is not 2.
    expect(buildTagaccBank({ bank: TAG_MEMORY_BANK.TID })).toBe(0x02);
  });

  it('defaults to Reserved/zero with no arguments', () => {
    expect(buildTagaccBank()).toBe(0x00);
  });

  it('masks each bank to its two bits', () => {
    // 0xFF would otherwise bleed across both fields and into the reserved bits.
    expect(buildTagaccBank({ bank: 0xFF, bank2: 0xFF })).toBe(0x0F);
  });
});

describe('buildTagaccPtr', () => {
  it('packs the first-bank offset into the low half', () => {
    expect(buildTagaccPtr({ ptr: 0x0002 })).toBe(0x00000002);
  });

  it('packs the second-bank offset into the high half', () => {
    expect(buildTagaccPtr({ ptr: 0x0000, ptr2: 0x0004 })).toBe(0x00040000);
  });

  it('packs both halves together', () => {
    expect(buildTagaccPtr({ ptr: 0xABCD, ptr2: 0x1234 })).toBe(0x1234ABCD);
  });

  it('defaults to zero', () => {
    expect(buildTagaccPtr()).toBe(0x00000000);
  });

  it('masks each offset to 16 bits', () => {
    expect(buildTagaccPtr({ ptr: 0x1FFFF, ptr2: 0x1FFFF })).toBe(0xFFFFFFFF >>> 0);
  });
});

describe('buildTagaccCnt', () => {
  it('packs the first-bank word count into bits 7:0', () => {
    expect(buildTagaccCnt({ length: 6 })).toBe(0x0006);
  });

  it('packs the second-bank word count into bits 15:8', () => {
    expect(buildTagaccCnt({ length: 6, length2: 4 })).toBe(0x0406);
  });

  it('leaves length2 zero when only one bank is read', () => {
    expect(buildTagaccCnt({ length: 6, length2: 0 })).toBe(0x0006);
  });

  it('defaults to zero', () => {
    expect(buildTagaccCnt()).toBe(0x0000);
  });

  it('masks each count to 8 bits', () => {
    // Read length maxes at 255 words; anything wider is a caller bug, and
    // bleeding it into the neighbouring field would silently read the wrong
    // number of words from the wrong bank.
    expect(buildTagaccCnt({ length: 0x1FF, length2: 0x1FF })).toBe(0xFFFF);
  });
});
