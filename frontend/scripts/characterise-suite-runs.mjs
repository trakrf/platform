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
 * Runners (TRA-1206):
 *   vitest   tests/integration/, the default, and the one every historical
 *            record was measured under
 *   e2e      tests/e2e/ under Playwright — the suite TRA-1200 measures
 *
 * The two backends share the rep loop, the archive conventions and the record
 * format on purpose. A sibling driver would have duplicated all three and left
 * summarise-suite-runs.mjs reading two formats.
 *
 * They do NOT share a needle table. Most of `SIGNALS` is vitest-shaped —
 * `[Harness]` comes from an integration-only file — so an e2e record carries
 * explicit `null` for every signal that path cannot produce. See
 * suite-run-signals.mjs, where the per-needle reasoning lives. A structurally
 * absent signal and a genuinely zero one must not look alike; recording 0 for
 * `harnessLines` would make the watchdog's void-capture abort fire on every
 * single e2e rep.
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
 *   node scripts/characterise-suite-runs.mjs --runner e2e --shape alone --reps 40 \
 *     --target tests/e2e/inventory.spec.ts
 */

import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, appendFileSync, rmSync, existsSync, openSync, closeSync } from 'node:fs';
import { formatRepLine, formatProgressBlock } from './arm-progress.mjs';
import path from 'node:path';
import { readSignals, readReadCycles } from './suite-run-signals.mjs';

// 2 adds `signals` + `outputLog`; schema-1 records carry neither.
// 3 adds `runner`, and `appPreflight` on e2e records only (TRA-1206).
//
// `appPreflight` is deliberately absent rather than null on a vitest record: a
// vitest rep has no application to reach, so a field about its reachability
// would be a measurement of a subject that does not exist. Same asymmetry as
// the signals — see readSignals().
//
// Everything schema 2 carried is unchanged in name, type and derivation. That
// is load-bearing: TRA-1189's 528 reps and TRA-1193's 200-rep verification are
// the comparison baseline for this milestone, and they are only comparable if
// the vitest path still measures what it measured then.
const RECORD_SCHEMA = 3;

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

/**
 * How often the progress block prints, in reps (TRA-1240).
 *
 * Ten is roughly 22 minutes at a healthy 135s rep — often enough to catch a
 * wedge while it is still worth stopping for, rare enough that the block does
 * not become the noise it exists to cut through.
 */
const PROGRESS_EVERY = 10;
const SUITE_ROOTS = {
  vitest: 'tests/integration/',
  e2e: 'tests/e2e/',
};
const VALID_RUNNERS = Object.keys(SUITE_ROOTS);
const VALID_SHAPES = ['fixed', 'shuffle', 'alone', 'cold'];

/** The suite a runner drives when no --target narrows it. */
export function suiteRootFor(runner) {
  const root = SUITE_ROOTS[runner];
  if (!root) {
    throw new Error(`--runner must be one of ${VALID_RUNNERS.join('|')}, got: ${runner}`);
  }
  return root;
}

/**
 * Refuse a shape the runner cannot actually produce.
 *
 * `shuffle` has no Playwright analogue. Playwright can shard and can repeat, but
 * it has no seeded file-order shuffle, and there is no flag combination that
 * reproduces one from the record. Accepting the flag and running the default
 * order would write a record claiming a shape the run never had — an instrument
 * reporting a confident, well-formed, wrong answer, which is the failure this
 * whole script exists to avoid.
 *
 * Rejecting loudly is the only honest option: the alternative is a soak whose
 * order-dependence conclusion is drawn from repetitions that all ran in the same
 * order. That already happened once here, on the vitest path, for a different
 * reason (see readVitestReport's note on startTime).
 */
