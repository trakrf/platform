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
 * Count each signature in a captured run log.
 *
 * Returns `{ logMissing: true }` when the log is gone — distinct from a log
 * that exists and contains nothing, which is what the canary catches.
 */
export function readSignals(logPath) {
  if (!logPath || !existsSync(logPath)) return { logMissing: true };
  let text;
  try {
    text = readFileSync(logPath, 'utf8');
  } catch {
    return { logMissing: true };
  }
  const counts = { logMissing: false };
  for (const [name, needle] of Object.entries(SIGNALS)) {
    counts[name] = text.split(needle).length - 1;
  }
  return counts;
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
