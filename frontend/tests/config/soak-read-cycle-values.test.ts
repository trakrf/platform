import { describe, it, expect } from 'vitest';
import { writeFileSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import path from 'path';
// @ts-expect-error — .mjs instrument module, no types by design
import { readReadCycles, READ_CYCLE_FIELDS } from '../../scripts/suite-run-signals.mjs';

/**
 * TRA-1200. Field density is the run condition whose absence let an arm be
 * compared against a reference it did not match: the reader had been pulled back
 * from the tag stack to gun a barcode, the field ran ~17% sparser than the
 * 2026-08-23 baseline, and nothing recorded it. It surfaced only by parsing 150
 * logs after the run was over. Cell A was halted for the same shortfall on
 * 2026-08-23 and diagnosed the same way. Twice is a missing instrument.
 *
 * These values are EXTRACTED, not counted, which is why they live beside the
 * needle table rather than in it — and why null-vs-zero is load-bearing here in
 * a way it is not for an occurrence count.
 */
const withLog = (text: string) => {
  const dir = mkdtempSync(path.join(tmpdir(), 'readcycles-'));
  const file = path.join(dir, 'out.log');
  writeFileSync(file, text);
  return file;
};

const allNull = () => Object.fromEntries(READ_CYCLE_FIELDS.map((k: string) => [k, null]));

describe('readReadCycles', () => {
  it('extracts both cycles from an e2e log', () => {
    const log = withLog(
      '[Test] First read: 98 reads, 63 unique tags\n' +
        '[Test] Second read: 196 reads, 102 unique tags\n'
    );

    expect(readReadCycles(log, 'e2e')).toEqual({
      firstReads: 98,
      firstUnique: 63,
      secondReads: 196,
      secondUnique: 102,
    });
  });

  /**
   * The whole reason this module cannot default a missing value to 0.
   * `first == 0` is TRA-1150's DOMINANT wedge signature — 31 of its 33 wedges,
   * a scan path that is dead rather than thin. A zero substituted for an absent
   * reading would fabricate the exact failure the soak exists to detect.
   */
  it('keeps a genuine zero distinct from an absent value', () => {
    const log = withLog('[Test] First read: 0 reads, 0 unique tags\n');
    const out = readReadCycles(log, 'e2e');

    expect(out.firstReads).toBe(0);
    expect(out.firstUnique).toBe(0);
    expect(out.secondReads).toBeNull();
    expect(out.secondUnique).toBeNull();
  });

  it('reports every field as null for a vitest rep, which has no read cycles', () => {
    const log = withLog('[Test] First read: 98 reads, 63 unique tags\n');

    expect(readReadCycles(log, 'vitest')).toEqual(allNull());
  });

  it('reports every field as null when the log is gone', () => {
    expect(readReadCycles('/nonexistent/out.log', 'e2e')).toEqual(allNull());
  });

  /**
   * The pattern is /g and therefore stateful. A module-level regex shared across
   * calls carries `lastIndex`, so the second call in a process silently skips
   * its first match — a per-rep instrument that is wrong on every rep but the
   * first, which is precisely the shape that survives review.
   */
  it('does not carry regex state between calls', () => {
    const log = withLog(
      '[Test] First read: 98 reads, 63 unique tags\n' +
        '[Test] Second read: 196 reads, 102 unique tags\n'
    );

    expect(readReadCycles(log, 'e2e')).toEqual(readReadCycles(log, 'e2e'));
  });
});
