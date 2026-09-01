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

import { readFileSync, existsSync, realpathSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  resolveSignals,
  captureCanaryCount,
  resolveReadCycles,
  READ_CYCLE_FIELDS,
  cohortWarning,
} from './suite-run-signals.mjs';

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

/**
 * Failure rate by EXECUTION POSITION, not by predecessor.
 *
 * Added 2026-08-28 because the predecessor table asks "what poisoned this file?"
 * and the 210-repetition soak answered a different question: nothing poisoned
 * it. Whichever file ran FIRST failed at 41%, every later position at 5-10% —
 * a cold-start defect on the run's first BLE connect, invisible to a table
 * organised around predecessors because "(ran first)" is only one row among
 * many there.
 *
 * Reads `files` as execution order, which is only true since the startTime sort
 * in characterise-suite-runs.mjs. Records written before that are in report
 * order and will show a flat profile whether or not one exists — so this table
 * is only meaningful for records carrying `startTime`.
 */
function positionTable(records) {
  const dated = records.filter((r) => r.files.some((f) => f.startTime != null));
  const slots = [];
  for (const r of records) {
    r.files.forEach((f, i) => {
      slots[i] = slots[i] || { runs: 0, failures: 0 };
      slots[i].runs += 1;
      if (f.status === 'failed') slots[i].failures += 1;
    });
  }
  if (!slots.length) return '_No records._';

  const lines = ['| position | failures / runs | rate |', '| -- | -- | -- |'];
  slots.forEach((c, i) => {
    const pct = c.runs ? Math.round((c.failures / c.runs) * 100) : 0;
    lines.push(`| ${i + 1}${i === 0 ? ' (first)' : ''} | ${c.failures}/${c.runs} | ${pct}% |`);
  });
  if (dated.length !== records.length) {
    lines.push('');
    lines.push(
      `_⚠️ ${records.length - dated.length} of ${records.length} records predate the ` +
        'execution-order fix and carry report order instead. Their positions are not ' +
        'execution positions; re-derive from `.suite-runs/report-*.json` startTime.'
    );
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
  // The canary is asked for by ROLE, not by needle name. `harnessLines` is
  // emitted only by the integration harness, so on a Playwright record it is
  // null by construction — and `null ?? 0` would exclude every e2e repetition
  // from the evidence while reporting them as void captures. That is a filter
  // discarding good data for a reason that reads as a data-quality finding
  // (TRA-1206). `captureCanaryCount` resolves the right needle per runner.
  const captured = resolved.filter(
    ({ r, signals }) => signals && !signals.logMissing && (captureCanaryCount(r, signals) ?? 0) > 0
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
      ? ` ${excluded} record(s) excluded: no signals, or the capture canary is 0 ` +
        '(`[Harness]` for a vitest run, `[Connection]` for an e2e one), ' +
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

/**
 * Field density, printed beside the reference baseline.
 *
 * The comparison is the whole point. TRA-1200's arm ran ~17% short on unique
 * tags — the reader had been pulled back from the stack to gun a barcode — and a
 * bare distribution would not have shown that to anyone. It became visible only
 * when put next to the 2026-08-23 numbers, after the run was over. Printing the
 * baseline is what turns a statistic into a check.
 *
 * ⚠ Judge density on UNIQUE, never on reads. Read volume is confounded with the
 * variable a CPU-swap arm measures: a faster host issues stop-scanning sooner
 * and accumulates fewer reads inside the fixed 2s window, so two hosts facing an
 * identical pile can disagree by 40%. Reads is reported because it is evidence
 * about the run; it is not evidence about the field.
 */
const REFERENCE_DENSITY = {
  firstReads: 'mean 153, median 115',
  firstUnique: 'mean 83, median 81',
  secondReads: 'mean 321, median 243',
  secondUnique: 'mean 125, median 127',
};

/**
 * EVERY op code the device stopped answering — including ones nobody predicted.
 *
 * ⚠ THE SECTION ABOVE REPORTS ONE OP CODE, AND THAT WAS ONCE THE WHOLE PICTURE.
 * It is not. On the 2026-08-31 arm the device also stopped answering
 * `GET_TRIGGER_STATE` (0xA001) — 12-13 times per rep, in the same reps as the
 * power-offs — and nothing said so, because no needle counted it. Meanwhile
 * TRA-1223 asserted the reader ignored "exactly one op code" while its own
 * first-occurrence packet table carried 0xA001 at 76 TX / 14 RX. The data
 * contradicted the summary in the same document, and no instrument objected.
 *
 * So this table is deliberately NOT a list of known op codes. It prints whatever
 * `countCommandTimeouts` parsed out of the logs, which means **the next op code
 * to go silent shows up here without anyone having predicted it.** A fixed list
 * can only ever report the silences somebody already knew about.
 *
 * Read it alongside the window table, not instead of it: this one says WHAT went
 * unanswered, that one says whether the tolerance held.
 */
/**
 * Device-raised rejections per rep (TRA-1229).
 *
 * `0xA101` is how the CS108 refuses a command — the rejection never comes back
 * under the op code being rejected, so the command that caused it sees nothing
 * and times out. Until TRA-1229 these were discarded in `reader.ts` and this
 * table would have read empty through an 86-minute fault storm carrying 1543 of
 * them.
 *
 * Read it ALONGSIDE the op-code table above: a rep with many unanswered
 * commands and a matching error count was refused, not ignored, and those are
 * different defects. A rep with unanswered commands and NO errors is the case
 * where the device really did go quiet.
 */
export function errorNotificationTable(records) {
  const seen = records.filter(r => r.signals?.errorNotifications != null);
  if (seen.length === 0) {
    return '_No rep could observe error notifications — a runner that cannot see them._';
  }
  const withErrors = seen.filter(r => r.signals.errorNotifications > 0);
  const total = seen.reduce((n, r) => n + r.signals.errorNotifications, 0);
  if (withErrors.length === 0) {
    return `_No rep raised a device rejection (0 across ${seen.length} rep(s) that could see them)._`;
  }
  const worst = withErrors.reduce((a, b) =>
    b.signals.errorNotifications > a.signals.errorNotifications ? b : a);
  const lines = [
    '| measure | value |',
    '| -- | -- |',
    `| reps raising at least one rejection | ${withErrors.length}/${seen.length} |`,
    `| total rejections | ${total} |`,
    `| worst rep | ${worst.rep} (${worst.signals.errorNotifications}) |`,
    '',
    '⚠ **A rejection means the device refused the command, not that it ignored it.**',
    'Compare against the op-code table above: unanswered commands WITH a matching',
    'rejection count are refusals; unanswered commands with none are silence. Refs TRA-1229.',
  ];
  return lines.join('\n');
}

export function commandTimeoutsByOpTable(records) {
  // RESOLVED, not read raw. Every archived arm predates this field, and its logs
  // are usually still on disk — reading `r.signals` directly would report the
  // whole back catalogue as unobservable while the answer sat next to it. That
  // is how TRA-1226's own numbers were obtained: by hand-grepping the logs of an
  // arm that had already finished.
  const usable = records
    .map((r) => resolveSignals(r).signals)
    .filter((signals) => signals && !signals.logMissing);
  if (usable.length === 0) {
    return '_No repetition carried a verified capture — command timeouts are unobservable in this run._';
  }

  // null means the runner cannot see these lines at all (e2e drops
  // `[CommandManager]` warns at the console forwarder). Different claim from an
  // empty map, and summing the two together would turn "could not look" into
  // "looked and found nothing".
  const observable = usable.filter((signals) => signals.commandTimeouts != null);
  if (observable.length === 0) {
    return (
      '_`[CommandManager]` lines are vitest-only; no repetition in this run could produce them. ' +
      'Command timeouts are unobservable here, which is not the same as absent._'
    );
  }

  const totals = new Map();
  const repsWith = new Map();
  for (const signals of observable) {
    for (const [op, count] of Object.entries(signals.commandTimeouts)) {
      if (!count) continue;
      totals.set(op, (totals.get(op) ?? 0) + count);
      repsWith.set(op, (repsWith.get(op) ?? 0) + 1);
    }
  }

  const n = observable.length;
  if (totals.size === 0) {
    return (
      '**No command went unanswered in this run, which says NOTHING about the device.** ' +
      'Zero here is the absence of the condition, not evidence the reader is answering ' +
      'reliably — the silent window appeared twice in ~400 reps and cannot be summoned. ' +
      'Report it as "did not occur", never as a clean bill of health.'
    );
  }

  const rows = [...totals.entries()].sort((a, b) => b[1] - a[1]);
  const lines = [
    '| op code | timeouts | reps affected |',
    '| -- | -- | -- |',
    ...rows.map(([op, total]) => `| \`${op}\` | ${total} | ${repsWith.get(op)}/${n} |`),
    '',
  ];

  // The reason the table exists: name what is NOT already tracked by a needle,
  // so a new silence is read as a finding rather than scrolled past.
  const untracked = rows.filter(([op]) => op !== 'RFID_POWER_OFF').map(([op]) => op);
  if (untracked.length > 0) {
    lines.push(
      `⚠ **${untracked.length} op code(s) beyond \`RFID_POWER_OFF\` went unanswered: ` +
        `${untracked.map((op) => `\`${op}\``).join(', ')}.** The window is not confined to one ` +
        'op code. Check the bridge packet capture for TX-vs-RX on each before concluding the ' +
        'device was silent — an app-side timeout alone cannot distinguish "no answer sent" ' +
        'from "answer sent and we did not match it". Refs TRA-1223.'
    );
  }

  return lines.join('\n');
}

/**
 * Did the CS108's silent window happen, and did TRA-1217 absorb it?
 *
 * ⚠ THIS SECTION EXISTS BECAUSE THE FIX REMOVED ITS OWN SYMPTOM. Before
 * TRA-1217 the window announced itself by killing reps and emitting
 * `link-close`; now it is tolerated, so a recurrence and a quiet night produce
 * the same rep table. Reading a clean arm as "the fix worked" is only valid if
 * the condition occurred, and nothing else in this report can tell you that.
 *
 * The three counts are the falsification test written out, and they are meant to
 * be read TOGETHER rather than as three statistics:
 *
 *   timeouts > 0, cleanup-failures 0  ->  the window happened and was absorbed.
 *                                         This is the fix working, and the only
 *                                         reading that earns TRA-1217 credit.
 *   timeouts 0                        ->  NOT EVIDENCE OF ANYTHING. The device
 *                                         was quiet. Says nothing about the fix.
 *   cleanup-failures > 0              ->  the tolerance did not hold. Regression.
 *
 * The middle row is the one that matters most and is easiest to misread, so it
 * is printed as a verdict rather than left to the reader. A soak that spends
 * eight hours proving nothing should say so in words.
 */
export function powerOffWindowTable(records) {
  const usable = records.filter((r) => r.signals && !r.signals.logMissing);
  if (usable.length === 0) {
    return '_No repetition carried a verified capture — the window is unobservable in this run._';
  }

  // null means the needle cannot fire on this runner (e2e), which is a different
  // claim from zero and must not be summed into one.
  const observable = usable.filter((r) => typeof r.signals.powerOffTimeouts === 'number');
  if (observable.length === 0) {
    return (
      '_These needles are vitest-only; no repetition in this run could produce them. ' +
      'The window is unobservable here, which is not the same as absent._'
    );
  }

  const sum = (key) => observable.reduce((acc, r) => acc + (r.signals[key] ?? 0), 0);
  const repsWith = (key) => observable.filter((r) => (r.signals[key] ?? 0) > 0).length;

  const timeouts = sum('powerOffTimeouts');
  const tolerated = sum('toleratedPowerOffs');
  const cleanupFailed = sum('modeSwitchFailed');
  const n = observable.length;

  // Refusals are the THIRD state, and without them a refusal window falls into
  // the branch written for a quiet device. Since TRA-1229 a refused command is
  // settled from its 0xA101 reply in ~34ms, which clears the timeout, so
  // `timeouts` is 0 through a storm of them. Refs TRA-1230.
  const rejByOp = {};
  for (const r of observable) {
    for (const [op, c] of Object.entries(r.signals.commandRejections ?? {})) {
      rejByOp[op] = (rejByOp[op] ?? 0) + c;
    }
  }
  const rejections = Object.values(rejByOp).reduce((a, b) => a + b, 0);
  const repsRefused = observable.filter(
    (r) => Object.keys(r.signals.commandRejections ?? {}).length > 0
  ).length;

  const lines = [
    '| signal | occurrences | reps affected |',
    '| -- | -- | -- |',
    `| \`Command timeout: RFID_POWER_OFF\` — device silent | ${timeouts} | ${repsWith('powerOffTimeouts')}/${n} |`,
    `| tolerated, sequence continued — rescued by TRA-1217 | ${tolerated} | ${repsWith('toleratedPowerOffs')}/${n} |`,
    `| \`Mode switching failed during cleanup\` — tolerance did NOT hold | ${cleanupFailed} | ${repsWith('modeSwitchFailed')}/${n} |`,
    `| \`Command rejected\` — device REFUSED, did not go silent | ${rejections} | ${repsRefused}/${n} |`,
    '',
  ];

  if (rejections > 0) {
    const byOp = Object.entries(rejByOp)
      .sort((a, b) => b[1] - a[1])
      .map(([op, c]) => `\`${op}\` ${c}`)
      .join(', ');
    lines.push(`Refused op codes: ${byOp}`, '');
  }

  if (cleanupFailed > 0) {
    lines.push(
      `**REGRESSION — the tolerance did not hold on ${repsWith('modeSwitchFailed')} rep(s).** ` +
        'TRA-1217 makes a mode change survive an unanswered `RFID_POWER_OFF`; a cleanup failure ' +
        'means something got past it. Read the affected reps before trusting anything else here.'
    );
  } else if (timeouts > 0) {
    lines.push(
      `**The window occurred and was absorbed.** ${timeouts} unanswered \`RFID_POWER_OFF\` ` +
        `attempt(s) across ${repsWith('powerOffTimeouts')} rep(s), no cleanup failures. Before ` +
        'TRA-1217 these reps would have failed. This is the arm that earns the fix its credit.'
    );
  } else if (rejections > 0) {
    lines.push(
      `**REFUSED, not silent — and this is a THIRD state the older wording had no room for.** ` +
        `${rejections} command(s) across ${repsRefused} rep(s) came back with an explicit ` +
        'rejection, answered in ~34ms rather than left unanswered. `powerOffTimeouts` reads 0 ' +
        'because TRA-1229 settles a refused command from its `0xA101` reply before the timeout ' +
        'can fire — so a zero there is NOT a quiet device here. Read the refused op codes above: ' +
        '"no answer came" and "the answer was no" are different device behaviours and the ' +
        'distinction is what TRA-1223 turned on.'
    );
  } else {
    lines.push(
      '**The device never went silent in this run, so this arm says NOTHING about TRA-1217.** ' +
        'Zero here is the absence of the condition, not evidence the fix works — the window ' +
        'appeared once in 200 reps and cannot be summoned. Do not report a clean arm as ' +
        'confirmation; it only shows nothing regressed.'
    );
  }

  return lines.join('\n');
}

export function densityTable(records) {
  const resolved = records.map((r) => resolveReadCycles(r));
  const measured = resolved.filter(({ readCycles }) =>
    READ_CYCLE_FIELDS.some((k) => typeof readCycles[k] === 'number')
  );

  if (measured.length === 0) {
    return '_No rep recorded read cycles — a vitest run, or logs no longer retained._';
  }

  const stat = (key) => {
    const xs = measured
      .map(({ readCycles }) => readCycles[key])
      .filter((v) => typeof v === 'number')
      .sort((a, b) => a - b);
    if (xs.length === 0) return { n: 0, text: '—' };
    const mean = xs.reduce((a, b) => a + b, 0) / xs.length;
    const median = xs[Math.floor(xs.length / 2)];
    return {
      n: xs.length,
      text: `mean ${mean.toFixed(0)}, median ${median}, min ${xs[0]}, max ${xs[xs.length - 1]}`,
    };
  };

  const lines = [
    '⚠ Unique-tag count is the field proxy; read volume is NOT. Reads are',
    'confounded with host speed — a faster host stops scanning sooner and',
    'accumulates fewer inside the fixed 2s window — so judge whether the field',
    'matched on unique alone.',
    '',
    '| measure | this run | reference (knuckles, 2026-08-23, n=407) |',
    '| -- | -- | -- |',
  ];
  for (const key of READ_CYCLE_FIELDS) {
    const { n, text } = stat(key);
    lines.push(`| \`${key}\` (n=${n}) | ${text} | ${REFERENCE_DENSITY[key]} |`);
  }

  const rebuilt = resolved.filter(({ source }) => source === 'recomputed').length;
  if (rebuilt > 0) {
    lines.push(
      '',
      `_${rebuilt} row(s) reconstructed from retained logs — recorded before this instrument existed._`
    );
  }
  return lines.join('\n');
}

/**
 * TRA-1150's two wedge modes, scored separately.
 *
 * The ticket asked for this in as many words — *"those 33 are two distinct
 * failure modes scored as one number; if the driver can separate them, do"* —
 * and it could not be done until read-cycle values were recorded, because both
 * modes are defined by read COUNTS rather than by any log needle:
 *
 *   mode 1  first == 0             the scan path is dead    31/407 = 7.62%
 *   mode 2  first == second, > 0   frozen accumulation       2/407 = 0.49%
 *
 * Mode 1 is 94% of the reference's wedges, which means the aggregate everyone
 * quoted was substantially a measurement of mode 1 alone. Splitting them is what
 * makes that visible instead of implied.
 *
 * ⚠ `> 0` on mode 2 is load-bearing. A dead rep satisfies `first === second`
 * numerically at 0 === 0, so without it every mode 1 would be double-counted
 * into mode 2 as well, and the rarest failure in the campaign would appear to
 * be as common as the commonest.
 *
 * ⚠ Unscoreable reps are excluded and counted, never scored as mode 1. A rep
 * with 0 reads and a rep whose reads were never observed are different claims,
 * and conflating them manufactures the dominant wedge signature out of a missing
 * log. This is the same rule the null-vs-zero convention enforces upstream.
 */
const WEDGE_REFERENCE = {
  dead: '31/407 = 7.62%',
  frozen: '2/407 = 0.49%',
};

export function wedgeModeTable(records) {
  const cycles = records.map((r) => resolveReadCycles(r).readCycles);
  const scoreable = cycles.filter(
    (c) => typeof c.firstReads === 'number' && typeof c.secondReads === 'number'
  );
  const unscoreable = cycles.length - scoreable.length;

  if (scoreable.length === 0) {
    return '_No rep carried both read cycles — nothing is scoreable for wedge mode._';
  }

  const dead = scoreable.filter((c) => c.firstReads === 0).length;
  const frozen = scoreable.filter(
    (c) => c.firstReads > 0 && c.firstReads === c.secondReads
  ).length;
  const n = scoreable.length;
  const pct = (k) => `${((k / n) * 100).toFixed(2)}%`;

  const lines = [
    '| mode | this run | reference (knuckles, 2026-08-23) |',
    '| -- | -- | -- |',
    `| 1 — \`first == 0\`, the scan path is dead | ${dead}/${n} = ${pct(dead)} | ${WEDGE_REFERENCE.dead} |`,
    `| 2 — frozen accumulation (\`first == second\`, > 0) | ${frozen}/${n} = ${pct(frozen)} | ${WEDGE_REFERENCE.frozen} |`,
    `| combined | ${dead + frozen}/${n} = ${pct(dead + frozen)} | 33/407 = 8.11% |`,
  ];
  if (unscoreable > 0) {
    lines.push(
      '',
      `_${unscoreable} rep(s) excluded as unscoreable — one or both read cycles were not ` +
        'observed. Not counted as mode 1: a rep with 0 reads and a rep whose reads were ' +
        'never seen are different claims._'
    );
  }
  return lines.join('\n');
}

function main() {
  const records = loadRecords();
  console.log(`# Integration suite — run-shape record\n`);
  // Before any number it would qualify. A caveat printed after the rate it
  // undercuts is one the reader has already acted on.
  const mixed = cohortWarning(records);
  if (mixed) console.log(`${mixed}\n`);
  console.log(`${records.length} repetitions recorded.\n`);
  console.log(`## Per-run failures\n`);
  console.log(perRunTable(records));
  console.log(`\n## Failure rate by file and shape\n`);
  console.log(perFileTable(records));
  console.log(`\n## Failure rate by execution position\n`);
  console.log(positionTable(records));
  console.log(`\n## Order-dependence — what preceded each failure\n`);
  console.log(predecessorTable(records));
  console.log(`\n## Failure vs. scan-start signature\n`);
  console.log(signalPairingTable(records));
  console.log(`\n## Wedge mode — TRA-1150's two failure modes, scored separately\n`);
  console.log(wedgeModeTable(records));
  console.log(`\n## The CS108's silent window — did it happen, and did TRA-1217 absorb it?\n`);
  console.log(powerOffWindowTable(records));
  console.log(`\n## Every unanswered command, by op code\n`);
  console.log(commandTimeoutsByOpTable(records));
  console.log(`\n## Device-raised rejections (0xA101)\n`);
  console.log(errorNotificationTable(records));
  console.log(`\n## Field density\n`);
  console.log(densityTable(records));
  console.log(`\n## Record integrity\n`);
  console.log(contaminationNote(records));
}

// Run the report only when invoked directly. Importing this module — which its
// tests must do to exercise the table builders — would otherwise execute main(),
// load records that are not there, and process.exit(1) before a single test ran.
if (process.argv[1] && realpathSync(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
