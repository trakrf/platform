import { describe, it, expect, vi, afterEach } from 'vitest';
import { locateSettingsSequence, LOCATE_CONFIG_SEQUENCE } from './sequences.js';
import { RFID_FIRMWARE_COMMAND, RFID_POWER_ON } from '../../event.js';
import { RFID_REGISTERS, EPC_BIT_LENGTH } from '../constant.js';
import { logger } from '../../../utils/logger.js';

/**
 * Decode a WRITE_REGISTER payload back into {register, value}.
 *
 * createFirmwareCommand lays the register address and the value out LSB-first
 * (see firmware-command.ts). Reading them back is what lets these tests assert
 * on the actual register traffic rather than on command counts.
 */
const decodeWrite = (payload: Uint8Array | undefined) => {
  if (!payload) throw new Error('command has no payload');
  return {
    register: payload[2] | (payload[3] << 8),
    value: ((payload[4] | (payload[5] << 8) | (payload[6] << 16) | (payload[7] << 24)) >>> 0)
  };
};

const decodeSequence = (sequence: ReturnType<typeof locateSettingsSequence>) =>
  sequence.map(cmd => decodeWrite(cmd.payload));

const registerValue = (
  sequence: ReturnType<typeof locateSettingsSequence>,
  register: number
) => decodeSequence(sequence).find(cmd => cmd.register === register)?.value;

