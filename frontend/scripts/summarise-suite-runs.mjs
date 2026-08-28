#!/usr/bin/env node
/**
 * Summarise the run-shape record produced by characterise-suite-runs.mjs.
 *
 * Pure function of .suite-runs/runs.jsonl. Safe to re-run at any time; it never
 * touches hardware and never mutates the record.
 *
 * The load-bearing output is the PREDECESSOR table. Order-dependence shows up
 * as "locate.spec.ts fails when it runs after inventory.spec.ts" — a flat
 * failure count cannot show that, and reading a flat count as evidence of a
 * leak is the mistake this whole tool exists to prevent.
 */

import { readFileSync, existsSync } from 'node:fs';
import path from 'node:path';
import { resolveSignals } from './suite-run-signals.mjs';

const RECORD_PATH = path.resolve(process.cwd(), '.suite-runs', 'runs.jsonl');

function loadRecords() {
  if (!existsSync(RECORD_PATH)) {
    console.error(`No record at ${RECORD_PATH} — run characterise-suite-runs.mjs first.`);
    process.exit(1);
  }
  return readFileSync(RECORD_PATH, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));
}

const short = (f) => f.replace(/^tests\/integration\//, '');

function perRunTable(records) {
  const lines = [
    '| shape | rep | seed | dur | exit | clients@start | failed files | failed tests |',
    '| -- | -- | -- | -- | -- | -- | -- | -- |',
  ];
  for (const r of records) {
    const failedFiles = r.files.filter((f) => f.status === 'failed');
    const names = failedFiles.length ? failedFiles.map((f) => `\`${short(f.name)}\``).join('<br>') : '—';
    const tests = failedFiles.flatMap((f) => f.failed);
    const testCell = tests.length ? tests.map((t) => `${t}`).join('<br>') : '—';
    const flags = [];
    if (r.reportMissing) flags.push('**REPORT MISSING**');
    if (r.wsClientsAtStart) flags.push(`**${r.wsClientsAtStart} client(s) attached**`);
    lines.push(
      `| ${r.shape}${r.target ? ` (${short(r.target)})` : ''} | ${r.rep} | ${r.seed ?? '—'} ` +
        `| ${Math.round(r.durationMs / 1000)}s | ${r.exitCode} | ${r.wsClientsAtStart ?? '?'} ` +
        `| ${names} | ${testCell} ${flags.length ? `<br>${flags.join(' ')}` : ''}|`
    );
  }
  return lines.join('\n');
}

function perFileTable(records) {
  // file -> shape -> {runs, failures}
  const stats = new Map();
  const shapes = [...new Set(records.map((r) => r.shape))];
  for (const r of records) {
    for (const f of r.files) {
      if (!stats.has(f.name)) stats.set(f.name, new Map());
      const byShape = stats.get(f.name);
      if (!byShape.has(r.shape)) byShape.set(r.shape, { runs: 0, failures: 0 });
      const cell = byShape.get(r.shape);
      cell.runs += 1;
      if (f.status === 'failed') cell.failures += 1;
    }
  }
  const lines = [
    `| file | ${shapes.join(' | ')} | total |`,
    `| -- | ${shapes.map(() => '--').join(' | ')} | -- |`,
  ];
  for (const [file, byShape] of [...stats.entries()].sort()) {
    const cells = shapes.map((s) => {
      const c = byShape.get(s);
      return c ? `${c.failures}/${c.runs}` : '—';
    });
    const totalRuns = [...byShape.values()].reduce((a, c) => a + c.runs, 0);
    const totalFail = [...byShape.values()].reduce((a, c) => a + c.failures, 0);
    lines.push(`| \`${short(file)}\` | ${cells.join(' | ')} | **${totalFail}/${totalRuns}** |`);
  }
  return lines.join('\n');
}

/**
 * For every file that failed, what ran immediately before it in that
 * repetition? If a file only ever fails behind one particular predecessor,
 * that is the isolation leak, visible rather than inferred.
 */
function predecessorTable(records) {
  // Keyed by file, then by the file that ran immediately before it. Nested maps
  // rather than a delimited string key — predecessors like '(ran first)' contain
  // spaces, and any delimiter safe against a filename is one more thing to get
  // wrong.
  const pairs = new Map();
  for (const r of records) {
    r.files.forEach((f, i) => {
      const file = short(f.name);
      const predecessor = i === 0 ? '(ran first)' : short(r.files[i - 1].name);
      if (!pairs.has(file)) pairs.set(file, new Map());
      const byPredecessor = pairs.get(file);
      if (!byPredecessor.has(predecessor)) byPredecessor.set(predecessor, { fails: 0, total: 0 });
      const cell = byPredecessor.get(predecessor);
      cell.total += 1;
      if (f.status === 'failed') cell.fails += 1;
    });
  }
  const rows = [...pairs.entries()]
    .flatMap(([file, byPredecessor]) =>
      [...byPredecessor.entries()].map(([predecessor, cell]) => ({ file, predecessor, ...cell }))
    )
    .filter((r) => r.fails > 0)
    .sort((a, b) => b.fails / b.total - a.fails / a.total || b.total - a.total);

  if (!rows.length) return '_No failures recorded — nothing to attribute to run order._';

  const lines = [
    '| failed file | ran immediately after | failures / times in that position |',
    '| -- | -- | -- |',
  ];
  for (const r of rows) {
    lines.push(`| \`${r.file}\` | \`${r.predecessor}\` | ${r.fails}/${r.total} |`);
  }
  return lines.join('\n');
}

function contaminationNote(records) {
  const dirty = records.filter((r) => r.wsClientsAtStart);
  const missing = records.filter((r) => r.reportMissing);
  const notes = [];
  if (dirty.length) {
    notes.push(
      `⚠️ ${dirty.length} repetition(s) started with another client already attached to the bridge ` +
        `(${dirty.map((r) => `${r.shape}#${r.rep}`).join(', ')}). Those reps are contended and must not ` +
        `be read as clean evidence.`
    );
  }
  if (missing.length) {
    notes.push(
      `⚠️ ${missing.length} repetition(s) produced no parseable JSON report ` +
        `(${missing.map((r) => `${r.shape}#${r.rep}`).join(', ')}). Those are recorded failures of the ` +
        `run, not passes.`
    );
  }
  const pids = [...new Set(records.map((r) => `${r.bridgePid}@${r.bridgeStartedAt}`))];
  if (pids.length > 1) {
    notes.push(
      `ℹ️ The bridge process changed during the soak (${pids.length} distinct PID/start pairs). ` +
        `Cold-shape boundaries are expected here; anywhere else it means the warm link was reset ` +
        `mid-baseline.`
    );
  }
  return notes.length ? notes.join('\n\n') : '_No contention, no missing reports, one continuous bridge process._';
}

/**
 * Cross-tabulate suite failure against the log signatures captured for the same
 * repetition.
 *
 * The question this answers: when a repetition fails, does the failure coincide
 * with a signature naming a DIFFERENT subsystem than the error message does?
 * A `Timeout waiting for event: TRIGGER_STATE_CHANGED` that always co-occurs
 * with `[Reader] Failed to start scanning:` is a symptom of scan-start, not a
 * trigger defect — and the test report argues for the wrong one.
 *
 * Records whose capture was void are EXCLUDED rather than counted as zeros.
 * A zero from a working capture and a zero from a broken one mean opposite
 * things, and merging them is how a detector that sees nothing gets read as a
 * subsystem that did nothing.
 */
function signalPairingTable(records) {
  const resolved = records.map((r) => ({ r, ...resolveSignals(r) }));
  const captured = resolved.filter(
    ({ signals }) => signals && !signals.logMissing && (signals.harnessLines ?? 0) > 0
  );
  // A repetition that never reached the bridge measured the absence of a
  // transport, not the behaviour of one. Counting it as a failure-without-the-
  // signature turns an environmental outage into evidence against a hypothesis.
  //
  // BOTH unreachable shapes, or the exclusion silently stops working on the
  // path that now produces most of the runs: since TRA-1187 the integration
  // suite reaches the bridge through jsdom's WebSocket, which reports a bare
  // `WebSocket error` and never an errno. Partitioning on `transportRefused`
  // alone would have put every dead-bridge repetition of an overnight soak into
  // `usable` — i.e. into the evidence.
  const unreachable = ({ signals }) =>
    (signals.transportRefused ?? 0) > 0 || (signals.transportUnreachable ?? 0) > 0;
  const refused = captured.filter(unreachable);
  const usable = captured.filter((entry) => !unreachable(entry));
  const excluded = resolved.length - captured.length;
  const recomputed = usable.filter((u) => u.source === 'recomputed').length;
  if (!usable.length) {
    const why = excluded
      ? ` ${excluded} record(s) excluded: no signals, or the \`[Harness]\` canary is 0, ` +
        'which means the capture was void rather than the signatures absent.'
      : '';
    return `_No repetition carries a verified capture._${why}`;
  }

  const cell = { 'fail+sig': 0, 'fail+nosig': 0, 'pass+sig': 0, 'pass+nosig': 0 };
  for (const { r, signals } of usable) {
    const failed = r.files.some((f) => f.status === 'failed');
    // EITHER limb. The trigger case awaits startScanning() on press and
    // stopScanning() on release; either rethrowing skips the postWorkerEvent()
    // below it, so both produce an identical missing-event symptom.
    const sig = ((signals.startScanFailed ?? 0) + (signals.stopScanFailed ?? 0)) > 0;
    cell[`${failed ? 'fail' : 'pass'}+${sig ? 'sig' : 'nosig'}`] += 1;
  }

  const lines = [
    '| | `Failed to start/stop scanning:` present | absent |',
    '| -- | -- | -- |',
    `| **suite failed** | ${cell['fail+sig']} | ${cell['fail+nosig']} |`,
    `| **suite passed** | ${cell['pass+sig']} | ${cell['pass+nosig']} |`,
    '',
    `_${usable.length} repetition(s) with a verified capture` +
      (recomputed ? `, ${recomputed} recomputed from retained logs` : '') +
      (excluded ? `; ${excluded} excluded as void or unverifiable` : '') +
      (refused.length
        ? `; ${refused.length} excluded as bridge-unreachable (no bridge — measured an outage, not the subsystem)`
        : '') +
      '._',
  ];

  if (cell['fail+sig'] === 0 && cell['fail+nosig'] > 0) {
    lines.push(
      '',
      `**No failure coincided with a scan-start error** (${cell['fail+nosig']} failure(s) checked). ` +
        'That is evidence against scan-start being the mechanism — read it as such only if the ' +
        'count is large enough to carry the claim.'
    );
  }
  return lines.join('\n');
}

function main() {
  const records = loadRecords();
  console.log(`# Integration suite — run-shape record\n`);
  console.log(`${records.length} repetitions recorded.\n`);
  console.log(`## Per-run failures\n`);
  console.log(perRunTable(records));
  console.log(`\n## Failure rate by file and shape\n`);
  console.log(perFileTable(records));
  console.log(`\n## Order-dependence — what preceded each failure\n`);
  console.log(predecessorTable(records));
  console.log(`\n## Failure vs. scan-start signature\n`);
  console.log(signalPairingTable(records));
  console.log(`\n## Record integrity\n`);
  console.log(contaminationNote(records));
}

main();
