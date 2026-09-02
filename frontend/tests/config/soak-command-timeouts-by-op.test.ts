import { describe, it, expect } from 'vitest';
import { writeFileSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import path from 'path';
// @ts-expect-error — .mjs instrument module, no types by design
import {
  readSignals,
  readCommandTimeouts,
  resolveSignals,
  SIGNALS,
} from '../../scripts/suite-run-signals.mjs';

/**
 * TRA-1226. The needle table counted ONE command timeout — `RFID_POWER_OFF` —
 * and so a third of the device's non-answers reached no summary at all.
 *
 * Measured on the 2026-08-31 after-arm, first 12 reps: 203 command timeouts, of
 * which 138 were counted and 65 were invisible. The invisible ones were
 * `GET_TRIGGER_STATE` (0xA001, 63 occurrences) and `RFID_FIRMWARE_COMMAND`
 * (0x8002, 2 occurrences) — and the 0x8002 one is what failed rep 1, by putting
 * the reader into Error so a deferred `targetEPC` push was abandoned and Locate
 * ran against the previously applied mask.
 *
 * ⚠ THE POINT IS NOT "ADD TWO MORE NEEDLES." A fixed list can only count what
 * someone thought to enumerate, and reads a confident 0 for everything else —
 * which is the same defect one level up, and precisely how 0xA001 stayed
 * invisible while TRA-1223's narrative said the device ignored "exactly one op
 * code". Its own first-occurrence table had 0xA001 at 76 TX / 14 RX the whole
 * time. So this parses the op name out of the line instead: a newly-silent op
 * code appears without anyone having predicted it.
 */
const withLog = (text: string) => {
  const dir = mkdtempSync(path.join(tmpdir(), 'cmd-timeouts-'));
  const file = path.join(dir, 'out.log');
  writeFileSync(file, text);
  return file;
};

const timeoutLine = (op: string) => `[Worker] WARN: [CommandManager] Command timeout: ${op}\n`;

describe('readCommandTimeouts', () => {
  it('counts an op code nobody enumerated', () => {
    // The whole reason this is a parser and not a needle list. NOBODY_PREDICTED
    // appears in no table, in no source file, and in no ticket — and it must
    // still be counted, because the next silent op code will be exactly that.
    const log = withLog(timeoutLine('NOBODY_PREDICTED_THIS_ONE'));

    expect(readCommandTimeouts(log)).toEqual({ NOBODY_PREDICTED_THIS_ONE: 1 });
  });

  it('counts each op code separately, at the observed per-rep proportions', () => {
    // Shaped like a real wedged rep: 0x8001 and 0xA001 both silent, 0x8002 rare.
    const log = withLog(
      timeoutLine('RFID_POWER_OFF').repeat(26) +
        timeoutLine('GET_TRIGGER_STATE').repeat(12) +
        timeoutLine('RFID_FIRMWARE_COMMAND')
    );

    expect(readCommandTimeouts(log)).toEqual({
      RFID_POWER_OFF: 26,
      GET_TRIGGER_STATE: 12,
      RFID_FIRMWARE_COMMAND: 1,
    });
  });

  it('returns an empty map for a clean log — measured, and nothing happened', () => {
    // `{}` is a real reading: the capture existed and carried no timeouts. It is
    // NOT the same claim as `null` below, and the two must never collapse.
    expect(readCommandTimeouts(withLog('[Harness] connected\n'))).toEqual({});
  });

  it('returns null when the log is gone, never an empty map', () => {
    // The null-vs-zero rule this module turns on. A missing log measured
    // NOTHING; an empty map would read as "the device answered everything".
    expect(readCommandTimeouts('/no/such/path.log')).toBeNull();
  });

  it('ignores a timeout line for a different logger', () => {
    // The needle is the CommandManager's. Another component's timeout is not a
    // command going unanswered, and counting it would inflate device silence.
    const log = withLog('[Worker] WARN: [SomethingElse] Command timeout: RFID_POWER_OFF\n');

    expect(readCommandTimeouts(log)).toEqual({});
  });
});

describe('readSignals carries the per-op breakdown', () => {
  it('reports commandTimeouts alongside the legacy powerOffTimeouts count', () => {
    const log = withLog(timeoutLine('RFID_POWER_OFF').repeat(3) + timeoutLine('GET_TRIGGER_STATE'));
    const signals = readSignals(log);

    expect(signals.commandTimeouts).toEqual({ RFID_POWER_OFF: 3, GET_TRIGGER_STATE: 1 });
  });

  it('keeps powerOffTimeouts in agreement with its own entry in the map', () => {
    // Two counts of the same thing is how a number silently means two things.
    // `powerOffTimeouts` is retained because every archived record carries it
    // and cross-arm comparisons depend on it — but it must never disagree.
    const log = withLog(timeoutLine('RFID_POWER_OFF').repeat(7) + timeoutLine('GET_TRIGGER_STATE'));
    const signals = readSignals(log);

    expect(signals.commandTimeouts.RFID_POWER_OFF).toBe(signals.powerOffTimeouts);
  });

  it('reports commandTimeouts as null on an e2e rep, not as an empty map', () => {
    // `[CommandManager]` lines are logger.warn and the e2e console forwarder
    // drops them, so a browser rep cannot observe this however loud the device
    // is. An empty map there would be an absence dressed as data.
    const log = withLog('[Connection] opened\n');

    expect(readSignals(log, 'e2e').commandTimeouts).toBeNull();
  });

  it('reports no commandTimeouts key at all when the log is missing', () => {
    // A void capture already says everything with logMissing; adding a map to it
    // would invite a caller to read the map instead of the flag.
    expect(readSignals('/no/such/path.log')).toEqual({ logMissing: true });
  });
});

describe('resolveSignals treats a record without commandTimeouts as stale', () => {
  /**
   * ⚠ `resolveSignals`'s own docstring predicted this failure before it existed:
   *
   *   "The gate used to name `harnessLines` alone, which meant the next needle
   *    added would find it satisfied and return the stored snapshot — a record
   *    silently missing the very signal the new needle was added to detect,
   *    reported as a zero."
   *
   * `commandTimeouts` is not a member of `SIGNALS`, so a record written before
   * TRA-1226 satisfies the every-needle-present gate and is returned verbatim —
   * carrying no per-op breakdown at all, on a run whose log is sitting right
   * there and could be re-read. The warning was left in the file by whoever last
   * got caught by it; this is the test that keeps it honest.
   */
  it('recomputes a pre-TRA-1226 record whose log still exists', () => {
    const log = withLog(timeoutLine('RFID_POWER_OFF').repeat(2) + timeoutLine('GET_TRIGGER_STATE'));

    // Exactly what an archived record looks like: every needle of its day
    // present and correct, and no commandTimeouts because the field did not
    // exist yet.
    const legacySignals: Record<string, unknown> = { logMissing: false };
    for (const name of Object.keys(SIGNALS)) legacySignals[name] = 0;

    const out = resolveSignals({ outputLog: log, signals: legacySignals });

    expect(out.source).toBe('recomputed');
    expect(out.signals.commandTimeouts).toEqual({ RFID_POWER_OFF: 2, GET_TRIGGER_STATE: 1 });
  });

  it('still trusts a record that already carries commandTimeouts', () => {
    // The other half: having added a staleness trigger, it must not fire on
    // every record forever and re-read 200 logs on each summary.
    const legacySignals: Record<string, unknown> = { logMissing: false, commandTimeouts: {} };
    for (const name of Object.keys(SIGNALS)) legacySignals[name] = 0;

    const out = resolveSignals({ outputLog: '/no/such/path.log', signals: legacySignals });

    expect(out.source).toBe('record');
  });
});
