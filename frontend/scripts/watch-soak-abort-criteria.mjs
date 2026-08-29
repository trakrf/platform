#!/usr/bin/env node
/**
 * Overnight soak watchdog: abort the moment the run stops being able to produce
 * evidence, and stay silent otherwise.
 *
 * Provenance: TRA-1189 established the abort criteria, TRA-1193 ran the first
 * long soak under them, TRA-1203 promoted this out of a scratchpad and replaced
 * its restart check. Silence means healthy — this exits only when the run ends
 * or something is actually wrong, because an exit is what notifies the operator.
 *
 * It deliberately does NOT abort on a `Device is busy` refusal. That is a rate
 * to be measured across the whole run, not a hazard; stopping at the first hit
 * trades the measurement for an anecdote.
 *
 * ## What it does not do: judge whether the daemon runs current code
 *
 * That is ble-mcp-test's job, in its own `pretest` staleness guard, and it is a
 * genuinely different question. This watchdog asks *"is it the same process"*;
 * that guard asks *"is that process running current code"*. A daemon can hold
 * one identity for six days, keep this watchdog green throughout, and be exactly
 * the staleness failure the other check exists to catch. Building a second
 * currency check here is how two checks drift into disagreeing.
 *
 * ## Exit codes
 *
 *   0  the run ended normally (the driver is gone)
 *   2  the bridge restarted, or stopped answering
 *   3  five or more consecutive reps with transport failures
 *   4  the newest rep is a void capture
 *   5  the field was not clear at start (pre-flight gate)
 *  64  usage error
 */

import net from 'net';
import path from 'path';
import { appendFileSync, existsSync, readFileSync } from 'fs';
import { spawnSync } from 'child_process';
import { captureCanaryCount } from './suite-run-signals.mjs';

/**
 * Has the process we started with been replaced?
 *
 * Capture uptime AND wall time at run start, then assert that the daemon has
 * been up for at least as long as the wall clock says has elapsed:
 *
 *     uptime_now >= uptime_start + (wall_now - wall_start) - tolerance
 *
 * The naive form — "did uptime go down" — is only correct until uptime re-grows
 * past where it started. A daemon 100s old at run start that restarts 10s in
 * reports 590s at the next 600s poll, which is larger than 100, so the naive
 * check passes and the night silently spans two daemons. The arithmetic fails
 * wherever in the interval the restart landed.
 *
 * `uptime_seconds` is monotonic-derived (ble-mcp-test builds its ControlServer
 * with `started_at=time.monotonic()`), so it cannot be walked by an NTP step and
 * the tolerance covers sample skew only. Do not widen it into a defensive fudge
 * for clock drift: there is nothing to defend against, and every second of
 * slack is a second a real restart can hide in.
 *
 * Known and deliberate: a host SUSPEND advances the wall clock while
 * CLOCK_MONOTONIC stands still, so this fires. That is the correct outcome —
 * a soak whose host suspended mid-run is void regardless of what the daemon did.
 *
 * A missing reading is a restart. An unreachable daemon cannot be distinguished
 * from a wedged one, and either way the rest of the night is junk rows;
 * detection needs the daemon to answer at all, not to answer honestly.
 */
export function hasRestarted({ uptimeStart, wallStart, uptimeNow, wallNow, toleranceSeconds }) {
  if (typeof uptimeNow !== 'number' || !Number.isFinite(uptimeNow)) return true;
  const elapsed = wallNow - wallStart;
  return uptimeNow < uptimeStart + elapsed - toleranceSeconds;
}

/**
 * Is anything already holding or watching the command path?
 *
 * `observer_count > 0` is the hazard that appears in no process listing and no
 * log — most often a leftover mock-injected browser tab. On 2026-08-26
 * contention of exactly this kind invalidated two hardware runs inside ten
 * minutes, and neither was visible until the data came out wrong.
 *
 * A reply that does not carry both fields is not evidence of a clear field.
 */
export function fieldIsClear(state) {
  if (!state || typeof state !== 'object') return false;
  if (typeof state.held !== 'boolean') return false;
  if (typeof state.observer_count !== 'number') return false;
  return state.held === false && state.observer_count === 0;
}

/**
 * The bridge's control socket.
 *
 * Under `/run/user/<uid>` by construction — which is also why the unit serving
 * it must be user-scoped. A system unit has no `/run/user/<uid>` and comes up
 * looking healthy with no control surface at all.
 */
export function controlSocketPath() {
  if (process.env.BLE_MCP_SOCKET_PATH) return process.env.BLE_MCP_SOCKET_PATH;
  const runtimeDir = process.env.XDG_RUNTIME_DIR || `/run/user/${process.getuid()}`;
  return path.join(runtimeDir, 'ble-bridge.sock');
}

/**
 * One request, one reply, newline-delimited JSON, connection closed by us.
 *
 * Returns null on any failure — refused, timed out, malformed. Every caller
 * treats null as an abort condition rather than as an unknown, so there is no
 * path where a broken socket reads as a healthy bridge.
 */
