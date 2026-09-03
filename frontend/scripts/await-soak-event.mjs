#!/usr/bin/env node
/**
 * Block until a running arm produces news, print it, and EXIT.
 *
 * ## `await-`, not `watch-`. The contract is that it TERMINATES.
 *
 * The other two soak processes are long-lived: `characterise-suite-runs.mjs`
 * runs the arm for ~7 h and `watch-soak-abort-criteria.mjs` guards it for the
 * same. This one lives for seconds to minutes and is thrown away:
 *
 *   runner    ~7 h, detached   does the arm
 *   watchdog  ~7 h, detached   aborts on instrument faults
 *   watcher   seconds, disposable   blocks until news, prints it, exits
 *
 * A name saying "watch" invites someone to turn it back into a stream, and a
 * stream is precisely the defect. It serves a human and an agent identically:
 * run it in a terminal and it blocks then prints; background it from a session
 * and its EXIT is what re-invokes the model, which reports and re-runs it. A
 * chain of one-shots.
 *
 * ## Why an exit, rather than a stream of lines
 *
 * TRA-1240 fixed what the driver EMITS and nothing about who reads it. Every one
 * of those lines goes to a file, which was fine while §2 was typed into a
 * terminal somebody could `tail -f`, and is false when the arm is launched from
 * a session: the log is a dead end and the operator sees nothing for seven
 * hours. It happened twice, on two consecutive arms.
 *
 * Notifying is not invoking, and that distinction is the whole design:
 *
 *   Monitor                NO turn — lines reach context and sit there unread
 *   CronCreate / a loop    a turn on a CLOCK, whether or not anything happened
 *   a backgrounded exit    a turn on an EVENT — one per real event, the minimum
 *
 * In-REPL delivery always costs one model turn; the only choice is what triggers
 * it. A 15-minute cron over a 7.3 h arm is ~29 turns to report a number that
 * changed ~20 times. An exit is one turn per actual event, and none while quiet.
 *
 * ## Filter coverage IS the correctness requirement
 *
 * A watcher that can hang stops the chain, and a dead chain is indistinguishable
 * from a quiet arm — the same silence-is-not-success rule the abort criteria
 * follow. So it exits on ANY of: a new progress block, a new watchdog line, a
 * pre-registered signal moving, or the driver disappearing. One that greps only
 * the happy path is silent through a crash.
 *
 * Liveness has to be checkable without self-matching (§9): `pgrep -af "sleep 30"`
 * returned the grep's own argv while this was being written. The driver pid in
 * this process's argv is the distinctive part —
 *
 *     pgrep -c -f 'driver-pid 59482'
 *
 * Usage:
 *   node scripts/await-soak-event.mjs --driver-pid <pid> [options]
 *
 *   --driver-pid <pid>   the arm to watch. Required, explicit, never a name match.
 *   --driver-log <path>  default the newest .suite-runs/ARM-*-driver.log
 *   --watchdog-log <p>   default the newest .suite-runs/ARM-*-watchdog.log
 *   --runs <path>        default .suite-runs/runs.jsonl
 *   --signal <name>      a needle from suite-run-signals.mjs whose running total
 *                        is this arm's primary read. Pick one that is expected
 *                        to stay put: a noisy needle turns the chain back into a
 *                        stream.
 *   --poll <seconds>     default 60.
 *
 * Refs: TRA-1242.
 */

