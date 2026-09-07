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
  ignoresQuietPeriod: true,

  // One unanswered START_INVENTORY used to wedge the reader in ERROR until a
  // page reload, because an omitted schedule means exactly one attempt
  // (`command.ts`: `cmd.retryDelays ?? []`). The stop path got its retry in
  // TRA-1197 and the locate mask write in TRA-1239; this was the third instance
  // of the same gap and the only one still uncovered.
  //
  // Measured 2026-09-07 on a 30-rep arm: 3 wedges, and
  // "[Reader] Failed to start scanning" discriminated them perfectly — 3 of 3
  // failures, 0 of 27 passes. The RFID_FIRMWARE_COMMAND timeout underneath it
  // does NOT discriminate: 11 of 15 reps on the earlier arm carried one and
  // passed. The timeout is common and survivable wherever it can be retried,
  // and was lethal only here, where it could not be.
  //
  // Same shape as the stop path's schedule, and short at the front for the same
  // reason: the residual risk is a retry slow enough for an unrelated
  // abortSequence() to cancel it, and a first retry at ~300ms is a far smaller
  // window for that than a patient one.
  //
  // ⚠ Retry does NOT reintroduce the quiet-window stall this step exists to
  // avoid. The loop re-dispatches with this step's own `ignoresQuietPeriod`, so
  // every attempt ignores the window exactly as the first does. Pinned by
  // `start-retry-on-timeout.test.ts`, because it is a property of the loop
  // rather than of this declaration.
  //
  // ⚠ Retry, do NOT reach for `toleratesFailure`. It looks like the right knob —
  // its docstring describes TRA-1217, a structurally identical failure — but
  // tolerating a failed start would publish `finalState: SCANNING` for a reader
  // that is not scanning. The wedge at least announces itself; a tolerated
  // failure is an operator waving a handheld at tags that never read.
  retryDelays: [100, 200, 500, 1000]
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
  // something new, it is the thing the window is protecting.
  //
  // CSL hangs no delay on ABORT either — 57 StopOperation() call sites, not one
  // followed by a sleep. But state that accurately, because two earlier versions
  // of this comment did not (TRA-1214):
  //
  //   - NOT "they have no such machinery". They do: a per-command delay table
  //     (BTSend.cs:379-401) and a state gate (BTSend.cs:426-429). They own the
  //     hook and chose not to hang an abort delay on it, which is STRONGER
  //     evidence for this decision than an absence would be.
  //   - NOT "0xA004 wires an abort to the trigger so a stop cannot be lost".
  //     `grep A004` over all 281 vendor .cs files returns zero hits. The binding
  //     is a firmware default, but their software never sets it, never reads it
  //     and does not depend on it — a firmware-local abort needs no BLE round
  //     trip, so latency is the sufficient explanation. Nothing supports the
  //     reliability motive, and their capture shows them NOT tolerating a lost
  //     abort: the pipeline blocks and re-sends.
  //
  // Where we genuinely diverge, and it is worth knowing: CSL's post-ABORT
  // protection is RESPONSE-gated, not time-gated. Nothing leaves their send
  // buffer until the ABORT is answered. We send the restart regardless. So this
  // exemption is not "more conservative than the vendor" — the practical
  // conclusion matches theirs, the mechanism does not. Replacing the window with
  // a response barrier is the follow-on that would close that gap.
  //
  // What still waits: RFID_POWER_OFF and the register writes on the mode-change
  // path, which is the case the vendor's worked examples actually illustrate.
  ignoresQuietPeriod: true,

  // The reader sometimes never answers an ABORT, and one miss used to fail the
  // stop outright.
  //
  // Measured 2026-08-30 on a 20-rep arm with the bridge's packet ring dumped and
  // 0x8002 frames counted per rep, windowed by each rep's own start/end:
  //
  //   15 passing reps   248 downlink / 248 uplink   deficit 0, every one
  //    5 failing reps   deficit 1, 1, 1, 1, 2
  //
  // Every one of those 6 gaps is this exact frame — the ABORT, byte-for-byte
  // CSL's own StopOperation() payload:
  //
  //   A7 B3 0A C2 82 37 00 00 80 02 | 40 03 00 00 00 00 00 00
  //
  // So the reply is genuinely absent, not late and not mis-matched: had the
  // replies arrived and settled the wrong command (0x8002 is shared by every
  // RFID firmware command, so that was the competing hypothesis), the failing
  // reps would have balanced. They do not.
  //
  // It is transient rather than a wedged reader — rep 10 had two unanswered
  // ABORTs 63s apart while its other 169 command/reply pairs all balanced —
  // which is why a second attempt is worth anything at all.
  //
  // CSL budgets 21 attempts over ~42s for every RFID command (BTSend.cs:313-334,
  // the send buffer entry survives a timeout and is re-sent). Their entire
  // margin is attempt COUNT, not patience per attempt.
  //
  // Measured on a 20-rep arm with one retry at +5.1s: rep failures fell 25% ->
  // 5%, and every one of the 13 retries that fired was answered (0 missed).
  // The retry works. The single residual failure was a retry that never got to
  // run — a teardown called abortSequence() inside the 5.1s window and the
  // re-dispatch hit `isAborted`. So the remaining risk is the retry being SLOW
  // enough for something else to cancel it, which is why the schedule is short
  // at the front. At a 200ms timeout the first retry lands at 300ms rather than
  // 5100ms: a 17x smaller window for an unrelated abort to land inside.
  retryDelays: [100, 200, 500, 1000]
}];