export function callControl(op, { socketPath = controlSocketPath(), timeoutMs = 5000 } = {}) {
  return new Promise((resolve) => {
    let settled = false;
    let buf = '';
    const done = (value) => {
      if (settled) return;
      settled = true;
      sock.destroy();
      resolve(value);
    };

    const sock = net.connect(socketPath);
    sock.setTimeout(timeoutMs);
    sock.on('connect', () => sock.write(`${JSON.stringify({ op })}\n`));
    sock.on('data', (chunk) => {
      buf += chunk;
      const nl = buf.indexOf('\n');
      if (nl === -1) return;
      try {
        const reply = JSON.parse(buf.slice(0, nl));
        done(reply && reply.ok ? reply.result : null);
      } catch {
        done(null);
      }
    });
    sock.on('timeout', () => done(null));
    sock.on('error', () => done(null));
    sock.on('close', () => done(null));
  });
}

/**
 * Is anything serving the bridge port?
 *
 * Identified by what it does, not by what it is called — the same technique
 * `characterise-suite-runs.mjs` settled on after two name-based versions were
 * wrong in a row. `pgrep` for a daemon name has failed here three separate
 * ways: it named a deleted Rust binary, then a Python module name that never
 * appears in a cmdline, and it matches its own shell's argv besides.
 *
 * This is a local question about a local listener, so a local tool answers it.
 */
export function portIsServed(port) {
  const res = spawnSync('ss', ['-ltnH', 'sport', `= :${port}`], { encoding: 'utf8' });
  return res.status === 0 && typeof res.stdout === 'string' && res.stdout.trim() !== '';
}

/** Consecutive newest-first reps whose transport never came up. */
export function transportFailureStreak(rows) {
  let streak = 0;
  for (let i = rows.length - 1; i >= 0; i -= 1) {
    const s = rows[i].signals ?? {};
    if (s.transportUnreachable || s.transportRefused) streak += 1;
    else break;
  }
  return streak;
}

/**
 * A rep that captured no output at all.
 *
 * Its other counts are not low, they are uninformative: zero canary lines means
 * nothing was observed, so every other signal in that row is an absence of
 * evidence being read as evidence of absence.
 *
 * ## Why this asks for the canary by ROLE and not by name (TRA-1206)
 *
 * This used to read `(row.signals?.harnessLines ?? 1) === 0`, which is correct
 * for a vitest rep and silently wrong for every other kind. `[Harness]` is
 * emitted by `tests/integration/cs108/CS108WorkerTestHarness.ts` and by nothing
 * else, so a Playwright rep cannot produce one however healthy it is. The driver
 * therefore records `harnessLines: null` on an e2e row rather than 0 — and the
 * old expression would then have gone BOTH ways wrong:
 *
 *   with 0    every e2e rep aborts at rep 1, on the absence of the emitter
 *             rather than on a void capture
 *   with null `null ?? 1` is 1, so the check never fires again on that path —
 *             a working abort quietly becoming a no-op, which is worse, because
 *             nothing looks broken
 *
 * `captureCanaryCount` resolves the needle from the row's own runner, so the
 * question stays "did this rep observe anything" on both paths. A record with no
 * `runner` field is a vitest record — every row written before TRA-1206 is.
 *
 * An UNKNOWN canary is void. A row whose log went missing did not measure a
 * clean capture, it measured nothing, and the safe reading of nothing is to stop
 * the run rather than to assume health.
 */
export function isVoidCapture(row) {
  if (!row) return false;
  const count = captureCanaryCount(row);
  return count === null || count === 0;
}

