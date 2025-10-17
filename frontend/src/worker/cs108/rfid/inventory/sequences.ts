/**
 * CS108 RFID Inventory Command Sequences
 */

import type { CommandSequence } from '../../type.js';
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
  buildInvCfg
} from '../constant.js';
import { RFID_POWER_ON, RFID_FIRMWARE_COMMAND } from '../../event.js';

/**
 * INVENTORY Mode Sequence
 *
 * Powers up RFID module and configures for tag inventory
 * Based on production CS108 rfidManager.ts PREPARE_INVENTORY_COMMANDS
 */
export const INVENTORY_CONFIG_SEQUENCE: CommandSequence = [
  {
    event: RFID_POWER_ON,
    retryOnError: true  // Power commands may fail initially
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