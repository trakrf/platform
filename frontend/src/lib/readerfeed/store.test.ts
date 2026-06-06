import { describe, it, expect } from 'vitest';
import {
  applyEvent,
  ageSeconds,
  freshness,
  gradientBackground,
  filterByReader,
  expireStale,
  tagKey,
  BACKSTOP_TTL_SECONDS,
} from './store';
import type { TagState } from '@/types/readerfeed';

const tag = (over: Partial<TagState> = {}): TagState => ({
  readerKey: 'dock-1',
  epc: 'AAA',
  capturePointName: 'Dock 1',
  antennaPort: 1,
  firstSeen: 1000,
  lastSeen: 1000,
  readCount: 1,
  lastRssi: -50,
  rssiAvg: -50,
  rssiMin: -50,
  rssiMax: -50,
  ...over,
});

describe('applyEvent', () => {
  it('snapshot replaces the whole map, keyed by (reader,epc)', () => {
    const before = new Map([[tagKey('dock-9', 'OLD'), tag({ readerKey: 'dock-9', epc: 'OLD' })]]);
    const out = applyEvent(before, {
      type: 'snapshot',
      data: { tags: [tag({ epc: 'A' }), tag({ epc: 'B' })], uniqueTags: 2, readRate: 5 },
    });
    expect([...out.keys()].sort()).toEqual([tagKey('dock-1', 'A'), tagKey('dock-1', 'B')].sort());
  });

  it('enter inserts a tag', () => {
    const out = applyEvent(new Map(), { type: 'enter', data: tag({ epc: 'A' }) });
    expect(out.get(tagKey('dock-1', 'A'))?.epc).toBe('A');
  });

  it('update replaces the tag state (refreshed aggregates)', () => {
    let m = applyEvent(new Map(), { type: 'enter', data: tag({ epc: 'A', readCount: 1 }) });
    m = applyEvent(m, {
      type: 'update',
      data: tag({ epc: 'A', readCount: 7, lastRssi: -40, lastSeen: 9000 }),
    });
    const t = m.get(tagKey('dock-1', 'A'))!;
    expect(t.readCount).toBe(7);
    expect(t.lastRssi).toBe(-40);
    expect(t.lastSeen).toBe(9000);
  });

  it('leave deletes the tag', () => {
    let m = applyEvent(new Map(), { type: 'enter', data: tag({ epc: 'A' }) });
    m = applyEvent(m, { type: 'leave', data: { readerKey: 'dock-1', epc: 'A' } });
    expect(m.has(tagKey('dock-1', 'A'))).toBe(false);
  });

  it('keys the same EPC at different readers distinctly', () => {
    let m = applyEvent(new Map(), { type: 'enter', data: tag({ readerKey: 'dock-1', epc: 'A' }) });
    m = applyEvent(m, { type: 'enter', data: tag({ readerKey: 'dock-2', epc: 'A' }) });
    expect(m.size).toBe(2);
  });

  it('does not mutate the input map', () => {
    const before = new Map<string, TagState>();
    applyEvent(before, { type: 'enter', data: tag({ epc: 'A' }) });
    expect(before.size).toBe(0);
  });
});

describe('ageSeconds', () => {
  it('is whole seconds since lastSeen, clamped at 0', () => {
    expect(ageSeconds(1000, 1000)).toBe(0);
    expect(ageSeconds(1000, 3400)).toBe(2);
    expect(ageSeconds(1000, 999)).toBe(0); // clock skew never goes negative
  });
});

describe('freshness / gradientBackground', () => {
  it('freshness is 1 at age 0, 0 at the clamp, monotonically decreasing', () => {
    expect(freshness(0)).toBe(1);
    expect(freshness(16)).toBe(0);
    expect(freshness(8)).toBeCloseTo(0.5);
    expect(freshness(100)).toBe(0); // clamped, never negative
  });

  it('gradientBackground fades from a visible tint to transparent', () => {
    const fresh = gradientBackground(0);
    const stale = gradientBackground(16);
    expect(fresh).not.toBe(stale);
    expect(stale).toMatch(/0\)$|0\.000\)$/); // ~zero alpha when stale
  });
});

describe('filterByReader', () => {
  it('returns all tags unchanged when no key is given', () => {
    const tags = [tag({ readerKey: 'dock-1' }), tag({ readerKey: 'dock-2' })];
    expect(filterByReader(tags, undefined)).toBe(tags);
  });

  it('keeps only tags whose readerKey matches', () => {
    const tags = [
      tag({ readerKey: 'dock-1', epc: 'A' }),
      tag({ readerKey: 'dock-2', epc: 'B' }),
      tag({ readerKey: 'dock-1', epc: 'C' }),
    ];
    expect(filterByReader(tags, 'dock-1').map((t) => t.epc).sort()).toEqual(['A', 'C']);
  });
});

describe('expireStale (client backstop only)', () => {
  it('keeps tags within the backstop window', () => {
    const m = new Map([[tagKey('dock-1', 'A'), tag({ epc: 'A', lastSeen: 1000 })]]);
    const out = expireStale(m, 1000 + BACKSTOP_TTL_SECONDS * 1000, BACKSTOP_TTL_SECONDS);
    expect(out.has(tagKey('dock-1', 'A'))).toBe(true);
  });

  it('drops tags past the backstop window (covers a missed LEAVE)', () => {
    const m = new Map([[tagKey('dock-1', 'A'), tag({ epc: 'A', lastSeen: 1000 })]]);
    const out = expireStale(m, 1000 + (BACKSTOP_TTL_SECONDS + 1) * 1000, BACKSTOP_TTL_SECONDS);
    expect(out.has(tagKey('dock-1', 'A'))).toBe(false);
  });
});