/**
 * Field density for a rep, or `undefined` where the runner has none.
 *
 * `undefined` rather than an all-null object, because the caller assigns
 * conditionally and that is what keeps the key OFF a vitest record instead of
 * present-and-null. Same asymmetry as `appPreflight`, and it is not cosmetic:
 * a vitest rep runs no application and reads no tags, so a null density field
 * would be a measurement of a subject that does not exist — and any new key on a
 * vitest record breaks comparability with TRA-1189's 528 reps and TRA-1193's
 * 200, which are the baseline this milestone is measured against.
 *
 * An e2e rep whose log is gone still gets the full null-filled shape: the runner
 * HAS read cycles, they just were not observed, and those are different claims.
 */
export function densityFor(logPath, runner) {
  if (runner !== 'e2e') return undefined;
  return readReadCycles(logPath, runner);
}

export function assertShapeSupported(runner, shape) {
  suiteRootFor(runner);
  if (runner === 'e2e' && shape === 'shuffle') {
    throw new Error(
      '--shape shuffle is not available for --runner e2e: Playwright has no seeded ' +
        'file-order shuffle, so the shape could not be reproduced from the record. ' +
        'Use --shape alone with --target to vary what runs first.'
    );
  }
}

function parseArgs(argv) {
  const args = { runner: 'vitest', shape: null, reps: 1, target: null, note: null };
  for (let i = 0; i < argv.length; i += 1) {
    const flag = argv[i];
    const value = argv[i + 1];
    switch (flag) {
      case '--runner': args.runner = value; i += 1; break;
      case '--shape': args.shape = value; i += 1; break;
      case '--reps': args.reps = Number(value); i += 1; break;
      case '--target': args.target = value; i += 1; break;
      case '--note': args.note = value; i += 1; break;
      default:
        throw new Error(`Unknown argument: ${flag}`);
    }
  }
  if (!VALID_RUNNERS.includes(args.runner)) {
    throw new Error(`--runner must be one of ${VALID_RUNNERS.join('|')}, got: ${args.runner}`);
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
  assertShapeSupported(args.runner, args.shape);
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
  const filter = target ?? suiteRootFor('vitest');
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
 * Worst-case wall clock for one Playwright repetition, in ms.
 *
 * THE `hookTimeout` QUESTION, ANSWERED FOR THE OTHER RUNNER — and the answer is
 * that the direct analogue is already set, by the suite, and the driver must not
 * touch it. Playwright applies one timeout to both tests and hooks, and
 * `inventory.spec.ts` self-configures it:
 *
 *     test.describe.configure({ timeout: HARDWARE_TEST_TIMEOUT_MS })   // 90000
 *
 * A describe-level timeout BEATS the CLI `--timeout` flag, so passing --timeout
 * here would change nothing on the very spec the soak runs while reading, in the
 * argv and in the record, as though a bound had been set. A flag that cannot
 * take effect is worse than no flag: it answers the question in the reader's
 * head without answering it in the process.
 *
 * What genuinely has no analogue is the REP-LEVEL bound. vitest's hook timeout
 * bounds a wedged connect and therefore bounds the invocation. Playwright's
 * per-test timeout bounds each test but nothing bounds the run: a browser that
 * never launches, a webServer probe that hangs, a reporter that never closes,
 * and spawnSync waits forever. Overnight that is worse than a failure — the
 * driver stops appending rows, the watchdog sees a live driver and a healthy
 * bridge, and the night silently produces nothing after 22:15.
 *
 * So the bound the driver owns is --global-timeout, derived from the spec:
 *
 *     beforeAll                          90s   (shared connection + mode change)
 *     5 tests x 90s                     450s   (2 active, 3 skipped — bounded
 *                                               for all five, because TRA-1200
 *                                               owns that spec and may enable
 *                                               them; a bound that breaks when
 *                                               a skip is lifted is a trap)
 *     ------------------------------------------
 *     suite worst case                  540s
 *     browser launch, webServer probe,
 *     teardown, reporter flush          ~120s  (bounded from above; not
 *                                               separately measured)
 *     ------------------------------------------
 *     total                             660s
 *
 * Resolved toward patience for the same reason the 90s hookTimeout was: being
 * too patient costs wall-clock on a rep that fails anyway, being too impatient
 * kills healthy reps and the soak measures the driver instead of the suite.
 *
 * Deliberately NOT equal to any vitest number here. It bounds a whole
 * invocation; --hookTimeout bounds one hook. They are different quantities over
 * different scopes and must be free to diverge.
 */
const PLAYWRIGHT_GLOBAL_TIMEOUT_MS = 660000;

function buildPlaywrightArgs(shape, rep, target) {
  // Same rule as the vitest path: a target narrows any shape, not just `alone`.
  const filter = target ?? suiteRootFor('e2e');

  // What is NOT here matters as much as what is:
  //
  //   --workers      playwright.config.ts pins workers:1 because 11 specs reach
  //                  ONE physical CS108 through one bridge. Restating it here
  //                  would mean the driver silently keeps working after someone
  //                  fixes the config, and silently overrides them if they ever
  //                  split the non-hardware specs into a parallel project.
  //   --retries      the config sets 0 outside CI. A soak measures a rate; a
  //                  retry turns a failure into a pass and destroys the rate.
  //   --timeout      inert against this spec — see the note above.
  //
  // The driver's standing invariant: it produces every shape from CLI flags and
  // process lifecycle only, and never restates or overrides the suite's own
  // settings. The moment it changes the subject, its measurements stop
  // describing the suite anyone else runs.
  const args = [
    'playwright',
    'test',
    filter,
    `--global-timeout=${PLAYWRIGHT_GLOBAL_TIMEOUT_MS}`,
  ];

  // No seed: Playwright has no seeded file-order shuffle, and `shuffle` is
  // rejected for this runner rather than silently degraded. assertShapeSupported
  // is the gate; this null is what the record then carries, meaning "this shape
  // had no seed" rather than "the seed was lost".
  void shape;
  void rep;
  return { args, seed: null };
}

/** Runner-dispatching argv builder. The vitest branch is byte-for-byte what it
 * always was — frozen by test, because TRA-1189's and TRA-1193's reps are only
 * comparable against a path that has not moved. */
export function buildRunnerArgs(runner, shape, rep, target) {
  suiteRootFor(runner);
  return runner === 'e2e'
    ? buildPlaywrightArgs(shape, rep, target)
    : buildVitestArgs(shape, rep, target);
}

/**
 * Turn vitest's JSON report into the per-file record.
 *
 * The JSON report is the source of truth for pass/fail — never stdout scraping.
 * A missing or unparseable report is recorded as such, so a broken run can
 * never read as an empty pass.
 */
export function readVitestReport(reportPath) {
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

/**
 * Turn Playwright's JSON report into the SAME per-file record shape.
 *
 * Same rule as the vitest reader: the JSON report is the source of truth for
 * pass/fail, never stdout scraping, and a missing or unparseable report is
 * recorded as such so a broken run can never read as an empty pass.
 *
 * Three things differ from vitest's report and each is a place to get it wrong:
 *
 *   1. SPECS NEST. Playwright's top-level `suites` are files; every real spec
 *      lives inside a child suite, because every real spec is inside a
 *      `describe`. A reader that only walks `suites[].specs` finds nothing in
 *      inventory.spec.ts and reports a file with zero failures — a confident,
 *      well-formed, wrong answer.
 *   2. startTime IS AN ISO STRING, not epoch ms. Downstream reads `files` as
 *      EXECUTION ORDER and the summariser's predecessor table calls element i-1
 *      the predecessor of element i, so this must sort by real time and emit the
 *      same numeric type the vitest path does. String-sorting ISO stamps happens
 *      to work; mixing types downstream does not.
 *   3. A LOAD ERROR PRODUCES NO SPECS AT ALL. A spec that fails to compile, or a
 *      global-setup failure, yields `errors[]` and an empty `suites[]`. Emitting
 *      `files: []` there is indistinguishable from "the run passed and nothing
 *      failed" — so each error becomes a failed row, named for its file where it
 *      has one and for the run where it does not.
 */
export function readPlaywrightReport(reportPath) {
  if (!existsSync(reportPath)) {
    return { files: [], reportMissing: true };
  }
  let parsed;
  try {
    parsed = JSON.parse(readFileSync(reportPath, 'utf8'));
  } catch {
    return { files: [], reportMissing: true };
  }

  // Walk the describe nesting, carrying the title path so a failure names the
  // same thing a reader sees in the reporter output.
  const collect = (suite, titlePath, out) => {
    for (const spec of suite.specs || []) {
      const results = (spec.tests || []).flatMap((t) => t.results || []);
      const startTimes = results.map((r) => Date.parse(r.startTime)).filter((n) => Number.isFinite(n));
      out.push({
        ok: spec.ok !== false,
        // `ok` is true for a SKIPPED spec as well as a passing one, so it cannot
        // answer "did anything actually run". Playwright reports the status on
        // the test rather than the spec.
        ran: (spec.tests || []).some((t) => t.status !== 'skipped'),
        fullName: [...titlePath, spec.title].filter(Boolean).join(' > '),
        startTime: startTimes.length ? Math.min(...startTimes) : null,
      });
    }
    for (const child of suite.suites || []) {
      collect(child, [...titlePath, child.title], out);
    }
  };

  // `suite.file` is relative to Playwright's rootDir, and rootDir is the
  // testDir — `.../frontend/tests/e2e` — so a bare `file` is
  // `inventory.spec.ts` where the vitest path records
  // `tests/integration/cs108/locate.spec.ts`.
  //
  // Left unresolved, the two runners write differently-rooted strings into the
  // SAME `files[].name` field: anything joining on that name across runners
  // silently matches nothing, and the recorded name does not even match the
  // `--target tests/e2e/inventory.spec.ts` the operator typed. Resolving against
  // rootDir the way Playwright itself does puts both runners in one namespace.
  const rootDir = parsed.config?.rootDir;
  const nameOf = (fileSuite) => {
    const file = fileSuite.file ?? fileSuite.title;
    if (!file || !rootDir) return file;
    return path.relative(process.cwd(), path.resolve(rootDir, file));
  };

  const files = [];
  for (const fileSuite of parsed.suites || []) {
    const specs = [];
    collect(fileSuite, [], specs);
    const times = specs.map((s) => s.startTime).filter((n) => n !== null);
    files.push({
      name: nameOf(fileSuite),
      // Three values, not two. A file whose specs ALL skipped is not a pass: no
      // assertion ran, so the row carries no evidence either way. A soak
      // measuring a failure rate would otherwise score a night of
      // nothing-executed reps as forty clean passes — the empty-pass hazard the
      // reportMissing flag exists for, arriving through a different door.
      // `status === 'failed'` consumers are unaffected.
      status: specs.some((s) => !s.ok)
        ? 'failed'
        : specs.length && !specs.some((s) => s.ran)
          ? 'skipped'
          : 'passed',
      startTime: times.length ? Math.min(...times) : null,
      failed: specs.filter((s) => !s.ok).map((s) => s.fullName),
    });
  }

  for (const error of parsed.errors || []) {
    const name = error?.location?.file ?? '(run-level error)';
    const existing = files.find((f) => f.name === name);
    const label = error?.message ?? 'unknown error';
    if (existing) {
      existing.status = 'failed';
      existing.failed.push(label);
    } else {
      files.push({ name, status: 'failed', startTime: null, failed: [label] });
    }
  }

  // Same sort, same reason as the vitest reader — see its note. A file with no
  // timed spec (a load error) sorts last rather than claiming to have run first.
  files.sort((a, b) => (a.startTime ?? Number.MAX_SAFE_INTEGER) - (b.startTime ?? Number.MAX_SAFE_INTEGER));
  return { files, reportMissing: false };
}

function readReportFor(runner, reportPath) {
  return runner === 'e2e' ? readPlaywrightReport(reportPath) : readVitestReport(reportPath);
}

function runOnce({ runner, shape, rep, target, note, appPreflight }) {
  const { args, seed } = buildRunnerArgs(runner, shape, rep, target);
  // The vitest artifact names are FROZEN — no `vitest-` infix — so a fresh
  // vitest record is comparable field-for-field against a known-good one from
  // TRA-1189 or TRA-1193 without a path difference to explain away. The infix
  // exists to stop an e2e rep and a vitest rep of the same shape+rep from
  // overwriting each other's report, which `report-<shape>-<rep>.json` alone
  // does not prevent.
  const tag = runner === 'vitest' ? `${shape}` : `${runner}-${shape}`;
  const reportPath = path.join(ARTIFACT_DIR, `report-${tag}-${rep}.json`);
  const logPath = path.join(ARTIFACT_DIR, `output-${RUN_ID}-${tag}-${rep}.log`);
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
    res =
      runner === 'e2e'
        ? // Playwright's equivalent of the two-reporter trick, and it exists for
          // exactly the same reason: `json` alone would take stdout and the
          // captured log would hold a machine-readable verdict and none of the
          // console lines the signal needles grep for.
          //
          // Where vitest takes `--outputFile.json=`, Playwright's json reporter
          // has no CLI form for its destination at all — it writes to stdout
          // unless PLAYWRIGHT_JSON_OUTPUT_NAME is set in the environment. Miss
          // that and the report interleaves with the console output in the log,
          // `readPlaywrightReport` finds no file, and the rep records
          // `reportMissing` on a run that worked perfectly.
          spawnSync('npx', [...args, '--reporter=json,list'], {
            encoding: 'utf8',
            stdio: ['ignore', logFd, logFd],
            env: { ...process.env, PLAYWRIGHT_JSON_OUTPUT_NAME: reportPath },
          })
        : spawnSync('npx', [...args, '--reporter=json', '--reporter=default', `--outputFile.json=${reportPath}`], {
            encoding: 'utf8',
            stdio: ['ignore', logFd, logFd],
          });
  } finally {
    closeSync(logFd);
  }

  const endedAt = new Date();
  const { files, reportMissing } = readReportFor(runner, reportPath);
  const signals = readSignals(logPath, runner);

  const record = {
    schema: RECORD_SCHEMA,
    runner,
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

  // e2e only. A vitest rep has no application to reach, so a field about its
  // reachability would describe a subject that does not exist — the same
  // conflation the explicit-null signals convention exists to prevent, one level
  // up. Present-and-null and absent mean different things here and both are used.
  if (appPreflight) record.appPreflight = appPreflight;

  // e2e only, same rule as appPreflight directly above — see densityFor().
  // TRA-1200: this is the run condition whose absence let an arm be compared
  // against a reference field it did not match.
  const readCycles = densityFor(logPath, runner);
  if (readCycles) record.readCycles = readCycles;

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

/**
 * Refuse to run an e2e soak against an application that is not there — and
 * record which question was actually asked.
 *
 * ## Read the config the subject comes from
 *
 * The target is resolved with the SAME expression `playwright.config.ts` uses,
 * `process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173'`, and not by
 * hardcoding 5173. Those look identical until PLAYWRIGHT_BASE_URL is set, at
 * which point the config also drops its `webServer` block entirely and the run
 * goes somewhere this check never looked. The general shape, worth recognising
 * elsewhere: A CHECK WHOSE SUBJECT IS CHOSEN BY CONFIGURATION THE CHECK DOES NOT
 * READ. It is the same defect as identifying the bridge by a process name — a
 * check that picks its own subject rather than asking what will actually run.
 * This driver got that wrong twice before it started asking the socket.
 *
 * ## Why the outcome goes in the record
 *
 * A remote base URL is not observable with local `ss`, so the honest answer
 * there is "not checked" rather than a fabricated pass. But an honest skip is
 * still a skip: if most real runs set PLAYWRIGHT_BASE_URL, this check is
 * *usually* skipped, and a check that almost never runs does not exist while
 * still appearing in the source and in the docs — a control that cannot go red,
 * wearing an honest label.
 *
 * Returning the mode and letting `runOnce` store it makes that auditable. A
 * night of forty reps where the reachability question was never asked is then a
 * thing you can count in the archive, instead of something you would have to
 * infer from the absence of an error.
 */
function appPreflight() {
  const baseUrl = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:5173';
  let url;
  try {
    url = new URL(baseUrl);
  } catch {
    console.error(`[suite-runs] FATAL: PLAYWRIGHT_BASE_URL is not a URL: ${baseUrl}`);
    process.exit(1);
  }

  if (!['localhost', '127.0.0.1', '::1', '[::1]'].includes(url.hostname)) {
    console.log(`[suite-runs] app target ${baseUrl} is remote; the listening check was NOT run.`);
    return { target: baseUrl, mode: 'skipped-remote', listening: null };
  }

  const port = url.port || (url.protocol === 'https:' ? '443' : '80');
  const res = spawnSync('ss', ['-ltn', 'sport', `= :${port}`], { encoding: 'utf8' });
  const listening =
    res.status === 0 && typeof res.stdout === 'string' && res.stdout.trim().split('\n').length > 1;

  if (!listening) {
    console.error(
      `[suite-runs] FATAL: nothing is listening on ${baseUrl}.\n` +
        '  playwright.config.ts expects the dev server to be up already — its `webServer`\n' +
        '  command in dev mode is an error message and `exit 1`, so every repetition would\n' +
        '  die before it reached the reader. Start it with `pnpm dev:bridge`, or set\n' +
        '  PLAYWRIGHT_BASE_URL to a deployment that is running.'
    );
    process.exit(1);
  }
  return { target: baseUrl, mode: 'checked', listening: true };
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  mkdirSync(ARTIFACT_DIR, { recursive: true });
  preflight();
  // e2e only, and passed through to every record — see appPreflight's note on
  // why an honest skip still has to be visible in the data.
  const app = args.runner === 'e2e' ? appPreflight() : null;

  // Stop an unattended run without killing it mid-repetition, which would leave
  // the reader held and the record's last row a lie. Checked between reps only.
  const stopFile = path.join(ARTIFACT_DIR, 'STOP');
  if (existsSync(stopFile)) rmSync(stopFile);

  console.log(
    `[suite-runs] runner=${args.runner} shape=${args.shape} reps=${args.reps}` +
      `${args.target ? ` target=${args.target}` : ''}`
  );
  console.log(`[suite-runs] stop cleanly with: touch ${path.relative(process.cwd(), stopFile)}`);

  // Held in memory only for the progress block; `runs.jsonl` remains the record
  // and is unchanged. TRA-1240.
  const done = [];
  const startedAt = Date.now();

  for (let rep = 1; rep <= args.reps; rep += 1) {
    if (existsSync(stopFile)) {
      console.log(`[suite-runs] STOP seen; finished ${rep - 1}/${args.reps} repetitions.`);
      rmSync(stopFile);
      break;
    }
    const record = runOnce({ ...args, rep, appPreflight: app });
    done.push(record);

    // One line per rep, now carrying WHY. The previous line reported `exit=1`
    // and a list of absolute spec paths, which meant every diagnosis started by
    // opening a per-rep log, and a wedge rep — the one most worth reading —
    // rendered as ~350 characters of five full paths. TRA-1240.
    console.log(`[suite-runs] ${formatRepLine(record, args.reps)}`);

    // An aggregate every PROGRESS_EVERY reps. The strip is the part that earns
    // its place: a wedge is a RUN of consecutive failures, and totals cannot
    // show a run — seven scattered and seven consecutive are the same number and
    // completely different arms.
    if (rep % PROGRESS_EVERY === 0 && rep !== args.reps) {
      console.log(formatProgressBlock(done, args.reps, startedAt));
    }
  }

  if (done.length) console.log(formatProgressBlock(done, args.reps, startedAt));

  console.log(`[suite-runs] record: ${RECORD_PATH}`);
}

// Only run the loop when invoked directly; importing must be side-effect free so
// the argv builders and report readers can be unit-tested. The frozen-argv test
// is the executable form of "the vitest path did not move" — an intention that
// is asserted rather than stated, which is what TRA-1206 asked for.
if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  main();
}
