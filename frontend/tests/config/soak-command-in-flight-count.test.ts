import { describe, it, expect } from 'vitest';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
// @ts-expect-error — .mjs instrument module, no types by design
import { readSignals, SIGNALS } from '../../scripts/suite-run-signals.mjs';

/**
 * The needle that makes TRA-1239 measurable by an arm rather than by hand.
 *
 * `CommandInFlightError` is the one signal the TRA-1239 fix actually moves, and
 * until now nothing counted it. That is not a neutral omission: the ticket's own
 * pre-registered baseline — "6 per 200, in reps 5/6/39, where the device was
 * refusing" — was hand-counted from per-rep logs and is wrong in both halves.
 * The archive says **13 occurrences, all in reps 137-143**, the teardown window.
 * A number nobody can regenerate is a number that drifts, and this one did.
 *
 * ⚠ It is deliberately NOT parsed per op, which breaks from `commandTimeouts`
 * and `commandRejections`. Those parse because a fixed list can only count what
 * somebody enumerated (TRA-1226). The argument does not carry here: this line
 * comes from ONE constructor with ONE message, the ERROR half of each
 * occurrence carries no op name at all, and the question being asked is
 * "did the wire get leaked, yes or no" rather than "by which command". A per-op
 * map would report `{}` for the ERROR lines and read as coverage it does not
 * have.
 *
 * The target is ZERO — including inside a wedge window, which is the only place
 * it has ever been observed. A non-zero count after TRA-1239 means something
 * else claims the in-flight slot without giving it back, which is what the
 * error now says it means.
 *
 * Refs: TRA-1239, TRA-1143.
 */

/** Write `text` to a scratch log and read the instrument's signals off it. */
function signalsFor(text: string): Record<string, number | null | boolean> {
  const dir = mkdtempSync(path.join(tmpdir(), 'soak-in-flight-'));
  const logPath = path.join(dir, 'rep.log');
  writeFileSync(logPath, text, 'utf8');
  return readSignals(logPath) as Record<string, number | null | boolean>;
}

describe('commandInFlight', () => {
  it('is a needle the instrument actually carries', () => {
    // A signal that exists only in a summary is a signal no arm records.
    expect(SIGNALS).toHaveProperty('commandInFlight');
  });

  it('counts the tolerated-step WARN, verbatim from the 2026-09-01 arm', () => {
    // rep 137, line 478. The op is named here, but the sequence carried on —
    // so this line alone never failed anything and would go unnoticed.
    const line =
      '[Worker] WARN: [CommandManager] RFID_POWER_OFF (0x8001) went unanswered ' +
      'after 2 attempt(s): Command already active - executeCommand called ' +
      'concurrently — tolerated, continuing the sequence';

    expect(signalsFor(line).commandInFlight).toBe(1);
  });

  it('counts the thrown CommandInFlightError, verbatim from the same rep', () => {
    // rep 137, line 481 — the half that actually killed the mode change. It
    // carries no op name, which is why this signal is a count and not a table.
    const line =
      '[Worker] ERROR: [setMode] Failed to set Idle mode: CommandInFlightError: ' +
      'Command already active - executeCommand called concurrently';

    expect(signalsFor(line).commandInFlight).toBe(1);
  });

  it('counts both halves of one occurrence as two lines', () => {
    // Stated rather than left for a reader to infer at 3am: an occurrence emits
    // a WARN and an ERROR, so the arm's line count is TWICE the event count.
    // The 2026-09-01 arm's 26 lines are 13 events. Zero is zero either way,
    // which is what makes the target unambiguous even though the scale is not.
    const rep =
      '[Worker] WARN: [CommandManager] RFID_POWER_OFF (0x8001) went unanswered ' +
      'after 2 attempt(s): Command already active - executeCommand called ' +
      'concurrently — tolerated, continuing the sequence\n' +
      '[Worker] ERROR: [setMode] Failed to set Idle mode: CommandInFlightError: ' +
      'Command already active - executeCommand called concurrently\n';

    expect(signalsFor(rep).commandInFlight).toBe(2);
  });

  it('reads zero on a healthy rep rather than null', () => {
    // 0 is a measurement — this runner could have seen the line and did not.
    // It has to stay distinct from the null a blind runner gets.
    const clean =
      '[Harness] connected\n' +
      '[ble-timing] write-ack t=1788263701236 ms=38 attempt=1/3 outcome=ok\n' +
      '[Worker] WARN: [CommandManager] Command timeout: RFID_FIRMWARE_COMMAND\n';

    expect(signalsFor(clean).commandInFlight).toBe(0);
  });

  it('does not fire on the timeout line it sits next to', () => {
    // These are different phenomena and were conflated once already: a timeout
    // is the device not answering, this is the host never asking. Counting them
    // together would have hidden TRA-1239 inside `commandTimeouts`, where 45 of
    // 47 were ABORT retries working exactly as designed.
    const timeoutOnly =
      '[Worker] WARN: [CommandManager] Command timeout: RFID_POWER_OFF\n' +
      '[Worker] WARN: [CommandManager] Command rejected: GET_TRIGGER_STATE — ' +
      'Wrong header prefix (0x0000)\n';

    const signals = signalsFor(timeoutOnly);
    expect(signals.commandInFlight).toBe(0);
    // The neighbours still count, so this is a discrimination test rather than
    // a log that simply says nothing.
    expect(signals.powerOffTimeouts).toBe(1);
  });
});
