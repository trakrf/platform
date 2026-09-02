import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { commandInFlightTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * The section that makes TRA-1239 answerable at 3am instead of by hand.
 *
 * A needle in `SIGNALS` reaches the per-rep record and stops there — the report
 * prints named sections, not the whole table. TRA-1237's own method note is the
 * warning: *a diagnostic nobody aggregates is a diagnostic that has not been
 * read.* The `did NOT reach the radio` line was logged loudly in every rep it
 * happened in for weeks and still took a contingency table to find.
 *
 * ⚠ The verdict is the deliverable, not the number. Unlike `powerOffWindowTable`
 * — where a zero proves nothing, because the device's silent window cannot be
 * summoned — a zero HERE is a real pass: the leak is host-side and deterministic
 * given a failed send, so an arm that exercised teardown and saw none is
 * evidence. That asymmetry is exactly the kind a reader gets backwards, so each
 * test below pins one verdict rather than a count.
 *
 * Refs: TRA-1239.
 */

/** A rep record carrying a verified capture with the given signal counts. */
function rep(signals: Record<string, number | null>) {
  return { signals: { logMissing: false, ...signals } };
}

describe('commandInFlightTable', () => {
  it('refuses to score reps whose capture went missing', () => {
    const out = commandInFlightTable([{ signals: { logMissing: true } }]);

    expect(out).toMatch(/unobservable/i);
    expect(out).not.toMatch(/never leaked/i);
  });

  it('separates a blind runner from a clean one', () => {
    // null means the needle cannot fire on this runner, which is a different
    // claim from zero. Summing them into one would report an e2e-only run as
    // proof the wire was never leaked.
    const out = commandInFlightTable([rep({ commandInFlight: null })]);

    expect(out).toMatch(/unobservable|vitest-only/i);
    expect(out).not.toMatch(/never leaked/i);
  });

  it('reads a clean arm as a real pass, unlike the silent-window section', () => {
    const out = commandInFlightTable([
      rep({ commandInFlight: 0 }),
      rep({ commandInFlight: 0 }),
    ]);

    expect(out).toMatch(/never leaked/i);
    expect(out).not.toMatch(/REGRESSION/);
  });

  it('says plainly that a clean arm only counts if teardown ran', () => {
    // The one way a zero here lies: the leak needs a failed send, and a failed
    // send needs a torn-down transport. An arm that never disconnected did not
    // test this.
    const out = commandInFlightTable([rep({ commandInFlight: 0 })]);

    expect(out).toMatch(/teardown/i);
  });

  it('calls any non-zero count a regression, with the reps named', () => {
    const out = commandInFlightTable([
      rep({ commandInFlight: 4 }),
      rep({ commandInFlight: 0 }),
      rep({ commandInFlight: 2 }),
    ]);

    expect(out).toMatch(/REGRESSION/);
    expect(out).toMatch(/6/);        // 4 + 2 lines
    expect(out).toMatch(/2\/3/);     // reps affected, not reps total
  });

  it('warns that the count is lines, not occurrences', () => {
    // Two lines per event. Reporting 26 as "26 collisions" is how the wrong
    // baseline got written the first time.
    const out = commandInFlightTable([rep({ commandInFlight: 26 })]);

    expect(out).toMatch(/line/i);
    expect(out).toMatch(/13/);       // the event count it implies
  });

  it('points at the slot, not at concurrency', () => {
    // The error's own message blames a concurrent caller. TRA-1239 was a
    // transport throw that never released the slot, and a reader sent looking
    // for two callers will not find one.
    const out = commandInFlightTable([rep({ commandInFlight: 1 })]);

    expect(out).toMatch(/slot|release/i);
  });
});
