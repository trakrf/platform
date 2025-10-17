/**
 * Shared RFID command sequences used by both inventory and locate modes
 */

import { CommandSequence } from '../type.js';
import { RFID_FIRMWARE_COMMAND } from '../event';
import { createFirmwareCommand, CommandType } from './firmware-command';
import { RFID_REGISTERS } from './constant';
import { ReaderState } from '../../types/reader';

/**
 * Set transmit power sequence
 * @param power Power in dBm (10-30), or undefined to skip
 */
export function transmitPowerSequence(power?: number): CommandSequence {
  if (power === undefined) {
    return [];
  }

  return [{
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.ANT_PORT_POWER,
      value: Math.round(power * 10)  // Convert dBm to device units
    })
  }];
}

/**
 * Start RFID scanning sequence
 * Sends START_INVENTORY command to begin tag reading
 */
export const RFID_START_SEQUENCE: CommandSequence = [{
  event: RFID_FIRMWARE_COMMAND,
  payload: createFirmwareCommand(CommandType.START_INVENTORY),
  finalState: ReaderState.SCANNING  // Transition to Scanning state on success
}];

/**
 * Stop RFID scanning sequence
 * Sends ABORT command to halt tag reading
 */
export const RFID_STOP_SEQUENCE: CommandSequence = [{
  event: RFID_FIRMWARE_COMMAND,
  payload: createFirmwareCommand(CommandType.ABORT)
}];