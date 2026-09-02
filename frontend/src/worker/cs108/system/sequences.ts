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
 * Powers down modules and enables basic reporting
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
    event: GET_TRIGGER_STATE  // Check if trigger is already pressed on connect
  },
  ...BATTERY_VOLTAGE_SEQUENCE,
  // TODO: replace automated battery reporting with internal timer based GET_BATTERY_VOLTAGE updates
  // use ReaderSettings.system.batteryCheckInterval setting to set the timer, default 60 seconds
  // default to 0 when app is in test mode to simplify reader rx/tx activity analysis and debugging
  // for now we just disable this and rely on the idle GET_BATTERY_VOLTAGE above to display battery level
];

