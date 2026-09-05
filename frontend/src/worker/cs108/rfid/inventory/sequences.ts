/**
 * CS108 RFID Inventory Command Sequences
 */

import type { CommandSequence } from '../../type.js';
import type { ReaderSettings } from '../../../types/reader.js';
import { createFirmwareCommand, CommandType } from '../firmware-command.js';
import {
  RFID_REGISTERS,
  HST_CMD_VALUES,
  LINK_PROFILE,
  RSSI_THRESHOLD,
  INV_SEL_VALUES,
  ALG_PARM_VALUES,
  TAG_MEMORY_BANK,
  REG_DEFAULT,
  buildInvCfg,
  buildTagaccBank,
  buildTagaccPtr,
  buildTagaccCnt
} from '../constant.js';
import { RFID_POWER_ON, RFID_FIRMWARE_COMMAND } from '../../event.js';

/**
 * Defaults for the capture settings, applied when the flag is on but a field
 * was never set.
 *
 * 6 words of TID covers an extended TID carrying a 48-bit serial. Some chips
 * only carry 2, which is exactly why this is a setting and not a constant —
 * an over-long read is refused by the tag, and the operator needs to be able
 * to shorten it without a code change.
 */
const CAPTURE_DEFAULTS = {
  tidWords: 6,
  userOffset: 0,
  userWords: 4
} as const;

/**
 * INVENTORY Mode Sequence
 *
 * Powers up RFID module and configures for tag inventory
 * Based on production CS108 rfidManager.ts PREPARE_INVENTORY_COMMANDS
 */
export const INVENTORY_CONFIG_SEQUENCE: CommandSequence = [
  {
    event: RFID_POWER_ON,
    retryDelays: [100]  // Power commands may fail initially
  },
  // Set antenna power to 30dBm (default, can be overridden by setSettings)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.ANT_PORT_POWER,
      value: 300  // 30dBm * 10
    })
  },
  // Set Dynamic Q algorithm (default, can be overridden by setSettings)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_SEL,
      value: INV_SEL_VALUES.DYNAMIC_Q
    })
  },
  // Set Dynamic Q parameters
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_ALG_PARM_0,
      value: ALG_PARM_VALUES.DYNAMIC_Q_DEFAULT
    })
  },
  // Clear QUERY_CFG
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.QUERY_CFG,
      value: REG_DEFAULT.QUERY_DEFAULT
    })
  },
  // Set current profile to 1 (best range in dense reader mode)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.CURRENT_PROFILE,
      value: LINK_PROFILE.PROFILE_1
    })
  },
  // MAC Bypass Write
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.HST_CMD,
      value: HST_CMD_VALUES.MAC_BYPASS_WRITE
    })
  },
  // Set RSSI filtering threshold
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.RSSI_FILTERING_THRESHOLD,
      value: RSSI_THRESHOLD.DEFAULT
    })
  },
  // Configure INV_CFG with compact mode enabled (from production capture)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_CFG,
      value: buildInvCfg({
        inv_algo: 3,     // Algorithm from production capture (0x04040003)
        tag_delay: 20,    // 20ms delay between tag reads (increased for capacity)
        inv_mode: 1       // Enable compact mode
      })
    })
  },
  // Set Tag Access Bank to RESERVED (from PREPARE_INVENTORY_COMMANDS)
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.TAGACC_BANK,
      value: TAG_MEMORY_BANK.RESERVED
    })
  },
  // Clear Tag Access Pointer
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.TAGACC_PTR,
      value: REG_DEFAULT.ZERO
    })
  },
  // Clear Tag Access Count
  {
    event: RFID_FIRMWARE_COMMAND,
    payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.TAGACC_CNT,
      value: REG_DEFAULT.ZERO
    })
  },

  // Note: START_INVENTORY (0xF000 = 0x0F) is called separately when scanning starts
];

/**
 * Switch inventory into a tag-data capture read (TRA-1251).
 *
 * ## Why this is a mode change, not an extra register
 *
 * Compact mode's response payload is documented as PC + EPC + NB_RSSI. There is
 * no field in it for bank data — none, at any setting. Bank data rides only the
 * NORMAL mode inventory response, where inv_data becomes
 * `PC + EPC + DATA1 [+ DATA2] + CRC16`. So capturing TID or USER means leaving
 * compact mode, which costs throughput: the vendor puts tag_delay at 30 for
 * Bluetooth normal mode against 0-7 for compact.
 *
 * That is why this is opt-in rather than always-on.
 *
 * ## Ordering
 *
 * Returns an EMPTY sequence when capture is off, and is spliced in AFTER
 * `INVENTORY_CONFIG_SEQUENCE` — which writes these same four registers to their
 * no-capture values. Later writes win, so the disabled path stays byte-for-byte
 * identical to what shipped before this existed, without a branch in the shared
 * sequence.
 *
 * ## The userWords: 0 path
 *
 * Reading two banks fails as a unit on a chip that has no USER bank, and the
 * operator may be standing in front of the only reader that matters when they
 * find that out. Setting userWords to 0 drops to a single-bank TID read, with
 * acc_bank2, ptr2 and length2 all zeroed as the spec requires when tag_read is
 * not 2. One field changes instead of a code change.
 */
export function tagCaptureSequence(
  rfid?: ReaderSettings['rfid']
): CommandSequence {
  if (!rfid?.captureAllTagData) {
    return [];
  }

  const tidWords = rfid.tidWords ?? CAPTURE_DEFAULTS.tidWords;
  const userOffset = rfid.userOffset ?? CAPTURE_DEFAULTS.userOffset;
  const userWords = rfid.userWords ?? CAPTURE_DEFAULTS.userWords;

  // Asking for zero words of a bank is not a read of that bank — the spec is
  // explicit that zero is not a "whole bank" shorthand.
  const readsUserBank = userWords > 0;

  return [
    // Normal mode, reading one or two banks after each inventory round
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.INV_CFG,
        value: buildInvCfg({
          inv_algo: 3,                        // matches INVENTORY_CONFIG_SEQUENCE
          tag_delay: 30,                      // vendor guidance for BT normal mode
          inv_mode: 0,                        // normal mode — compact carries no bank data
          tag_read: readsUserBank ? 2 : 1
        })
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGACC_BANK,
        value: buildTagaccBank({
          bank: TAG_MEMORY_BANK.TID,
          bank2: readsUserBank ? TAG_MEMORY_BANK.USER : TAG_MEMORY_BANK.RESERVED
        })
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGACC_PTR,
        value: buildTagaccPtr({
          ptr: 0,                             // TID is read from word 0
          ptr2: readsUserBank ? userOffset : 0
        })
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGACC_CNT,
        value: buildTagaccCnt({
          length: tidWords,
          length2: readsUserBank ? userWords : 0
        })
      })
    }
  ];
}