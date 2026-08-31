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
    retryDelays: [100]  // Power commands may fail initially
  },
  {
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

