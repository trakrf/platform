/**
 * Asking the reader what it is.
 *
 * ## Why anything we produce should carry this
 *
 * Nothing we write down records the reader's firmware — not a bug report, not a
 * bridge status line, not a soak capture. The 2026-09-01 campaign produced four
 * transport captures of a device-side defect and none of them can say what
 * firmware it was observed on; that had to be reconstructed afterwards, which
 * is the "quoted from a note rather than measured" failure the campaign spent
 * itself correcting. Flashing firmware destroys the attribution permanently.
 *
 * ## Why the reads are split in two
 *
 * The three values on the Bluetooth board — Silicon Labs firmware, Bluetooth
 * firmware, serial number — are readable the moment the link is up, so they are
 * read at connect.
 *
 * The two RFID registers are not. They live on the R2000, and the R2000 is
 * POWERED OFF at connect: `IDLE_SEQUENCE` opens with `RFID_POWER_OFF`, and it
 * is `INVENTORY_CONFIG_SEQUENCE` / `LOCATE_CONFIG_SEQUENCE` that open with
 * `RFID_POWER_ON`. Reading them at connect would mean powering the radio up and
 * down again for two register values — on a device whose characterised fault is
 * `RFID_POWER_OFF` going silent for minutes at a time (TRA-1217). So they ride
 * the first RFID mode sequence of a connection instead, where the radio is
 * already on and the commands are already flowing.
 *
 * ## Everything here is best-effort
 *
 * A version string is worth having and it is not worth a failed connect or a
 * failed mode change, so every step tolerates failure. An unanswered read
 * leaves the field undefined, which reads as "we did not get an answer" — never
 * as a value.
 *
 * Refs: TRA-1232, TRA-1223.
 */

import type { CommandSequence } from '../type.js';
import type { ReaderDetails } from '../../types/reader.js';
import { RFID_FIRMWARE_COMMAND } from '../event.js';
import { GET_SILICON_LAB_VERSION, GET_BLUETOOTH_VERSION, GET_SERIAL_NUMBER } from './device-info.js';
import { createFirmwareCommand, CommandType } from '../rfid/firmware-command.js';
import { RFID_REGISTERS } from '../rfid/constant.js';
import {
  decodeFirmwareVersion,
  isRegisterResponse,
  parseRegisterResponse,
  type RegisterResponse,
} from '../rfid/register-response.js';

/**
 * The Bluetooth board's own identity, read once per connection.
 *
 * No `retryDelays`, and no `toleratesFailure` either — not because a failure
 * should stop anything, but because the tolerance is not this list's to claim.
 * These run on the connect path, which cannot afford the BUSY/CONNECTED
 * announcements a sequence publishes on its way through, so the reader issues
 * them one at a time through `runExclusive` and swallows each failure itself.
 * A flag nothing reads is worse than no flag: it reads as a guarantee.
 */
export const IDENTITY_SEQUENCE: CommandSequence = [
  { event: GET_SILICON_LAB_VERSION },
  { event: GET_BLUETOOTH_VERSION },
  { event: GET_SERIAL_NUMBER },
];

/**
 * The two RFID processor registers, read once per connection while the radio
 * is powered.
 *
 * ⚠ These are READS. `createFirmwareCommand` distinguishes them from a write by
 * one byte — access `0x00` rather than `0x01` — and a read built as a write
 * would overwrite the register it meant to inspect. On `MAC_ERROR` that would
 * destroy the value being asked for.
 */
export const RFID_IDENTITY_SEQUENCE: CommandSequence = [
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.READ_REGISTER, {
      register: RFID_REGISTERS.FIRMWARE_VER,
    }),
    toleratesFailure: true,
  },
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.READ_REGISTER, {
      register: RFID_REGISTERS.MAC_ERROR,
    }),
    toleratesFailure: true,
  },
];

/**
 * Fold a register response into what we know, or `null` if it is not ours.
 *
 * Returning `null` rather than the unchanged object is what lets the caller
 * tell "nothing to report" from "reported the same thing again", so an
 * unrelated register write's reply does not re-emit reader details to the UI.
 *
 * ⚠ The register in the response is the register the DEVICE says it read, not
 * the one we asked about. Spec A.3 echoes `reg_addr` back, which makes a
 * register read the one self-identifying exchange on `0x8002` — every other
 * firmware command is indistinguishable at the op-code level, the defect
 * TRA-1154 was opened for. Switching on the echo rather than on what we last
 * sent is what makes that property worth having.
 */
export function applyRegisterResponse(
  details: ReaderDetails,
  response: RegisterResponse
): ReaderDetails | null {
  switch (response.register) {
    case RFID_REGISTERS.FIRMWARE_VER:
      return { ...details, rfidFirmware: decodeFirmwareVersion(response.value).text };
    case RFID_REGISTERS.MAC_ERROR:
      return { ...details, macError: response.value };
    default:
      return null;
  }
}

/**
 * Fold whatever a packet tells us about the reader's identity into what we
 * already know, or `null` if it tells us nothing.
 *
 * Reached from the reader's single packet choke point, BEFORE the split into
 * command responses and notifications, and deliberately so: a register response
 * may settle the command that asked for it or arrive a beat later with nothing
 * in flight, depending on whether the device also sends a status byte first.
 * Observing at the choke point is right under either behaviour, which matters
 * because nothing here has ever read a register from this device and the answer
 * is not knowable from the vendor source.
 */
export function applyIdentityPacket(
  details: ReaderDetails,
  packet: { eventCode: number; payload?: unknown; rawPayload: Uint8Array }
): ReaderDetails | null {
  switch (packet.eventCode) {
    case GET_SILICON_LAB_VERSION.eventCode: {
      const text = boardVersionText(packet.payload);
      return text ? { ...details, siliconLabsFirmware: text } : null;
    }
    case GET_BLUETOOTH_VERSION.eventCode: {
      const text = boardVersionText(packet.payload);
      return text ? { ...details, bluetoothFirmware: text } : null;
    }
    case GET_SERIAL_NUMBER.eventCode:
      // The parser yields '' for an unreadable payload, and an empty serial is
      // not something to report as if we had read one.
      return typeof packet.payload === 'string' && packet.payload.length > 0
        ? { ...details, serialNumber: packet.payload }
        : null;
    case RFID_FIRMWARE_COMMAND.eventCode:
      return isRegisterResponse(packet.rawPayload)
        ? applyRegisterResponse(details, parseRegisterResponse(packet.rawPayload))
        : null;
    default:
      return null;
  }
}

/** The decoded version string, if this payload is one. */
function boardVersionText(payload: unknown): string | undefined {
  if (typeof payload !== 'object' || payload === null) return undefined;
  const text = (payload as { text?: unknown }).text;
  return typeof text === 'string' ? text : undefined;
}

/**
 * The prefix the soak instrument keys on. Keep it in step with
 * `READER_DETAILS_PREFIX` in `scripts/suite-run-signals.mjs`.
 */
export const READER_DETAILS_LOG_PREFIX = '[Reader] Reader details: ';

/**
 * One line, so every rep of every arm attributes its own capture.
 *
 * JSON rather than space-separated `key=value` because the latter has to decide
 * what happens when a serial number contains a space — and "it never will" is
 * how a parser comes to work until the day it does not.
 */
export function formatReaderDetails(details: ReaderDetails): string {
  return `${READER_DETAILS_LOG_PREFIX}${JSON.stringify(details)}`;
}
