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
 *   6  a client connected with a mock version the bridge did not expect
 *   7  the mock connected at pre-flight is the wrong version (see TRA-1216)
 *  64  usage error
 */

import net from 'net';
import os from 'os';
import path from 'path';
import { appendFileSync, existsSync, readFileSync } from 'fs';
import { spawnSync } from 'child_process';
import dotenv from 'dotenv';
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
 * Pull the identity fields out of a `status` reply, tolerating everything else.
 *
 * Deliberately schema-less: it reads the keys it knows and ignores the rest.
 * That property is load-bearing rather than incidental — it is the only reason
 * this automated consumer was unaffected when the peer added fields on
 * 2026-08-29, while the interactive path broke on the same change. Nothing
 * asserted it until TRA-1205, so it held by luck rather than by test.
 *
 * Absent is `null`, never a guess and never `undefined`. An empty-string
 * `instance_id` counts as absent: treating it as a value would make two daemons
 * that both failed to report compare EQUAL, which reads as "no restart" —
 * the fail-open direction.
 */
export function readIdentity(status) {
  const s = status && typeof status === 'object' ? status : {};
  const str = (v) => (typeof v === 'string' && v !== '' ? v : null);
  return {
    instanceId: str(s.instance_id),
    codeFingerprint: str(s.code_fingerprint),
    codeSourceRoot: str(s.code_source_root),
    uptimeSeconds: typeof s.uptime_seconds === 'number' ? s.uptime_seconds : null,
  };
}

/**
 * Did the bridge stop being the process this run started with?
 *
 * BOTH detectors always run and each is reported by name. They answer different
 * questions and neither subsumes the other:
 *
 *   instance_id   is this a DIFFERENT PROCESS — an equality test, with no
 *                 tolerance window for a real restart to hide in
 *   the uptime    has this process run for the WHOLE INTERVAL I measured — a
 *   arithmetic    host SUSPEND fires this and leaves instance_id untouched,
 *                 because CLOCK_MONOTONIC does not advance across suspend. That
 *                 run is void and instance_id cannot see it.
 *
 * TRA-1204's acceptance originally said adopting instance_id let consumers drop
 * the arithmetic. That was wrong and was corrected on the ticket; this function
 * is where the correction lives. Deleting the arithmetic silently accepts a
 * suspended run.
 *
 * ⚠ systemd's `NRestarts` is deliberately NOT consulted. Measured on mssb
 * 2026-08-29, a manual `systemctl --user restart ble-bridge` changed
 * instance_id, reset uptime 2051s -> 24s, and left NRestarts at 0 — it counts
 * restarts made by the `Restart=` policy, not by an operator, which is the most
 * likely way a bridge is actually restarted mid-soak.
 */
export function restartVerdict({
  startId,
  nowId,
  uptimeStart,
  wallStart,
  uptimeNow,
  wallNow,
  toleranceSeconds,
}) {
  const by = [];
  const notes = [];

  if (startId && nowId && startId !== nowId) {
    by.push('instance_id');
    notes.push(`instance_id changed ${startId} -> ${nowId}`);
  } else if (startId && !nowId && typeof uptimeNow === 'number') {
    // Still answering, but no longer identifying itself: a different process, or
    // one downgraded under the run. Either way continuity is unprovable. The
    // reverse (an id APPEARING mid-run) is an upgrade and is not an id change.
    by.push('instance_id');
    notes.push(`the daemon stopped reporting instance_id (was ${startId})`);
  }

  if (hasRestarted({ uptimeStart, wallStart, uptimeNow, wallNow, toleranceSeconds })) {
    by.push('uptime');
    notes.push(
      `uptime ${uptimeStart.toFixed(3)}s -> ` +
        `${typeof uptimeNow === 'number' ? `${uptimeNow.toFixed(3)}s` : 'UNREACHABLE'}` +
        ` over ${(wallNow - wallStart).toFixed(3)}s of wall clock`
    );
  }

  return { restarted: by.length > 0, by, reason: notes.join('; ') };
}

