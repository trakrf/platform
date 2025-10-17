/**
 * EPC Filtering Utilities
 *
 * Functions for converting EPC hex strings to CS108 tag mask register values
 * for hardware-level tag filtering in LOCATE mode.
 */

import { createFirmwareCommand, CommandType } from './firmware-command.js';
import {
  RFID_REGISTERS,
  TAG_MEMORY_BANK,
  TAGMSK_DESC_CFG_VAL
} from './constant.js';
import { RFID_FIRMWARE_COMMAND } from '../event.js';
import type { SequenceCommand } from '../type.js';

/**
 * Convert EPC hex string to mask values for CS108 registers
 *
 * The CS108 uses three 32-bit registers to store the EPC mask:
 * - TAGMSK_0_3: First 4 bytes (32 bits) of EPC
 * - TAGMSK_4_7: Next 4 bytes (32 bits) of EPC
 * - TAGMSK_8_11: Last 4 bytes (32 bits) of EPC
 *
 * IMPORTANT: The createFirmwareCommand function already handles little-endian
 * byte ordering, so we pass values as standard 32-bit integers.
 *
 * @param epc - EPC hex string (e.g., short decimal ID or full commercial EPC)
 * @returns Object with mask0_3, mask4_7, mask8_11 values
 */
export function hexEpcToMaskValues(epc: string): {
  mask0_3: number;
  mask4_7: number;
  mask8_11: number;
} {
  // Remove spaces and convert to uppercase
  const cleanEpc = epc.replace(/\s/g, '').toUpperCase();

  // Pad EPC to 24 hex chars (12 bytes) with leading zeros
  const paddedEpc = cleanEpc.padStart(24, '0');
  
  // Convert to byte array
  const bytes: number[] = [];
  for (let i = 0; i < paddedEpc.length; i += 2) {
    bytes.push(parseInt(paddedEpc.substring(i, i + 2), 16));
  }
  
  // The vendor documentation says tag masks use "natural byte ordering"
  // which means the bytes appear in the packet in the same order as the EPC string.
  // 
  // For example, a zero-padded EPC like "000000000000000000010020":
  // - Bytes are: [00, 00, 00, 00, 00, 00, 00, 00, 00, 01, 00, 20]
  // - TAGMSK_0_3 should contain bytes 0-3:   [00, 00, 00, 00]
  // - TAGMSK_4_7 should contain bytes 4-7:   [00, 00, 00, 00]
  // - TAGMSK_8_11 should contain bytes 8-11: [00, 01, 00, 20]
  //
  // Since createFirmwareCommand converts to little-endian, we need to
  // reverse the byte order: [3,2,1,0], [7,6,5,4], [11,10,9,8]
  // so that createFirmwareCommand's reversal produces natural order in the packet.
  
  // Build 32-bit values with reversed byte order to compensate for createFirmwareCommand
  const mask0_3 = ((bytes[3] << 24) | (bytes[2] << 16) | (bytes[1] << 8) | bytes[0]) >>> 0;
  const mask4_7 = ((bytes[7] << 24) | (bytes[6] << 16) | (bytes[5] << 8) | bytes[4]) >>> 0;
  const mask8_11 = ((bytes[11] << 24) | (bytes[10] << 16) | (bytes[9] << 8) | bytes[8]) >>> 0;

  return { mask0_3, mask4_7, mask8_11 };
}


/**
 * Create CS108 command sequence for EPC filtering in LOCATE mode
 *
 * This configures the CS108 hardware to filter tags at the RF level,
 * drastically reducing BLE traffic by only reporting the target EPC.
 *
 * @param epc - Target EPC hex string to filter for
 * @returns Array of CS108 firmware commands for EPC filtering
 * @deprecated Use hexEpcToMaskValues directly with createFirmwareCommand for better clarity
 */
export function createEpcFilterCommands(epc: string): SequenceCommand[] {
  // Use the existing hexEpcToMaskValues function which has the correct byte ordering
  const { mask0_3, mask4_7, mask8_11 } = hexEpcToMaskValues(epc);
  
  // Clean EPC for logging
  const cleanEpc = epc.replace(/\s/g, '').toUpperCase().padStart(24, '0');
  
  console.log(`[EPC Filter] Configuring tag mask for EPC: ${cleanEpc}`);
  console.log(`[EPC Filter] Mask values: 0_3=0x${mask0_3.toString(16).padStart(8, '0')}, 4_7=0x${mask4_7.toString(16).padStart(8, '0')}, 8_11=0x${mask8_11.toString(16).padStart(8, '0')}`);

  // The vendor doc shows TAGMSK_PTR = 0x20 (32 decimal) 
  // This skips the CRC (16 bits) and PC (16 bits) to start at the actual EPC data
  const epcMemoryBankStart = 32;  // Start at bit 32 where EPC data begins
  const epcBitLength = 96;        // Always use full 96-bit mask

  // Return the mask configuration commands
  return [
    // Configure tag selection mask - THE ORDER OF THESE COMMANDS IS CRITICAL
    // 0. FIRST select which mask descriptor to use (0-7, we use 0)
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL,
        value: 0  // Use mask slot 0
      })
    },
    // 1. Then set the mask description and bank
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_DESC_CFG,
        value: TAGMSK_DESC_CFG_VAL.DEFAULT
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_BANK,
        value: TAG_MEMORY_BANK.EPC
      })
    },

    // 2. Then set the mask pointer and length
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_PTR,
        value: epcMemoryBankStart
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_LEN,
        value: epcBitLength
      })
    },

    // 3. Then set the actual mask values using hexEpcToMaskValues results
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_0_3,
        value: mask0_3
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_4_7,
        value: mask4_7
      })
    },
    {
      event: RFID_FIRMWARE_COMMAND,
      payload: createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.TAGMSK_8_11,
        value: mask8_11
      })
    }
  ];
}