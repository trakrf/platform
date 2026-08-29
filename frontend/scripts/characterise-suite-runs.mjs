#!/usr/bin/env node
/**
 * Run-shape driver for characterising the hardware integration suite.
 *
 * Answers "does this suite fail because of WHAT ran before, or because of WHICH
 * file it is?" by driving the suite repeatedly under controlled run shapes and
 * recording which file fails each time.
 *
 * It produces every shape from CLI flags and process lifecycle ONLY. It never
 * edits vitest.config.ts, package.json, or any spec — the suite under test
 * cannot observe that it is being characterised. Keep that property: the moment
 * this script changes the subject, its measurements stop describing the suite
 * anyone else runs.
 *
 * First used for TRA-1167, whose full run record and findings are attached to that
 * ticket — investigation records are deliberately not kept in the repo.
 *
 * Shapes:
 *   fixed    current behaviour — the same flags `pnpm test:integration` uses
 *   shuffle  --sequence.shuffle.files with a recorded seed, so any interesting
 *            order is reproducible
 *   alone    one file per invocation (--target)
 *   cold     same as fixed, but the caller restarted the bridge process first;
 *            this script only records that the claim was made
 *
 * NOTE ON "alone" AND "cold" — REVISED 2026-08-27, and the revision reverses it.
 *
 * This used to read: "alone does NOT give a cold reader. The Rust bridge calls
 * transport.connect() once at process start and holds the BLE link for the life
 * of the process; a WS disconnect tears down nothing." That was true of
 * `rust-ble-test` and is false of what runs now.
 *
 * The Python bridge constructs its transport INSIDE the per-connection handler
 * (TRA-1157, docs/design/2026-08-23-transport-lifecycle-decision.md in the
 * ble-mcp-test repo). A daemon with no WebSocket clients holds no device, and
 * the last client disconnecting releases it. So every run boundary is already a
 * full BLE teardown and reconnect:
 *
 *   - `alone` now DOES give a cold reader, which is what it always claimed to.
 *   - `cold` is close to redundant. Restarting the bridge process no longer
 *     releases anything a normal run boundary did not already release. It still
 *     distinguishes "fresh process" from "fresh link", which is a narrower
 *     question than it used to be — do not read a cold-vs-fixed difference as
 *     evidence about the radio.
 *
 * This also changes what a long soak of this suite measures. Since TRA-1187 the
 * suite reaches the bridge through CS108BLETransport, one connect and disconnect
 * per spec file, against a bridge that acquires and releases the radio on each
 * one. A soak is therefore substantially a BLE connect/disconnect cycle test,
 * and connect-time flakes will dominate anything the specs assert.
 *
 * Usage:
 *   node scripts/characterise-suite-runs.mjs --shape fixed --reps 5
 *   node scripts/characterise-suite-runs.mjs --shape shuffle --reps 5
 *   node scripts/characterise-suite-runs.mjs --shape alone --reps 3 --target tests/integration/cs108/locate.spec.ts
 *   node scripts/characterise-suite-runs.mjs --shape cold --reps 3
 */

import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, appendFileSync, rmSync, existsSync, openSync, closeSync } from 'node:fs';
import path from 'node:path';
import { readSignals } from './suite-run-signals.mjs';

// 2 adds `signals` + `outputLog`; schema-1 records carry neither.
const RECORD_SCHEMA = 2;

/**
 * Per-invocation stamp, so a later invocation cannot overwrite an earlier one's
 * captured output.
 *
 * `runs.jsonl` accumulates across invocations but repetition numbers restart at
 * 1 every time, so keying the log by shape+rep alone means the next run of the
 * same shape silently overwrites the logs the previous records still point at —
 * the record survives, its evidence does not. Found the hard way: an instrument
 * check's log was replaced by rep 1 of the run it was meant to validate.
 */
