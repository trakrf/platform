/**
 * Log signatures shared by the run-shape driver and its summariser.
 *
 * Lives in its own module so the two scripts cannot drift apart: a needle that
 * exists in only one of them produces a detector that reads 0 in the summary
 * while the driver saw occurrences, or vice versa.
 */

import { readFileSync, existsSync } from 'node:fs';

export const SIGNALS = {
  triggerTimeout: 'Timeout waiting for event: TRIGGER_STATE_CHANGED',
  // BOTH limbs, and the reason is a near-miss worth keeping. The trigger case
  // awaits startScanning() on press and stopScanning() on release, and either
  // rethrowing skips the postWorkerEvent() below it. The first version of this
  // detector encoded only the press limb, so it reported 0 across four genuine
  // failures — which reads as "hypothesis refuted" rather than "detector looked
  // in one of the two places". The real line every time was the RELEASE limb.
  //
  // A detector that encodes half a hypothesis returns a confident zero.
  startScanFailed: '[Reader] Failed to start scanning:',
  stopScanFailed: '[Reader] Failed to stop scanning:',
  // CANARY, not a finding. 0 here means the capture was void and the other
  // counts are uninformative rather than zero. Without it, a broken capture and
  // a clean run are the same record: all-zero counts with logMissing:false.
  // That already happened — `--reporter=json` alone intercepts console output,
  // and the detector read 0 timeouts on repetitions that had just timed out.
  //
  // The margin here narrowed on 2026-08-27 and the canary is now load-bearing
  // rather than incidental. It used to count hundreds of lines per repetition,
  // because the old harness logged every packet in both directions. TRA-1187
  // replaced that harness with one that drives the production transport and
  // logs once per connect, on purpose — so this needle now matches roughly one
  // line per spec file rather than hundreds. Still non-zero for any captured
  // run, which is all the canary asserts, but do not read a low count as a
  // problem, and do not delete that log line thinking it is noise.
  harnessLines: '[Harness]',
  // ENVIRONMENTAL, not a defect. The bridge holds the BLE link for its process
  // lifetime, so if it dies mid-soak every subsequent repetition fails — a
  // valid-looking JSON report saying the suite failed, with no clue in it that
  // there was no transport at all. Those repetitions measured a missing bridge,
  // not the subsystem under test, and pooling them with real failures shows up
  // as failures that lack the signature — i.e. as counter-evidence against
  // whatever hypothesis is being tested. That happened here: a dead bridge
  // briefly read as 6 failures disproving a mechanism they never exercised.
  //
  // TWO needles, because the shape changed with the path. The old integration
  // route used ble-mcp-test's Node client, whose socket surfaced a Node error:
  // `connect ECONNREFUSED 127.0.0.1:8080`. Since TRA-1187 the route is
  // `CS108BLETransport -> navigator.bluetooth -> mock -> ws-transport`, and the
  // mock's transport uses whatever global WebSocket the runtime provides — under
  // vitest that is jsdom's, which reports a bare `WebSocket error` and no errno
  // at all. Verified 2026-08-27 by running connection.spec.ts with no bridge
  // listening: the failure was `Error: WebSocket error` with zero ECONNREFUSED
  // anywhere in the output.
  //
  // Keeping only the old needle would have made every dead-bridge repetition of
  // an overnight soak look like a genuine suite failure. Keep both: e2e and any
  // Node-side caller can still produce the errno form.
  transportRefused: 'ECONNREFUSED',
  transportUnreachable: 'WebSocket error',
  // CANARY for the ack-latency instrument, and it is a canary in the same sense
  // as `harnessLines`: a captured run with zero of these did not measure a clean
  // link, it measured nothing. Every write attempt emits one, so 0 across a
  // repetition that ran any command means the transport lines are not reaching
  // the captured log — a detector that cannot see what it measures reads as an
  // empty distribution, which is indistinguishable from a healthy one.
  ackSamples: '[ble-timing] write-ack',
  // A link close with a write outstanding is the signature the soak watches for.
  // Counted here so the driver's own record shows it; the JOIN that decides
  // whether it landed inside a write window needs the timestamps and lives in
  // scripts/ack-latency-report.mjs.
  linkCloses: '[ble-timing] link-close',
  connectSamples: '[ble-timing] connect',
};

