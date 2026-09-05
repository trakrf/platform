/**
 * CS108 System Command Sequences
 */

import type { CommandSequence } from '../type.js';
import {
  RFID_POWER_OFF,
  BARCODE_POWER_OFF,
  GET_BATTERY_VOLTAGE,
  GET_TRIGGER_STATE
} from '../event.js';

/**
 * Battery Voltage Check Sequence
 *
 * Single command to get current battery voltage
 * Used by both IDLE sequence and battery check timer
 */
export const BATTERY_VOLTAGE_SEQUENCE: CommandSequence = [
  {
    event: GET_BATTERY_VOLTAGE  // Get immediate battery reading
  }
];

/**
 * IDLE Mode Sequence
 *
 * Powers down the RFID and barcode modules, queries the trigger position, and
 * takes one battery reading. `buildModeSequences()` prefixes it to EVERY mode,
 * so every one of these runs on every mode change.
 *
 * ⚠ It enables no reporting, despite the name this sequence shares with the
 * device's auto-reporting commands. The device's own reporting is deliberately
 * off (ADR 0019), so there is no reporting path here to go looking for.
 */
export const IDLE_SEQUENCE: CommandSequence = [
  {
    event: RFID_POWER_OFF,
    retryDelays: [100],  // Power commands may fail initially

    // An unanswered power-off must not fail the mode change.
    //
    // Observed 2026-08-31 in the archived bridge capture of a 200-rep arm.
    // Inside a contiguous 82-minute window (reps 46-108) the CS108 answered
    // 421 of 421 RFID firmware commands (0x8002), streamed 1071 tag
    // notifications, and answered 12 of 1257 RFID_POWER_OFF downlinks. It
    // stopped acknowledging exactly this op code, held that state, and then
    // resumed on its own. The 0x8001 deficit is exactly zero in every bucket
    // outside the window, across all 137 clean reps on both sides.
    //
    // buildModeSequences() prefixes this sequence to EVERY mode, so one silent
    // op code failed every mode change: setMode() threw, published ERROR, and
    // the reader was finished for the session — 63 of 200 reps, 31%.
    //
    // Tolerating it is right whatever the device turns out to be doing, because
    // the alternative was never a working power-off. What we lose is the
    // confirmation, not the attempt: the command is still sent, still retried,
    // and its silence is still logged. TRA-1217.
    toleratesFailure: true
  },
  {
    // Deliberately NOT tolerant. Tolerance is a claim about what this device
    // does with one op code, and 0x9001 has never been watched going silent on
    // a reader — the same discipline the note on `ignoresQuietPeriod` sets.
    event: BARCODE_POWER_OFF,
    retryDelays: [100]  // Barcode module may need retry
  },
  {
    // Check if the trigger is already held, and re-read it on every mode change.
    //
    // The device answers 0xA001 in ~22ms, every time, with 0=released /
    // 1=pushed exactly as the vendor byte-stream API §10.1 specifies — measured
    // on the wire. The answer is not thrown away either: it settles the command
    // in flight AND is forwarded to the notification handler, because 0xA000
    // and 0xA001 are on `CommandManager`'s data-emission list (`command.ts`).
    // So it reaches `TriggerStateHandler`, emits TRIGGER_STATE_CHANGED, and
    // overwrites the host latch at `reader.ts:179`.
    //
    // Two consequences, both load-bearing and both easy to lose:
    //
    //   - The "operator already squeezing the trigger as the reader connects"
    //     case IS covered, by this step, and only by this step: there is no
    //     edge to observe when the trigger went down before the link came up.
    //   - Because `buildModeSequences()` prefixes IDLE_SEQUENCE to EVERY mode,
    //     every mode change re-reads the physical trigger and the device's
    //     answer wins. That is what revokes a SIMULATED press — see ADR 0016
    //     and `tests/e2e/trigger-level-is-reread-on-mode-change.spec.ts`.
    //
    // ⚠ Do not confuse this with the AUTO-REPORTING (0xA008/0xA009), which is a
    // different command, does not work on this firmware, and is unsent by
    // decision — ADR 0019, and the note above the reporting commands in
    // `event.ts`. "The trigger poll does not work" is about those, not this.
    event: GET_TRIGGER_STATE
  },
  ...BATTERY_VOLTAGE_SEQUENCE,
  // TODO: replace automated battery reporting with internal timer based GET_BATTERY_VOLTAGE updates
  // use ReaderSettings.system.batteryCheckInterval setting to set the timer, default 60 seconds
  // default to 0 when app is in test mode to simplify reader rx/tx activity analysis and debugging
  // for now we just disable this and rely on the idle GET_BATTERY_VOLTAGE above to display battery level
];

