/**
 * Locate mode command sequences
 *
 * This module contains all command sequences related to LOCATE mode operation,
 * including the main LOCATE_SEQUENCE and dynamic EPC mask configuration.
 */

import type { CommandSequence, SequenceCommand } from '../../type.js';
import { createFirmwareCommand, CommandType } from '../firmware-command.js';
import {
  RFID_REGISTERS,
  INV_SEL_VALUES,
  TAG_MEMORY_BANK,
  TAGMASK_DESCRIPTOR,
  TAGMSK_DESCRIPTOR_INDEX,
  EPC_MEMORY_OFFSET,
  EPC_BIT_LENGTH,
  buildInvCfg,
  buildQueryCfg,
} from '../constant.js';
import { RFID_FIRMWARE_COMMAND, RFID_POWER_ON } from '../../event.js';
import { logger } from '../../../utils/logger.js';

/**
 * How a register write survives one unanswered frame.
 *
 * The same schedule RFID_STOP_SEQUENCE carries, and deliberately the same: both
 * are op code 0x8002, so both are governed by the one measured answer
 * distribution behind RFID_FIRMWARE_COMMAND's 200ms timeout (p99.9 59.8ms, max
 * 67.8ms over 4,879 responses). Two schedules for one op code would be two
 * claims about the same hardware.
 *
 * Register writes are answered almost always — 45,226 of 45,228 in the
 * 2026-09-01 200-rep arm — which is why this was never the headline defect and
 * why the original framing of TRA-1239 (the mask write as the CAUSE of the 47
 * unanswered commands) is refuted by that ring: 45 of the 47 were the ABORT,
 * which already retries. What the two survivors cost is out of proportion to
 * their rate, and that is the actual argument for this:
 *
 *   locateSettingsSequence is ~19 writes and reader.ts splices
 *   LOCATE_CONFIG_SEQUENCE in front of it, so a single silent frame ends the
 *   whole run at that step — descriptor registers half written, and INV_CFG,
 *   the write that puts the Selects to work, never sent at all.
 *
 * The retry is worth having because the device ANSWERS one: 44 of the 45
 * unanswered ABORTs in the same arm were recovered on retry, 0 went un-retried.
 * That was the open question this change was gated on, and it is the reason a
 * retry is not just a slower way to fail.
 *
 * NOT `toleratesFailure`. Tolerating a mask write continues the sequence with
 * the descriptor in an unknown state and raises nothing, so Locate searches on
 * a mask that is part this tag and part the last one — which an operator reads
 * as the item not being there. A sequence that fails is recoverable; one that
 * lies is not. Refs TRA-1239.
 */
const REGISTER_WRITE_RETRIES = [100, 200, 500, 1000];

/** One WRITE_REGISTER command. */
const writeRegister = (register: number, value: number): SequenceCommand => ({
  event: RFID_FIRMWARE_COMMAND,
  payload: createFirmwareCommand(CommandType.WRITE_REGISTER, { register, value }),
  retryDelays: REGISTER_WRITE_RETRIES
});

/**
 * LOCATE Mode Sequence
 * Based on CS108 API Spec Appendix C.5 - Search Tag Example
 *
 * This sequence configures the reader for LOCATE mode with:
 * - Continuous antenna cycles (0x00000000)
 * - Standard query configuration (0x00000180)
 * - Fixed Q = 0 for single tag search
 *
 * Note: Tag mask and INV_CFG are set by setSettings() when targetEPC is provided
 */
export const LOCATE_CONFIG_SEQUENCE: CommandSequence = [
  {
    event: RFID_POWER_ON,
    retryDelays: [100]  // Power commands may fail initially
  },

  // Set Inventory Parameters matching vendor app configuration
  // Specify antenna port dwell zero to never cycle between antennas - cs108 only has 1 antenna
  // 0x00000000 indicates that dwell time should not be used.
  writeRegister(RFID_REGISTERS.ANT_PORT_DWELL, 0x00000000),        // 0x0705
  writeRegister(RFID_REGISTERS.QUERY_CFG, buildQueryCfg({          // 0x0900
    query_sel: 3,     // 11 (binary) = 3 = SL ✓
    query_target: 0,  // 0 = A
    query_session: 0  // 0 = S0
  })),

  // Set Inventory Algorithm - Fixed Q = 5 (vendor app configuration)
  writeRegister(RFID_REGISTERS.INV_SEL, INV_SEL_VALUES.FIXED_Q),   // 0x0902, 0x00
  writeRegister(RFID_REGISTERS.INV_ALG_PARM_0, 0x05),              // 0x0903, Fixed Q = 5
  writeRegister(RFID_REGISTERS.INV_ALG_PARM_2, 0x00000000)         // 0x0905, default for Fixed Q

  // NOTE: Tag mask (TAGMSK_*) and INV_CFG will be set by setSettings() when targetEPC is provided
];