import { existsSync, readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';

/** A progress block: its `--- ` header through the `  last N:` strip. */
export function progressBlocks(text) {
  const blocks = [];
  let current = null;
  for (const line of (text ?? '').split('\n')) {
    if (line.startsWith('--- ')) current = [line];
    else if (current) {
      current.push(line);
      if (line.startsWith('  last ')) {
        blocks.push(current.join('\n'));
        current = null;
      }
    }
  }
  return blocks;
}

/**
 * The running total of one needle across the record.
 *
 * A row whose value is `null` could not observe the needle at all, and is
 * skipped rather than counted as zero — the null-vs-zero rule the whole
 * instrument turns on. Counting it as 0 makes a total that cannot move look like
 * a clean arm, which is the failure this is supposed to report.
 */
export function signalTotal(rows, name) {
  let total = 0;
  for (const row of rows ?? []) {
    const value = row?.signals?.[name];
    if (typeof value === 'number') total += value;
    else if (value && typeof value === 'object') {
      total += Object.values(value).reduce((a, b) => a + (typeof b === 'number' ? b : 0), 0);
    }
  }
  return total;
}

/** The arm is over. Final counts, and the capture that has a deadline. */
export function driverGoneReport(rows) {
  const done = (rows ?? []).length;
  const failed = (rows ?? []).filter((r) => r.exitCode !== 0).length;
  const rate = done ? ((100 * failed) / done).toFixed(1) : '0.0';
  return [
    `ARM ENDED — the driver pid is gone. ${done} rep(s): ${done - failed} passed, ${failed} failed (${rate}%).`,
    '',
    '§6 — capture BEFORE you analyse, and the first one has a deadline:',
    '  1. the bridge ring dies with the daemon, so dump it FIRST:',
    '       node scripts/dump-bridge-ring.mjs ~/soak-archives/<arm>/ring.jsonl',
    '  2. archive .suite-runs/ — the per-rep logs, not just runs.jsonl. The next',
    '     arm overwrites them, and the previous arm was very nearly lost this way.',
    '  3. verify the copy by file count and summed bytes. NOT du.',
  ].join('\n');
}

/**
 * The one event to report, or null to keep blocking.
 *
 * Order is deliberate. A watchdog abort and a dead driver are both true on an
 * aborted arm, and the watchdog line is the one that says WHY; the chain
 * re-arms, so the driver-gone event still lands on the next run.
 */
export function detectEvent(before, after, { signal = null } = {}) {
  const wasLines = (before.watchdogLog ?? '').split('\n').filter(Boolean);
  const nowLines = (after.watchdogLog ?? '').split('\n').filter(Boolean);
  if (nowLines.length > wasLines.length) {
    return { kind: 'watchdog', text: nowLines.slice(wasLines.length).join('\n'), terminal: false };
  }

  // Checked against the CURRENT observation, not against a change, so a watcher
  // armed after the arm already ended exits immediately instead of blocking
  // forever on an arm that will never speak again.
  if (!after.alive) {
    return { kind: 'driver-gone', text: driverGoneReport(after.rows), terminal: true };
  }

  const wasBlocks = progressBlocks(before.driverLog);
  const nowBlocks = progressBlocks(after.driverLog);
  if (nowBlocks.length > wasBlocks.length) {
    return { kind: 'progress', text: nowBlocks[nowBlocks.length - 1], terminal: false };
  }

  if (signal) {
    const was = signalTotal(before.rows, signal);
    const now = signalTotal(after.rows, signal);
    if (now > was) {
      return {
        kind: 'signal',
        text: `${signal} moved: ${was} -> ${now} over ${after.rows.length} rep(s).`,
        terminal: false,
      };
    }
  }

  return null;
}

const read = (file) => (file && existsSync(file) ? readFileSync(file, 'utf8') : '');

function readRows(file) {
  return read(file)
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

/** The newest `.suite-runs/ARM-*-<kind>.log`, so the ordinary call needs no paths. */
function newestArmLog(dir, kind) {
  let names;
  try {
    names = readdirSync(dir).filter((n) => n.startsWith('ARM-') && n.endsWith(`-${kind}.log`));
  } catch {
    return null;
  }
  names.sort();
  return names.length ? path.join(dir, names[names.length - 1]) : null;
}

function driverIsAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function usage(message) {
  process.stderr.write(`${message}

usage: await-soak-event.mjs --driver-pid <pid> [--driver-log <p>] [--watchdog-log <p>]
                            [--runs <p>] [--signal <name>] [--poll <seconds>]

Blocks until the arm produces news, prints ONE event, and exits 0. Re-run it to
wait for the next one. See docs/runbooks/running-a-soak-arm.md §2.
`);
  process.exit(64);
}

function parseArgs(argv) {
  const opts = {
    driverPid: null,
    driverLog: null,
    watchdogLog: null,
    runs: null,
    signal: null,
    pollSeconds: 60,
  };
  for (let i = 0; i < argv.length; i += 2) {
    const value = argv[i + 1];
    switch (argv[i]) {
      case '--driver-pid': opts.driverPid = Number(value); break;
      case '--driver-log': opts.driverLog = value; break;
      case '--watchdog-log': opts.watchdogLog = value; break;
      case '--runs': opts.runs = value; break;
      case '--signal': opts.signal = value; break;
      case '--poll': opts.pollSeconds = Number(value); break;
      default: usage(`unrecognised argument: ${argv[i]}`);
    }
  }
  if (!Number.isInteger(opts.driverPid)) usage('--driver-pid is required.');
  const dir = path.resolve(process.cwd(), '.suite-runs');
  opts.driverLog ??= newestArmLog(dir, 'driver');
  opts.watchdogLog ??= newestArmLog(dir, 'watchdog');
  opts.runs ??= path.join(dir, 'runs.jsonl');
  return opts;
}

async function main() {
  const opts = parseArgs(process.argv.slice(2));
  const observe = () => ({
    driverLog: read(opts.driverLog),
    watchdogLog: read(opts.watchdogLog),
    rows: readRows(opts.runs),
    alive: driverIsAlive(opts.driverPid),
  });

  const baseline = observe();
  for (;;) {
    const event = detectEvent(baseline, observe(), { signal: opts.signal });
    if (event) {
      process.stdout.write(`[${new Date().toISOString()}] ${event.kind}\n${event.text}\n`);
      if (!event.terminal) {
        process.stdout.write(
          `\nre-arm: node scripts/await-soak-event.mjs --driver-pid ${opts.driverPid}` +
            `${opts.signal ? ` --signal ${opts.signal}` : ''}\n`
        );
      }
      process.exit(0);
    }
    await new Promise((resolve) => { setTimeout(resolve, opts.pollSeconds * 1000); });
  }
}

// Importing must stay side-effect free so the detection above can be unit-tested.
if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  await main();
}
