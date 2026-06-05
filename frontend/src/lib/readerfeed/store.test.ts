import { describe, it, expect } from 'vitest';
import { mergeReads, expireReads, ageSeconds, ageBandClass, READ_TTL_SECONDS } from './store';
import type { LiveRead, ParsedRead } from '@/types/readerfeed';

const read = (epc: string, over: Partial<ParsedRead> = {}): ParsedRead => ({
  epc,
  readerKey: 'dock-01',
  capturePointName: 'Dock 1',
  antennaPort: 1,
  rssi: -50,
  readerTimestampMs: 0,
  ...over,
});

describe('mergeReads', () => {
  it('adds new reads keyed by epc with receivedAt and id', () => {
    const out = mergeReads(new Map(), [read('AAA')], 1000);
    expect(out.size).toBe(1);
    const r = out.get('AAA') as LiveRead;
    expect(r.id).toBe('AAA');
    expect(r.receivedAt).toBe(1000);
  });

  it('dedups by epc — a later read replaces the earlier and bumps receivedAt', () => {
    let m = mergeReads(new Map(), [read('AAA', { rssi: -70 })], 1000);
    m = mergeReads(m, [read('AAA', { rssi: -40 })], 5000);
    expect(m.size).toBe(1);
    const r = m.get('AAA') as LiveRead;
    expect(r.rssi).toBe(-40);
    expect(r.receivedAt).toBe(5000);
  });

  it('does not mutate the input map', () => {
    const original = new Map<string, LiveRead>();
    const out = mergeReads(original, [read('AAA')], 1000);
    expect(original.size).toBe(0);
    expect(out.size).toBe(1);
  });

  it('keeps distinct epcs from the same batch', () => {
    const out = mergeReads(new Map(), [read('AAA'), read('BBB')], 1000);
    expect([...out.keys()].sort()).toEqual(['AAA', 'BBB']);
  });
});

describe('expireReads', () => {
  it('keeps reads within the TTL window', () => {
    const m = mergeReads(new Map(), [read('AAA')], 1000);
    const out = expireReads(m, 1000 + READ_TTL_SECONDS * 1000, READ_TTL_SECONDS);
    expect(out.has('AAA')).toBe(true);
  });

  it('drops reads older than the TTL', () => {
    const m = mergeReads(new Map(), [read('AAA')], 1000);
    const out = expireReads(m, 1000 + (READ_TTL_SECONDS + 1) * 1000, READ_TTL_SECONDS);
    expect(out.has('AAA')).toBe(false);
  });

  it('does not mutate the input map', () => {
    const m = mergeReads(new Map(), [read('AAA')], 0);
    expireReads(m, 999_999_999, READ_TTL_SECONDS);
    expect(m.has('AAA')).toBe(true);
  });
});

describe('ageSeconds', () => {
  it('is whole seconds since receivedAt', () => {
    const r = { receivedAt: 1000 } as LiveRead;
    expect(ageSeconds(r, 1000)).toBe(0);
    expect(ageSeconds(r, 3400)).toBe(2);
    expect(ageSeconds(r, 9000)).toBe(8);
  });
});

describe('ageBandClass', () => {
  it('maps age to a non-empty class string across bands', () => {
    const bands = [0, 1, 2, 3, 5, 6, 9, 14].map(ageBandClass);
    bands.forEach((b) => expect(typeof b).toBe('string'));
    // freshest and stalest bands differ
    expect(ageBandClass(0)).not.toBe(ageBandClass(14));
  });
});