/**
 * Configure one Select descriptor to match `paddedEpc` exactly at `maskBitLength`.
 *
 * TAGMSK_DESC_SEL leads, and has to: it decides which of the 8 register sets
 * every write below it lands on. Everything after it is that descriptor's own
 * state, so two calls with different indices configure two independent Selects.
 *
 * The mask registers take their bytes reversed, compensating for
 * createFirmwareCommand's little-endian conversion.
 *
 * TAGMSK_12_15 is written only for a 128-bit mask. A 96-bit one deliberately
 * does NOT clear it: per the vendor spec the firmware scans only TAGMSK_LEN
 * bits when it builds the Select, so a value a previous locate left in the tail
 * register is inert.
 */
function maskDescriptorCommands(
  descriptor: number,
  descriptorCfg: number,
  paddedEpc: string,
  maskBitLength: number
): SequenceCommand[] {
  const bytes: number[] = [];
  for (let i = 0; i < paddedEpc.length; i += 2) {
    bytes.push(parseInt(paddedEpc.substring(i, i + 2), 16));
  }

  const maskValue = (offset: number) =>
    ((bytes[offset + 3] << 24)
      | (bytes[offset + 2] << 16)
      | (bytes[offset + 1] << 8)
      | bytes[offset]) >>> 0;

  return [
    writeRegister(RFID_REGISTERS.HST_TAGMSK_DESC_SEL, descriptor),
    writeRegister(RFID_REGISTERS.TAGMSK_DESC_CFG, descriptorCfg),
    writeRegister(RFID_REGISTERS.TAGMSK_BANK, TAG_MEMORY_BANK.EPC),
    writeRegister(RFID_REGISTERS.TAGMSK_PTR, EPC_MEMORY_OFFSET.AFTER_PC_BITS),
    writeRegister(RFID_REGISTERS.TAGMSK_LEN, maskBitLength),
    writeRegister(RFID_REGISTERS.TAGMSK_0_3, maskValue(0)),
    writeRegister(RFID_REGISTERS.TAGMSK_4_7, maskValue(4)),
    writeRegister(RFID_REGISTERS.TAGMSK_8_11, maskValue(8)),
    ...(maskBitLength === EPC_BIT_LENGTH.EXTENDED_128
      ? [writeRegister(RFID_REGISTERS.TAGMSK_12_15, maskValue(12))]
      : [])
  ];
}

/**
 * Generate command sequence for EPC mask configuration in LOCATE mode
 *
 * Configures one or two Select descriptors to match a specific EPC, then
 * enables tag select so the inventory only answers from tags the Select
 * asserted SL on.
 *
 * ## Mask width (TRA-1108)
 *
 * The width is taken from the value's own length: anything longer than 24 hex
 * chars can only be a 128-bit EPC, so it pads out to 32 and masks all 128 bits
 * via TAGMSK_12_15. Anything shorter pads to 24 and masks 96.
 *
 * 96 and 128 are the only widths this masks exactly, because they are the only
 * ones seen in the field. GS1 also defines longer fixed forms (170, 174, 195,
 * 198, 202, 212) and Gen2 allows up to 496 bits. Those are reachable in
 * principle — TAGMSK_LEN is a bit count that explicitly supports non-byte-
 * aligned masks, and the register file runs to TAGMSK_28_31 — but TAGMSK_LEN is
 * an 8-bit field, so the hardware ceiling is 255 bits, and none of those higher
 * registers has ever been put on a reader by this code.
 *
 * So anything wider than 128 bits deliberately falls through to a 128-bit
 * PREFIX search, and says so in the log rather than failing silently. Widening
 * further is a hardware-validation exercise, not a code change.
 *
 * ## Two descriptors when the width is ambiguous (TRA-1120)
 *
 * A leading-zero-stripped '533034313633' is indistinguishable between a 96-bit
 * and a 128-bit origin, and the width cannot be recovered any later than this.
 * TRA-1108 handled that by having the Scan-tab Locate link send the untruncated
 * `tag.epc`, but the entry points that matter here structurally cannot: the
 * manual EPC field is whatever an operator read off a label, and the tag
 * registry itself holds stripped values because the Scan-tab commissioning
 * modal pre-fills them that way.
 *
 * One descriptor has to pick a width, so a stripped 128-bit EPC never matched.
 * Two do not have to pick:
 *
 * | Descriptor        | Mask                    | TAGMSK_LEN |
 * | ----------------- | ----------------------- | ---------- |
 * | LOCATE            | value padded to 24 hex  | 0x60 (96)  |
 * | LOCATE_ALT_WIDTH  | value padded to 32 hex  | 0x80 (128) |
 *
 * Both are anchored at TAGMSK_PTR 0x20 and both are EXACT at their own width,
 * so this adds no new false-positive class. That is why it beats matching on
 * the rightmost 24 chars, which would ignore a 128-bit tag's leading 32 bits —
 * where SGTIN-128 puts the header and company prefix — and so reintroduce the
 * TRA-1108 bug mirrored.
 *
 * ### How two Selects become OR rather than AND
 *
 * Every enabled descriptor issues its own Select before the inventory, in index
 * order, and each one's sel_action (TAGMSK_DESC_CFG bits 6:4) decides what it
 * does to SL on a match and on a miss.
 *
 * LOCATE keeps the default sel_action 000 — assert on match, deassert on miss —
 * and runs first. LOCATE_ALT_WIDTH uses 001, assert on match, do nothing on a
 * miss. A tag matching either width therefore ends with SL asserted, and one
 * matching neither ends with it deasserted:
 *
 *   matches 96      LOCATE asserts,   ALT does nothing  -> selected
 *   matches 128     LOCATE deasserts, ALT asserts       -> selected
 *   matches neither LOCATE deasserts, ALT does nothing  -> not selected
 *
 * The last row is why no separate clearing Select is needed. Gen2 SL persists
 * after a Select, so a tag left asserted by the previous search would otherwise
 * read as selected forever; LOCATE's deassert-on-miss clears it. That only
 * holds while LOCATE runs first, which is what pins it to descriptor index 0.
 *
 * ### Off the ambiguous path
 *
 * A value over 24 chars can only be 128-bit, so one exact descriptor is both
 * correct and cheaper — and LOCATE_ALT_WIDTH is explicitly DISABLED rather than
 * left alone. This function runs again on every settings change without
 * re-running LOCATE_CONFIG_SEQUENCE, so a descriptor left enabled by an earlier
 * ambiguous locate would keep issuing its stale Select and OR a wrong tag into
 * the new search.
 *
 * Note that exactly 24 chars is still ambiguous — a 128-bit EPC with eight
 * leading zero hex chars strips to 24 — so the boundary is >24, not ≥24.
 *
 * @param targetEPC - Normalized EPC hex string (≤32 chars), or undefined to skip
 * @returns Command sequence to configure tag mask for locate mode
 */