function readRows(file) {
  if (!existsSync(file)) return [];
  return readFileSync(file, 'utf-8')
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
 * Why --driver-pid is an explicit pid and not a name match.
 *
 * A name match scans argv, so it matches the argv of the very pipeline running
 * it — the watchdog reports the driver alive because it can see its own command
 * line. That produced a false abort during TRA-1189. A pid answers identity
 * directly and cannot match itself.
 */
function usage(message) {
  process.stderr.write(`${message}

usage: watch-soak-abort-criteria.mjs --driver-pid <pid> --runs <runs.jsonl> [options]

  --driver-pid <pid>   the soak driver to watch. Required, and an explicit pid
                       rather than a name match — see the note in the source.
  --runs <path>        the append-only run log to read signals from.
  --identity <path>    append the captured start-of-run facts here.
  --poll <seconds>     default 600.
  --tolerance <secs>   uptime/wall sample skew allowance, default 10.
  --port <port>        bridge port, default $BLE_MCP_WS_PORT or 25153.
`);
  process.exit(64);
}

function parseArgs(argv) {
  const opts = {
    driverPid: null,
    runs: null,
    identity: null,
    pollSeconds: 600,
    toleranceSeconds: 10,
    port: process.env.BLE_MCP_WS_PORT || '25153',
  };
  for (let i = 0; i < argv.length; i += 2) {
    const value = argv[i + 1];
    switch (argv[i]) {
      case '--driver-pid': opts.driverPid = Number(value); break;
      case '--runs': opts.runs = value; break;
      case '--identity': opts.identity = value; break;
      case '--poll': opts.pollSeconds = Number(value); break;
      case '--tolerance': opts.toleranceSeconds = Number(value); break;
      case '--port': opts.port = value; break;
      default: usage(`unrecognised argument: ${argv[i]}`);
    }
  }
  if (!Number.isInteger(opts.driverPid)) usage('--driver-pid is required.');
  if (!opts.runs) usage('--runs is required.');
  return opts;
}

const say = (msg) => process.stdout.write(`[${new Date().toISOString()}] ${msg}\n`);

function driverIsAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

async function main() {
  const opts = parseArgs(process.argv.slice(2));

  // ---- pre-flight: the field must be clear BEFORE the first rep -----------
  const state = await callControl('get_connection_state');

  // "No answer" and "answered, field busy" are different faults with different
  // fixes, and collapsing them sends the operator hunting for a browser tab
  // when the actual problem is that the bridge is not running. Reporting the
  // wrong subject is the same defect class this watchdog exists to remove.
  if (state === null) {
    say(`ABORT before start: the bridge did not answer on ${controlSocketPath()}.`);
    say('Check it is up:  systemctl --user status ble-bridge');
    process.exit(2);
  }
  if (!fieldIsClear(state)) {
    say(`ABORT before start: the field is not clear — ${JSON.stringify(state)}`);
    say('Something already holds or observes the command path. A leftover');
    say('mock-injected browser tab is the usual cause and appears in no process list.');
    process.exit(5);
  }

  const status = await callControl('status');
  if (!status || typeof status.uptime_seconds !== 'number') {
    say('ABORT before start: the bridge did not answer status.');
    process.exit(2);
  }

  const uptimeStart = status.uptime_seconds;
  const wallStart = Date.now() / 1000;

  // The run-identity record. "The field was clear at start" is evidence only if
  // it was written down at the time; reconstructed afterwards it is an
  // assumption wearing evidence's clothes.
  const identity = [
    `watchdog started      ${new Date().toISOString()}`,
    `driver pid            ${opts.driverPid}`,
    `bridge uptime start   ${uptimeStart.toFixed(3)}s`,
    `bridge transport      ${status.esphome_configured ? status.esphome_proxy : 'NOT CONFIGURED'}`,
    `bridge device         ${status.device_mac ?? 'none'}`,
    `field at start        held=${state.held} observer_count=${state.observer_count}`,
  ].join('\n');
  say(`pre-flight clear.\n${identity}`);
  if (opts.identity) appendFileSync(opts.identity, `${identity}\n`);

  for (;;) {
    if (!driverIsAlive(opts.driverPid)) {
      say(`RUN ENDED — driver pid ${opts.driverPid} is gone.`);
      process.exit(0);
    }

    // ---- the same process that started the run? --------------------------
    const now = await callControl('status');
    const wallNow = Date.now() / 1000;
    const uptimeNow = now && typeof now.uptime_seconds === 'number' ? now.uptime_seconds : null;

    if (hasRestarted({ uptimeStart, wallStart, uptimeNow, wallNow, toleranceSeconds: opts.toleranceSeconds })) {
      say('ABORT: the bridge is not the process this run started with.');
      say(`  uptime at start ${uptimeStart.toFixed(3)}s, now ${uptimeNow === null ? 'UNREACHABLE' : `${uptimeNow.toFixed(3)}s`}`);
      say(`  wall elapsed    ${(wallNow - wallStart).toFixed(3)}s`);
      say('Rows from here on would span two daemons with no marker in the data.');
      say('Forensics: journalctl --user -u ble-bridge --since <run start>. The');
      say("daemon's own log ring is per-process and cannot explain a restart it did not survive.");
      process.exit(2);
    }

    if (!portIsServed(opts.port)) {
      say(`ABORT: nothing is serving the bridge port ${opts.port}.`);
      process.exit(2);
    }

    // ---- is the run still producing usable rows? -------------------------
    const rows = readRows(opts.runs);
    const streak = transportFailureStreak(rows);
    if (streak >= 5) {
      say(`ABORT: ${streak} consecutive reps with transportUnreachable/Refused — the stack is down.`);
      process.exit(3);
    }
    if (isVoidCapture(rows[rows.length - 1])) {
      say('ABORT: newest rep captured no output — VOID capture, its other counts are uninformative.');
      say(`  canary=${JSON.stringify(captureCanaryCount(rows[rows.length - 1]))} ` +
        `runner=${rows[rows.length - 1]?.runner ?? 'vitest'}`);
      process.exit(4);
    }

    await new Promise((resolve) => { setTimeout(resolve, opts.pollSeconds * 1000); });
  }
}

// Only run the loop when invoked directly; importing must be side-effect free
// so the arithmetic above can be unit-tested.
if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  main();
}
