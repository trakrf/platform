/**
 * Arm the mixed-cohort guard, and notice an arm that was never archived.
 *
 * ## Why this is a guard and not a runbook sentence
 *
 * `describeCohorts` groups on `runner` + `note`, and `cohortWarning` prints only
 * when the record is NON-homogeneous. Two vitest arms that both carry
 * `note=null` are therefore ONE cohort: the banner stays silent, and
 * `summarise-suite-runs.mjs` pools both into a single denominator.
 *
 * That happened. On 2026-09-02 `runs.jsonl` held 200 rows of the TRA-1237
 * after-arm (`2026-09-01T14-03-01-607Z`, `runner=vitest`, `note=null`) and the
 * runbook's launch line carries no `--note`, so the arm being launched would
 * have written 200 more rows into the same cohort:
 *
 *     before:  { 'vitest | note=null': 200 }
 *     after:   { 'vitest | note=null': 400 }   <- two arms, one denominator
 *
 * Every rate roughly halves, silently, and the result is indistinguishable on
 * sight from a correct one — which is the exact failure `describeCohorts` was
 * written to prevent. The guard is defeated by OMISSION: the thing that would
 * arm it is a flag nobody was ever asked for.
 *
 * So the check lives at LAUNCH, where the two arms are still distinguishable.
 * Afterwards they are 400 identical-looking rows and no tool can separate them.
 *
 * ## And why it also carries the archive check
 *
 * The TRA-1237 after-arm — the arm its ticket was CLOSED on — had no directory
 * under `~/soak-archives/` at all. Its 200 per-rep logs and its `runs.jsonl`
 * rows existed in exactly one gitignored place, and the driver overwrites those
 * logs on the next arm. TRA-1226 already recorded a near-miss of the same shape
 * one arm earlier, survived only because an operator happened to notice. Twice
 * is a missing instrument.
 *
 * Launch is the one moment somebody is present, and it is the last moment before
 * the overwrite. Both questions are asked there for the same reason.
 *
 * Every function here is pure except `readArchiveNames`, which is the only one
 * that touches a filesystem — the split is what makes the messages testable.
 *
 * Refs: TRA-1242.
 */

