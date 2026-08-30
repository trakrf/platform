/**
 * Every ABORT declares the vendor's post-ABORT quiet window.
 *
 * The spec attaches the constraint to the command, not to one call site:
 *
 *   "After the 'ABORT' command to stop inventory, a 2 seconds delay is required
 *    for the reader to clear buffer before it can execute another command"
 *   — CS108/CS463 Byte Stream API Specifications, p.106, printed twice: once
 *     under "Stop Inventory" and once under "Stop Searching".
 *
 * So a per-sequence declaration is only as good as whoever remembers to write
 * it. This test is the mechanism: it enumerates every CommandSequence the
 * worker exports, finds every entry whose payload is the 0x40 0x03 ABORT
 * control command, and fails if one of them does not carry the window. A new
 * sequence that aborts and forgets is caught here rather than on a reader.
 *
 * ⚠ The default scope is "another command", not "another inventory". The reader
 * cycles the radio on every mode change and on tab navigation — IDLE_SEQUENCE
 * opens with RFID_POWER_OFF, and buildModeSequences() prefixes it to every mode
 * — so on that path the command landing inside the window is a POWER OFF. That
 * case is the one the vendor's examples illustrate and it stays gated.
 *
 * ONE command opts out: RFID_START_SEQUENCE, for a same-mode trigger restart,
 * where nothing is powered or reconfigured. Measured on hardware, gating it
 * cost ~2s per trigger cycle and that stall is what provoked the trigger
 * cycling that lost an edge. Exemptions are enumerated below so the flag cannot
 * spread by copy-paste.
 *
 * Refs: TRA-1197, TRA-1185, ADR 0011.
 */

import { describe, it, expect } from 'vitest';
import type { CommandSequence, SequenceCommand } from './type.js';
import { POST_ABORT_QUIET_MS, RFID_START_SEQUENCE, RFID_STOP_SEQUENCE, transmitPowerSequence } from './rfid/sequences.js';
import { BATTERY_VOLTAGE_SEQUENCE, IDLE_SEQUENCE, SHUTDOWN_SEQUENCE } from './system/sequences.js';
import { BARCODE_CONFIG_SEQUENCE, BARCODE_START_SEQUENCE, BARCODE_STOP_SEQUENCE } from './barcode/sequences.js';
import { INVENTORY_CONFIG_SEQUENCE } from './rfid/inventory/sequences.js';
import { LOCATE_CONFIG_SEQUENCE, locateSettingsSequence } from './rfid/locate/sequences.js';
import { createFirmwareCommand, CommandType } from './rfid/firmware-command.js';

/**
 * Every sequence the worker can put on the wire.
 *
 * Listed by hand because the alternative — globbing the modules — would quietly
 * cover nothing if a path changed, and a detector that silently inspects an
 * empty set reports success. If a new sequence module appears, add it here; the
 * canary below is what makes forgetting visible.
 */
const ALL_SEQUENCES: Array<[string, CommandSequence]> = [
  ['RFID_START_SEQUENCE', RFID_START_SEQUENCE],
  ['RFID_STOP_SEQUENCE', RFID_STOP_SEQUENCE],
  ['transmitPowerSequence(25)', transmitPowerSequence(25)],
  ['BATTERY_VOLTAGE_SEQUENCE', BATTERY_VOLTAGE_SEQUENCE],
  ['IDLE_SEQUENCE', IDLE_SEQUENCE],
  ['SHUTDOWN_SEQUENCE', SHUTDOWN_SEQUENCE],
  ['BARCODE_CONFIG_SEQUENCE', BARCODE_CONFIG_SEQUENCE],
  ['BARCODE_START_SEQUENCE', BARCODE_START_SEQUENCE],
  ['BARCODE_STOP_SEQUENCE', BARCODE_STOP_SEQUENCE],
  ['INVENTORY_CONFIG_SEQUENCE', INVENTORY_CONFIG_SEQUENCE],
  ['LOCATE_CONFIG_SEQUENCE', LOCATE_CONFIG_SEQUENCE],
  ['locateSettingsSequence(epc)', locateSettingsSequence('E280689400000000001018DD')]
];

const ABORT_PAYLOAD = createFirmwareCommand(CommandType.ABORT);

/** Identify the ABORT by its bytes, not by the sequence it happens to sit in. */
function isAbort(cmd: SequenceCommand): boolean {
  if (!cmd.payload || cmd.payload.length !== ABORT_PAYLOAD.length) return false;
  return ABORT_PAYLOAD.every((byte, i) => cmd.payload![i] === byte);
}

const abortEntries = ALL_SEQUENCES.flatMap(([name, sequence]) =>
  sequence
    .map((cmd, index) => ({ name, index, cmd }))
    .filter(({ cmd }) => isAbort(cmd))
);

describe('post-ABORT quiet window', () => {
  it('finds the ABORTs it is meant to be checking', () => {
    // CANARY. Without it, a change to the ABORT payload shape or to an import
    // makes `abortEntries` empty and every assertion below passes vacuously —
    // a green suite asserting nothing about anything. There are two known
    // ABORTs today; a third is a reason to look, not to edit this number
    // reflexively.
    expect(abortEntries.map(e => `${e.name}[${e.index}]`).sort())
      .toEqual(['RFID_STOP_SEQUENCE[0]', 'SHUTDOWN_SEQUENCE[0]']);
  });

  it.each(abortEntries.map(e => [`${e.name}[${e.index}]`, e.cmd] as const))(
    'declares the window on %s',
    (_label, cmd) => {
      expect(cmd.quietPeriodAfter).toBe(POST_ABORT_QUIET_MS);
    }
  );

  it('names every command exempted from the window, and no others', () => {
    // The exemption is a claim about the DEVICE — that this command is safe to
    // issue inside a post-ABORT window. The vendor note says "another command"
    // unqualified, so each exemption is an extrapolation and belongs in a list
    // somebody has to edit deliberately. Without this, the flag spreads by
    // copy-paste and the window quietly stops meaning anything.
    const exempt = ALL_SEQUENCES.flatMap(([name, sequence]) =>
      sequence
        .map((cmd, index) => ({ label: `${name}[${index}]`, cmd }))
        .filter(({ cmd }) => cmd.ignoresQuietPeriod)
        .map(({ label }) => label)
    );

    expect(exempt.sort()).toEqual(['RFID_START_SEQUENCE[0]']);
  });

  it('holds the vendor figure, not a rounded-down one', () => {
    // The defect this replaced cited 2000 in a comment and slept 1000. The
    // number and its enforcement drifted because they lived in different
    // places; pinning it here keeps them together.
    expect(POST_ABORT_QUIET_MS).toBe(2000);
  });

  it('puts a radio power-off inside the window on the mode-change path', () => {
    // Not a hypothetical. Every mode change and every tab navigation runs
    // buildModeSequences(), which prefixes IDLE_SEQUENCE, whose FIRST command
    // is RFID_POWER_OFF. Following a scan stop that is the command dispatched
    // straight after the ABORT — so this is the case the window exists for,
    // and it is why the constraint could not live in stopScanning's caller.
    expect(IDLE_SEQUENCE[0].event.name).toBe('RFID_POWER_OFF');
  });
});
