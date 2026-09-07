import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { unattributableFailureTable, perRunTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * TRA-1243. Three reps of the TRA-1239 after-arm exited non-zero with no failing
 * spec, no failing assertion and a report that parsed fine — an unhandled
 * CommandManager rejection escaping a floating promise. The headline `failed`
 * count includes them; the per-spec failure table cannot see them. Two counters
 * measuring different populations, and 43% of that arm's failures were invisible
 * to the per-spec view.
 *
 * The point of the row is what happens NEXT: the catch-and-log that fixes the
 * escape turns those reps into clean passes, dropping the arm's headline rate by
 * ~1.6% for a reason that is not a fix. With this row landed first the movement
 * is explained by the instrument rather than credited to the change.
 */

const rep = (
  n: number,
  overrides: Record<string, unknown> = {}
): Record<string, unknown> => ({
  shape: 'fixed',
  rep: n,
  seed: null,
  target: null,
  durationMs: 148_000,
  exitCode: 0,
  files: [{ name: 'tests/integration/cs108/locate.spec.ts', status: 'passed', failed: [] }],
  ...overrides,
});

/** Exit 1, report parsed, nothing named. This ticket's signature. */
const unattributable = (n: number) => rep(n, { exitCode: 1 });

/** An ordinary failure: the report names the spec and the assertion. */
const attributed = (n: number) =>
  rep(n, {
    exitCode: 1,
    files: [
      {
        name: 'tests/integration/cs108/locate.spec.ts',
        status: 'failed',
        failed: ['locate > holds the gauge'],
      },
    ],
  });

/** No parseable report at all — a broken capture, not a verdict. */
const voidCapture = (n: number) => rep(n, { exitCode: 1, files: [], reportMissing: true });

describe('unattributableFailureTable', () => {
  it('counts a rep that exited non-zero while naming no failing test', () => {
    const out = unattributableFailureTable([unattributable(52), rep(1), rep(2)]);

    expect(out).toMatch(/no failing test/i);
    expect(out).toMatch(/1\/3/);
  });

  it('does not count a failure whose report names a failing spec', () => {
    const out = unattributableFailureTable([attributed(7), rep(1), rep(2)]);

    expect(out).toMatch(/0\/3/);
  });

  /**
   * The third clause of the detector, and the one that is easy to drop. A rep
   * whose report never parsed also has no file with status 'failed' — so a
   * two-clause detector silently absorbs every broken capture into this class
   * and reports a defect rate built out of missing data.
   */
  it('excludes a rep whose report was missing, and gives it its own row', () => {
    const out = unattributableFailureTable([voidCapture(9), rep(1), rep(2)]);

    expect(out).toMatch(/0\/3/);
    expect(out).toMatch(/report missing|no parseable report/i);
    expect(out).toMatch(/1\/3/);
  });

  /**
   * The reps have to be nameable, because the follow-up is to window the bridge
   * ring dump to each one's startedAt/endedAt — which is how the mechanism was
   * confirmed in the first place.
   */
  it('names the offending reps so they can be windowed', () => {
    const out = unattributableFailureTable([unattributable(52), unattributable(185), rep(1)]);

    expect(out).toMatch(/fixed#52/);
    expect(out).toMatch(/fixed#185/);
  });

  /**
   * Exhaustive and mutually exclusive: the arithmetic has to be checkable by a
   * reader, or the row is one more number to take on trust.
   */
  it('classifies every rep exactly once, so the rows sum to n', () => {
    const out = unattributableFailureTable([
      rep(1),
      rep(2),
      attributed(3),
      unattributable(4),
      voidCapture(5),
    ]);

    expect(out).toMatch(/2\/5/); // passed
    expect(out).toMatch(/1\/5/); // each of the three failure verdicts
    expect(out).not.toMatch(/[^\d]0\/5/);
  });

  it('says plainly that none occurred rather than leaving a bare zero', () => {
    const out = unattributableFailureTable([rep(1), rep(2)]);

    expect(out).toMatch(/[Nn]o rep/);
  });

  /**
   * The instrument's job here is to survive the fix. When the catch-and-log
   * lands, this row goes to zero while `startScanFailed`/`stopScanFailed` stay
   * non-zero — the occurrence is still counted, just no longer conflated with a
   * rep verdict. A reader who does not know that reads the drop as a fix.
   */
  it('warns that the class going to zero is not by itself evidence of a fix', () => {
    expect(unattributableFailureTable([unattributable(52)])).toMatch(/not.*(a fix|evidence)/i);
  });
});

describe('perRunTable', () => {
  it('flags the offending rep in the per-run table, so it is findable there too', () => {
    expect(perRunTable([unattributable(52)])).toMatch(/\*\*NO FAILING TEST\*\*/);
  });

  it('does not flag a failure that names a spec', () => {
    expect(perRunTable([attributed(7)])).not.toMatch(/NO FAILING TEST/);
  });

  it('does not flag a rep whose report was missing — that is already its own flag', () => {
    const out = perRunTable([voidCapture(9)]);

    expect(out).toMatch(/\*\*REPORT MISSING\*\*/);
    expect(out).not.toMatch(/NO FAILING TEST/);
  });
});