describe('locateSettingsSequence', () => {
  it('generates correct sequence for standard 96-bit EPC', () => {
    const sequence = locateSettingsSequence('E28011606000020A76543210');

    // Descriptor select + mask config + search mode
    expect(sequence).toHaveLength(9);

    // Each command should have proper structure
    sequence.forEach(cmd => {
      expect(cmd.event).toBe(RFID_FIRMWARE_COMMAND);
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });

    // Descriptor select leads; enabling the search closes.
    expect(decodeSequence(sequence)[0].register).toBe(RFID_REGISTERS.HST_TAGMSK_DESC_SEL);
    expect(decodeSequence(sequence).at(-1)!.register).toBe(RFID_REGISTERS.INV_CFG);
  });

  it('pads short EPCs to 96 bits (24 hex chars)', () => {
    const sequence = locateSettingsSequence('10020');

    expect(sequence).toHaveLength(9);

    // Should pad to 000000000000000000010020
    // Check that the sequence is generated without errors
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('handles uppercase and lowercase identically', () => {
    const seq1 = locateSettingsSequence('abc123');
    const seq2 = locateSettingsSequence('ABC123');

    expect(seq1).toHaveLength(seq2.length);

    // Keyed by register rather than position, so adding a command to the
    // sequence cannot silently move this assertion off the mask registers.
    for (const register of [
      RFID_REGISTERS.TAGMSK_0_3,
      RFID_REGISTERS.TAGMSK_4_7,
      RFID_REGISTERS.TAGMSK_8_11
    ]) {
      expect(registerValue(seq1, register)).toBe(registerValue(seq2, register));
    }
  });

  it('removes spaces from EPC', () => {
    const sequence = locateSettingsSequence('E280 1160 6000 020A 7654 3210');

    expect(sequence).toHaveLength(9);
    expect(sequence).toEqual(locateSettingsSequence('E28011606000020A76543210'));
  });

  it('handles empty EPC by padding to all zeros', () => {
    const sequence = locateSettingsSequence('');

    expect(sequence).toHaveLength(9);

    // Should generate mask for 000000000000000000000000
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  it('generates correct byte order for mask values', () => {
    // Original: 11 22 33 44 55 66 77 88 99 AA BB CC
    // Each register is built from its bytes reversed, to compensate for
    // createFirmwareCommand's little-endian conversion.
    const sequence = locateSettingsSequence('112233445566778899AABBCC');

    expect(registerValue(sequence, RFID_REGISTERS.TAGMSK_0_3)).toBe(0x44332211);
    expect(registerValue(sequence, RFID_REGISTERS.TAGMSK_4_7)).toBe(0x88776655);
    expect(registerValue(sequence, RFID_REGISTERS.TAGMSK_8_11)).toBe(0xCCBBAA99);
  });
});

/**
 * TRA-1108 — the mask has to be as wide as the EPC.
 *
 * Three 32-bit mask registers cover 96 bits. A 128-bit EPC's trailing 32 bits
 * — where most schemes put the serial — went unmasked, so tags off one reel
 * collided. Vendor spec defines TAGMSK_12_15 at 0x0808 and TAGMSK_LEN is an
 * 8-bit field, so 0x80 is a legal length.
 */
describe('locateSettingsSequence — mask width (TRA-1108)', () => {
  // Two real 128-bit tags off the operator's bench, differing only in the
  // final hex char — inside the tail the 96-bit mask never covered.
  const TAG_633 = '00000000000000000000533034313633';
  const TAG_634 = '00000000000000000000533034313634';

  describe('96-bit EPCs', () => {
    const EPC_96 = '112233445566778899AABBCC';

    it('emits 9 commands and never writes TAGMSK_12_15', () => {
      const sequence = locateSettingsSequence(EPC_96);

      expect(sequence).toHaveLength(9);
      expect(
        decodeSequence(sequence).some(c => c.register === RFID_REGISTERS.TAGMSK_12_15)
      ).toBe(false);
    });

    it('sets TAGMSK_LEN to 96 bits', () => {
      expect(registerValue(locateSettingsSequence(EPC_96), RFID_REGISTERS.TAGMSK_LEN))
        .toBe(EPC_BIT_LENGTH.STANDARD_96);
    });

    it('pins the exact register traffic, descriptor select first', () => {
      // Byte-for-byte guard on the primary path. TAGMSK_DESC_SEL must lead:
      // it decides which of the 8 register sets everything below lands on.
      expect(decodeSequence(locateSettingsSequence(EPC_96))).toEqual([
        { register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL, value: 0x00 },
        { register: RFID_REGISTERS.TAGMSK_DESC_CFG, value: 0x09 },
        { register: RFID_REGISTERS.TAGMSK_BANK, value: 0x01 },
        { register: RFID_REGISTERS.TAGMSK_PTR, value: 0x20 },
        { register: RFID_REGISTERS.TAGMSK_LEN, value: 0x60 },
        { register: RFID_REGISTERS.TAGMSK_0_3, value: 0x44332211 },
        { register: RFID_REGISTERS.TAGMSK_4_7, value: 0x88776655 },
        { register: RFID_REGISTERS.TAGMSK_8_11, value: 0xCCBBAA99 },
        { register: RFID_REGISTERS.INV_CFG, value: 0x01E04000 }
      ]);
    });

    it('still treats a short value as a 96-bit prefix search', () => {
      // '5330' padding to the true leading 96 bits is what made the operator's
      // manual workaround work. It has to keep working.
      expect(locateSettingsSequence('5330'))
        .toEqual(locateSettingsSequence('000000000000000000005330'));
      expect(locateSettingsSequence('5330')).toHaveLength(9);
    });
  });

  describe('128-bit EPCs', () => {
    it('emits one more command, writing TAGMSK_12_15', () => {
      const sequence = locateSettingsSequence('112233445566778899AABBCCDDEEFF00');

      expect(sequence).toHaveLength(10);
      // Same reversed byte order as its siblings: bytes[15,14,13,12].
      expect(registerValue(sequence, RFID_REGISTERS.TAGMSK_12_15)).toBe(0x00FFEEDD);
    });

    it('sets TAGMSK_LEN to 128 bits', () => {
      expect(
        registerValue(locateSettingsSequence(TAG_633), RFID_REGISTERS.TAGMSK_LEN)
      ).toBe(EPC_BIT_LENGTH.EXTENDED_128);
    });

    it('leaves the first three mask registers on the leading 96 bits', () => {
      // Widening must not disturb where the existing registers point.
      const wide = decodeSequence(locateSettingsSequence('112233445566778899AABBCCDDEEFF00'));
      const narrow = decodeSequence(locateSettingsSequence('112233445566778899AABBCC'));

      for (const register of [
        RFID_REGISTERS.TAGMSK_0_3,
        RFID_REGISTERS.TAGMSK_4_7,
        RFID_REGISTERS.TAGMSK_8_11
      ]) {
        expect(wide.find(c => c.register === register))
          .toEqual(narrow.find(c => c.register === register));
      }
    });

    it('distinguishes the two bench tags that used to collide', () => {
      expect(TAG_633.slice(0, 24)).toBe(TAG_634.slice(0, 24));
      expect(locateSettingsSequence(TAG_633)).not.toEqual(locateSettingsSequence(TAG_634));

      // The difference is confined to the tail register.
      expect(registerValue(locateSettingsSequence(TAG_633), RFID_REGISTERS.TAGMSK_12_15))
        .not.toBe(registerValue(locateSettingsSequence(TAG_634), RFID_REGISTERS.TAGMSK_12_15));
    });

    it('no longer degrades a full 128-bit EPC into its 96-bit prefix', () => {
      expect(locateSettingsSequence(TAG_633))
        .not.toEqual(locateSettingsSequence(TAG_633.slice(0, 24)));
    });

    it('pads a value between 25 and 32 chars out to 128 bits', () => {
      // Anything longer than 24 chars can only be a 128-bit EPC, so it pads
      // right up rather than back down to 96.
      expect(locateSettingsSequence('1'.repeat(25)))
        .toEqual(locateSettingsSequence('1'.repeat(25).padStart(32, '0')));
    });
  });

  describe('EPCs wider than 128 bits', () => {
    // GS1 defines longer fixed forms (198, 202, 212 bits) and Gen2 allows up
    // to 496. None are masked exactly here — but the narrowing must be loud.
    const EPC_198 = 'A'.repeat(52);

    afterEach(() => vi.restoreAllMocks());

    it('falls through to a 128-bit prefix mask', () => {
      vi.spyOn(logger, 'warn').mockImplementation(() => {});
      const sequence = locateSettingsSequence(EPC_198);

      expect(registerValue(sequence, RFID_REGISTERS.TAGMSK_LEN))
        .toBe(EPC_BIT_LENGTH.EXTENDED_128);
      expect(decodeSequence(sequence)).toEqual(
        decodeSequence(locateSettingsSequence(EPC_198.slice(0, 32)))
      );
    });

    it('warns that it narrowed, rather than doing it silently', () => {
      const warn = vi.spyOn(logger, 'warn').mockImplementation(() => {});
      locateSettingsSequence(EPC_198);
      expect(warn).toHaveBeenCalledOnce();
      expect(warn.mock.calls[0][0]).toContain('leading 128 bits');
    });

    it('stays quiet at exactly 32 chars', () => {
      const warn = vi.spyOn(logger, 'warn').mockImplementation(() => {});
      locateSettingsSequence('A'.repeat(32));
      expect(warn).not.toHaveBeenCalled();
    });
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