/**
 * Unit tests for CS108 RFID Firmware Command Builder
 *
 * Tests validate byte-for-byte compatibility with legacy implementation
 * and ensure correct CS108 protocol formatting.
 */

import { describe, it, expect } from 'vitest';
import {
  createFirmwareCommand,
  applyRfidSettings,
  CommandType,
  type RegisterOptions,
  type RfidSettings
} from './firmware-command.js';
import { RFID_REGISTERS } from './constant.js';

describe('createFirmwareCommand', () => {
  describe('register operations', () => {
    it('creates register write command matching legacy format', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.ANT_PORT_POWER,  // 0x0706
        value: 300  // 30 dBm × 10
      });

      // Validate exact legacy format
      expect(command).toBeInstanceOf(Uint8Array);
      expect(command).toHaveLength(8);
      expect(command[0]).toBe(0x70);  // LOW_LEVEL_API
      expect(command[1]).toBe(0x01);  // WRITE access (0x01)
      expect(command[2]).toBe(0x06);  // Register LSB (0x0706 & 0xFF)
      expect(command[3]).toBe(0x07);  // Register MSB ((0x0706 >> 8) & 0xFF)
      expect(command[4]).toBe(0x2C);  // Value byte 0 (300 & 0xFF = 0x2C)
      expect(command[5]).toBe(0x01);  // Value byte 1 ((300 >> 8) & 0xFF = 0x01)
      expect(command[6]).toBe(0x00);  // Value byte 2
      expect(command[7]).toBe(0x00);  // Value byte 3
    });

    it('creates register read command with correct format', () => {
      const command = createFirmwareCommand(CommandType.READ_REGISTER, {
        register: RFID_REGISTERS.HST_CMD  // 0xF000
      });

      expect(command).toHaveLength(8);
      expect(command[0]).toBe(0x70);  // LOW_LEVEL_API
      expect(command[1]).toBe(0x00);  // READ access (0x00)
      expect(command[2]).toBe(0x00);  // Register LSB (0xF000 & 0xFF)
      expect(command[3]).toBe(0xF0);  // Register MSB ((0xF000 >> 8) & 0xFF)
      expect(command[4]).toBe(0x00);  // No value for reads
      expect(command[5]).toBe(0x00);
      expect(command[6]).toBe(0x00);
      expect(command[7]).toBe(0x00);
    });

    it('handles large register values correctly', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: 0xF000,
        value: 0x12345678
      });

      // Verify LSB-first ordering for both register and value
      expect(command[2]).toBe(0x00);  // Register LSB
      expect(command[3]).toBe(0xF0);  // Register MSB
      expect(command[4]).toBe(0x78);  // Value byte 0 (LSB)
      expect(command[5]).toBe(0x56);  // Value byte 1
      expect(command[6]).toBe(0x34);  // Value byte 2
      expect(command[7]).toBe(0x12);  // Value byte 3 (MSB)
    });

    it('defaults value to 0 for write operations when not specified', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.INV_SEL
      });

      expect(command[4]).toBe(0x00);  // Value byte 0
      expect(command[5]).toBe(0x00);  // Value byte 1
      expect(command[6]).toBe(0x00);  // Value byte 2
      expect(command[7]).toBe(0x00);  // Value byte 3
    });

    it('handles register addresses from constants correctly', () => {
      // Test various register addresses from constants
      const testCases = [
        { register: RFID_REGISTERS.ANT_CYCLES, expected: [0x00, 0x07] },
        { register: RFID_REGISTERS.INV_CFG, expected: [0x01, 0x09] },
        { register: RFID_REGISTERS.CURRENT_PROFILE, expected: [0x60, 0x0B] },
        { register: RFID_REGISTERS.HST_CMD, expected: [0x00, 0xF0] }
      ];

      for (const { register, expected } of testCases) {
        const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
          register,
          value: 0
        });

        expect(command[2]).toBe(expected[0]);  // Register LSB
        expect(command[3]).toBe(expected[1]);  // Register MSB
      }
    });
  });

  describe('special commands', () => {
    it('creates START_INVENTORY command (HST_CMD write)', () => {
      const command = createFirmwareCommand(CommandType.START_INVENTORY);

      // Should be HST_CMD register write with value 0x0F
      expect(command).toHaveLength(8);
      expect(command[0]).toBe(0x70);  // LOW_LEVEL_API
      expect(command[1]).toBe(0x01);  // WRITE access (0x01)
      expect(command[2]).toBe(0x00);  // HST_CMD LSB (0xF000 & 0xFF)
      expect(command[3]).toBe(0xF0);  // HST_CMD MSB ((0xF000 >> 8) & 0xFF)
      expect(command[4]).toBe(0x0F);  // START_INVENTORY value (CMD_18K6CINV)
      expect(command[5]).toBe(0x00);
      expect(command[6]).toBe(0x00);
      expect(command[7]).toBe(0x00);
    });

    it('creates ABORT command with exact sequence from specification', () => {
      const command = createFirmwareCommand(CommandType.ABORT);

      // From CS108 spec Appendix A.8: "40:03:00:00:00:00:00:00"
      const expected = new Uint8Array([0x40, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
      expect(command).toEqual(expected);
    });

    it('ABORT command returns new array instance each time', () => {
      const command1 = createFirmwareCommand(CommandType.ABORT);
      const command2 = createFirmwareCommand(CommandType.ABORT);

      // Should be equal but not the same instance
      expect(command1).toEqual(command2);
      expect(command1).not.toBe(command2);
    });
  });

  describe('error handling', () => {
    it('throws error for invalid command type', () => {
      expect(() => {
        createFirmwareCommand('INVALID_COMMAND' as CommandType);
      }).toThrow('Unsupported command type: INVALID_COMMAND');
    });

    it('throws error for missing register options on WRITE_REGISTER', () => {
      expect(() => {
        createFirmwareCommand(CommandType.WRITE_REGISTER);
      }).toThrow('WRITE_REGISTER requires register options');
    });

    it('throws error for missing register options on READ_REGISTER', () => {
      expect(() => {
        createFirmwareCommand(CommandType.READ_REGISTER);
      }).toThrow('READ_REGISTER requires register options');
    });

    it('throws error for undefined register in options', () => {
      expect(() => {
        createFirmwareCommand(CommandType.WRITE_REGISTER, {} as RegisterOptions);
      }).toThrow('WRITE_REGISTER requires register options');
    });
  });

  describe('edge cases', () => {
    it('handles zero register address correctly', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: 0x0000,
        value: 42
      });

      expect(command[2]).toBe(0x00);  // Register LSB
      expect(command[3]).toBe(0x00);  // Register MSB
      expect(command[4]).toBe(0x2A);  // Value (42 = 0x2A)
    });

    it('handles maximum 32-bit value correctly', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.INV_CFG,
        value: 0xFFFFFFFF
      });

      expect(command[4]).toBe(0xFF);  // Value byte 0 (LSB)
      expect(command[5]).toBe(0xFF);  // Value byte 1
      expect(command[6]).toBe(0xFF);  // Value byte 2
      expect(command[7]).toBe(0xFF);  // Value byte 3 (MSB)
    });

    it('handles negative values by treating as unsigned', () => {
      const command = createFirmwareCommand(CommandType.WRITE_REGISTER, {
        register: RFID_REGISTERS.INV_CFG,
        value: -1
      });

      // -1 should be treated as 0xFFFFFFFF in unsigned 32-bit
      expect(command[4]).toBe(0xFF);  // Value byte 0 (LSB)
      expect(command[5]).toBe(0xFF);  // Value byte 1
      expect(command[6]).toBe(0xFF);  // Value byte 2
      expect(command[7]).toBe(0xFF);  // Value byte 3 (MSB)
    });
  });
});

