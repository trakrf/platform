import { describe, it, expect } from 'vitest';
import { latestBarcodeLocation } from './barcodeLocation';
import type { TagInfo } from '@/stores/tagStore';

const tag = (overrides: Partial<TagInfo>): TagInfo => ({
  epc: 'E1',
  count: 1,
  source: 'rfid',
  type: 'unknown',
  ...overrides,
});

describe('latestBarcodeLocation (TRA-1031)', () => {
  it('returns null when no barcode-sourced location tags exist', () => {
    expect(latestBarcodeLocation([
      tag({ type: 'location', locationId: 1, source: 'rfid', lastSeenTime: 5, rssi: -50 }),
      tag({ type: 'asset', source: 'barcode', lastSeenTime: 9 }),
    ])).toBeNull();
  });

  it('picks a barcode location even without RSSI', () => {
    expect(latestBarcodeLocation([
      tag({ type: 'location', locationId: 7, source: 'barcode', lastSeenTime: 5 }),
    ])).toEqual({ locationId: 7, lastSeenTime: 5 });
  });

  it('most recently seen barcode location wins', () => {
    const result = latestBarcodeLocation([
      tag({ epc: 'A', type: 'location', locationId: 1, source: 'barcode', lastSeenTime: 5 }),
      tag({ epc: 'B', type: 'location', locationId: 2, source: 'barcode', lastSeenTime: 9 }),
    ]);
    expect(result?.locationId).toBe(2);
  });

  it('ignores barcode reads not yet classified as locations', () => {
    expect(latestBarcodeLocation([
      tag({ type: 'unknown', source: 'barcode', lastSeenTime: 5 }),
    ])).toBeNull();
  });
});