/**
 * The session id the soak's own driver will connect under.
 *
 * DERIVED, not declared, and that is the point (TRA-1209). `--session` exists as
 * an override, but a flag alone would have been the same mistake in a new place:
 * a check whose subject is chosen by configuration the check does not read. The
 * operator would have to keep it in step with `BLE_SESSION_ID` by hand, and a
 * stale flag reads as contention exactly like the bug this fixes.
 *
 * So it is computed the way `tests/config/ble-bridge.config.ts` computes it —
 * including loading `.env.local`, because that is where `BLE_SESSION_ID` lives
 * when it is set at all and the config loads it the same way. The suite's value
 * would otherwise differ from ours for a reason invisible on both sides.
 *
 * This is a THIRD COPY of that derivation, which is the very shape that keeps
 * going wrong here. The others are `tests/config/ble-bridge.config.ts` and
 * `tests/config/vite-bridge.config.ts` — and the vite one is the load-bearing
 * one for e2e, because it is what gets injected into the browser and therefore
 * what the bridge reports back in `get_connection_state.session`.
 *
 * All three are asserted equal by
 * `tests/config/soak-watchdog-recognises-its-own-driver.test.ts`. Guarding this
 * against the integration config alone would have been the same
 * two-legs-scoped-differently defect the gate below is fixing.
 *
 * The copies exist because those modules are TypeScript and import the transport
 * for its UUID constants; this is a plain .mjs that must run under bare node
 * with no loader.
 *
 * `trakrf-platform-dev-` names this project. It replaced `trakrf-handheld-dev-`
 * on 2026-08-29 (TRA-1200) — a prefix inherited from the predecessor project
 * platform was built from, kept until then only because the value has to MATCH
 * across the places that derive or observe it rather than mean anything.
 *
 * Renaming it changes the identity the bridge reports in
 * `get_connection_state.session`, so it is a second variable moving. It was done
 * deliberately in a window with nothing measuring, rather than between two arms
 * of a comparison — the campaign's own rule, applied to the campaign's tooling.
 */
export function expectedSessionId() {
  dotenv.config({ path: '.env.local' });
  return process.env.BLE_SESSION_ID || `trakrf-platform-dev-${os.hostname()}`;
}

/**
 * Is anything OTHER THAN OUR OWN DRIVER holding or watching the command path?
 *
 * `observer_count > 0` is the hazard that appears in no process listing and no
 * log — most often a leftover mock-injected browser tab. On 2026-08-26
 * contention of exactly this kind invalidated two hardware runs inside ten
 * minutes, and neither was visible until the data came out wrong.
 *
 * ## Why the question has "other than our own" in it (TRA-1209)
 *
 * This used to ask only "is anything holding it", and aborted on the driver's
 * own rep 1. The documented usage takes `--driver-pid <pid>`, so the driver
 * necessarily starts first, and the gate necessarily races its first connect —
 * a couple of seconds on the vitest path, the browser launch on e2e. Arming
 * inside that window works, but it makes correctness depend on operator timing
 * in exactly the unattended overnight case where nobody is there to get it
 * right. The abort also misdirected, naming a leftover browser tab as the usual
 * cause and sending the operator after a tab that did not exist.
 *
 * The holder's identity was in the reply the whole time.
 *
 * ## The fail-open trap this avoids
 *
 * `state.session === ownSession` is TRUE when both are undefined — a reply that
 * omits `session`, checked by a watchdog given no expected session, would read a
 * held field as clear. That converts a fail-safe gate into a fail-open one,
 * which is worse than the bug being fixed. Hence the explicit non-empty-string
 * requirement on both sides.
 *
 * An observer is contention whoever holds the command path, so `observer_count`
 * must be 0 either way. A reply that does not carry both fields is not evidence
 * of a clear field.
 */
export function fieldIsClear(state, ownSession) {
  if (!state || typeof state !== 'object') return false;
  if (typeof state.held !== 'boolean') return false;
  if (typeof state.observer_count !== 'number') return false;
  if (state.observer_count !== 0) return false;
  if (state.held === false) return true;
  return heldByUs(state, ownSession);
}

