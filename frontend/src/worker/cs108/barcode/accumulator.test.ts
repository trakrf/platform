import { describe, it, expect } from 'vitest';
import { BarcodeAccumulator } from './accumulator';

// Helper: hex string to Uint8Array. Accepts space-separated hex bytes.
const hex = (s: string): Uint8Array =>
  new Uint8Array(s.trim().split(/\s+/).map(b => parseInt(b, 16)));

describe('BarcodeAccumulator', () => {
  it('extracts one record from a single payload containing one 0x0D-terminated record', () => {
    // Canonical CLEAN_SINGLE payload from fixtures (Newland prefix + AIM + data + suffix + CR)
    const payload = hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    );

    const acc = new BarcodeAccumulator();
    const records = acc.appendAndExtract(payload);

    expect(records).toHaveLength(1);
    expect(records[0]).toEqual(payload);
  });

  it('extracts multiple records when one payload contains two 0x0D-terminated frames', () => {
    // BUNDLED_SECOND_PKT canonical: two clean Newland frames concatenated.
    const recordA = hex(
      '06 02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    );
    const recordB = hex(
      '02 00 07 10 17 13 51 5D 51 31 37 31 32 41 43 31 32 46 31 30 30 ' +
      '37 30 30 30 30 30 30 32 32 34 34 30 31 05 01 11 16 03 04 0D'
    );
    const bundled = new Uint8Array(recordA.length + recordB.length);
    bundled.set(recordA);
    bundled.set(recordB, recordA.length);

    const acc = new BarcodeAccumulator();
    const records = acc.appendAndExtract(bundled);

    expect(records).toHaveLength(2);
    expect(records[0]).toEqual(recordA);
    expect(records[1]).toEqual(recordB);
  });
});