/**
 * The needles a PLAYWRIGHT repetition can actually produce (TRA-1206).
 *
 * The soak driver gained an e2e backend so it could run the suite TRA-1200
 * measures. The runner is the easy half; this table is the hard half, because
 * most of `SIGNALS` above is vitest-shaped and a needle that cannot fire on a
 * path must not be counted as zero on it.
 *
 * WHAT IS MISSING FROM HERE, AND WHY — each one is a needle whose emitter cannot
 * reach a Playwright rep's captured log, not a needle nobody cared about:
 *
 *   harnessLines     `[Harness]` is written by
 *                    tests/integration/cs108/CS108WorkerTestHarness.ts and by
 *                    nothing else. That file is integration-only; no browser
 *                    ever loads it.
 *   triggerTimeout   Same file — it is CS108WorkerTestHarness that rejects with
 *                    `Timeout waiting for event: ...`.
 *
 * Both are STRUCTURAL: the emitter is a file no browser loads, so no change to
 * the e2e path can make them fire. Do not "fix" them.
 *
 * The three `[ble-timing]` needles used to be listed here too, and they were the
 * other kind — absent for an INCIDENTAL reason. They are `console.info` from
 * src/lib/device/transport/cs108-ble-transport.ts, which under e2e runs inside
 * the browser, and the console forwarder's filter matched `BLE`/`Connect`
 * case-sensitively against a lowercase `[ble-timing] connect`. TRA-1209 fixed
 * the forwarder, so they are counted again. The distinction is the reason the
 * original null was recorded rather than a zero: a zero would have been read as
 * "the transport did nothing" and the question would never have been asked.
 */
export const E2E_SIGNALS = {
  // CANARY, and the e2e counterpart to `harnessLines` — see `CAPTURE_CANARY`.
  // `[Connection]` is logged by tests/e2e/helpers/connection.ts on the Node
  // side, so it reaches the captured log directly rather than through the
  // browser console. Every hardware e2e spec connects through that helper.
  //
  // A rep with zero of these never reached the connect helper at all: the dev
  // server was down, the browser failed to launch, the file failed to load.
  // That is a void capture in the same sense `[Harness]` means it — nothing was
  // observed, so every other count in the row is uninformative rather than low.
  e2eConnectLines: '[Connection]',
  // Logged by src/worker/cs108/reader.ts, which runs in the browser under e2e.
  // These DO survive the forwarder: its first limb passes any text containing
  // `Failed`, and both needles do. Reliable for the shared page every hardware
  // spec connects through, because the listener is registered inside
  // connectToDevice() and lives as long as the page.
  startScanFailed: SIGNALS.startScanFailed,
  stopScanFailed: SIGNALS.stopScanFailed,
  // Already documented above as e2e-capable: "Keep both: e2e and any Node-side
  // caller can still produce the errno form." The browser reports the bare
  // `WebSocket error` shape and the forwarder passes it (`WebSocket` is in its
  // allowlist, and it is capitalised the same way there).
  transportRefused: SIGNALS.transportRefused,
  transportUnreachable: SIGNALS.transportUnreachable,
  // Restored by TRA-1209. `console.info` from cs108-ble-transport.ts, which runs
  // in the browser under e2e; they reach the captured log only because
  // `shouldForwardConsoleLine` now matches the `[ble-timing]` prefix explicitly.
  // Without that limb these count 0 on every rep however healthy the link is —
  // which is why they are named in E2E_BROWSER_NEEDLES below.
  //
  // Measured on hardware after the fix: connect 1, write-ack 28, link-close 0.
  //
  // THAT ZERO IS GENUINE, and worth writing down because it looks like the bug
  // that was just fixed. `link-close` is emitted only by `handleDisconnect()`,
  // which is the `gattserverdisconnected` listener — and
  // `CS108BLETransport.disconnect()` REMOVES that listener before tearing the
  // link down. So a deliberate disconnect never emits one. The needle fires on
  // an UNEXPECTED drop, which is the condition it exists to catch ("a link close
  // while a write is outstanding").
  //
  // This is not our local quirk — it is the peer's published contract, and
  // citing it beats re-deriving it, so a future change is found where the rule
  // lives rather than inferred from a zero somebody distrusts:
  //
  //   ble-mcp-test docs/design/2026-08-27-client-contract.md:204
  //     `gattserverdisconnected` fires on a TRANSPORT-LEVEL DROP only, never on
  //     an explicit gatt.disconnect().
  //
  // ⚠ That contract line is an assertion about what real Chrome does, and it is
  // not flagged as a deliberate divergence. So a clean rep scoring 0 confirms
  // THE MOCK MATCHES ITS CONTRACT; it does not confirm the contract matches
  // Chrome. Nothing either repo runs tests the second claim — checking live
  // `navigator.bluetooth` needs the gesture-bound interactive arm TRA-1187
  // deferred to a human at a keyboard. A green e2e run is never evidence of
  // mock fidelity.
  ackSamples: SIGNALS.ackSamples,
  linkCloses: SIGNALS.linkCloses,
  connectSamples: SIGNALS.connectSamples,
};

