import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { powerOffWindowTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * The section that stops a quiet arm from being read as a passing one.
 *
 * TRA-1217 made the CS108's silent window survivable, which removed the symptom
 * that used to reveal it: reps stopped dying and `link-close` stopped firing. So
 * "the window recurred and was absorbed" and "the window never happened" now
 * produce an identical rep table, and only these counts separate them.
 *
 * ⚠ The verdicts are the point, not the numbers. A reader who sees three zeroes
 * and a green suite will conclude the fix works unless the report says
 * otherwise, and eight hours of hardware time is exactly the wrong place to
 * leave that inference to the reader. Each test below pins one verdict.
 *
 * Refs: TRA-1223.
 */

/** A rep record carrying a verified capture with the given signal counts. */
function rep(signals: Record<string, number | null>) {
  return { signals: { logMissing: false, ...signals } };
}

describe('powerOffWindowTable', () => {
  it('refuses to score reps whose capture went missing', () => {
    // A void capture measured nothing. Reporting it as zero would manufacture
    // the quiet-night verdict out of a broken log — the null-vs-zero rule the
    // rest of this instrument already enforces.
    const out = powerOffWindowTable([{ signals: { logMissing: true } }]);

    expect(out).toMatch(/unobservable/i);
    expect(out).not.toMatch(/never went silent/i);
  });

  it('says a quiet run proves NOTHING, rather than reporting it as clean', () => {
    const out = powerOffWindowTable([
      rep({ powerOffTimeouts: 0, toleratedPowerOffs: 0, modeSwitchFailed: 0 }),
      rep({ powerOffTimeouts: 0, toleratedPowerOffs: 0, modeSwitchFailed: 0 }),
    ]);

    expect(out).toMatch(/NOTHING about TRA-1217/);
    // The reading that must never be offered by this section.
    expect(out).not.toMatch(/earns the fix its credit/);
  });

  it('credits the fix only when the window actually occurred', () => {
    const out = powerOffWindowTable([
      rep({ powerOffTimeouts: 18, toleratedPowerOffs: 3, modeSwitchFailed: 0 }),
      rep({ powerOffTimeouts: 0, toleratedPowerOffs: 0, modeSwitchFailed: 0 }),
    ]);

    expect(out).toMatch(/occurred and was absorbed/);
    expect(out).toMatch(/18/);
    // Reps affected, not reps total — one of the two was quiet.
    expect(out).toMatch(/1\/2/);
  });

  it('reports a cleanup failure as a regression, and says so ahead of the good news', () => {
    // Both conditions at once is the dangerous case: timeouts non-zero would
    // otherwise print the congratulatory verdict, and the regression is what
    // the reader needs first.
    const out = powerOffWindowTable([
      rep({ powerOffTimeouts: 20, toleratedPowerOffs: 2, modeSwitchFailed: 1 }),
    ]);

    expect(out).toMatch(/REGRESSION/);
    expect(out).not.toMatch(/occurred and was absorbed/);
  });

  it('does not sum a needle that cannot fire on this runner into a zero', () => {
    // e2e reps carry null for these — "cannot be asked" rather than "asked and
    // got none". Treating null as 0 would report the quiet-night verdict for a
    // runner on which the question was never posed.
    const out = powerOffWindowTable([
      rep({ powerOffTimeouts: null, toleratedPowerOffs: null, modeSwitchFailed: null }),
    ]);

    expect(out).toMatch(/unobservable/i);
    expect(out).not.toMatch(/never went silent/i);
  });
});