export function locateSettingsSequence(targetEPC?: string): CommandSequence {
  if (targetEPC === undefined) {
    return [];
  }
  // Inline EPC to mask conversion (avoid dependency on epc-filter.ts)
  // Remove spaces and convert to uppercase
  const cleanEpc = targetEPC.replace(/\s/g, '').toUpperCase();

  if (cleanEpc.length > 32) {
    // Only the leading 128 bits get masked, so this is a prefix search and any
    // tag sharing that prefix will answer. Silent narrowing is exactly the
    // failure this function was fixed for; say it out loud instead.
    logger.warn(
      `[Locate] EPC is ${cleanEpc.length} hex chars (>128 bits); masking the leading 128 bits only. ` +
      'Locate may report a different tag sharing that prefix.'
    );
  }

  // Over 24 chars the value can only have come from a 128-bit EPC. At or below
  // 24 it could be either width, and the second descriptor covers the other one.
  const isExtended = cleanEpc.length > 24;

  const primaryDescriptor = maskDescriptorCommands(
    TAGMSK_DESCRIPTOR_INDEX.LOCATE,
    // sel_action left at its default 000: assert SL on a match, deassert on a
    // miss. The deassert is what clears the SL Gen2 persists from the last
    // search, so this descriptor has to be the one that runs first.
    TAGMASK_DESCRIPTOR.ENABLE | TAGMASK_DESCRIPTOR.TARGET_SL,
    cleanEpc.padStart(isExtended ? 32 : 24, '0'),
    isExtended ? EPC_BIT_LENGTH.EXTENDED_128 : EPC_BIT_LENGTH.STANDARD_96
  );

  const alternateDescriptor: SequenceCommand[] = isExtended
    ? [
        // Unambiguous, so there is nothing to OR — but an earlier ambiguous
        // locate may have left this descriptor enabled with a stale mask.
        writeRegister(
          RFID_REGISTERS.HST_TAGMSK_DESC_SEL,
          TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH
        ),
        writeRegister(RFID_REGISTERS.TAGMSK_DESC_CFG, TAGMASK_DESCRIPTOR.DISABLED)
      ]
    : maskDescriptorCommands(
        TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH,
        // sel_action 001: assert SL on a match, do NOTHING on a miss, so this
        // descriptor only ever adds to what the primary one selected.
        TAGMASK_DESCRIPTOR.ENABLE
          | TAGMASK_DESCRIPTOR.TARGET_SL
          | TAGMASK_DESCRIPTOR.SEL_ACTION_ASSERT_ON_MATCH_ONLY,
        cleanEpc.padStart(32, '0'),
        EPC_BIT_LENGTH.EXTENDED_128
      );

  return [
    ...primaryDescriptor,
    ...alternateDescriptor,
    // Enable locate mode with mask. Must come last — it is what puts the
    // configured Selects to work.
    writeRegister(
      RFID_REGISTERS.INV_CFG,
      buildInvCfg({
        tag_delay: 30,  // 30ms delay (matches CS108 Library geiger mode)
        tag_sel: 1      // Enable tag select
      })
    )
  ];
}