/**
 * Which e2e needles have a producer that runs IN THE BROWSER.
 *
 * Those lines reach a Playwright rep's captured log only by way of the console
 * forwarder in `tests/e2e/helpers/console-forwarding.ts`. A needle here that the
 * forwarder drops does not read as "dropped" — it reads as a confident `0`,
 * indistinguishable from "the reader never did that". That is exactly how the
 * three `[ble-timing]` needles were lost (TRA-1209).
 *
 * Declaring the coupling is what makes it checkable:
 * `tests/config/e2e-console-forwarding.test.ts` asserts the forwarder passes
 * every needle named here. Adding a browser-emitted needle without teaching the
 * forwarder is now a failing test rather than a silent zero.
 *
 * Two deliberate omissions, so neither reads as an oversight:
 *   e2eConnectLines   `[Connection]` is logged by the Playwright process itself,
 *                     not by the page, so it never touches the forwarder.
 *   transportRefused  `ECONNREFUSED` is a Node errno. A browser has no errno to
 *                     report — it produces the bare `WebSocket error` shape,
 *                     which is what `transportUnreachable` covers. Any
 *                     ECONNREFUSED in an e2e log came from a Node-side caller.
 */
export const E2E_BROWSER_NEEDLES = [
  'startScanFailed',
  'stopScanFailed',
  'transportUnreachable',
  'ackSamples',
  'linkCloses',
  'connectSamples',
];

/**
 * The needle that answers "did this rep produce ANY observable output", per runner.
 *
 * `harnessLines` is named for the STRING. Its two consumers — the watchdog's
 * void-capture abort and the summariser's usable-record filter — want the ROLE,
 * and the string filling that role differs per runner. Reading the vitest needle
 * on an e2e record is how a working check becomes a silent no-op: `harnessLines`
 * is null there, `null ?? 1` is 1, and the abort never fires again.
 */
const CAPTURE_CANARY = {
  vitest: 'harnessLines',
  e2e: 'e2eConnectLines',
};

const SIGNAL_TABLES = { vitest: SIGNALS, e2e: E2E_SIGNALS };

/** The needle table for a runner. Throws rather than defaulting, because a
 * typo'd runner silently measured against the wrong table is the whole failure
 * class this module exists inside. */
