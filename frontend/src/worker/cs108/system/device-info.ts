/**
 * The Bluetooth board's own identity — firmware versions and serial number.
 *
 * ⚠ **These are NOT byte-swapped. The RFID registers next door ARE.**
 *
 * Spec A.3's "REVERSELY POPULATED" note is scoped to *"RFID firmware commands
 * and replies"* — the `0x8002` register path in `rfid/register-response.ts`.
 * The BT-board commands here are plain, MSB-first:
 *
 * ```
 * 0xB000  Silicon Labs firmware   Byte 0 Major, Byte 1 Minor, Byte 2 Build
 * 0xC000  Bluetooth firmware      Byte 0 Major, Byte 1 Minor, Byte 2 Build
 * 0xB004  Serial number           13-byte UTF-8
 * ```
 *
 * Two adjacent decode conventions in one protocol is the shape that yields a
 * plausible wrong number rather than a failure, so both directions are pinned
 * by tests rather than trusted to this comment.
 *
 * Refs: TRA-1232.
 */

import { CS108_MODULES, type CS108Event } from '../type';

/** Major, minor and build are one byte each. */
const BOARD_VERSION_LENGTH = 3;

/** The vendor library reads 13 bytes of UTF-8 (`ClassSiliconLabIC`, case 0xb004). */
const SERIAL_NUMBER_LENGTH = 13;

export interface BoardVersion {
  major: number;
  minor: number;
  patch: number;
  /** `major.minor.patch`, for display and logs. */
  text: string;
  /**
   * The same value packed as the vendor packs it — `(major<<16)|(minor<<8)|build`.
   *
   * Kept because CSL's own library gates behaviour on this form: it enables
   * trigger-state auto-reporting only at `>= 0x00010010` (V1.0.16), and flags
   * `_firmwareOlderT108` below `0x00010008`. Any comparison we make against
   * their thresholds has to use their encoding.
   */
  packed: number;
}

/**
 * Decode a 3-byte board version. MSB-first — see the header note.
 *
 * Throws on a short payload rather than returning zeros: the vendor's own
 * `_firmwareVersion = 0` fallback makes "read failed" indistinguishable from
 * "version 0.0.0", and a caller here should be able to tell those apart.
 */
export function parseBoardVersion(payload: Uint8Array): BoardVersion {
  if (payload.length < BOARD_VERSION_LENGTH) {
    throw new Error(
      `Board version needs ${BOARD_VERSION_LENGTH} bytes, got ${payload.length}`
    );
  }
  const [major, minor, patch] = payload;
  return {
    major,
    minor,
    patch,
    text: `${major}.${minor}.${patch}`,
    packed: (major << 16) | (minor << 8) | patch,
  };
}

/**
 * Decode the serial number, trimming padding.
 *
 * Returns `''` rather than throwing on an unreadable payload — the vendor wraps
 * the same read in try/catch and yields `""`, and a missing serial is not worth
 * failing a connect over.
 */
export function parseSerialNumber(payload: Uint8Array): string {
  if (payload.length === 0) return '';
  try {
    return new TextDecoder()
      .decode(payload.subarray(0, SERIAL_NUMBER_LENGTH))
      .replace(/\0+$/, '')
      .trim();
  } catch {
    return '';
  }
}

export const GET_SILICON_LAB_VERSION: CS108Event = {
  name: 'GET_SILICON_LAB_VERSION',
  eventCode: 0xB000,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: BOARD_VERSION_LENGTH,
  description: 'Get Silicon Lab IC firmware version',
};

export const GET_BLUETOOTH_VERSION: CS108Event = {
  name: 'GET_BLUETOOTH_VERSION',
  eventCode: 0xC000,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: BOARD_VERSION_LENGTH,
  description: 'Get Bluetooth IC firmware version',
};

export const GET_SERIAL_NUMBER: CS108Event = {
  name: 'GET_SERIAL_NUMBER',
  eventCode: 0xB004,
  module: CS108_MODULES.NOTIFICATION,
  isCommand: true,
  isNotification: false,
  payloadLength: 0,
  responseLength: SERIAL_NUMBER_LENGTH,
  description: 'Get reader serial number',
};
