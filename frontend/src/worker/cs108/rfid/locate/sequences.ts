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
    retryOnError: true  // Power commands may fail initially
  },

  // Set Inventory Parameters matching vendor app configuration
  // Specify antenna port dwell zero to never cycle between antennas - cs108 only has 1 antenna
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.ANT_PORT_DWELL,  // 0x0705
      value: 0x00000000 // 0x00000000 indicates that dwell time should not be used.
    })
  },
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.QUERY_CFG,  // 0x0900
      value: buildQueryCfg({
        query_sel: 3,     // 11 (binary) = 3 = SL ✓
        query_target: 0,  // 0 = A
        query_session: 0  // 0 = S0
      })
    })
  },

  // Set Inventory Algorithm - Fixed Q = 5 (vendor app configuration)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_SEL,  // 0x0902
      value: INV_SEL_VALUES.FIXED_Q  // 0x00
    })
  },
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_ALG_PARM_0,  // 0x0903
      value: 0x05  // Fixed Q = 5 (vendor app uses this for locate)
    })
  },
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_ALG_PARM_2,  // 0x0905
      value: 0x00000000  // Default for Fixed Q
    })
  }

  // NOTE: Tag mask (TAGMSK_*) and INV_CFG will be set by setSettings() when targetEPC is provided
];

/**
 * Generate command sequence for EPC mask configuration in LOCATE mode
 *
 * This function creates a command sequence that configures the tag mask registers
 * to search for a specific EPC. The sequence includes:
 * 0. Select which mask descriptor the rest of the writes land on
 * 1. Configure mask descriptor (enable + target SL)
 * 2. Select EPC memory bank
 * 3. Set starting bit position (after PC bits)
 * 4. Set mask length (96 or 128 bits, matching the EPC width)
 * 5. Set mask values (3 registers for a 96-bit EPC, 4 for a 128-bit one)
 * 6. Enable search mode with mask
 *
 * ## Descriptor selection
 *
 * The CS108 has 8 Select descriptors, each with its own mask register set, and
 * TAGMSK_DESC_SEL decides which set every subsequent TAGMSK_* write lands on.
 * This used to be omitted and worked only because the power-up default is 0;
 * anything that ever left the register non-zero would have silently aimed these
 * writes at a descriptor nobody enables. Pinning it makes the sequence
 * self-contained rather than dependent on reader state we do not control.
 *
 * ## Mask width (TRA-1108)
 *
 * The width is taken from the value's own length: anything longer than 24 hex
 * chars can only be a 128-bit EPC, so it pads out to 32 and masks all 128 bits
 * via TAGMSK_12_15. Anything shorter pads to 24 and masks 96, which keeps a
 * short value working as a prefix search.
 *
 * The width cannot be recovered any later than this. A leading-zero-stripped
 * '533034313633' is indistinguishable between a 96-bit and a 128-bit origin,
 * which is why the Scan-tab Locate link sends the untruncated `tag.epc`.
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
 * The 96-bit branch deliberately does NOT clear TAGMSK_12_15. Per the vendor
 * spec the firmware scans only TAGMSK_LEN bits when it builds the Select, so a
 * value a previous 128-bit locate left in the tail register is inert.
 *
 * Only one descriptor is used, so a short value is still a prefix search rather
 * than an exact match at the other width. Matching a stripped value at BOTH
 * widths needs two descriptors OR'd together via TAGMSK_DESC_CFG's sel_action
 * — tracked separately.
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

  // Pad with leading zeros to whichever standard EPC width the value fits.
  const isExtended = cleanEpc.length > 24;
  const paddedEpc = cleanEpc.padStart(isExtended ? 32 : 24, '0');

  if (cleanEpc.length > 32) {
    // Only the leading 128 bits get masked, so this is a prefix search and any
    // tag sharing that prefix will answer. Silent narrowing is exactly the
    // failure this function was fixed for; say it out loud instead.
    logger.warn(
      `[Locate] EPC is ${cleanEpc.length} hex chars (>128 bits); masking the leading 128 bits only. ` +
      'Locate may report a different tag sharing that prefix.'
    );
  }
  const maskBitLength = isExtended
    ? EPC_BIT_LENGTH.EXTENDED_128
    : EPC_BIT_LENGTH.STANDARD_96;

  // Convert to byte array
  const bytes: number[] = [];
  for (let i = 0; i < paddedEpc.length; i += 2) {
    bytes.push(parseInt(paddedEpc.substring(i, i + 2), 16));
  }

  // Build 32-bit values with reversed byte order
  // (compensates for createFirmwareCommand's little-endian conversion)
  const mask0_3 = ((bytes[3] << 24) | (bytes[2] << 16) | (bytes[1] << 8) | bytes[0]) >>> 0;
  const mask4_7 = ((bytes[7] << 24) | (bytes[6] << 16) | (bytes[5] << 8) | bytes[4]) >>> 0;
  const mask8_11 = ((bytes[11] << 24) | (bytes[10] << 16) | (bytes[9] << 8) | bytes[8]) >>> 0;
  const mask12_15 = isExtended
    ? ((bytes[15] << 24) | (bytes[14] << 16) | (bytes[13] << 8) | bytes[12]) >>> 0
    : undefined;

  // 7b. Set mask values (bytes 12-15) — 128-bit EPCs only. This is the tail
  // where most schemes put the serial, so without it tags off one reel share
  // a mask and Locate reports the wrong one.
  const extendedMaskCommands: SequenceCommand[] = isExtended
    ? [
        {
          event: RFID_FIRMWARE_COMMAND,
          payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
            register: RFID_REGISTERS.TAGMSK_12_15,
            value: mask12_15
          })
        }
      ]
    : [];

  return [
    // 0. Select the mask descriptor every write below applies to. Must come
    // first — it decides which of the 8 register sets they land on.
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL,
        value: TAGMSK_DESCRIPTOR_INDEX.LOCATE
      })
    },
    // 1. Configure mask descriptor (enable + target SL)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_DESC_CFG,
        value: TAGMASK_DESCRIPTOR.ENABLE | TAGMASK_DESCRIPTOR.TARGET_SL  // 0x09
      })
    },
    // 2. Select EPC bank
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_BANK,
        value: TAG_MEMORY_BANK.EPC  // 0x01
      })
    },
    // 3. Set starting bit position (after PC bits)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_PTR,
        value: EPC_MEMORY_OFFSET.AFTER_PC_BITS  // 0x20 (32 bits)
      })
    },
    // 4. Set mask length in bits
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_LEN,
        value: maskBitLength  // 0x60 (96 bits) or 0x80 (128 bits)
      })
    },
    // 5. Set mask values (bytes 0-3)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_0_3,
        value: mask0_3
      })
    },
    // 6. Set mask values (bytes 4-7)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_4_7,
        value: mask4_7
      })
    },
    // 7. Set mask values (bytes 8-11)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_8_11,
        value: mask8_11
      })
    },
    ...extendedMaskCommands,
    // 8. Enable locate mode with mask
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.INV_CFG,
        value: buildInvCfg({
          tag_delay: 30,  // 30ms delay (matches CS108 Library geiger mode)
          tag_sel: 1      // Enable tag select
        })
      })
    }
  ];
}