export function signalsFor(runner) {
  const table = SIGNAL_TABLES[runner];
  if (!table) {
    throw new Error(
      `Unknown runner: ${runner}. Expected one of ${Object.keys(SIGNAL_TABLES).join('|')}.`
    );
  }
  return table;
}

/**
 * Which runner produced a record.
 *
 * Absent means vitest, and that is a fact about the archive rather than a
 * convenience: every record written before TRA-1206 was a vitest run, so there
 * is no historical row the default can be wrong about. Reading an old record as
 * anything else would recompute its signals against a table it was never
 * measured under.
 */
export function runnerOf(record) {
  return record?.runner ?? 'vitest';
}

/**
 * Count each signature in a captured run log.
 *
 * Returns `{ logMissing: true }` when the log is gone — distinct from a log
 * that exists and contains nothing, which is what the canary catches.
 *
 * `runner` defaults to vitest so every pre-TRA-1206 call site is unchanged, and
 * the vitest result is byte-identical to what it always was — no e2e key is
 * added to it. The asymmetry is deliberate: a vitest record must stay comparable
 * against TRA-1189's 528 reps and TRA-1193's 200, and an e2e-only field on it
 * would be an absence dressed as data in the other direction.
 *
 * An e2e result DOES carry the vitest-only needles, explicitly `null`. A
 * consumer reading `signals.harnessLines` on an e2e record has to deal with
 * "unavailable"; it must never be handed a `0` that reads as "measured, and it
 * never happened".
 */
export function readSignals(logPath, runner = 'vitest') {
  const table = signalsFor(runner);
  if (!logPath || !existsSync(logPath)) return { logMissing: true };
  let text;
  try {
    text = readFileSync(logPath, 'utf8');
  } catch {
    return { logMissing: true };
  }
  const counts = { logMissing: false };
  for (const [name, needle] of Object.entries(table)) {
    counts[name] = text.split(needle).length - 1;
  }
  // Structurally absent, stated. Only ever widens a NON-vitest runner's record.
  for (const name of Object.keys(SIGNALS)) {
    if (!(name in counts)) counts[name] = null;
  }
  return counts;
}

/**
 * How many capture-canary lines a record saw — or null when that is unknowable.
 *
 * null means "no answer", and every caller must treat it as such rather than as
 * a zero or as health. A record whose log went missing did not measure a clean
 * capture; it measured nothing, and the honest reading of nothing is null.
 */
export function captureCanaryCount(record, signals = record?.signals) {
  if (!signals || signals.logMissing) return null;
  const value = signals[CAPTURE_CANARY[runnerOf(record)]];
  return typeof value === 'number' ? value : null;
}

/**
 * Signals for a record, recomputed from its retained log when the record itself
 * predates a signature.
 *
 * The log is the evidence; the record's `signals` are a snapshot taken at write
 * time. When a new needle is added later, every existing record is missing it —
 * and treating "field absent" as "count zero" would silently answer a question
 * the run never asked. Recomputing keeps old runs analysable without rewriting
 * the record, which stays append-only.
 */
export function resolveSignals(record) {
  const stored = record.signals;
  // Every CURRENT needle must be present, not one nominated field. The gate used
  // to name `harnessLines` alone, which meant the next needle added would find
  // it satisfied and return the stored snapshot — a record silently missing the
  // very signal the new needle was added to detect, reported as a zero. That is
  // the failure this function's docstring exists to prevent, so it cannot be
  // keyed on a single field that happens to be current today.
  const isCurrent =
    stored && !stored.logMissing && Object.keys(SIGNALS).every((name) => stored[name] !== undefined);
  if (isCurrent) {
    return { signals: stored, source: 'record' };
  }
  const log = record.outputLog ?? record.stdoutLog;
  const fresh = readSignals(log);
  if (fresh.logMissing) {
    return { signals: stored ?? fresh, source: 'unverifiable' };
  }
  return { signals: fresh, source: 'recomputed' };
}
