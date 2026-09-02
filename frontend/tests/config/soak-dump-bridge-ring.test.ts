import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { dumpRing, seekTo, PAGE } from '../../scripts/dump-bridge-ring.mjs';

/**
 * The two ways a ring dump lies, and the bisection that scopes it.
 *
 * This tool feeds `ring-unanswered-commands.mjs`, whose output goes onto tickets
 * as evidence — so a dump that silently truncates makes the analysis wrong while
 * looking entirely fine. Both failure paths below produce a plausible file:
 *
 *   disabled buffer   -> an EMPTY dump, which downstream reads as a clean run
 *   stalled cursor    -> the same page written forever (27 GB, once, for real)
 *
 * Neither announces itself. That is the whole reason they are guarded, and the
 * reason the guards are tested rather than assumed — the same argument
 * `every-signal-needle-has-a-producer` makes about a needle nothing can emit.
 *
 * Refs: TRA-1242.
 */

/** A ring of `n` entries, one per second from `startMs`, served page by page. */
function fakeRing(n: number, startMs = Date.parse('2026-09-02T00:00:00Z')) {
  const entries = Array.from({ length: n }, (_, i) => ({
    id: 1000 + i,
    timestamp: new Date(startMs + i * 1000).toISOString(),
    direction: i % 2 ? 'RX' : 'TX',
    text: 'A7 B3',
    is_packet: true,
  }));
  return async (_op: string, args: { cursor?: number; limit: number }) => {
    const after = args.cursor ?? 999;
    const page = entries.filter((e) => e.id > after).slice(0, args.limit);
    return {
      entries: page,
      next_cursor: page.length ? page[page.length - 1].id : after,
      dropped_before: null,
      buffer_enabled: true,
    };
  };
}

describe('dumpRing', () => {
  it('refuses to write an empty dump when the buffer is disabled', async () => {
    // A zero-byte file downstream reads as "the device was quiet". It has to
    // fail loudly instead — absence of recording is not absence of traffic.
    const call = async () => ({ entries: [], next_cursor: 0, dropped_before: null, buffer_enabled: false });

    await expect(dumpRing({ call, write: () => {} })).rejects.toThrow(/recording nothing/i);
  });

  it('aborts rather than looping when the cursor stops advancing', async () => {
    // The 27 GB failure: a page comes back full, but next_cursor never moves.
    // Guarding on a SHORT page would not catch this — the page is not short.
    const call = async () => ({
      entries: [{ id: 1, timestamp: '2026-09-02T00:00:00Z' }],
      next_cursor: 0,
      dropped_before: null,
      buffer_enabled: true,
    });
    const written: string[] = [];

    await expect(dumpRing({ call, write: (l: string) => written.push(l) }))
      .rejects.toThrow(/cursor stalled/i);
    // It stops after one page rather than spinning.
    expect(written).toHaveLength(1);
  });

  it('writes every record exactly once, across page boundaries', async () => {
    const n = PAGE * 2 + 7; // deliberately not a multiple of the page size
    const written: string[] = [];

    const { records, truncated } = await dumpRing({ call: fakeRing(n), write: (l: string) => written.push(l) });

    expect(records).toBe(n);
    expect(written).toHaveLength(n);
    expect(truncated).toBe(false);
    const ids = written.map((l) => JSON.parse(l).id);
    expect(new Set(ids).size).toBe(n);          // no duplicates
    expect(ids[0]).toBe(1000);                  // nothing skipped at the head
    expect(ids[n - 1]).toBe(1000 + n - 1);      // nothing dropped at the tail
  });

  it('reports truncation rather than presenting a partial dump as whole', async () => {
    // Eviction mid-dump means the record has a hole. It keeps the records it
    // got — a partial dump still has evidence in it — but it must say so, in
    // the log AND in the return value, so the caller cannot mistake it for a
    // complete one. `main()` turns that flag into a non-zero exit.
    const ring = fakeRing(3);
    const call = async (op: string, args: { cursor?: number; limit: number }) => ({
      ...(await ring(op, args)),
      dropped_before: 3,
    });
    const logs: string[] = [];
    const written: string[] = [];

    const result = await dumpRing({
      call, write: (l: string) => written.push(l), log: (m: string) => logs.push(m),
    });

    expect(result.truncated).toBe(true);
    expect(result.records).toBe(3);
    expect(written).toHaveLength(3);
    expect(logs.join('\n')).toMatch(/INCOMPLETE/);
  });
});

describe('seekTo', () => {
  it('lands on the first entry at or after the target', async () => {
    const call = fakeRing(500);

    const cursor = await seekTo(call, '2026-09-02T00:01:40Z', 999, 1500);

    // 100 seconds in => id 1100. The cursor is exclusive, so the next read
    // starts AT that entry.
    const { entries } = await call('read_stream', { cursor, limit: 1 });
    expect(entries[0].id).toBe(1100);
  });

  it('returns the high bound when nothing is new enough', async () => {
    const call = fakeRing(100);

    const cursor = await seekTo(call, '2030-01-01T00:00:00Z', 999, 1200);

    const { entries } = await call('read_stream', { cursor, limit: 1 });
    expect(entries).toHaveLength(0);
  });
});
