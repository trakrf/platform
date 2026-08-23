#!/usr/bin/env node
/**
 * TRA-1167 — run-shape driver for characterising the hardware integration suite.
 *
 * Phase 1 of TRA-1167 is "characterise, change nothing". This script therefore
 * produces every run shape from CLI flags and process lifecycle ONLY. It never
 * edits vitest.config.ts, package.json, or any spec — the suite under test
 * cannot observe that it is being characterised.
 *
 * Shapes:
 *   fixed    current behaviour — the same flags `pnpm test:integration` uses
 *   shuffle  --sequence.shuffle.files with a recorded seed, so any interesting
 *            order is reproducible
 *   alone    one file per invocation (--target)
 *   cold     same as fixed, but the caller restarted the bridge process first;
 *            this script only records that the claim was made
 *
 * NOTE ON "alone": it does NOT give a cold reader. The Rust bridge calls
 * transport.connect() once at process start and holds the BLE link for the life
 * of the process; a WS disconnect tears down nothing. So a file run alone still
 * attaches to a link carrying whatever the previous run left. That is why the
 * `cold` shape exists separately.
 *
 * Usage:
 *   node scripts/tra-1167-characterise.mjs --shape fixed --reps 5
 *   node scripts/tra-1167-characterise.mjs --shape shuffle --reps 5
 *   node scripts/tra-1167-characterise.mjs --shape alone --reps 3 --target tests/integration/cs108/locate.spec.ts
 *   node scripts/tra-1167-characterise.mjs --shape cold --reps 3
 */

import { spawnSync } from 'node:child_process';
import { mkdirSync, readFileSync, appendFileSync, rmSync, existsSync } from 'node:fs';
import path from 'node:path';

const RECORD_SCHEMA = 1;
const ARTIFACT_DIR = path.resolve(process.cwd(), '.tra-1167');
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
  const res = spawnSync('ss', ['-tn', 'state', 'established', 'dst', '127.0.0.1:8080'], {
    encoding: 'utf8',
  });
  if (res.status !== 0 || typeof res.stdout !== 'string') return null;
  // First line is the ss header; every remaining line is one client socket.
  const lines = res.stdout.trim().split('\n').filter(Boolean);
  return Math.max(0, lines.length - 1);
}

function readBridgeProcess() {
  const pidRes = spawnSync('pgrep', ['-f', 'rust-ble-test'], { encoding: 'utf8' });
  if (pidRes.status !== 0) return { bridgePid: null, bridgeStartedAt: null };
  const pid = Number(pidRes.stdout.trim().split('\n')[0]);
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
  const args = ['vitest', 'run', filter, '--no-file-parallelism'];
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
  const files = suites.map((suite) => ({
    name: path.relative(process.cwd(), suite.name),
    status: suite.status === 'passed' ? 'passed' : 'failed',
    failed: (suite.assertionResults || [])
      .filter((a) => a.status === 'failed')
      .map((a) => a.fullName || a.title),
  }));
  return { files, reportMissing: false };
}

function runOnce({ shape, rep, target, note }) {
  const { args, seed } = buildVitestArgs(shape, rep, target);
  const reportPath = path.join(ARTIFACT_DIR, `report-${shape}-${rep}.json`);
  rmSync(reportPath, { force: true });

  const wsClientsAtStart = countBridgeClients();
  const { bridgePid, bridgeStartedAt } = readBridgeProcess();
  const startedAt = new Date();

  // stdio 'inherit' for stderr keeps live progress visible; the exit status is
  // read straight off the spawned process. Never pipe this into anything — a
  // pipeline reports its LAST stage's status, which is how a red suite reads
  // green.
  const res = spawnSync('npx', [...args, '--reporter=json', `--outputFile=${reportPath}`], {
    encoding: 'utf8',
    stdio: ['ignore', 'ignore', 'inherit'],
  });

  const endedAt = new Date();
  const { files, reportMissing } = readReport(reportPath);

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
    bridgePid,
    bridgeStartedAt,
    wsClientsAtStart,
    note: note ?? null,
  };

  appendFileSync(RECORD_PATH, `${JSON.stringify(record)}\n`);
  return record;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  mkdirSync(ARTIFACT_DIR, { recursive: true });

  console.log(`[tra-1167] shape=${args.shape} reps=${args.reps}${args.target ? ` target=${args.target}` : ''}`);

  for (let rep = 1; rep <= args.reps; rep += 1) {
    const record = runOnce({ ...args, rep });
    const failedFiles = record.files.filter((f) => f.status === 'failed');
    const summary = failedFiles.length
      ? failedFiles.map((f) => `${f.name} (${f.failed.length})`).join(', ')
      : 'none';
    console.log(
      `[tra-1167] ${args.shape} rep ${rep}/${args.reps}` +
        ` exit=${record.exitCode}` +
        ` ${Math.round(record.durationMs / 1000)}s` +
        ` clients@start=${record.wsClientsAtStart}` +
        ` failed: ${summary}` +
        (record.reportMissing ? ' [REPORT MISSING]' : '')
    );
  }

  console.log(`[tra-1167] record: ${RECORD_PATH}`);
}

main();
