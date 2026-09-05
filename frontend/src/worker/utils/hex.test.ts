import { describe, it, expect } from 'vitest';
import { toHex } from './hex';

describe('toHex', () => {
  it('renders bytes as contiguous uppercase hex', () => {
    expect(toHex(new Uint8Array([0xE2, 0x80, 0x11, 0x60]))).toBe('E2801160');
  });

  it('pads a byte below 0x10 to two characters', () => {
    // Without the pad, 0x0F 0x0E renders as "fe" — a different, shorter, and
    // entirely plausible-looking value. This is the whole reason the helper
    // exists rather than a bare map(toString(16)).
    expect(toHex(new Uint8Array([0x0F, 0x0E, 0x01, 0x00]))).toBe('0F0E0100');
  });

  it('renders an all-zero run at full width', () => {
    expect(toHex(new Uint8Array([0x00, 0x00, 0x00]))).toBe('000000');
  });

  it('returns an empty string for no bytes', () => {
    // An absent bank read is expressed as undefined by the caller, not as an
    // empty string, so this is only about not throwing.
    expect(toHex(new Uint8Array(0))).toBe('');
  });

  it('accepts a subarray view without reading past its bounds', () => {
    // Every caller in the parser passes a subarray of a larger packet buffer.
    const packet = new Uint8Array([0xAA, 0xBB, 0xCC, 0xDD, 0xEE]);
    expect(toHex(packet.subarray(1, 3))).toBe('BBCC');
  });
});
