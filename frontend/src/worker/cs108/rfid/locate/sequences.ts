/**
 * Locate mode command sequences
 *
 * This module contains all command sequences related to LOCATE mode operation,
 * including the main LOCATE_SEQUENCE and dynamic EPC mask configuration.
 */

import type { CommandSequence } from '../../type.js';
import { createFirmwareCommand, CommandType } from '../firmware-command.js';
import {
  RFID_REGISTERS,
  INV_SEL_VALUES,
  TAG_MEMORY_BANK,
  TAGMASK_DESCRIPTOR,
  EPC_MEMORY_OFFSET,
  EPC_BIT_LENGTH,
  buildInvCfg,
  buildQueryCfg,
} from '../constant.js';
import { RFID_FIRMWARE_COMMAND, RFID_POWER_ON } from '../../event.js';

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
 * 1. Configure mask descriptor (enable + target SL)
 * 2. Select EPC memory bank
 * 3. Set starting bit position (after PC bits)
 * 4. Set mask length (96 bits)
 * 5. Set mask values (3 registers for 96-bit EPC)
 * 6. Enable search mode with mask
 *
 * @param targetEPC - Normalized EPC hex string (≤24 chars), or undefined to skip
 * @returns Command sequence to configure tag mask for locate mode
 */
export function locateSettingsSequence(targetEPC?: string): CommandSequence {
  if (targetEPC === undefined) {
    return [];
  }
  // Inline EPC to mask conversion (avoid dependency on epc-filter.ts)
  // Remove spaces and convert to uppercase
  const cleanEpc = targetEPC.replace(/\s/g, '').toUpperCase();

  // Pad to 24 hex chars (96 bits) with leading zeros
  const paddedEpc = cleanEpc.padStart(24, '0');

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

  return [
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
        value: EPC_BIT_LENGTH.STANDARD_96  // 0x60 (96 bits)
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