const RUN_ID = new Date().toISOString().replace(/[:.]/g, '-');
const ARTIFACT_DIR = path.resolve(process.cwd(), '.suite-runs');
const RECORD_PATH = path.join(ARTIFACT_DIR, 'runs.jsonl');
const SUITE_ROOT = 'tests/integration/';
const VALID_SHAPES = ['fixed', 'shuffle', 'alone', 'cold'];

function parseArgs(argv) {
  const args = { shape: null, reps: 1, target: null, note: null };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = argv[i + 1];
    switch (flag) {
      case '--shape': args.shape = value; i += 1; break;
      case '--reps': args.reps = Number(value); i += 1; break;
      case '--target': args.target = value; i += 1; break;
      case '--note': args.note = value; i += 1; break;
      default:
        throw new Error(`Unknown argument: ${flag}`);
    }
  }
  if (!VALID_SHAPES.includes(args.shape)) {
    throw new Error(`--shape must be one of ${VALID_SHAPES.join('|')}, got: ${args.shape}`);
  }
  if (!Number.isInteger(args.reps) || args.reps < 1) {
    throw new Error(`--reps must be a positive integer, got: ${args.reps}`);
  }
  if (args.shape === 'alone' && !args.target) {
    throw new Error('--shape alone requires --target <spec path>');
  }
  return args;
}

/**
 * Count established TCP connections whose peer is the bridge's WS port.
 *
 * The soak assumes exclusive use of a shared reader, but the bridge process is
 * orphaned and nothing can actually prevent a third party attaching. Recording
 * the count per repetition makes contamination visible in the record instead of
 * silently mixed into it.
 */
function countBridgeClients() {
  // Resolved the way the suite resolves it, not hardcoded. This read
  // `dst 127.0.0.1:8080` until 2026-08-27, which by then was the wrong port
  // (TRA-1179 moved the bridge to 25153, off the backend's) — so it matched
  // nothing and recorded a confident 0 clients for every repetition. A
  // contention detector that cannot see the socket reports "uncontended", which
  // is the answer everyone wants and nobody can check.
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const port = process.env.BLE_MCP_WS_PORT || '25153';

  // A bridge on another host is not observable with local `ss`, and 0 would be a
  // lie rather than a measurement. null means "unknown", which the summariser
  // already renders as `?`.
  if (!['localhost', '127.0.0.1', '::1'].includes(host)) return null;

  const res = spawnSync('ss', ['-tn', 'state', 'established', 'dst', `127.0.0.1:${port}`], {
    encoding: 'utf8',
  });
  if (res.status !== 0 || typeof res.stdout !== 'string') return null;
  // First line is the ss header; every remaining line is one client socket.
  const lines = res.stdout.trim().split('\n').filter(Boolean);
  return Math.max(0, lines.length - 1);
}

/**
 * The pid LISTENING on the bridge port — identified by what it does, not by what
 * it is called.
 *
 * Two name-based versions of this were wrong in a row, each silently:
 *   `rust-ble-test`  — a binary deleted in the Python replatform (TRA-1155)
 *   `ble_bridge`     — the Python MODULE name, which never appears in the
 *                      process cmdline. The console script is `ble-bridge`,
 *                      with a hyphen. Testing it from a shell gave a false
 *                      positive because the test command's own argv contained
 *                      the literal string; `pgrep` matched THAT.
 *
 * Both returned null forever, so `bridgePid` and `bridgeStartedAt` were null on
 * every repetition and the summariser's "the bridge process changed mid-soak"
 * check had nothing to compare — a detector that reads as covered because the
 * field exists. Found 2026-08-28 by the bridge session reading runs.jsonl.
 *
 * Asking the socket removes the whole class: whatever is serving the port IS
 * the bridge, whatever it is called, and a rename cannot break it.
 */