/**
 * Did a mock-version mismatch appear during the run?
 *
 * Keyed on `status.mock_version_mismatches`, a monotonic counter, rather than on
 * `get_connection_state.mock_version_match`. The reason is poll timing: between
 * reps the connection state reads held:false with no client attached, and with
 * ~27s reps against a 300s poll most samples land in that gap. A point-in-time
 * field is unmissable only if you happen to sample mid-rep; a counter cannot be
 * missed. Same reasoning as the restart check above, where "is a daemon alive"
 * was replaced by "did THIS one restart".
 *
 * WHY IT EXISTS. TRA-1200's 150-rep arm ran browser mock 0.12.0 against bridge
 * 0.13.0 start to finish. The bridge noticed every single time and warned into
 * its journal; nothing here read it, so the run completed and was analysed
 * before anyone knew.
 *
 * ⚠ ABSENT IS NOT CLEAN, AND IS ALSO NOT AN ABORT.
 * ble-mcp-test publishes this field from 0.14.0 (TRA-1211). Against an older
 * bridge the honest answer is "cannot check", reported as such rather than
 * silently passing — but aborting on it would refuse to run against the bridge
 * that is actually deployed, which makes the instrument unusable. The
 * client-side detector in tests/config/resolve-mock-bundle.ts is the cover for
 * that window, and it observes a different thing (what was loaded from disk,
 * versus what arrived over the wire), so neither replaces the other.
 */
export function mockVersionBreach(baseline, now) {
  if (typeof baseline !== 'number' || typeof now !== 'number') {
    return {
      breached: false,
      reason:
        'bridge does not publish mock_version_mismatches — cannot check ' +
        '(needs ble-mcp-test >= 0.14.0, TRA-1211)',
    };
  }
  if (now > baseline) {
    return { breached: true, reason: `mock_version_mismatches rose ${baseline} -> ${now}` };
  }
  return { breached: false, reason: 'no mismatch observed' };
}

/**
 * Is the mock CURRENTLY connected a different version from the one this bridge
 * expects? A pre-flight snapshot, and the companion to `mockVersionBreach()`.
 *
 * The two observe different things and neither replaces the other: this reads
 * the version of the client attached right now, the counter catches a stale mock
 * that connected and disconnected between two polls.
 *
 * ⚠ `mock_version_match` is THREE-VALUED and collapsing it is the whole hazard:
 *
 *   false  asked, and the answer disagrees. The only case that aborts.
 *   null   nothing connected yet, or the bridge could not ask. "Cannot check" —
 *          and it is the NORMAL reading when the watchdog starts before the
 *          driver's first connect, so it must not abort. It must also never be
 *          read as agreement.
 *   true   asked and agreed.
 *
 * Costing the null case wrong in either direction breaks the instrument: abort
 * and no arm can ever start; treat as clean and the guard is decorative.
 */
export function mockVersionMismatchAtStart(state) {
  if (!state || state.mock_version_match !== false) {
    const known = state && state.mock_version_match === true;
    return {
      mismatched: false,
      reason: known
        ? `connected mock ${state.mock_version} matches`
        : 'no mock connected yet, or the bridge could not ask — cannot check',
    };
  }
  return {
    mismatched: true,
    reason:
      `connected mock is ${state.mock_version}, bridge expects ${state.mock_version_expected}`,
  };
}

/** Held, and held by the session our own driver connects under. Both sides must
 * be non-empty strings — see the fail-open note above. */
