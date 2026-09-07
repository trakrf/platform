import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { signalPairingTable } from '../../scripts/summarise-suite-runs.mjs';
// @ts-expect-error — .mjs instrument module, no types by design
import { SIGNALS } from '../../scripts/suite-run-signals.mjs';

/**
 * The cross-tabulation has to partition on the REP's verdict, not on whether the
 * report named a failed file. Those differ for exactly the class TRA-1243 is
 * about, and the difference inverted this table's own printed conclusion.
 *
 * On `~/soak-archives/2026-09-02-tra1239-after-arm` the three reps carrying
 * `[Reader] Failed to start/stop scanning:` are 52, 174 and 185 (stop, start,
 * stop limbs) — the same three reps that exited non-zero naming no test.
 * Partitioning on failed files filed all three as PASSES, so the table read
 *
 *     suite failed | 0 | 4        →  "No failure coincided with a scan-start
 *     suite passed | 3 | 193          error ... evidence AGAINST scan-start
 *                                     being the mechanism"
 *
 * on an arm where the correlation is 3/3 in favour of it. Read off the rep
 * verdict the same records give
 *
 *     suite failed | 3 | 4
 *     suite passed | 0 | 193
 *
 * — a perfect discriminator, and the false conclusion is not printed at all.
 * A wrong cell is a number someone can check; a wrong sentence is one they act on.
 */
describe('signalPairingTable', () => {
  /** Every current needle must be present, or the record is treated as stale. */
  const withSignals = (over: Record<string, unknown>) => ({
    ...Object.fromEntries(Object.keys(SIGNALS).map((k) => [k, 0])),
    logMissing: false,
    harnessLines: 12,
    commandTimeouts: {},
    ...over,
  });

  const rep = (n: number, over: Record<string, unknown> = {}) => ({
    shape: 'fixed',
    rep: n,
    exitCode: 0,
    files: [{ name: 'tests/integration/cs108/locate.spec.ts', status: 'passed', failed: [] }],
    runner: 'vitest',
    signals: withSignals({ stopScanFailed: 1 }),
    ...over,
  });

  it('puts a rep that exited non-zero naming no test in the FAILED row', () => {
    const out = signalPairingTable([rep(52, { exitCode: 1 })]);

    expect(out).toMatch(/\*\*suite failed\*\* \| 1 \| 0 \|/);
    expect(out).toMatch(/\*\*suite passed\*\* \| 0 \| 0 \|/);
  });

  /** The inverted headline is the actual harm — a reader acts on the sentence. */
  it('does not claim the signature is absent from failures when it is present in one', () => {
    expect(signalPairingTable([rep(52, { exitCode: 1 })])).not.toMatch(/No failure coincided/);
  });

  it('still reads a genuine pass as a pass', () => {
    expect(signalPairingTable([rep(1)])).toMatch(/\*\*suite passed\*\* \| 1 \| 0 \|/);
  });

  /**
   * A rep with no parseable report is not a pass either. `contaminationNote` has
   * said so since it was written — "recorded failures of the run, not passes" —
   * and this table disagreed with it.
   */
  it('does not read a rep with no parseable report as a pass', () => {
    const out = signalPairingTable([rep(9, { exitCode: 1, files: [], reportMissing: true })]);

    expect(out).toMatch(/\*\*suite passed\*\* \| 0 \| 0 \|/);
  });
});