function readBridgeProcess() {
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const port = process.env.BLE_MCP_WS_PORT || '25153';
  if (!['localhost', '127.0.0.1', '::1'].includes(host)) {
    return { bridgePid: null, bridgeStartedAt: null };
  }

  let pid = null;
  const res = spawnSync('ss', ['-ltnpH', 'sport', `= :${port}`], { encoding: 'utf8' });
  if (res.status === 0 && typeof res.stdout === 'string') {
    // ss renders it as: users:(("ble-bridge",pid=338646,fd=12))
    const match = res.stdout.match(/pid=(\d+)/);
    if (match) pid = Number(match[1]);
  }

  if (!Number.isInteger(pid)) return { bridgePid: null, bridgeStartedAt: null };
  const startRes = spawnSync('ps', ['-o', 'lstart=', '-p', String(pid)], { encoding: 'utf8' });
  return {
    bridgePid: pid,
    bridgeStartedAt: startRes.status === 0 ? startRes.stdout.trim() : null,
  };
}

function buildVitestArgs(shape, rep, target) {
  // A target narrows any shape, not just `alone`. `cold` in particular needs it:
  // the useful cold measurement is one file run against a freshly restarted
  // bridge, directly comparable against the same file run warm.
  const filter = target ?? SUITE_ROOT;
  // Same flags package.json's test:integration uses, plus JSON reporting.
  //
  // `--hookTimeout` must stay in step with that script. THREE copies of this
  // number exist (both package.json scripts and here); a single owner is
  // TRA-1189 follow-up work, not done. If you change one, change all three.
  //
  // These hooks do a real BLE connect plus RFID power-on and mode
  // configuration; measured file duration is 16.5-20.7s against vitest's 10s
  // default, so the default made a passing hook a coin flip. That is what the
  // 2026-08-27 soak was actually measuring: 79 of 90 position-1 failures
  // carried `Hook timed out in 10000ms` and 0 of 127 position-1 passes did.
  // Raising it took locate.spec.ts from 30-83% failure to 0/24 across two arms.
  //
  // 30000 -> 90000 on 2026-08-29. NOT a response to any observed failure: the
  // 111-rep baseline recorded ZERO `Hook timed out`, so at 30s this bound never
  // fired. It is a PREREQUISITE for adopting ble-mcp-test's connect-backstop
  // change, and the ordering is load-bearing:
  //
  //   Today the mock's hardcoded 10s connect bound is the only thing keeping a
  //   connect inside the hook budget. Once that becomes a 60s backstop, a
  //   worst-case connect is the bridge's own budget --
  //     ALLOCATION_REPORT 2s + ADVERTISEMENT 30s + CONNECT 20s = 52s
  //   -- which exceeds 30s and would turn a busy reader into a hook timeout
  //   that kills the whole file and cascades. Raise this BEFORE bumping
  //   ble-mcp-test, never after.
  //
  // 90000 rather than 60000: worst-case beforeAll is that connect plus the
  // hook's non-connect work. That second term is NOT directly measurable here
  // (vitest reports file totals, not hook durations), so it was bounded from
  // above at ~22.9s from the worst observed single-test file. 52 + 22.9 = 74.9s,
  // leaving 15.1s at 90s and only 149ms at 60s. A derivation is only as measured
  // as its weakest term, so the unmeasured one is resolved toward safety: being
  // too patient costs wall-clock on a run that fails anyway, being too impatient
  // destroys the diagnosis the backstop change exists to deliver.
  //
  // Deliberately NOT unified with HARDWARE_TEST_TIMEOUT_MS in tests/e2e/
  // e2e.config.ts even though it is also 90000 -- that is a Playwright per-test
  // budget for the e2e suite, a different quantity over a different scope. They
  // are equal by coincidence and must be free to diverge.
  const args = ['vitest', 'run', filter, '--no-file-parallelism', '--hookTimeout=90000'];
  let seed = null;
  if (shape === 'shuffle') {
    // Derived from rep so the order is reproducible from the record alone.
    seed = 11670000 + rep;
    args.push('--sequence.shuffle.files', `--sequence.seed=${seed}`);
  }
  return { args, seed };
}

/**
 * Turn vitest's JSON report into the per-file record.
 *
 * The JSON report is the source of truth for pass/fail — never stdout scraping.
 * A missing or unparseable report is recorded as such, so a broken run can
 * never read as an empty pass.
 */