import { existsSync, readFileSync, readdirSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { runnerOf } from './suite-run-signals.mjs';

/** Where §6 says an arm's evidence goes. */
export const ARCHIVE_ROOT = path.join(os.homedir(), 'soak-archives');

/**
 * The per-invocation stamp the driver puts in every per-rep log name.
 *
 * `output-<RUN_ID>-<tag>-<rep>.log`, RUN_ID being an ISO timestamp with `:` and
 * `.` replaced by `-`. It is the only place a row records WHICH INVOCATION wrote
 * it, which makes it the join key between `runs.jsonl` and an archive directory.
 */
const RUN_ID_IN_LOG_NAME = /output-(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-\d{3}Z)-/;

/**
 * The cohort this arm would land in, if it already exists.
 *
 * `runnerOf`, not `record.runner`: every row written before TRA-1206 carries no
 * `runner` field and IS a vitest row. Reading the raw field would let the oldest
 * archives — the comparison baselines — pool in silence, which is the failure
 * this guard exists for rather than an edge case of it.
 */
export function pooledCohortConflict(records, { runner, note = null }) {
  const want = note ?? null;
  const matching = (records ?? []).filter(
    (r) => runnerOf(r) === runner && (r?.note ?? null) === want
  );
  return matching.length ? { runner, note: want, count: matching.length } : null;
}

/** The distinct invocations represented in a record, oldest first. */
export function runIdsOf(records) {
  const seen = [];
  for (const r of records ?? []) {
    // A row whose log name carries no stamp cannot be placed in an archive.
    // Omitted rather than guessed: "cannot tell" must never read as "archived".
    const id = RUN_ID_IN_LOG_NAME.exec(r?.outputLog ?? '')?.[1];
    if (id && !seen.includes(id)) seen.push(id);
  }
  return seen;
}

/**
 * The run ids visible in a set of archived file names.
 *
 * Keyed on the per-rep LOG names, never on the directory name: archives are
 * named for what the arm was (`2026-09-01-tra1237-after-arm`) and that carries
 * no stamp. Matching on the directory would report every arm archived the moment
 * anybody created a folder.
 */
export function archivedRunIds(names) {
  const ids = new Set();
  for (const name of names ?? []) {
    const id = RUN_ID_IN_LOG_NAME.exec(name)?.[1];
    if (id) ids.add(id);
  }
  return ids;
}

/** Runs present in the record whose logs are not also under the archive root. */
export function unarchivedRunIds(records, archived) {
  return runIdsOf(records).filter((id) => !archived.has(id));
}

/**
 * Every file name under the archive root, to a bounded depth.
 *
 * Depth 3 covers `<root>/<arm>/<file>` and `<root>/<arm>/analysis/<file>`, which
 * is every layout in use. A missing root is not an error — a machine that has
 * never archived an arm is exactly the machine this warns.
 */
export function readArchiveNames(root = ARCHIVE_ROOT, depth = 3) {
  if (depth === 0) return [];
  let entries;
  try {
    entries = readdirSync(root, { withFileTypes: true });
  } catch {
    return [];
  }
  return entries.flatMap((e) =>
    e.isDirectory() ? readArchiveNames(path.join(root, e.name), depth - 1) : [e.name]
  );
}

const cohortLabel = (c) => `runner=${c.runner} note=${c.note ?? '(none)'}`;

/**
 * What to say before an arm starts, and whether it may start at all.
 *
 * Returns `conflict: null, message: ''` on the ordinary first arm. A warning
 * that fires on every clean launch is one nobody reads by the time it matters —
 * the same rule `cohortWarning` follows.
 */
export function preflightReport({ records, runner, note = null, archived = new Set() }) {
  const conflict = pooledCohortConflict(records, { runner, note });
  const unarchived = unarchivedRunIds(records, archived);
  if (!conflict && !unarchived.length) return { conflict: null, unarchived: [], message: '' };

  const lines = [];

  if (conflict) {
    lines.push(
      `REFUSING TO START: .suite-runs/runs.jsonl already holds ${conflict.count} row(s) in`,
      `this arm's cohort (${cohortLabel(conflict)}).`,
      '',
      'describeCohorts groups on runner+note, so those rows and the ones this arm is',
      'about to write are ONE cohort. summarise-suite-runs.mjs would pool them into a',
      'single denominator and the mixed-record banner would stay SILENT, because by',
      'its own definition nothing is mixed. Every rate would come out roughly halved',
      'and would look entirely correct.'
    );
  }

  if (unarchived.length) {
    if (lines.length) lines.push('');
    lines.push(
      `⚠ NOT ARCHIVED — no file under ${ARCHIVE_ROOT}/ carries the stamp of:`,
      ...unarchived.map((id) => `    ${id}`),
      "  Those runs' per-rep logs exist in exactly one place and this arm overwrites",
      '  them. The TRA-1237 after-arm was lost this way and recovered only by hand.'
    );
  }

  if (conflict) {
    lines.push(
      '',
      'Archive the record first (§6), then move the rows out of the way:',
      `    cp -a .suite-runs ${ARCHIVE_ROOT}/<date>-<what-it-was>/`,
      '    # verify by file count and summed bytes, NOT du — see §6',
      `    mv .suite-runs/runs.jsonl ${ARCHIVE_ROOT}/<date>-<what-it-was>/runs.jsonl`,
      '',
      'Or give this arm a cohort of its own:',
      '    --note tra-1239-after',
      '',
      '--allow-pooling starts anyway. That is the right flag only when you MEANT to',
      'extend the previous arm with more reps of the same campaign.'
    );
  }

  return { conflict, unarchived, message: lines.join('\n') };
}

/** The record as rows, tolerating a truncated final line. */
export function readRecord(recordPath) {
  if (!recordPath || !existsSync(recordPath)) return [];
  return readFileSync(recordPath, 'utf8')
    .split('\n')
    .filter(Boolean)
    .flatMap((line) => {
      try {
        return [JSON.parse(line)];
      } catch {
        return [];
      }
    });
}

/**
 * The driver's launch gate: say what is wrong, and answer whether to start.
 *
 * Returns `{ blocked }` rather than exiting, so the decision is testable and
 * process control stays in the driver where it belongs.
 *
 * An unarchived run warns but never blocks: it is a previous arm's evidence, and
 * whether THIS arm pools is a separate question. A cohort conflict blocks,
 * because starting is what makes it unrecoverable.
 */
export function armCohortPreflight({
  recordPath,
  runner,
  note = null,
  allowPooling = false,
  records = readRecord(recordPath),
  archived = archivedRunIds(readArchiveNames()),
  warn = console.warn,
  error = console.error,
}) {
  const report = preflightReport({ records, runner, note, archived });
  if (!report.message) return { blocked: false, report };

  const blocked = Boolean(report.conflict) && !allowPooling;
  const speak = blocked ? error : warn;
  for (const line of report.message.split('\n')) speak(`[suite-runs] ${line}`);
  if (report.conflict && !blocked) {
    warn('[suite-runs] --allow-pooling given; starting into the existing cohort.');
  }
  return { blocked, report };
}
