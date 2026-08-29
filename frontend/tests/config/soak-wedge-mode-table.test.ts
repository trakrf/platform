import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { wedgeModeTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * TRA-1200 asked for this explicitly: *"those 33 are two distinct failure modes
 * scored as one number; if the driver can separate them, do — but the aggregate
 * is the registered comparison."*
 *
 * It could not be done until read-cycle values were recorded, because both modes
 * are defined by read COUNTS rather than by any log needle:
 *
 *   mode 1  first == 0            the scan path is dead   31/407 = 7.62% on knuckles
 *   mode 2  first == second, > 0  frozen accumulation      2/407 = 0.49%
 *
 * ⚠ `0 reads` being the dominant signature is why this table cannot default a
 * missing reading to zero: every unscoreable rep would be counted as mode 1, and
 * the rarest failure in the campaign would become the commonest.
 */
const rep = (firstReads: number | null, secondReads: number | null) => ({
  runner: 'e2e',
  readCycles: { firstReads, firstUnique: firstReads, secondReads, secondUnique: secondReads },
});

describe('wedgeModeTable', () => {
  it('scores mode 1 — a dead scan path — from a zero first cycle', () => {
    const out = wedgeModeTable([rep(0, 0), rep(98, 196), rep(98, 196)]);

    expect(out).toMatch(/1\/3/);
  });

  it('scores mode 2 — frozen accumulation — where the two cycles are equal and non-zero', () => {
    const out = wedgeModeTable([rep(259, 259), rep(98, 196)]);

    expect(out).toMatch(/frozen/i);
    expect(out).toMatch(/1\/2/);
  });

  /**
   * The distinction the whole table rests on. A rep with 0 reads is mode 1; a
   * rep whose reads were never observed is not scoreable at all. Counting the
   * second as the first would manufacture the dominant wedge signature out of a
   * missing log.
   */
  it('excludes unscoreable reps rather than counting them as mode 1', () => {
    const out = wedgeModeTable([rep(null, null), rep(98, 196)]);

    expect(out).toMatch(/0\/1/);
    expect(out).toMatch(/1 (rep|row)/i);
  });

  it('does not call a zero-first rep frozen as well, though 0 == 0', () => {
    // rep(0, 0) satisfies first === second numerically. Mode 2 requires > 0, or
    // every dead scan path would be double-counted into both modes.
    const out = wedgeModeTable([rep(0, 0)]);
    const frozenLine = out.split('\n').find((l: string) => /frozen/i.test(l));

    expect(frozenLine).toMatch(/0\/1/);
  });

  it('prints the knuckles reference beside each mode', () => {
    const out = wedgeModeTable([rep(98, 196)]);

    expect(out).toMatch(/31\/407/);
    expect(out).toMatch(/2\/407/);
  });

  it('says so plainly when nothing is scoreable', () => {
    expect(wedgeModeTable([{ runner: 'vitest' }])).toMatch(/no rep/i);
  });
});
