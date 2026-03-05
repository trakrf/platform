/**
 * CS108 Barcode Module Command Sequences
 */

import type { CommandSequence } from '../type.js';
import {
  BARCODE_POWER_ON,
  BARCODE_SEND_COMMAND,
  BARCODE_ESC_STOP,
  BARCODE_ESC_START,
} from '../event.js';
import { ReaderState } from '../../types/reader.js';

/**
 * BARCODE Configuration Sequence
 *
 * Powers up barcode module and configures for scanning
 * Note: RFID_POWER_OFF already handled by IDLE sequence
 */
export const BARCODE_CONFIG_SEQUENCE: CommandSequence = [
  {
    event: BARCODE_POWER_ON,
    retryOnError: true  // Power commands may fail initially
  },
  {
    event: BARCODE_SEND_COMMAND,
    payload: BARCODE_ESC_STOP,  // Ensure scanner is stopped before configuration
    delay: 100
  }
  // Additional barcode configuration commands can be added here
];

/**
 * Start Barcode Scanning Sequence
 *
 * Sends continuous reading command to start barcode scanning.
 * Scanner stays active until explicitly stopped via BARCODE_STOP_SEQUENCE.
 */
export const BARCODE_START_SEQUENCE: CommandSequence = [
  {
    event: BARCODE_SEND_COMMAND,
    payload: BARCODE_ESC_START,
    finalState: ReaderState.SCANNING  // Transition to Scanning state on success
  }
];

/**
 * Stop Barcode Scanning Sequence
 *
 * Sends stop command to halt barcode scanning
 */
export const BARCODE_STOP_SEQUENCE: CommandSequence = [
  {
    event: BARCODE_SEND_COMMAND,
    payload: BARCODE_ESC_STOP,
    finalState: ReaderState.CONNECTED  // Return to connected state after stop
  }
];