describe('applyRfidSettings', () => {
  it('converts power settings correctly', () => {
    const commands = applyRfidSettings({ transmitPower: 25 });

    expect(commands).toHaveLength(1);
    const powerCommand = commands[0];

    // Should write 250 (25 × 10) to ANT_PORT_POWER
    expect(powerCommand).toHaveLength(8);
    expect(powerCommand[0]).toBe(0x70);  // LOW_LEVEL_API
    expect(powerCommand[1]).toBe(0x01);  // WRITE access (0x01)
    expect(powerCommand[2]).toBe(0x06);  // ANT_PORT_POWER LSB (0x0706 & 0xFF)
    expect(powerCommand[3]).toBe(0x07);  // ANT_PORT_POWER MSB
    expect(powerCommand[4]).toBe(0xFA);  // 250 & 0xFF = 0xFA
    expect(powerCommand[5]).toBe(0x00);  // (250 >> 8) & 0xFF
    expect(powerCommand[6]).toBe(0x00);
    expect(powerCommand[7]).toBe(0x00);
  });

  it('converts session settings correctly', () => {
    const testCases: Array<{ session: RfidSettings['session']; expectedValue: number }> = [
      { session: 'S0', expectedValue: 0x00 },
      { session: 'S1', expectedValue: 0x01 },
      { session: 'S2', expectedValue: 0x02 },
      { session: 'S3', expectedValue: 0x03 }
    ];

    for (const { session, expectedValue } of testCases) {
      const commands = applyRfidSettings({ session });

      expect(commands).toHaveLength(1);
      const sessionCommand = commands[0];

      expect(sessionCommand[2]).toBe(0x02);  // INV_SEL LSB (0x0902 & 0xFF)
      expect(sessionCommand[3]).toBe(0x09);  // INV_SEL MSB
      expect(sessionCommand[4]).toBe(expectedValue);
    }
  });

  it('converts algorithm settings correctly', () => {
    // Test fixed algorithm
    let commands = applyRfidSettings({ algorithm: 'fixed' });
    expect(commands).toHaveLength(1);
    let algCommand = commands[0];
    expect(algCommand[2]).toBe(0x03);  // INV_ALG_PARM_0 LSB (0x0903 & 0xFF)
    expect(algCommand[3]).toBe(0x09);  // INV_ALG_PARM_0 MSB
    expect(algCommand[4]).toBe(0x00);  // Fixed Q = 0x0000
    expect(algCommand[5]).toBe(0x00);

    // Test dynamic algorithm
    commands = applyRfidSettings({ algorithm: 'dynamic' });
    expect(commands).toHaveLength(1);
    algCommand = commands[0];
    expect(algCommand[4]).toBe(0x03);  // Dynamic Q = 0x0003
    expect(algCommand[5]).toBe(0x00);
  });

  it('converts inventory mode settings correctly', () => {
    // Test normal mode
    let commands = applyRfidSettings({ inventoryMode: 'normal' });
    expect(commands).toHaveLength(1);
    let modeCommand = commands[0];
    expect(modeCommand[2]).toBe(0x01);  // INV_CFG LSB (0x0901 & 0xFF)
    expect(modeCommand[3]).toBe(0x09);  // INV_CFG MSB
    expect(modeCommand[4]).toBe(0x00);  // Normal mode = 0x00000000
    expect(modeCommand[5]).toBe(0x00);
    expect(modeCommand[6]).toBe(0x00);
    expect(modeCommand[7]).toBe(0x00);

    // Test compact mode (bit 26 set)
    commands = applyRfidSettings({ inventoryMode: 'compact' });
    expect(commands).toHaveLength(1);
    modeCommand = commands[0];
    expect(modeCommand[4]).toBe(0x00);  // 0x04000000 LSB
    expect(modeCommand[5]).toBe(0x00);
    expect(modeCommand[6]).toBe(0x00);
    expect(modeCommand[7]).toBe(0x04);  // 0x04000000 MSB (bit 26 set)
  });

  it('applies multiple settings in correct order', () => {
    const commands = applyRfidSettings({
      transmitPower: 30,
      session: 'S2',
      algorithm: 'dynamic',
      inventoryMode: 'normal'
    });

    expect(commands).toHaveLength(4);

    // Check power command (first)
    expect(commands[0][2]).toBe(0x06);  // ANT_PORT_POWER LSB
    expect(commands[0][3]).toBe(0x07);  // ANT_PORT_POWER MSB
    expect(commands[0][4]).toBe(0x2C);  // 300 & 0xFF (30 × 10)
    expect(commands[0][5]).toBe(0x01);  // (300 >> 8) & 0xFF

    // Check session command (second)
    expect(commands[1][2]).toBe(0x02);  // INV_SEL LSB
    expect(commands[1][3]).toBe(0x09);  // INV_SEL MSB
    expect(commands[1][4]).toBe(0x02);  // S2 value

    // Check algorithm command (third)
    expect(commands[2][2]).toBe(0x03);  // INV_ALG_PARM_0 LSB
    expect(commands[2][3]).toBe(0x09);  // INV_ALG_PARM_0 MSB
    expect(commands[2][4]).toBe(0x03);  // Dynamic Q value

    // Check inventory mode command (fourth)
    expect(commands[3][2]).toBe(0x01);  // INV_CFG LSB
    expect(commands[3][3]).toBe(0x09);  // INV_CFG MSB
    expect(commands[3][7]).toBe(0x00);  // Normal mode
  });

  it('returns empty array when no settings provided', () => {
    const commands = applyRfidSettings({});
    expect(commands).toEqual([]);
  });

  it('only includes commands for provided settings', () => {
    const commands = applyRfidSettings({
      transmitPower: 20,
      algorithm: 'fixed'
      // session and inventoryMode not provided
    });

    expect(commands).toHaveLength(2);

    // First command should be power
    expect(commands[0][2]).toBe(0x06);  // ANT_PORT_POWER LSB
    expect(commands[0][4]).toBe(0xC8);  // 200 & 0xFF (20 × 10)

    // Second command should be algorithm
    expect(commands[1][2]).toBe(0x03);  // INV_ALG_PARM_0 LSB
    expect(commands[1][4]).toBe(0x00);  // Fixed Q value
  });

  it('handles edge case power values', () => {
    // Minimum power
    let commands = applyRfidSettings({ transmitPower: 10 });
    expect(commands[0][4]).toBe(0x64);  // 100 & 0xFF (10 × 10)
    expect(commands[0][5]).toBe(0x00);

    // Maximum power
    commands = applyRfidSettings({ transmitPower: 30 });
    expect(commands[0][4]).toBe(0x2C);  // 300 & 0xFF (30 × 10)
    expect(commands[0][5]).toBe(0x01);

    // Fractional power (should round)
    commands = applyRfidSettings({ transmitPower: 25.7 });
    expect(commands[0][4]).toBe(0x01);  // 257 & 0xFF (25.7 × 10 rounded)
    expect(commands[0][5]).toBe(0x01);  // (257 >> 8) & 0xFF
  });
});

