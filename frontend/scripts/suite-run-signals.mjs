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
  // CANARY, not a finding. Every repetition that captured anything at all emits
  // hundreds of `[Harness]` lines, so 0 here means the capture was void and the
  // other counts are uninformative rather than zero. Without it, a broken
  // capture and a clean run are the same record: all-zero counts with
  // logMissing:false. That already happened — `--reporter=json` alone
  // intercepts console output, and the detector read 0 timeouts on repetitions
  // that had just timed out.
  harnessLines: '[Harness]',
  // ENVIRONMENTAL, not a defect. The bridge holds the BLE link for its process
  // lifetime, so if it dies mid-soak every subsequent repetition fails with
  // `connect ECONNREFUSED 127.0.0.1:8080` — a valid-looking JSON report saying
  // the suite failed, with no clue in it that there was no transport at all.
  // Those repetitions measured a missing bridge, not the subsystem under test,
  // and pooling them with real failures shows up as failures that lack the
  // signature — i.e. as counter-evidence against whatever hypothesis is being
  // tested. That happened here: a dead bridge briefly read as 6 failures
  // disproving a mechanism they never exercised.
  transportRefused: 'ECONNREFUSED',
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
  if (stored && !stored.logMissing && stored.harnessLines !== undefined) {
    return { signals: stored, source: 'record' };
  }
  const log = record.outputLog ?? record.stdoutLog;
  const fresh = readSignals(log);
  if (fresh.logMissing) {
    return { signals: stored ?? fresh, source: 'unverifiable' };
  }
  return { signals: fresh, source: 'recomputed' };
}