export function heldByUs(state, ownSession) {
  return (
    typeof ownSession === 'string' &&
    ownSession !== '' &&
    typeof state?.session === 'string' &&
    state.session === ownSession
  );
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
  --session <id>       the session id the driver connects under. Defaults to the
                       same value tests/config/ble-bridge.config.ts derives, so a
                       hold by our own rep 1 is not mistaken for contention.
                       Override only if the driver runs with a BLE_SESSION_ID this
                       process cannot see.

exit codes:
  0   run ended normally (driver gone)
  2   the bridge did not answer
  3   consecutive transport failures
  4   void capture — the canary read zero
  5   the field was not clear before the first rep
  6   mock_version_mismatches rose mid-run
  7   the connected mock is the wrong version at pre-flight
  64  bad usage

  See docs/runbooks/running-a-soak-arm.md for what each one means in practice.
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
    session: null,
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
      case '--session': opts.session = value; break;
      default: usage(`unrecognised argument: ${argv[i]}`);
    }
  }
  // Derived rather than required, so the ordinary invocation is unchanged and
  // still correct. See expectedSessionId().
  if (!opts.session) opts.session = expectedSessionId();
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
  if (!fieldIsClear(state, opts.session)) {
    say(`ABORT before start: the field is not clear — ${JSON.stringify(state)}`);
    // Name the actual subject. The old message asserted a leftover browser tab
    // unconditionally, which was wrong in the one case that fires most often and
    // sent the operator hunting for something that did not exist (TRA-1209).
    if (state.observer_count > 0 && heldByUs(state, opts.session)) {
      say(`Your own driver holds it (session ${opts.session}), but ${state.observer_count}`);
      say('observer(s) are attached. An observer is contention whoever holds the');
      say('command path — a leftover mock-injected browser tab is the usual cause');
      say('and appears in no process list.');
    } else if (state.held && typeof state.session === 'string') {
      say(`Held by session ${state.session}; this run expects ${opts.session}.`);
      say('If those should match, the driver is running with a BLE_SESSION_ID this');
      say('process cannot see — pass --session, or check .env.local. Otherwise');
      say('another client owns the command path.');
    } else {
      say('Something already holds or observes the command path. A leftover');
      say('mock-injected browser tab is the usual cause and appears in no process list.');
    }
    process.exit(5);
  }

  // ---- pre-flight: the browser mock must be the version we think it is ----
  //
  // ⚠ THIS COST 45 MINUTES AND A FALSE ARM START on 2026-08-31, and every fact
  // needed to catch it was already on screen.
  //
  // `frontend/package.json` said `^0.16.0`, the bump was merged, `main` was
  // current — and `node_modules` in the checkout the driver ran from still held
  // 0.15.0, because `pnpm install` had only ever run inside a worktree that was
  // then deleted. The arm's first rep connected with the old mock. Seven and a
  // half hours would have measured a configuration that does not exist.
  //
  // `mock_version_match` is THREE-VALUED and the distinction is the whole
  // point:
  //
  //   false  the bridge asked and the answer disagrees. A definite fault, and
  //          the only case that aborts here.
  //   null   nothing is connected yet, or the bridge could not ask. "Cannot
  //          check" — recorded, never treated as agreement. This is the NORMAL
  //          reading when the watchdog starts before the driver's first
  //          connect, so aborting on it would make the watchdog unusable.
  //   true   checked and agreed.
  //
  // The counter in `status.mock_version_mismatches` is the other half and is
  // handled at each poll by mockVersionBreach(); it catches a stale mock that
  // connects and disconnects between samples, which this snapshot cannot see.
  const mockAtStart = mockVersionMismatchAtStart(state);
  if (mockAtStart.mismatched) {
    say(`ABORT before start: ${mockAtStart.reason}. The arm would measure the wrong`);
    say('mock and every rep would be uninformative.');
    say('');
    say('Almost always a stale install in the checkout the driver runs from —');
    say('a dependency bump merged, but `pnpm install` never re-ran there:');
    say('');
    say('  pnpm install                     # from the repo root you are running in');
    say('  node -p "require(\'./frontend/node_modules/ble-mcp-test/package.json\').version"');
    say('');
    say('A guard proves the artifact in the checkout you run it from — green in a');
    say('worktree says nothing about the tree launching this arm.');
    process.exit(7);
  }

  const status = await callControl('status');
  if (!status || typeof status.uptime_seconds !== 'number') {
    say('ABORT before start: the bridge did not answer status.');
    process.exit(2);
  }

  const startIdentity = readIdentity(status);
  const uptimeStart = status.uptime_seconds;
  const wallStart = Date.now() / 1000;
  // null means the bridge predates TRA-1211's field. Recorded as such rather
  // than defaulted to 0, so "cannot check" never reads as "checked and clean".
  const mockMismatchBaseline =
    typeof status.mock_version_mismatches === 'number' ? status.mock_version_mismatches : null;

  // The run-identity record. "The field was clear at start" is evidence only if
  // it was written down at the time; reconstructed afterwards it is an
  // assumption wearing evidence's clothes.
  const identity = [
    `watchdog started      ${new Date().toISOString()}`,
    `driver pid            ${opts.driverPid}`,
    `bridge uptime start   ${uptimeStart.toFixed(3)}s`,
    `bridge instance       ${startIdentity.instanceId ?? 'NOT REPORTED (daemon predates 0.13.0)'}`,
    `bridge code           ${startIdentity.codeFingerprint ?? 'not reported'} @ ${
      startIdentity.codeSourceRoot ?? 'root not reported'
    }`,
    `bridge transport      ${status.esphome_configured ? status.esphome_proxy : 'NOT CONFIGURED'}`,
    `bridge device         ${status.device_mac ?? 'none'}`,
    `driver session        ${opts.session}`,
    `field at start        held=${state.held} observer_count=${state.observer_count}`,
    `mock version          ${
      mockMismatchBaseline === null
        ? 'NOT PUBLISHED by this bridge — client-side check only (TRA-1211)'
        : `mismatches at start ${mockMismatchBaseline}`
    }`,
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

    const nowIdentity = readIdentity(now);
    const verdict = restartVerdict({
      startId: startIdentity.instanceId,
      nowId: nowIdentity.instanceId,
      uptimeStart,
      wallStart,
      uptimeNow,
      wallNow,
      toleranceSeconds: opts.toleranceSeconds,
    });

    if (verdict.restarted) {
      say('ABORT: the bridge is not the process this run started with.');
      say(`  detected by     ${verdict.by.join(' + ')}`);
      say(`  ${verdict.reason}`);
      say('Rows from here on would span two daemons with no marker in the data.');
      say('Forensics: journalctl --user -u ble-bridge --since <run start>. The');
      say("daemon's own log ring is per-process and cannot explain a restart it did not survive.");
      say('Do NOT reach for systemd NRestarts to corroborate this: it counts only');
      say('restarts made by the Restart= policy, so a manual `systemctl restart`');
      say('leaves it at zero (measured 2026-08-29, TRA-1205).');
      process.exit(2);
    }

    if (!portIsServed(opts.port)) {
      say(`ABORT: nothing is serving the bridge port ${opts.port}.`);
      process.exit(2);
    }

    // ---- is the browser running the mock we think it is? -----------------
    const mockBreach = mockVersionBreach(
      mockMismatchBaseline,
      typeof now?.mock_version_mismatches === 'number' ? now.mock_version_mismatches : null
    );
    if (mockBreach.breached) {
      // The versions live on get_connection_state, NOT on status — status
      // carries only the counter. Reading them off `now` printed
      // "expected unknown, got unknown" on the very abort that needed them,
      // which the unit tests could not see and the hardware break test did.
      //
      // Even from the right surface they can be null: the field is scoped to
      // the current command-path holder, and the offending client may already
      // have disconnected by the time this poll fires. `mock_version_expected`
      // is the bridge's own package version and is answerable regardless, so a
      // null `got` still narrows it. The journal has the per-connection record
      // either way, which is why it is named here rather than implied.
      const versions = await callControl('get_connection_state');
      say('ABORT: a client connected with a mock version the bridge did not expect.');
      say(`  ${mockBreach.reason}`);
      say(
        `  bridge expects ${versions?.mock_version_expected ?? 'unknown'}, ` +
          `holder now reports ${versions?.mock_version ?? 'nothing attached'}`
      );
      say('Reps from here on measured a different mock than the run started with.');
      say('A clean tree and a correct lockfile do not rule this out — see TRA-1200.');
      say('Forensics: journalctl --user -u ble-bridge --since <run start> | grep "mock version"');
      process.exit(6);
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
