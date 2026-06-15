import { describe, it, expect } from 'vitest';
import {
  applyEvent,
  ageSeconds,
  freshness,
  gradientBackground,
  filterByReader,
  filterTags,
  toDisplayRows,
  sortRows,
  expireStale,
  tagKey,
  newestServerTimestamp,
  BACKSTOP_TTL_SECONDS,
} from './store';
import type { TagState } from '@/types/readerfeed';

const tag = (over: Partial<TagState> = {}): TagState => ({
  readerKey: 'dock-1',
  epc: 'AAA',
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
  it('snapshot replaces the whole map, keyed by (reader,epc,antenna)', () => {
    const before = new Map([
      [tagKey('dock-9', 'OLD', 1), tag({ readerKey: 'dock-9', epc: 'OLD' })],
    ]);
    const out = applyEvent(before, {
      type: 'snapshot',
      data: { tags: [tag({ epc: 'A' }), tag({ epc: 'B' })], uniqueTags: 2, readRate: 5 },
    });
    expect([...out.keys()].sort()).toEqual(
      [tagKey('dock-1', 'A', 1), tagKey('dock-1', 'B', 1)].sort(),
    );
  });

  it('upsert inserts a tag', () => {
    const out = applyEvent(new Map(), { type: 'upsert', data: tag({ epc: 'A' }) });
    expect(out.get(tagKey('dock-1', 'A', 1))?.epc).toBe('A');
  });

  it('upsert replaces the tag state (refreshed aggregates)', () => {
    let m = applyEvent(new Map(), { type: 'upsert', data: tag({ epc: 'A', readCount: 1 }) });
    m = applyEvent(m, {
      type: 'upsert',
      data: tag({ epc: 'A', readCount: 7, lastRssi: -40, lastSeen: 9000 }),
    });
    const t = m.get(tagKey('dock-1', 'A', 1))!;
    expect(t.readCount).toBe(7);
    expect(t.lastRssi).toBe(-40);
    expect(t.lastSeen).toBe(9000);
  });

  it('leave deletes the tag by (reader,epc,antenna)', () => {
    let m = applyEvent(new Map(), { type: 'upsert', data: tag({ epc: 'A', antennaPort: 2 }) });
    m = applyEvent(m, {
      type: 'leave',
      data: { readerKey: 'dock-1', epc: 'A', antennaPort: 2 },
    });
    expect(m.has(tagKey('dock-1', 'A', 2))).toBe(false);
  });

  it('keys the same EPC at different readers distinctly', () => {
    let m = applyEvent(new Map(), { type: 'upsert', data: tag({ readerKey: 'dock-1', epc: 'A' }) });
    m = applyEvent(m, { type: 'upsert', data: tag({ readerKey: 'dock-2', epc: 'A' }) });
    expect(m.size).toBe(2);
  });

  it('keys the same EPC at the same reader on different antennas distinctly', () => {
    let m = applyEvent(new Map(), { type: 'upsert', data: tag({ epc: 'A', antennaPort: 1 }) });
    m = applyEvent(m, { type: 'upsert', data: tag({ epc: 'A', antennaPort: 2 }) });
    expect(m.size).toBe(2);
  });

  it('does not mutate the input map', () => {
    const before = new Map<string, TagState>();
    applyEvent(before, { type: 'upsert', data: tag({ epc: 'A' }) });
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

describe('filterTags', () => {
  const tags = [
    tag({ epc: 'ABC123', antennaPort: 1 }),
    tag({ epc: 'XYZ789', antennaPort: 2, alias: 'Forklift' }),
    tag({ epc: 'ABC999', antennaPort: 2 }),
  ];

  it('returns all tags when no filters are set', () => {
    expect(filterTags(tags, {})).toHaveLength(3);
  });

  it('matches the EPC by case-insensitive substring', () => {
    expect(filterTags(tags, { text: 'abc' }).map((t) => t.epc)).toEqual(['ABC123', 'ABC999']);
  });

  it('matches the alias as well as the EPC', () => {
    expect(filterTags(tags, { text: 'fork' }).map((t) => t.epc)).toEqual(['XYZ789']);
  });

  it('keeps only the given antenna port', () => {
    expect(filterTags(tags, { antenna: 2 }).map((t) => t.epc)).toEqual(['XYZ789', 'ABC999']);
  });

  it('combines text and antenna filters', () => {
    expect(filterTags(tags, { text: 'abc', antenna: 2 }).map((t) => t.epc)).toEqual(['ABC999']);
  });
});

describe('toDisplayRows', () => {
  // Same tag at one reader on two antennas, distinct read histories.
  const ant1 = tag({
    epc: 'TAG', antennaPort: 1, readCount: 3,
    rssiAvg: -60, rssiMin: -65, rssiMax: -55, lastRssi: -58, firstSeen: 50, lastSeen: 100,
  });
  const ant2 = tag({
    epc: 'TAG', antennaPort: 2, readCount: 1,
    rssiAvg: -40, rssiMin: -40, rssiMax: -40, lastRssi: -40, firstSeen: 80, lastSeen: 200,
  });

  it('split view emits one row per antenna with the port as its label', () => {
    const rows = toDisplayRows([ant1, ant2], false);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.antennaLabel).sort()).toEqual(['1', '2']);
    // distinct row keys so React/leave target the right row
    expect(new Set(rows.map((r) => r.rowKey)).size).toBe(2);
  });

  it('aggregate view collapses antennas into one (reader,epc) row', () => {
    const rows = toDisplayRows([ant1, ant2], true);
    expect(rows).toHaveLength(1);
    const r = rows[0];
    expect(r.readCount).toBe(4); // 3 + 1
    expect(r.rssiMin).toBe(-65); // min of mins
    expect(r.rssiMax).toBe(-40); // max of maxes
    expect(r.rssiAvg).toBe(-55); // count-weighted: (-60*3 + -40*1)/4
    expect(r.lastSeen).toBe(200); // most recent
    expect(r.firstSeen).toBe(50); // earliest
    expect(r.lastRssi).toBe(-40); // from the most-recent antenna
    expect(r.antennaLabel).toBe('1,2'); // sorted, comma-joined list of contributing antennas
  });

  it('aggregate view labels a single-antenna tag with its port number', () => {
    const rows = toDisplayRows([ant1], true);
    expect(rows[0].antennaLabel).toBe('1');
  });

  it('aggregate view lists antennas sorted and de-duplicated', () => {
    const rows = toDisplayRows(
      [
        tag({ epc: 'TAG', antennaPort: 3 }),
        tag({ epc: 'TAG', antennaPort: 1 }),
        tag({ epc: 'TAG', antennaPort: 3 }),
      ],
      true,
    );
    expect(rows[0].antennaLabel).toBe('1,3');
  });

  it('aggregate view keeps tags at different readers separate', () => {
    const rows = toDisplayRows(
      [tag({ readerKey: 'dock-1', epc: 'A' }), tag({ readerKey: 'dock-2', epc: 'A' })],
      true,
    );
    expect(rows).toHaveLength(2);
  });

  it('preserves the alias when aggregating', () => {
    const rows = toDisplayRows([ant1, tag({ epc: 'TAG', antennaPort: 2, alias: 'Pallet' })], true);
    expect(rows[0].alias).toBe('Pallet');
  });
});

describe('sortRows', () => {
  const rows = toDisplayRows(
    [
      tag({ epc: 'B', readCount: 5, lastSeen: 300 }),
      tag({ epc: 'A', readCount: 9, lastSeen: 100 }),
      tag({ epc: 'C', readCount: 1, lastSeen: 200 }),
    ],
    false,
  );

  it('sorts by read count descending', () => {
    expect(sortRows(rows, { key: 'readCount', dir: 'desc' }).map((r) => r.readCount)).toEqual([
      9, 5, 1,
    ]);
  });

  it('sorts by read count ascending', () => {
    expect(sortRows(rows, { key: 'readCount', dir: 'asc' }).map((r) => r.readCount)).toEqual([
      1, 5, 9,
    ]);
  });

  it('sorts by EPC alphabetically', () => {
    expect(sortRows(rows, { key: 'epc', dir: 'asc' }).map((r) => r.epc)).toEqual(['A', 'B', 'C']);
  });

  it('sorts by lastSeen descending (freshest first)', () => {
    expect(sortRows(rows, { key: 'lastSeen', dir: 'desc' }).map((r) => r.lastSeen)).toEqual([
      300, 200, 100,
    ]);
  });

  it('does not mutate the input array', () => {
    const before = rows.map((r) => r.epc);
    sortRows(rows, { key: 'epc', dir: 'desc' });
    expect(rows.map((r) => r.epc)).toEqual(before);
  });

  it('breaks sort ties deterministically by rowKey so equal values keep a stable relative order', () => {
    const tied = toDisplayRows(
      [
        tag({ readerKey: 'r', epc: 'C', readCount: 5 }),
        tag({ readerKey: 'r', epc: 'A', readCount: 5 }),
        tag({ readerKey: 'r', epc: 'B', readCount: 5 }),
      ],
      false,
    );
    // All three share readCount 5; the tiebreaker (rowKey, which embeds the epc)
    // gives a single deterministic order regardless of incoming order.
    expect(sortRows(tied, { key: 'readCount', dir: 'desc' }).map((r) => r.epc)).toEqual([
      'A',
      'B',
      'C',
    ]);
  });

  // TRA-992: the default (no column selected) is a stable "first-seen" order that
  // does NOT churn as live reads stream in — matching Keypr's reads view.
  describe('natural order (key: null)', () => {
    it('orders rows by firstSeen ascending (oldest first, new tags append)', () => {
      const rows = toDisplayRows(
        [
          tag({ epc: 'SECOND', firstSeen: 200 }),
          tag({ epc: 'FIRST', firstSeen: 100 }),
          tag({ epc: 'THIRD', firstSeen: 300 }),
        ],
        false,
      );
      expect(sortRows(rows, { key: null, dir: 'asc' }).map((r) => r.epc)).toEqual([
        'FIRST',
        'SECOND',
        'THIRD',
      ]);
    });

    it('does not reorder when a row updates (count / RSSI / lastSeen change)', () => {
      const before = toDisplayRows(
        [
          tag({ epc: 'A', firstSeen: 100, lastSeen: 100, readCount: 1 }),
          tag({ epc: 'B', firstSeen: 200, lastSeen: 200, readCount: 1 }),
        ],
        false,
      );
      // B is re-seen many times and is now the freshest with the highest count —
      // under a lastSeen/readCount sort it would jump to the top. Natural order
      // must keep it in place.
      const after = toDisplayRows(
        [
          tag({ epc: 'A', firstSeen: 100, lastSeen: 100, readCount: 1 }),
          tag({ epc: 'B', firstSeen: 200, lastSeen: 9999, readCount: 500 }),
        ],
        false,
      );
      const natural = { key: null, dir: 'asc' } as const;
      expect(sortRows(before, natural).map((r) => r.epc)).toEqual(['A', 'B']);
      expect(sortRows(after, natural).map((r) => r.epc)).toEqual(['A', 'B']);
    });

    it('is independent of incoming row order (stable across snapshot keyframes)', () => {
      const a = tag({ epc: 'A', firstSeen: 100 });
      const b = tag({ epc: 'B', firstSeen: 200 });
      const natural = { key: null, dir: 'asc' } as const;
      const order1 = sortRows(toDisplayRows([a, b], false), natural).map((r) => r.epc);
      const order2 = sortRows(toDisplayRows([b, a], false), natural).map((r) => r.epc);
      expect(order1).toEqual(['A', 'B']);
      expect(order2).toEqual(['A', 'B']);
    });
  });
});

describe('expireStale (client backstop only)', () => {
  it('keeps tags within the backstop window', () => {
    const m = new Map([[tagKey('dock-1', 'A', 1), tag({ epc: 'A', lastSeen: 1000 })]]);
    const out = expireStale(m, 1000 + BACKSTOP_TTL_SECONDS * 1000, BACKSTOP_TTL_SECONDS);
    expect(out.has(tagKey('dock-1', 'A', 1))).toBe(true);
  });

  it('drops tags past the backstop window (covers a missed LEAVE)', () => {
    const m = new Map([[tagKey('dock-1', 'A', 1), tag({ epc: 'A', lastSeen: 1000 })]]);
    const out = expireStale(m, 1000 + (BACKSTOP_TTL_SECONDS + 1) * 1000, BACKSTOP_TTL_SECONDS);
    expect(out.has(tagKey('dock-1', 'A', 1))).toBe(false);
  });
});

describe('newestServerTimestamp', () => {
  it('returns the freshest lastSeen across snapshot and upsert events', () => {
    const ts = newestServerTimestamp([
      { type: 'snapshot', data: { tags: [tag({ lastSeen: 100 }), tag({ lastSeen: 300 })], uniqueTags: 2, readRate: 1 } },
      { type: 'upsert', data: tag({ lastSeen: 250 }) },
    ]);
    expect(ts).toBe(300);
  });

  it('ignores LEAVE events (they carry no timestamp) and returns null for a leaves-only batch', () => {
    const ts = newestServerTimestamp([
      { type: 'leave', data: { readerKey: 'dock-1', epc: 'A', antennaPort: 1 } },
    ]);
    expect(ts).toBeNull();
  });

  it('returns null for an empty batch', () => {
    expect(newestServerTimestamp([])).toBeNull();
  });
});
