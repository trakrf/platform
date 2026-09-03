import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import {
  pooledCohortConflict,
  runIdsOf,
  archivedRunIds,
  unarchivedRunIds,
  preflightReport,
  armCohortPreflight,
} from '../../scripts/soak-record-preflight.mjs';

/**
 * The guard that arms `describeCohorts`, and the one that notices an arm was
 * never archived.
 *
 * `cohortWarning` fires only on a NON-homogeneous record, so two vitest arms
 * that both carry `note=null` are one cohort and the banner stays silent while
 * every rate is computed over a doubled denominator. That is not hypothetical:
 * on 2026-09-02 `runs.jsonl` held 200 rows of the TRA-1237 after-arm, and the
 * runbook's launch line carries no `--note`, so the arm being launched would
 * have landed in the same cohort. Halved rates, no warning, indistinguishable
 * on sight from a correct summary.
 *
 * The guard is therefore at LAUNCH, where the two arms are still distinguishable
 * — after the fact they are 400 identical-looking rows. Same shape as every
 * other abort criterion: a precondition that can fail loudly is a guard, not a
 * sentence in a runbook nobody re-reads.
 *
 * Refs: TRA-1242.
 */

const row = (over: Record<string, unknown> = {}) => ({
  schema: 3,
  runner: 'vitest',
  shape: 'fixed',
  rep: 1,
  note: null,
  outputLog: '.suite-runs/output-2026-09-01T14-03-01-607Z-fixed-1.log',
  ...over,
});

describe('pooledCohortConflict', () => {
  it('fires when an existing cohort matches the arm being launched', () => {
    const records = [row(), row({ rep: 2 })];

    expect(pooledCohortConflict(records, { runner: 'vitest', note: null })).toEqual({
      runner: 'vitest',
      note: null,
      count: 2,
    });
  });

  it('is silent on an empty record — the ordinary first arm', () => {
    expect(pooledCohortConflict([], { runner: 'vitest', note: null })).toBeNull();
  });

  it('is silent when --note separates the arms', () => {
    // The fallback the runbook can offer. It works, which is exactly why the
    // guard has to exist: nothing made anyone type it.
    const records = [row({ note: 'tra-1237-after' })];

    expect(pooledCohortConflict(records, { runner: 'vitest', note: 'tra-1239-after' })).toBeNull();
  });

  it('is silent when the runners differ', () => {
    const records = [row({ runner: 'e2e' })];

    expect(pooledCohortConflict(records, { runner: 'vitest', note: null })).toBeNull();
  });

  it('treats a record with no runner field as vitest', () => {
    // Same rule describeCohorts follows: every row written before TRA-1206
    // carries no `runner` and IS a vitest row. Reading the raw field would let
    // the oldest and most valuable archives pool silently.
    const records = [row({ runner: undefined })];

    expect(pooledCohortConflict(records, { runner: 'vitest', note: null })).toMatchObject({ count: 1 });
  });

  it('counts only the matching cohort, not the whole file', () => {
    const records = [row(), row({ runner: 'e2e' }), row({ note: 'other' }), row({ rep: 2 })];

    expect(pooledCohortConflict(records, { runner: 'vitest', note: null })).toMatchObject({ count: 2 });
  });
});

describe('runIdsOf', () => {
  it('reads the per-invocation stamp out of outputLog', () => {
    const records = [row(), row({ rep: 2, outputLog: '.suite-runs/output-2026-09-01T14-03-01-607Z-fixed-2.log' })];

    expect(runIdsOf(records)).toEqual(['2026-09-01T14-03-01-607Z']);
  });

  it('omits rows whose log name carries no stamp rather than inventing one', () => {
    // Schema-1 rows predate RUN_ID. "Cannot tell" is not "archived": they are
    // left out of the run-id list and the report says how many it could not
    // place, so an unarchivable row never reads as a checked one.
    const records = [row({ outputLog: '.suite-runs/output-fixed-1.log' }), row()];

    expect(runIdsOf(records)).toEqual(['2026-09-01T14-03-01-607Z']);
  });
});

describe('archivedRunIds', () => {
  it('recognises a run by a per-rep log filename, not by the directory name', () => {
    // Archive directories are named for what the arm WAS
    // (`2026-09-01-tra1237-after-arm`), which carries no run id. The stamp only
    // ever appears on the per-rep logs inside.
    const names = [
      '2026-09-01-tra1237-after-arm',
      'output-2026-09-01T14-03-01-607Z-fixed-1.log',
      'runs.jsonl',
    ];

    expect(archivedRunIds(names)).toEqual(new Set(['2026-09-01T14-03-01-607Z']));
  });
});

