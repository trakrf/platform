/**
 * Reading a MAC register back from the RFID processor.
 *
 * Every register operation this codebase performs is a WRITE, and per spec A.3
 * a write gets no response at all:
 *
 * > This response packet only comes back when the operation is Read register.
 * > There is no response if the operation is write register.
 *
 * So `RFID_FIRMWARE_COMMAND` declares `responseLength: 1` with `parseUint8` —
 * correct for a write acknowledgement's status byte, and unable to represent a
 * register value. `createFirmwareCommand` has always had a `READ_REGISTER`
 * branch and nothing has ever called it: we could ask, and could not hear.
 *
 * ⚠ **`reg_addr` is echoed back**, which makes this the one self-identifying
 * exchange on `0x8002`. Every other firmware command is indistinguishable at
 * the op-code level — the defect TRA-1154 was opened for, where any reply could
 * settle any pending command. A caller here can and should check that the reply
 * names the register it asked about.
 *
 * Refs: TRA-1232, TRA-1223 (MAC_ERROR is the R2000's own account of a state we
 * have only ever seen from the Bluetooth board's side).
 */

/** REG_RESP is 8 bytes: pkt_ver, reserved, addr(2), data(4). */
const REG_RESP_LENGTH = 8;

/**
 * The low-level API packet version. Spec A.3 gives `0x70` for low level and
 * `0x00` for high level; we speak low level everywhere, so anything else here
 * means the payload is not the register response we think it is.
 */
const LOW_LEVEL_PKT_VER = 0x70;

export interface RegisterResponse {
  /** The register the device says it read — echoed back, per spec A.3. */
  register: number;
  /** Its 32-bit contents. Unsigned: a register with the top bit set is not negative. */
  value: number;
}

/**
 * Decode a REG_RESP payload.
 *
 * ⚠ Both multi-byte fields are **byte-swapped** — spec A.3's "REVERSELY
 * POPULATED", LSB first. Reading them the other way round produces
 * plausible-looking wrong numbers rather than an error, which is the whole
 * reason this has its own function and its own tests.
 */
export function parseRegisterResponse(payload: Uint8Array): RegisterResponse {
  if (payload.length < REG_RESP_LENGTH) {
    throw new Error(
      `Register response too short: got ${payload.length} bytes, need ${REG_RESP_LENGTH}`
    );
  }

  if (payload[0] !== LOW_LEVEL_PKT_VER) {
    throw new Error(
      `Not a low-level register response: pkt_ver is ` +
      `0x${payload[0].toString(16).padStart(2, '0')}, expected 0x70`
    );
  }

  const register = payload[2] | (payload[3] << 8);

  // `>>> 0` because `<< 24` on a byte ≥ 0x80 produces a negative int32 in JS.
  // Without it a register reading 0xFFFFFFFF comes back as -1, which compares
  // and prints wrongly everywhere downstream.
  const value = (
    payload[4] |
    (payload[5] << 8) |
    (payload[6] << 16) |
    (payload[7] << 24)
  ) >>> 0;

  return { register, value };
}

export interface FirmwareVersion {
  major: number;
  minor: number;
  patch: number;
  /** `major.minor.patch`, for display and for logs. */
  text: string;
}

/**
 * Decode `FIRMWARE_VER` (register 0x0000), per spec A.4.
 *
 * ```
 * bits 31:24  Major   8 bit
 * bits 23:12  Minor  12 bit
 * bits 11:0   Patch  12 bit
 * ```
 *
 * ⚠ **Minor and patch are TWELVE bits, not eight.** The obvious byte-wise
 * decode — three `& 0xFF` shifts — reads wrong numbers that still look like a
 * version, so it would be believed. CSL's published RFID image is V2.6.46,
 * whose patch alone does not fit the intuition that these are bytes.
 */
export function decodeFirmwareVersion(raw: number): FirmwareVersion {
  const major = (raw >>> 24) & 0xFF;
  const minor = (raw >>> 12) & 0xFFF;
  const patch = raw & 0xFFF;
  return { major, minor, patch, text: `${major}.${minor}.${patch}` };
}
