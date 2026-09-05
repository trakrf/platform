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
 * ⚠ It used to say "and enables basic reporting". It enables nothing, and never
 * has — the device's own reporting is deliberately off (ADR 0019). Corrected
 * under TRA-1247, where the old wording sent a session looking for a reporting
 * path that does not exist.
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
    // ⚠ Its answer is NOT relied on, and this comment used to claim it was:
    // "Check if trigger is already pressed on connect". That states intent and
    // reads as observed behaviour. Per Mike the polled trigger notification
    // does not work as advertised on the firmware in hand (SiLabs 1.0.15 /
    // BT 1.0.17 / RFID 2.6.41) and was tried and abandoned; the trigger level
    // is carried by the 0xA102/0xA103 edges instead. Corrected under TRA-1247.
    //
    // Two things follow, neither settled here because both need hardware:
    //
    //   - If 0xA001 never answers, the "operator already squeezing the trigger
    //     as the reader connects" case is a GAP, not coverage. Nothing else
    //     catches it: with no edge to observe, `triggerState` stays false until
    //     they let go and press again.
    //   - This step sits at position 3 of 4 in the idle prefix, so even a
    //     working answer would describe the trigger at the START of bring-up,
    //     while convergence consumes it only after the firmware config, power
    //     on, transmit power, identity reads and mask write.
    //
    // The experiment that decides whether this command stays: send 0xA008
    // (START_TRIGGER_REPORTING) once during bring-up, then 0xA001. An answer
    // makes the already-held case fixable with one command; silence makes this
    // step dead weight on every mode change and it should come out with this
    // comment. Adding 0xA008 permanently would reverse ADR 0019, so treat that
    // as an experiment, not a fix.
    event: GET_TRIGGER_STATE
  },
  ...BATTERY_VOLTAGE_SEQUENCE,
  // TODO: replace automated battery reporting with internal timer based GET_BATTERY_VOLTAGE updates
  // use ReaderSettings.system.batteryCheckInterval setting to set the timer, default 60 seconds
  // default to 0 when app is in test mode to simplify reader rx/tx activity analysis and debugging
  // for now we just disable this and rely on the idle GET_BATTERY_VOLTAGE above to display battery level
];