describe('unarchivedRunIds', () => {
  it('names the runs whose logs exist in exactly one place', () => {
    const records = [row()];

    expect(unarchivedRunIds(records, new Set())).toEqual(['2026-09-01T14-03-01-607Z']);
  });

  it('is empty once the run has been archived', () => {
    const records = [row()];

    expect(unarchivedRunIds(records, new Set(['2026-09-01T14-03-01-607Z']))).toEqual([]);
  });
});

describe('preflightReport', () => {
  it('reports nothing to say when the record is empty', () => {
    const report = preflightReport({ records: [], runner: 'vitest', note: null, archived: new Set() });

    expect(report.conflict).toBeNull();
    expect(report.message).toBe('');
  });

  it('names the cohort, the count and the escape hatches', () => {
    const report = preflightReport({
      records: [row(), row({ rep: 2 })],
      runner: 'vitest',
      note: null,
      archived: new Set(['2026-09-01T14-03-01-607Z']),
    });

    expect(report.conflict).toMatchObject({ count: 2 });
    expect(report.message).toContain('runner=vitest');
    expect(report.message).toContain('--note');
    expect(report.message).toContain('--allow-pooling');
  });

  it('escalates when those rows were never archived', () => {
    // Gap 1: the TRA-1237 after-arm — the arm its ticket was CLOSED on — had no
    // directory under ~/soak-archives/ at all. Its 200 per-rep logs existed in
    // one gitignored place that the next arm overwrites. Recurrence one arm
    // after TRA-1226 recorded the same near-miss makes it a missing guard.
    const report = preflightReport({
      records: [row()],
      runner: 'vitest',
      note: null,
      archived: new Set(),
    });

    expect(report.unarchived).toEqual(['2026-09-01T14-03-01-607Z']);
    expect(report.message).toContain('NOT ARCHIVED');
  });

  it('says so when a conflicting cohort IS archived, so the operator knows what is safe to move', () => {
    const report = preflightReport({
      records: [row()],
      runner: 'vitest',
      note: null,
      archived: new Set(['2026-09-01T14-03-01-607Z']),
    });

    expect(report.unarchived).toEqual([]);
    expect(report.message).not.toContain('NOT ARCHIVED');
  });

  it('warns about an unarchived run even when there is no cohort conflict', () => {
    // A different runner does not pool, so the arm may proceed — but the
    // previous arm's logs are still one `rm` from gone, and this is the one
    // moment somebody is present to hear it.
    const report = preflightReport({
      records: [row({ runner: 'e2e' })],
      runner: 'vitest',
      note: null,
      archived: new Set(),
    });

    expect(report.conflict).toBeNull();
    expect(report.message).toContain('NOT ARCHIVED');
  });
});

describe('armCohortPreflight', () => {
  const silent = { warn: () => {}, error: () => {} };

  it('starts a clean arm without saying anything', () => {
    const gate = armCohortPreflight({
      records: [],
      runner: 'vitest',
      archived: new Set(),
      ...silent,
    });

    expect(gate.blocked).toBe(false);
    expect(gate.report.message).toBe('');
  });

  it('blocks the arm that would pool', () => {
    const gate = armCohortPreflight({
      records: [row()],
      runner: 'vitest',
      archived: new Set(['2026-09-01T14-03-01-607Z']),
      ...silent,
    });

    expect(gate.blocked).toBe(true);
  });

  it('lets --allow-pooling through, having said so', () => {
    // The right flag when you MEANT to extend the previous arm with more reps
    // of the same campaign. Never the way past a surprise, which is why it is a
    // flag somebody types rather than a default.
    const said: string[] = [];
    const gate = armCohortPreflight({
      records: [row()],
      runner: 'vitest',
      allowPooling: true,
      archived: new Set(['2026-09-01T14-03-01-607Z']),
      warn: (m: string) => said.push(m),
      error: (m: string) => said.push(m),
    });

    expect(gate.blocked).toBe(false);
    expect(said.join('\n')).toContain('starting into the existing cohort');
  });

  it('does not block on an unarchived run alone', () => {
    // Whether the previous arm was archived and whether THIS arm pools into it
    // are different questions. The first is somebody else's evidence; only the
    // second becomes unrecoverable by starting.
    const gate = armCohortPreflight({
      records: [row({ runner: 'e2e' })],
      runner: 'vitest',
      archived: new Set(),
      ...silent,
    });

    expect(gate.blocked).toBe(false);
    expect(gate.report.message).toContain('NOT ARCHIVED');
  });
});