describe('integration with RFID_FIRMWARE_COMMAND event', () => {
  it('generates payloads compatible with event payload length', () => {
    // All generated commands should be 8 bytes for register operations
    const writeCmd = createFirmwareCommand(CommandType.WRITE_REGISTER, {
      register: RFID_REGISTERS.ANT_PORT_POWER,
      value: 300
    });
    expect(writeCmd).toHaveLength(8);

    const readCmd = createFirmwareCommand(CommandType.READ_REGISTER, {
      register: RFID_REGISTERS.HST_CMD
    });
    expect(readCmd).toHaveLength(8);

    const startCmd = createFirmwareCommand(CommandType.START_INVENTORY);
    expect(startCmd).toHaveLength(8);

    const abortCmd = createFirmwareCommand(CommandType.ABORT);
    expect(abortCmd).toHaveLength(8);
  });

  it('generates commands that can be executed in sequence', () => {
    // Simulate a typical inventory preparation sequence
    const settings: RfidSettings = {
      transmitPower: 25,
      session: 'S1',
      algorithm: 'dynamic',
      inventoryMode: 'normal'
    };

    const settingCommands = applyRfidSettings(settings);
    const startCommand = createFirmwareCommand(CommandType.START_INVENTORY);

    // All commands should be valid Uint8Arrays of length 8
    for (const cmd of settingCommands) {
      expect(cmd).toBeInstanceOf(Uint8Array);
      expect(cmd).toHaveLength(8);
    }

    expect(startCommand).toBeInstanceOf(Uint8Array);
    expect(startCommand).toHaveLength(8);
  });
});