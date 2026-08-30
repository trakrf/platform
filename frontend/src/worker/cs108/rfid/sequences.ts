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
 * How long the reader needs to clear its buffer after an ABORT before it can
 * execute another command.
 *
 * Vendor requirement, not a guess:
 *
 *   "After the 'ABORT' command to stop inventory, a 2 seconds delay is required
 *    for the reader to clear buffer before it can execute another command"
 *   — CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.pdf p.106
 *     (docs/frontend/cs108/... lines 6255 and 6400)
 *
 * The stop path used to cite this figure in a comment and then sleep for half
 * of it, in the caller — wrong in both directions at once: the operator waited
 * a second they did not need to, and the next command could reach a reader that
 * was still clearing. Declaring it here makes CommandManager gate the NEXT
 * dispatch, so the constraint is honoured in full and nobody waits for it.
 *
 * Do not "verify" this by watching the notification stream go quiet instead:
 * packets ceasing is not the buffer clearing. The spec gives a duration, not an
 * observable. See TRA-1185.
 */
export const POST_ABORT_QUIET_MS = 2000;

/**
 * Stop RFID scanning sequence
 * Sends ABORT command to halt tag reading
 */
export const RFID_STOP_SEQUENCE: CommandSequence = [{
  event: RFID_FIRMWARE_COMMAND,
  payload: createFirmwareCommand(CommandType.ABORT),
  quietPeriodAfter: POST_ABORT_QUIET_MS
}];