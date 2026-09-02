import { describe, it, expect } from 'vitest';
import { writeFileSync, mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import path from 'path';
// @ts-expect-error — .mjs instrument module, no types by design
import { commandTimeoutsByOpTable } from '../../scripts/summarise-suite-runs.mjs';

/**
 * The section that makes an UNPREDICTED silent op code visible (TRA-1226).
 *
 * `powerOffWindowTable` reports one op code, because for a long time that was
 * the only one anybody knew about. On the 2026-08-31 arm that framing cost us:
 * `GET_TRIGGER_STATE` (0xA001) went silent 63 times alongside the power-offs and
 * appeared in no summary, while TRA-1223's description asserted the device
 * ignored "exactly one op code" — its own packet table having carried 0xA001 at
 * 76 TX / 14 RX from the day it was written.
 *
 * ⚠ So this table's job is NOT to add two more rows. It is to print whatever the
 * device actually stopped answering, including an op code nobody has heard of
 * yet, because that is the one no fixed list can catch.
 */
function rep(commandTimeouts: Record<string, number> | null, extra: Record<string, unknown> = {}) {
  return { signals: { logMissing: false, commandTimeouts, ...extra } };
}

describe('commandTimeoutsByOpTable', () => {
  it('surfaces an op code that appears in no needle table', () => {
    // The whole point. If this row is missing, the instrument has reproduced the
    // defect it was built to fix.
    const out = commandTimeoutsByOpTable([rep({ SOME_UNKNOWN_OPCODE: 4 })]);

    expect(out).toContain('SOME_UNKNOWN_OPCODE');
    expect(out).toMatch(/4/);
  });

  it('sums each op across reps and orders the noisiest first', () => {
    const out = commandTimeoutsByOpTable([
      rep({ RFID_POWER_OFF: 26, GET_TRIGGER_STATE: 12 }),
      rep({ RFID_POWER_OFF: 28, GET_TRIGGER_STATE: 13, RFID_FIRMWARE_COMMAND: 1 }),
    ]);

    expect(out).toMatch(/RFID_POWER_OFF[^\n]*54/);
    expect(out).toMatch(/GET_TRIGGER_STATE[^\n]*25/);
    expect(out).toMatch(/RFID_FIRMWARE_COMMAND[^\n]*1/);
    // Ordered by volume, so the dominant silence is read first.
    expect(out.indexOf('RFID_POWER_OFF')).toBeLessThan(out.indexOf('GET_TRIGGER_STATE'));
    expect(out.indexOf('GET_TRIGGER_STATE')).toBeLessThan(out.indexOf('RFID_FIRMWARE_COMMAND'));
  });

  it('reports reps affected, not just occurrences', () => {
    // 40 timeouts in one rep and 40 spread over 40 reps are different phenomena;
    // the window is defined by its rep boundaries.
    const out = commandTimeoutsByOpTable([
      rep({ RFID_POWER_OFF: 20 }),
      rep({ RFID_POWER_OFF: 20 }),
      rep({}),
    ]);

    expect(out).toMatch(/RFID_POWER_OFF[^\n]*40[^\n]*2\/3/);
  });

  it('says a quiet arm proves nothing, rather than reporting it as clean', () => {
    // Same discipline as the power-off table: zero is the absence of the
    // condition, never evidence the device is healthy.
    const out = commandTimeoutsByOpTable([rep({}), rep({})]);

    expect(out).toMatch(/NOTHING|not evidence/i);
    expect(out).not.toMatch(/healthy|all good/i);
  });

  it('refuses to score a run whose captures went missing', () => {
    const out = commandTimeoutsByOpTable([{ signals: { logMissing: true } }]);

    expect(out).toMatch(/unobservable/i);
  });

  it('recomputes an archived rep that predates the field but still has its log', () => {
    // The case this instrument was built for. Every number in TRA-1226 came from
    // hand-grepping the archived logs of an arm that had already run — so the
    // tool has to be able to do the same thing, or it only ever works on arms
    // taken after it shipped and the back catalogue stays dark.
    const dir = mkdtempSync(path.join(tmpdir(), 'op-table-archive-'));
    const log = path.join(dir, 'out.log');
    writeFileSync(log, '[Worker] WARN: [CommandManager] Command timeout: GET_TRIGGER_STATE\n');

    // An archived record: needles of its day, no commandTimeouts key at all.
    const archived = {
      outputLog: log,
      signals: { logMissing: false, powerOffTimeouts: 0, harnessLines: 1 },
    };

    expect(commandTimeoutsByOpTable([archived])).toContain('GET_TRIGGER_STATE');
  });

  it('refuses to score a runner that cannot observe these lines', () => {
    // e2e drops `[CommandManager]` warns at the console forwarder, so null. An
    // empty table there would read as "the device answered everything".
    const out = commandTimeoutsByOpTable([rep(null)]);

    expect(out).toMatch(/unobservable/i);
    expect(out).not.toMatch(/never went silent/i);
  });
});
