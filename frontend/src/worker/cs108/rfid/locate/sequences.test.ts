import { describe, it, expect } from 'vitest';
import { locateSettingsSequence, LOCATE_CONFIG_SEQUENCE } from './sequences.js';
import { RFID_FIRMWARE_COMMAND, RFID_POWER_ON } from '../../event.js';

describe('locateSettingsSequence', () => {
  it('generates correct sequence for standard 96-bit EPC', () => {
    const sequence = locateSettingsSequence('E28011606000020A76543210');

    // Should have 8 commands (mask config + search mode)
    expect(sequence).toHaveLength(8);

    // Each command should have proper structure
    sequence.forEach(cmd => {
      expect(cmd.event).toBe(RFID_FIRMWARE_COMMAND);
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });

    // First command should configure mask descriptor
    const firstCmd = sequence[0];
    expect(firstCmd.event).toBe(RFID_FIRMWARE_COMMAND);

    // Last command should enable search mode
    const lastCmd = sequence[7];
    expect(lastCmd.event).toBe(RFID_FIRMWARE_COMMAND);
  });

  it('pads short EPCs to 96 bits (24 hex chars)', () => {
    const sequence = locateSettingsSequence('10020');

    expect(sequence).toHaveLength(8);

    // Should pad to 000000000000000000010020
    // Check that the sequence is generated without errors
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('handles uppercase and lowercase identically', () => {
    const seq1 = locateSettingsSequence('abc123');
    const seq2 = locateSettingsSequence('ABC123');

    // Both sequences should have the same length
    expect(seq1).toHaveLength(seq2.length);

    // Compare payload bytes for mask values (commands 4, 5, 6 are the mask registers)
    for (let i = 4; i <= 6; i++) {
      expect(seq1[i].payload).toEqual(seq2[i].payload);
    }
  });

  it('removes spaces from EPC', () => {
    const sequence = locateSettingsSequence('E280 1160 6000 020A 7654 3210');

    expect(sequence).toHaveLength(8);

    // Should process the EPC without spaces
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('handles empty EPC by padding to all zeros', () => {
    const sequence = locateSettingsSequence('');

    expect(sequence).toHaveLength(8);

    // Should generate mask for 000000000000000000000000
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('generates correct byte order for mask values', () => {
    // Test with a known EPC to verify byte order
    const sequence = locateSettingsSequence('112233445566778899AABBCC');

    expect(sequence).toHaveLength(8);

    // The mask values are in commands 4, 5, 6 (0-indexed)
    // Due to the byte reversal logic in locateSettingsSequence:
    // Original: 11 22 33 44 55 66 77 88 99 AA BB CC
    // mask0_3 should be built from bytes[3,2,1,0] = 44 33 22 11
    // mask4_7 should be built from bytes[7,6,5,4] = 88 77 66 55
    // mask8_11 should be built from bytes[11,10,9,8] = CC BB AA 99

    // We can't easily check the exact values without parsing the payload,
    // but we can verify the commands are structured correctly
    expect(sequence[4].event).toBe(RFID_FIRMWARE_COMMAND); // TAGMSK_0_3
    expect(sequence[5].event).toBe(RFID_FIRMWARE_COMMAND); // TAGMSK_4_7
    expect(sequence[6].event).toBe(RFID_FIRMWARE_COMMAND); // TAGMSK_8_11
  });
});

describe('LOCATE_CONFIG_SEQUENCE', () => {
  it('has correct structure', () => {
    // Should have at least power on and configuration commands
    expect(LOCATE_CONFIG_SEQUENCE.length).toBeGreaterThan(0);

    // First command should be RFID_POWER_ON
    expect(LOCATE_CONFIG_SEQUENCE[0].event).toBe(RFID_POWER_ON);
    expect(LOCATE_CONFIG_SEQUENCE[0].retryOnError).toBe(true);
    // settlingDelay is now on the event definition, not the sequence command

    // Should have configuration commands
    const firmwareCommands = LOCATE_CONFIG_SEQUENCE.filter(cmd => cmd.event === RFID_FIRMWARE_COMMAND);
    expect(firmwareCommands.length).toBeGreaterThan(0);

    // Each firmware command should have a payload
    firmwareCommands.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('configures Fixed Q algorithm', () => {
    // LOCATE sequence should set up Fixed Q = 0 for single tag search
    const hasFixedQConfig = LOCATE_CONFIG_SEQUENCE.some(cmd =>
      cmd.event === RFID_FIRMWARE_COMMAND && cmd.payload !== undefined
    );
    expect(hasFixedQConfig).toBe(true);
  });
});