function readReport(reportPath) {
  if (!existsSync(reportPath)) {
    return { files: [], reportMissing: true };
  }
  let parsed;
  try {
    parsed = JSON.parse(readFileSync(reportPath, 'utf8'));
  } catch {
    return { files: [], reportMissing: true };
  }
  const suites = Array.isArray(parsed.testResults) ? parsed.testResults : [];

  // SORTED BY startTime, because `files` is read as EXECUTION ORDER downstream —
  // the predecessor table in summarise-suite-runs.mjs is built by walking this
  // array and calling element i-1 the predecessor of element i.
  //
  // vitest's JSON report does not emit testResults in execution order; it was
  // stable across repetitions regardless of how the files actually ran. So on
  // the 2026-08-27 soak every one of 210 shuffled repetitions recorded the SAME
  // order, the predecessor table showed each file following one predecessor
  // 214/214 times, and the shape's whole purpose was silently defeated. The
  // shuffle itself was working the entire time — 209 distinct true orders, once
  // startTime was consulted.
  //
  // That is the failure this tool exists to avoid: an instrument reporting a
  // confident, well-formed, wrong answer. It read as "no order-dependence" when
  // the real signal was a 4-6x failure rate on whichever file ran FIRST.
  const files = [...suites]
    .sort((a, b) => (a.startTime ?? 0) - (b.startTime ?? 0))
    .map((suite) => ({
      name: path.relative(process.cwd(), suite.name),
      status: suite.status === 'passed' ? 'passed' : 'failed',
      startTime: suite.startTime ?? null,
      failed: (suite.assertionResults || [])
        .filter((a) => a.status === 'failed')
        .map((a) => a.fullName || a.title),
    }));
  return { files, reportMissing: false };
}

function runOnce({ shape, rep, target, note }) {
  const { args, seed } = buildVitestArgs(shape, rep, target);
  const reportPath = path.join(ARTIFACT_DIR, `report-${shape}-${rep}.json`);
  const logPath = path.join(ARTIFACT_DIR, `output-${RUN_ID}-${shape}-${rep}.log`);
  rmSync(reportPath, { force: true });
  rmSync(logPath, { force: true });

  const wsClientsAtStart = countBridgeClients();
  const { bridgePid, bridgeStartedAt } = readBridgeProcess();
  const startedAt = new Date();

  // BOTH streams go to the per-repetition file. stderr is the one that matters:
  // under `--reporter=json` the only thing on stdout is "JSON report written
  // to ...", while every console line the suite and the worker emit — including
  // `[Reader] Failed to start scanning:` — goes to stderr. Capturing stdout
  // alone produced a detector that reported 0 occurrences of everything, which
  // looks exactly like "it never happened".
  //
  // This costs the live progress stdio 'inherit' used to give. The driver's own
  // per-repetition line covers that, and evidence that survives the run is worth
  // more than a scrolling one.
  //
  // The exit status is read straight off the spawned process. Never pipe this
  // into anything — a pipeline reports its LAST stage's status, which is how a
  // red suite reads green.
  const logFd = openSync(logPath, 'w');
  let res;
  try {
    // Two reporters on purpose. `json` alone INTERCEPTS the suite's console
    // output and prints none of it, so capturing the streams yields nothing but
    // a "JSON report written to ..." line — a log-based detector reads 0
    // occurrences of everything and looks identical to "it never happened".
    // Adding `default` puts the console lines back on the streams while `json`
    // still writes the machine-readable verdict. With more than one reporter,
    // vitest 1.x requires the per-reporter `--outputFile.json=` form; plain
    // `--outputFile` is silently ignored and the report never appears.
    res = spawnSync('npx', [...args, '--reporter=json', '--reporter=default', `--outputFile.json=${reportPath}`], {
      encoding: 'utf8',
      stdio: ['ignore', logFd, logFd],
    });
  } finally {
    closeSync(logFd);
  }

  const endedAt = new Date();
  const { files, reportMissing } = readReport(reportPath);
  const signals = readSignals(logPath);

  const record = {
    schema: RECORD_SCHEMA,
    shape,
    rep,
    seed,
    target: target ?? null,
    startedAt: startedAt.toISOString(),
    endedAt: endedAt.toISOString(),
    durationMs: endedAt - startedAt,
    exitCode: res.status,
    files,
    reportMissing,
    signals,
    outputLog: path.relative(process.cwd(), logPath),
    bridgePid,
    bridgeStartedAt,
    wsClientsAtStart,
    note: note ?? null,
  };

  appendFileSync(RECORD_PATH, `${JSON.stringify(record)}\n`);
  return record;
}

