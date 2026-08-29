import { describe, it, expect } from 'vitest';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
// @ts-expect-error — .mjs instrument module, no types by design
import { densityTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * TRA-1200. A bare distribution would not have caught the thing this exists to
 * catch. The arm's field was ~17% short on unique tags and that only became
 * visible when it was put beside the 2026-08-23 numbers — so printing the
 * reference baseline is what turns a statistic into a check.
 */
const rep = (firstUnique: number | null, secondUnique: number | null) => ({
  runner: 'e2e',
  readCycles: {
    firstReads: firstUnique === null ? null : firstUnique * 2,
    firstUnique,
    secondReads: secondUnique === null ? null : secondUnique * 2,
    secondUnique,
  },
});

describe('densityTable', () => {
  it('reports mean, median, min and max for the unique-tag counts', () => {
    const out = densityTable([rep(60, 100), rep(70, 110), rep(65, 105)]);

    expect(out).toMatch(/firstUnique/);
    expect(out).toMatch(/mean 65/);
    expect(out).toMatch(/min 60/);
    expect(out).toMatch(/max 70/);
  });

  it('prints the reference baseline beside the measured values', () => {
    const out = densityTable([rep(65, 107)]);

    // knuckles 2026-08-23, n=407 — the arm this milestone compares against.
    expect(out).toMatch(/mean 83, median 81/);
    expect(out).toMatch(/mean 125, median 127/);
  });

  /**
   * Read volume is confounded with the variable a CPU-swap arm measures: a
   * faster host stops scanning sooner and accumulates fewer reads inside the
   * fixed 2s window. Two hosts facing an identical pile can disagree by 40%.
   * The warning has to travel with the table or the wrong column gets used.
   */
  it('warns that unique is the field proxy and reads is not', () => {
    expect(densityTable([rep(65, 107)])).toMatch(/[Uu]nique.*field proxy/);
  });

  it('says so plainly when no rep recorded read cycles, rather than printing zeros', () => {
    const out = densityTable([{ runner: 'vitest' }, { runner: 'vitest' }]);

    expect(out).toMatch(/No rep recorded read cycles/);
    expect(out).not.toMatch(/mean 0/);
  });

  /**
   * Every record written before this instrument lacks the field, so the table
   * must reconstruct from the retained log — otherwise every archived run,
   * TRA-1200's own 150 reps included, reads as having no density at all. The
   * count is surfaced because a reader who cannot tell recorded from
   * reconstructed cannot tell which runs actually had the instrument.
   */
  it('reconstructs from a retained log and says how many rows it rebuilt', () => {
    const dir = mkdtempSync(path.join(tmpdir(), 'summary-'));
    const log = path.join(dir, 'out.log');
    writeFileSync(
      log,
      '[Test] First read: 98 reads, 63 unique tags\n' +
        '[Test] Second read: 196 reads, 102 unique tags\n'
    );

    const out = densityTable([{ runner: 'e2e', outputLog: log }]);

    expect(out).toMatch(/63/);
    expect(out).toMatch(/1 row\(s\) reconstructed/);
  });
});
