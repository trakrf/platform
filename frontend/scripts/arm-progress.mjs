/**
 * Make an arm readable while it is running (TRA-1240).
 *
 * A 200-rep arm is ~7.3 hours. The driver already printed one line per rep —
 * that part of TRA-1240's description was wrong, and vitest output already went
 * to per-rep files rather than the driver log, so the 2026-09-01 driver log is
 * 203 lines for 200 reps. What was missing:
 *
 *   1. the line said `exit=1` and nothing about WHY, so every diagnosis meant
 *      opening a per-rep log;
 *   2. failing specs printed as full paths, so a wedge rep — the one most worth
 *      reading — was a single ~350 character line of five absolute paths;
 *   3. nothing aggregated, so "is this arm going wrong" could not be answered
 *      without stopping to summarise a partial file.
 *
 * ## Pure functions of the record, deliberately
 *
 * Neither formatter touches the filesystem, the clock, or `runs.jsonl`. The same
 * code renders a live rep and an archived one, which is what makes the five arms
 * already on disk readable and what makes any of this unit-testable.
 *
 * ## What is NOT here, and why
 *
 * No `DROP=` field for `targetEPC did NOT reach the radio`. That line is not a
 * recorded signal, and adding a needle for it would change `runs.jsonl` in the
 * middle of a campaign whose whole point is comparison against the 2026-09-01
 * baseline. It is the sharpest signal we have for TRA-1237 and it still belongs
 * in the record — after the arm, deliberately, not smuggled in beside a
 * formatting change.
 */

import path from 'node:path';

/**
 * Sum a per-op signal table, preserving the null the instrument records.
 *
 * `null` means the runner could not observe this at all. Summing it to 0 would
 * assert a clean measurement where none was taken — the same null-vs-zero rule
 * `suite-run-signals.mjs` spells out for every needle in the table, and the one
 * that makes an absent log indistinguishable from a healthy rep if broken.
 */
function sumTable(table) {
  if (table === null || table === undefined) return null;
  return Object.values(table).reduce((a, b) => a + b, 0);
}

/** Render a possibly-unknown number. `?` is not decoration; see sumTable. */
const show = (n) => (n === null || n === undefined ? '?' : String(n));

/** `tests/integration/cs108/locate.spec.ts` → `locate` */
export function specName(file) {
  return path.basename(file).replace(/\.(spec|test)\.tsx?$/, '');
}

/** The specs a record reports as failed, by short name. */
export function failedSpecs(record) {
  return (record.files ?? []).filter((f) => f.status === 'failed').map((f) => specName(f.name));
}

/**
 * One line for one rep — pass/fail, how long, which specs, and why.
 *
 * The signal set is chosen to discriminate, not to be complete: `rej` and `to`
 * separate a refusal from silence (the distinction the whole TRA-1223
 * investigation turned on), `err` is the 0xA101 fault count, and `ack` collapses
 * to a fraction of its healthy ~300 the moment a rep stops doing work — which is
 * what a wedge looks like from outside.
 */
export function formatRepLine(record, total) {
  const signals = record.signals ?? {};
  const specs = failedSpecs(record);
  const verdict = record.exitCode === 0 ? 'pass' : 'FAIL';
  const seconds = `${Math.round((record.durationMs ?? 0) / 1000)}s`;

  const parts = [
    `rep ${String(record.rep).padStart(String(total).length)}/${total}`,
    seconds.padStart(5),
    verdict.padEnd(4),
    `rej=${show(sumTable(signals.commandRejections))}`,
    `to=${show(sumTable(signals.commandTimeouts))}`,
    `err=${show(signals.errorNotifications)}`,
    `ack=${show(signals.ackSamples)}`
  ];

  if (specs.length) parts.push(`| ${specs.join(',')}`);
  if (record.reportMissing) parts.push('[REPORT MISSING]');
  if (signals.logMissing) parts.push('[LOG MISSING]');

  return parts.join('  ');
}

const median = (xs) => {
  if (!xs.length) return null;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)];
};

function humanDuration(ms) {
  if (ms === null || !Number.isFinite(ms) || ms < 0) return '?';
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  return h ? `${h}h${String(m).padStart(2, '0')}m` : `${m}m`;
}

/**
 * The block that answers "is this arm worth continuing".
 *
 * ## The strip is the point, not the totals
 *
 * A wedge is a RUN of consecutive failures. Totals cannot show one: seven
 * scattered failures and seven consecutive are the same number and completely
 * different arms — the first is flake, the second is the device stopping. The
 * 2026-09-01 wedge was reps 137-143, and in the driver log it was visible only
 * to somebody who read seven consecutive lines and noticed.
 *
 * ETA comes from the observed median rep, not from a constant. Failing reps are
 * usually FASTER (a wedged rep aborts early — 14s against a healthy 135s), so an
 * arm that starts failing appears to speed up. A median over the reps actually
 * run tracks that; a hard-coded 131s does not, and would quietly under-report
 * remaining time on exactly the arm somebody most wants to reason about.
 */
export function formatProgressBlock(records, total, startedAtMs, now = Date.now()) {
  const done = records.length;
  const failures = records.filter((r) => r.exitCode !== 0);
  const rate = done ? ((100 * failures.length) / done).toFixed(1) : '0.0';

  const elapsed = now - startedAtMs;
  const med = median(records.map((r) => r.durationMs).filter((d) => Number.isFinite(d)));
  const remaining = med === null ? null : med * Math.max(0, total - done);

  const specCounts = new Map();
  for (const record of failures) {
    for (const name of failedSpecs(record)) specCounts.set(name, (specCounts.get(name) ?? 0) + 1);
  }
  const specs = [...specCounts.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => `${name} ${n}`)
    .join('  ') || 'none';

  const sumAll = (key) =>
    records.reduce((acc, r) => {
      const v = sumTable(r.signals?.[key]);
      return acc === null || v === null ? null : acc + v;
    }, 0);
  const sumScalar = (key) =>
    records.reduce((acc, r) => {
      const v = r.signals?.[key];
      return acc === null || v === null || v === undefined ? null : acc + v;
    }, 0);

  // X for a failure, . for a pass. Deliberately one character each: a run has to
  // be visible as a shape rather than counted.
  const strip = records.slice(-40).map((r) => (r.exitCode === 0 ? '.' : 'X')).join('');

  return [
    `--- ${done}/${total} · elapsed ${humanDuration(elapsed)} · eta ${humanDuration(remaining)} ${'-'.repeat(20)}`,
    `  passed ${done - failures.length}  failed ${failures.length}  (${rate}%)`,
    `  failing specs: ${specs}`,
    `  signals: rejections ${show(sumAll('commandRejections'))}  timeouts ${show(sumAll('commandTimeouts'))}  errNotif ${show(sumScalar('errorNotifications'))}`,
    `  last ${Math.min(40, done)}: ${strip}`
  ].join('\n');
}
