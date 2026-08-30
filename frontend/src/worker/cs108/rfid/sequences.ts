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
  finalState: ReaderState.SCANNING,  // Transition to Scanning state on success

  // Restarting inventory does NOT wait out the post-ABORT quiet window.
  //
  // A trigger cycle within one scanning mode is ABORT then START_INVENTORY: no
  // power cycling, no reconfiguration, the radio never leaves the mode. Gating
  // the restart on the window costs ~2s per cycle, measured on hardware —
  // "[CommandManager] Holding 1775ms for the device's quiet window" sitting
  // between an operator's press and anything happening.
  //
  // That stall is not merely slow, it is the thing that starts the failure: an
  // operator who feels nothing happen cycles the trigger harder, the extra
  // edges arrive while the reader is BUSY and are dropped, and on hardware one
  // of them came back as two TRIGGER_RELEASED with no press between — a lost
  // edge. The reader then converged, correctly, onto a stale level and stopped
  // with the trigger held.
  //
  // The window still applies to everything else, including RFID_POWER_OFF on
  // the mode-change path, which is the case the vendor's examples actually
  // illustrate. See ADR 0011 for why the note's placement (Appendix C worked
  // examples, where the ABORT ends the flow and no restart is ever shown) makes
  // the unqualified reading an extrapolation rather than a certainty.
  ignoresQuietPeriod: true
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
  quietPeriodAfter: POST_ABORT_QUIET_MS,

  // An ABORT never waits out another ABORT's window.
  //
  // Observed on hardware: press, release, press, release in quick succession
  // produced "[CommandManager] Holding 1474ms for the device's quiet window"
  // on the STOP — a second ABORT queued behind the first one's window. The
  // operator had let go and the radio kept transmitting for another 1.5s.
  //
  // That inverts the constraint's purpose. The window exists so the reader can
  // clear its buffer before being asked to do something new; stopping is not
  // something new, it is the thing the window is protecting. A stop is also the
  // safety-critical direction — CSL's own firmware wires an abort to the
  // physical trigger (0xA004) precisely so a stop cannot be lost, and their
  // reference app issues StopOperation() with no delay of any kind.
  //
  // What still waits: RFID_POWER_OFF and the register writes on the mode-change
  // path, which is the case the vendor's worked examples actually illustrate.
  ignoresQuietPeriod: true
}];