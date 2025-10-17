/**
 * CS108 RFID Firmware Command Builder
 *
 * Unified firmware command builder system that consolidates all CS108 RFID operations (0x8002)
 * into a single, flexible interface. Maintains byte-for-byte compatibility with proven
 * legacy implementation while providing modern type-safe interface.
 *
 * Critical: All register commands use LSB-first byte ordering per CS108 specification.
 */

import {
  RFID_REGISTERS,
  HST_CMD_VALUES
} from './constant.js';

/**
 * Command types supported by the firmware command builder
 */
export enum CommandType {
  WRITE_REGISTER = 'WRITE_REGISTER',
  READ_REGISTER = 'READ_REGISTER',
  START_INVENTORY = 'START_INVENTORY',
  ABORT = 'ABORT'
}

/**
 * Register command options
 */
export interface RegisterOptions {
  /** Register address (e.g., 0x0706 for ANT_PORT_POWER) */
  register: number;
  /** Value for write operations (default: 0) */
  value?: number;
}

/**
 * Union type for all command options
 */
export type CommandOptions = RegisterOptions | undefined;

/**
 * RAIN RFID settings interface
 */
export interface RfidSettings {
  /** Transmit power in dBm (10-30), will be multiplied by 10 for register */
  transmitPower?: number;
  /** Session selection for tag inventory */
  session?: 'S0' | 'S1' | 'S2' | 'S3';
  /** Inventory mode selection */
  inventoryMode?: 'compact' | 'normal';
  /** Algorithm selection for tag singulation */
  algorithm?: 'fixed' | 'dynamic';
}

/**
 * Register access types (from CS108 specification)
 */
export enum RegisterAccess {
  /** Command type for low-level register operations */
  LOW_LEVEL_API = 0x70,
  /** Read access from register */
  READ = 0x00,
  /** Write access to register */
  WRITE = 0x01
}

/**
 * ABORT command parameters
 * From CS108 specification Appendix A.8: "ABORT control command | Downlink | 40:03:00:00:00:00:00:00"
 */
const ABORT_COMMAND_PARAMS = {
  OPCODE: 0x40,
  LENGTH: 0x03
} as const;

/**
 * Creates a unified firmware command for CS108 RFID operations
 *
 * @param type - Type of command to create
 * @param options - Command-specific options
 * @returns Uint8Array containing the formatted command bytes
 * @throws Error if command type is unsupported or required options are missing
 *
 * @example
 * // Write register command
 * const cmd = createFirmwareCommand(CommandType.WRITE_REGISTER, {
 *   register: RFID_REGISTERS.ANT_PORT_POWER,
 *   value: 300  // 30 dBm × 10
 * });
 *
 * @example
 * // Start inventory command
 * const cmd = createFirmwareCommand(CommandType.START_INVENTORY);
 *
 * @example
 * // Abort command
 * const cmd = createFirmwareCommand(CommandType.ABORT);
 */
export function createFirmwareCommand(
  type: CommandType,
  options?: CommandOptions
): Uint8Array {
  switch (type) {
    case CommandType.WRITE_REGISTER:
    case CommandType.READ_REGISTER: {
      if (!options || !('register' in options)) {
        throw new Error(`${type} requires register options`);
      }

      const { register, value = 0 } = options;
      const payload = new Uint8Array(8);

      // Replicate legacy createRegisterCommand format exactly
      // From CS108 spec Appendix A.3: "REVERSELY POPULATED" - LSB first
      payload[0] = RegisterAccess.LOW_LEVEL_API;  // 0x70 - pkt_ver
      payload[1] = type === CommandType.WRITE_REGISTER ? RegisterAccess.WRITE : RegisterAccess.READ;

      // Register address - LSB first per specification
      payload[2] = register & 0xFF;         // Register LSB
      payload[3] = (register >> 8) & 0xFF;  // Register MSB

      // Value bytes - LSB first per specification
      payload[4] = value & 0xFF;            // Value byte 0 (LSB)
      payload[5] = (value >> 8) & 0xFF;     // Value byte 1
      payload[6] = (value >> 16) & 0xFF;    // Value byte 2
      payload[7] = (value >> 24) & 0xFF;    // Value byte 3 (MSB)

      return payload;
    }

    case CommandType.START_INVENTORY: {
      // Write 0x0F to HST_CMD register (0xF000) to start inventory
      return createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.HST_CMD,
        value: HST_CMD_VALUES.START_INVENTORY
      });
    }

    case CommandType.ABORT: {
      // Build the special ABORT control command sequence
      // From Appendix A.8: "ABORT control command | Downlink | 40:03:00:00:00:00:00:00"
      // This is NOT a register write - it's a special control command
      const payload = new Uint8Array(8);
      payload[0] = ABORT_COMMAND_PARAMS.OPCODE;  // 0x40
      payload[1] = ABORT_COMMAND_PARAMS.LENGTH;  // 0x03
      // Remaining bytes are 0x00 (already initialized)
      return payload;
    }

    default: {
      // TypeScript exhaustiveness check
      const _exhaustive: never = type;
      throw new Error(`Unsupported command type: ${_exhaustive}`);
    }
  }
}

/**
 * Session string to register value mapping
 */
const SESSION_VALUES = {
  'S0': 0x00,
  'S1': 0x01,
  'S2': 0x02,
  'S3': 0x03
} as const;

/**
 * Algorithm type to register value mapping
 */
const ALGORITHM_VALUES = {
  'fixed': 0x0000,      // Fixed Q algorithm
  'dynamic': 0x0003     // Dynamic Q algorithm with defaults
} as const;

/**
 * Inventory mode to INV_CFG register value mapping
 */
const INVENTORY_MODE_VALUES = {
  'compact': 0x04000000,  // Compact mode (bit 26 set)
  'normal': 0x00000000    // Normal mode (bit 26 clear)
} as const;

/**
 * Applies RAIN RFID settings by generating appropriate register commands
 *
 * @param settings - RFID settings to apply
 * @returns Array of Uint8Array commands to execute in sequence
 *
 * @example
 * const commands = applyRfidSettings({
 *   transmitPower: 25,      // 25 dBm
 *   session: 'S1',          // Session 1
 *   algorithm: 'dynamic',   // Dynamic Q
 *   inventoryMode: 'normal' // Normal mode
 * });
 *
 * // Execute commands in sequence
 * for (const cmd of commands) {
 *   await commandManager.executeCommand(RFID_FIRMWARE_COMMAND, cmd);
 * }
 */
export function applyRfidSettings(settings: RfidSettings): Uint8Array[] {
  const commands: Uint8Array[] = [];

  // Apply transmit power (scaled by 10 per CS108 specification)
  if (settings.transmitPower !== undefined) {
    commands.push(createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.ANT_PORT_POWER,
      value: Math.round(settings.transmitPower * 10)
    }));
  }

  // Apply session selection
  if (settings.session !== undefined) {
    commands.push(createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_SEL,
      value: SESSION_VALUES[settings.session]
    }));
  }

  // Apply algorithm selection
  if (settings.algorithm !== undefined) {
    commands.push(createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_ALG_PARM_0,
      value: ALGORITHM_VALUES[settings.algorithm]
    }));
  }

  // Apply inventory mode
  if (settings.inventoryMode !== undefined) {
    commands.push(createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.INV_CFG,
      value: INVENTORY_MODE_VALUES[settings.inventoryMode]
    }));
  }

  return commands;
}