/**
 * Refuse to run against a bridge that is not there.
 *
 * Long unattended runs are where this earns its keep. Every repetition against a
 * dead bridge produces a well-formed record saying the suite failed, and a night
 * of those is indistinguishable at a glance from a night of real flakes — the
 * `transportUnreachable` signal exists to separate them afterwards, but there is
 * no reason to spend eight hours generating rows that get excluded.
 *
 * Deliberately only checks that something is listening. Whether that something
 * relays to a real radio is the bridge's own problem, and it now refuses to
 * start without an ESPHome device configured, so a bridge that is up is a bridge
 * with a device (ble-mcp-test `_select_transport`).
 */
function preflight() {
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const port = process.env.BLE_MCP_WS_PORT || '25153';

  if (!['localhost', '127.0.0.1', '::1'].includes(host)) {
    console.log(`[suite-runs] bridge host ${host} is remote; skipping the listening check.`);
    return;
  }

  const res = spawnSync('ss', ['-ltn', 'sport', `= :${port}`], { encoding: 'utf8' });
  const listening =
    res.status === 0 && typeof res.stdout === 'string' && res.stdout.trim().split('\n').length > 1;

  if (!listening) {
    console.error(
      `[suite-runs] FATAL: nothing is listening on ${host}:${port}.\n` +
        '  Every repetition would fail at WebSocket connect and measure the absence of a\n' +
        '  bridge rather than the suite. Start the bridge, or set BLE_MCP_WS_PORT/BLE_MCP_HOST\n' +
        '  to point at the one that is running.'
    );
    process.exit(1);
  }
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  mkdirSync(ARTIFACT_DIR, { recursive: true });
  preflight();

  // Stop an unattended run without killing it mid-repetition, which would leave
  // the reader held and the record's last row a lie. Checked between reps only.
  const stopFile = path.join(ARTIFACT_DIR, 'STOP');
  if (existsSync(stopFile)) rmSync(stopFile);

  console.log(`[suite-runs] shape=${args.shape} reps=${args.reps}${args.target ? ` target=${args.target}` : ''}`);
  console.log(`[suite-runs] stop cleanly with: touch ${path.relative(process.cwd(), stopFile)}`);

  for (let rep = 1; rep <= args.reps; rep += 1) {
    if (existsSync(stopFile)) {
      console.log(`[suite-runs] STOP seen; finished ${rep - 1}/${args.reps} repetitions.`);
      rmSync(stopFile);
      break;
    }
    const record = runOnce({ ...args, rep });
    const failedFiles = record.files.filter((f) => f.status === 'failed');
    const summary = failedFiles.length
      ? failedFiles.map((f) => `${f.name} (${f.failed.length})`).join(', ')
      : 'none';
    console.log(
      `[suite-runs] ${args.shape} rep ${rep}/${args.reps}` +
        ` exit=${record.exitCode}` +
        ` ${Math.round(record.durationMs / 1000)}s` +
        ` clients@start=${record.wsClientsAtStart}` +
        ` failed: ${summary}` +
        (record.reportMissing ? ' [REPORT MISSING]' : '')
    );
  }

  console.log(`[suite-runs] record: ${RECORD_PATH}`);
}

main();
