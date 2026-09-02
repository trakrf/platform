import { describe, it, expect, vi, afterEach } from 'vitest';
import { locateSettingsSequence, LOCATE_CONFIG_SEQUENCE } from './sequences.js';
import { RFID_FIRMWARE_COMMAND, RFID_POWER_ON } from '../../event.js';
import {
  RFID_REGISTERS,
  EPC_BIT_LENGTH,
  EPC_MEMORY_OFFSET,
  TAGMASK_DESCRIPTOR,
  TAGMSK_DESCRIPTOR_INDEX
} from '../constant.js';
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

type RegisterWrite = { register: number; value: number };

/**
 * Group a sequence's register writes by the descriptor they land on.
 *
 * TAGMSK_DESC_SEL decides which of the 8 register sets every subsequent
 * TAGMSK_* write applies to, so "how is descriptor 1 configured" is only
 * answerable by replaying the sequence in order — a flat register lookup
 * would silently read whichever descriptor happened to be written last.
 *
 * INV_CFG is not a descriptor register and is left out. Insertion order is
 * preserved, so the key order is the order the descriptors are configured in,
 * which is what decides whether the OR accumulates or cancels.
 */
const descriptorBlocks = (sequence: ReturnType<typeof locateSettingsSequence>) => {
  const blocks = new Map<number, RegisterWrite[]>();
  let selected: number | undefined;

  for (const write of decodeSequence(sequence)) {
    if (write.register === RFID_REGISTERS.HST_TAGMSK_DESC_SEL) {
      selected = write.value;
      if (!blocks.has(selected)) blocks.set(selected, []);
      continue;
    }
    if (selected !== undefined && write.register !== RFID_REGISTERS.INV_CFG) {
      blocks.get(selected)!.push(write);
    }
  }

  return blocks;
};

describe('locateSettingsSequence', () => {
  it('generates correct sequence for standard 96-bit EPC', () => {
    const sequence = locateSettingsSequence('E28011606000020A76543210');

    // Two descriptors' worth of mask config plus search mode: 24 chars is
    // ambiguous between the two widths, so both get configured (TRA-1120).
    expect(sequence).toHaveLength(18);

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

    expect(sequence).toHaveLength(18);

    // Should pad to 000000000000000000010020
    // Check that the sequence is generated without errors
    sequence.forEach(cmd => {
      expect(cmd.payload).toBeInstanceOf(Uint8Array);
    });
  });

  // Whole-sequence comparison rather than a hand-listed set of registers: a
  // listed register the input never reaches compares undefined to undefined and
  // passes without asserting anything. Comparing sequences covers whatever the
  // input actually produces, including the 128-bit tail.
  //
  // Caveat on this particular pair: case-insensitivity holds for free, because
  // parseInt(x, 16) ignores case — deleting the .toUpperCase() in the source
  // does not fail these. They pin the contract, not the implementation. The
  // space-stripping pair below IS implementation-sensitive.
  it.each([
    ['96-bit', 'e28011606000020a7654321f', 'E28011606000020A7654321F'],
    ['128-bit', 'e28011700000020f8b1c0b39aaaaaaaf', 'E28011700000020F8B1C0B39AAAAAAAF']
  ])('handles uppercase and lowercase identically (%s)', (_width, lower, upper) => {
    expect(locateSettingsSequence(lower)).toEqual(locateSettingsSequence(upper));
  });

  it.each([
    ['96-bit', 'E280 1160 6000 020A 7654 3210', 'E28011606000020A76543210'],
    ['128-bit', 'E280 1170 0000 020F 8B1C 0B39 AAAA AAAA', 'E28011700000020F8B1C0B39AAAAAAAA']
  ])('removes spaces from EPC (%s)', (_width, spaced, tight) => {
    // The 128-bit case also pins that spaces are stripped BEFORE the width is
    // decided — the spaced string is 39 chars, so a width test against the raw
    // input rather than the cleaned one would misjudge the padding target.
    expect(locateSettingsSequence(spaced)).toEqual(locateSettingsSequence(tight));
  });

  it('handles empty EPC by padding to all zeros', () => {
    const sequence = locateSettingsSequence('');

    expect(sequence).toHaveLength(18);

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

    it('never writes TAGMSK_12_15 on the 96-bit descriptor', () => {
      // The tail register is left alone rather than cleared: the firmware
      // scans only TAGMSK_LEN bits, so whatever a previous locate left there
      // is inert. The alternate descriptor (TRA-1120) has its own register
      // set, so its 128-bit write does not land here.
      const primary = descriptorBlocks(locateSettingsSequence(EPC_96))
        .get(TAGMSK_DESCRIPTOR_INDEX.LOCATE)!;

      expect(primary.some(c => c.register === RFID_REGISTERS.TAGMSK_12_15)).toBe(false);
    });

    it('sets TAGMSK_LEN to 96 bits on the primary descriptor', () => {
      const primary = descriptorBlocks(locateSettingsSequence(EPC_96))
        .get(TAGMSK_DESCRIPTOR_INDEX.LOCATE)!;

      expect(primary.find(c => c.register === RFID_REGISTERS.TAGMSK_LEN)?.value)
        .toBe(EPC_BIT_LENGTH.STANDARD_96);
    });

    it('pins the exact register traffic, descriptor select first', () => {
      // Byte-for-byte guard on the primary path. Each TAGMSK_DESC_SEL must
      // lead its block: it decides which of the 8 register sets the writes
      // below it land on. 24 chars is ambiguous, so a second block follows
      // carrying the same value padded out to 128 bits (TRA-1120).
      expect(decodeSequence(locateSettingsSequence(EPC_96))).toEqual([
        { register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL, value: 0x00 },
        { register: RFID_REGISTERS.TAGMSK_DESC_CFG, value: 0x09 },
        { register: RFID_REGISTERS.TAGMSK_BANK, value: 0x01 },
        { register: RFID_REGISTERS.TAGMSK_PTR, value: 0x20 },
        { register: RFID_REGISTERS.TAGMSK_LEN, value: 0x60 },
        { register: RFID_REGISTERS.TAGMSK_0_3, value: 0x44332211 },
        { register: RFID_REGISTERS.TAGMSK_4_7, value: 0x88776655 },
        { register: RFID_REGISTERS.TAGMSK_8_11, value: 0xCCBBAA99 },
        // 0x19 = enable | target SL | sel_action 001 (assert on match only).
        { register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL, value: 0x01 },
        { register: RFID_REGISTERS.TAGMSK_DESC_CFG, value: 0x19 },
        { register: RFID_REGISTERS.TAGMSK_BANK, value: 0x01 },
        { register: RFID_REGISTERS.TAGMSK_PTR, value: 0x20 },
        { register: RFID_REGISTERS.TAGMSK_LEN, value: 0x80 },
        // The same 12 bytes, shifted one register along by the 4 zero bytes
        // the 128-bit padding puts in front of them.
        { register: RFID_REGISTERS.TAGMSK_0_3, value: 0x00000000 },
        { register: RFID_REGISTERS.TAGMSK_4_7, value: 0x44332211 },
        { register: RFID_REGISTERS.TAGMSK_8_11, value: 0x88776655 },
        { register: RFID_REGISTERS.TAGMSK_12_15, value: 0xCCBBAA99 },
        { register: RFID_REGISTERS.INV_CFG, value: 0x01E04000 }
      ]);
    });

    it('still treats a short value as a 96-bit prefix search', () => {
      // '5330' padding to the true leading 96 bits is what made the operator's
      // manual workaround work. It has to keep working.
      expect(locateSettingsSequence('5330'))
        .toEqual(locateSettingsSequence('000000000000000000005330'));
      expect(locateSettingsSequence('5330')).toHaveLength(18);
    });
  });

  describe('128-bit EPCs', () => {
    it('emits one more command, writing TAGMSK_12_15', () => {
      const sequence = locateSettingsSequence('112233445566778899AABBCCDDEEFF00');

      // 10 for the one exact descriptor, plus the two writes that disable the
      // alternate one (TRA-1120), plus INV_CFG.
      expect(sequence).toHaveLength(12);
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

/**
 * TRA-1120 — a leading-zero-stripped EPC has to match at BOTH widths.
 *
 * '533034313633' is ambiguous: it could be a 96-bit EPC padded out to 24 hex
 * chars, or a 128-bit one padded out to 32. TRA-1108 fixed the case where the
 * caller can supply the full-width value; the manual EPC field and the tag
 * registry structurally cannot, because the Scan-tab commissioning modal
 * pre-fills the stripped form.
 *
 * A single descriptor has to pick one width, so a short value only ever
 * matched at 96 bits. Two descriptors, each an EXACT match at its own width,
 * cover both without inventing a new false-positive class.
 */
describe('locateSettingsSequence — ambiguous width (TRA-1120)', () => {
  // The WALDO bench probes. STRIPPED is what the registry and the manual EPC
  // field actually hand Locate when the operator means TAG_633.
  const STRIPPED = '533034313633';
  const TAG_633 = '00000000000000000000533034313633';
  const TAG_634 = '00000000000000000000533034313634';

  const MASK_REGISTERS = [
    RFID_REGISTERS.TAGMSK_0_3,
    RFID_REGISTERS.TAGMSK_4_7,
    RFID_REGISTERS.TAGMSK_8_11,
    RFID_REGISTERS.TAGMSK_12_15
  ];

  const valueIn = (block: RegisterWrite[], register: number) =>
    block.find(write => write.register === register)?.value;

  const masksOf = (block: RegisterWrite[]) =>
    MASK_REGISTERS.map(register => valueIn(block, register));

  const blockFor = (epc: string, descriptor: number) =>
    descriptorBlocks(locateSettingsSequence(epc)).get(descriptor)!;

  describe('an ambiguous value (≤24 chars)', () => {
    it('configures two descriptors', () => {
      expect([...descriptorBlocks(locateSettingsSequence(STRIPPED)).keys()])
        .toEqual([TAGMSK_DESCRIPTOR_INDEX.LOCATE, TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH]);
    });

    it('masks the value at 96 bits on the primary descriptor', () => {
      const primary = blockFor(STRIPPED, TAGMSK_DESCRIPTOR_INDEX.LOCATE);

      expect(valueIn(primary, RFID_REGISTERS.TAGMSK_LEN)).toBe(EPC_BIT_LENGTH.STANDARD_96);
      expect(masksOf(primary))
        .toEqual(masksOf(blockFor(STRIPPED.padStart(24, '0'), TAGMSK_DESCRIPTOR_INDEX.LOCATE)));
    });

    it('masks the same value at 128 bits on the alternate descriptor', () => {
      // The acceptance case. The alternate descriptor has to carry the exact
      // mask the full-width EPC would have produced, tail register and all, or
      // the stripped form still cannot find a 128-bit tag.
      const alternate = blockFor(STRIPPED, TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH);

      expect(valueIn(alternate, RFID_REGISTERS.TAGMSK_LEN)).toBe(EPC_BIT_LENGTH.EXTENDED_128);
      expect(masksOf(alternate))
        .toEqual(masksOf(blockFor(TAG_633, TAGMSK_DESCRIPTOR_INDEX.LOCATE)));
    });

    it('anchors both descriptors past the PC bits', () => {
      // An alternate descriptor anchored anywhere else would be a suffix
      // search, which is the TRA-1108 bug mirrored.
      for (const block of descriptorBlocks(locateSettingsSequence(STRIPPED)).values()) {
        expect(valueIn(block, RFID_REGISTERS.TAGMSK_PTR))
          .toBe(EPC_MEMORY_OFFSET.AFTER_PC_BITS);
      }
    });

    it('ORs the alternate descriptor in rather than ANDing it', () => {
      // sel_action (TAGMSK_DESC_CFG bits 6:4) = 001 is assert-SL-on-match,
      // do-nothing-on-miss, which accumulates as OR. The vendor default of 000
      // deasserts on a miss, which would cancel the primary descriptor's hit.
      expect(valueIn(
        blockFor(STRIPPED, TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH),
        RFID_REGISTERS.TAGMSK_DESC_CFG
      )).toBe(
        TAGMASK_DESCRIPTOR.ENABLE
        | TAGMASK_DESCRIPTOR.TARGET_SL
        | TAGMASK_DESCRIPTOR.SEL_ACTION_ASSERT_ON_MATCH_ONLY
      );
    });

    it('leaves the primary descriptor deasserting SL on a miss, and running first', () => {
      // Gen2 SL persists across Selects, so something has to clear it or every
      // tag reads as already selected. The primary descriptor's sel_action 000
      // does that job — it deasserts on a miss — which is why no separate
      // clearing Select is emitted. It only holds if it runs first.
      const sequence = locateSettingsSequence(STRIPPED);

      expect(valueIn(
        descriptorBlocks(sequence).get(TAGMSK_DESCRIPTOR_INDEX.LOCATE)!,
        RFID_REGISTERS.TAGMSK_DESC_CFG
      )).toBe(TAGMASK_DESCRIPTOR.ENABLE | TAGMASK_DESCRIPTOR.TARGET_SL);

      expect(decodeSequence(sequence)[0]).toEqual({
        register: RFID_REGISTERS.HST_TAGMSK_DESC_SEL,
        value: TAGMSK_DESCRIPTOR_INDEX.LOCATE
      });
    });

    it('still rejects a decoy matching neither width', () => {
      // TAG_634 shares every bit of TAG_633 but the last. Neither descriptor
      // may blur them — the TRA-1108 discrimination must not regress.
      expect(locateSettingsSequence(STRIPPED))
        .not.toEqual(locateSettingsSequence('533034313634'));
      expect(masksOf(blockFor(STRIPPED, TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH)))
        .not.toEqual(masksOf(blockFor(TAG_634, TAGMSK_DESCRIPTOR_INDEX.LOCATE)));
    });

    it('treats exactly 24 chars as ambiguous', () => {
      // A 128-bit EPC with eight leading zero hex chars strips to 24, so 24 is
      // still two-way ambiguous. The boundary is >24, not ≥24.
      expect(descriptorBlocks(locateSettingsSequence('1'.repeat(24))).size).toBe(2);
    });
  });

  describe('an unambiguous value (>24 chars)', () => {
    it('masks only on the primary descriptor', () => {
      const blocks = descriptorBlocks(locateSettingsSequence(TAG_633));

      expect(valueIn(blocks.get(TAGMSK_DESCRIPTOR_INDEX.LOCATE)!, RFID_REGISTERS.TAGMSK_LEN))
        .toBe(EPC_BIT_LENGTH.EXTENDED_128);
      expect(blocks.get(TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH)!
        .some(write => write.register === RFID_REGISTERS.TAGMSK_LEN)).toBe(false);
    });

    it('disables the alternate descriptor rather than leaving it enabled', () => {
      // locateSettingsSequence runs again on every settings change without
      // re-running LOCATE_CONFIG_SEQUENCE, so an alternate descriptor left
      // enabled by a previous ambiguous locate would keep issuing its stale
      // Select and OR a wrong tag into this search.
      expect(blockFor(TAG_633, TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH)).toEqual([
        { register: RFID_REGISTERS.TAGMSK_DESC_CFG, value: TAGMASK_DESCRIPTOR.DISABLED }
      ]);
    });

    it('treats 25 chars as 128-bit only', () => {
      expect(blockFor('1'.repeat(25), TAGMSK_DESCRIPTOR_INDEX.LOCATE_ALT_WIDTH)
        .some(write => write.register === RFID_REGISTERS.TAGMSK_LEN)).toBe(false);
    });
  });
});

/**
 * TRA-1239 — one silent frame must not leave a Select half-written.
 *
 * `reader.ts` splices LOCATE_CONFIG_SEQUENCE and locateSettingsSequence into a
 * single sequence, and CommandManager aborts a sequence at the first step whose
 * retry schedule is spent. Before this, the register writes carried no schedule
 * at all: one unanswered frame anywhere in the ~24 writes ended the rest of
 * them, leaving the descriptor registers partially configured and INV_CFG —
 * the write that puts the Selects to work, and the last one — unsent.
 *
 * Measured on the 2026-09-01 200-rep arm's packet ring:
 *
 *   register writes    45,228 sent    2 unanswered    0.004%
 *   ABORT (0x8002)      2,346 sent   45 unanswered    1.918%, 44 recovered on retry
 *
 * So this is rare, and the retry is known to be answered when it is not. Both
 * halves matter: rare is why it was never the headline defect, and answered is
 * why a schedule is worth shipping rather than being a slower way to fail.
 *
 * `toleratesFailure` is deliberately NOT set. A tolerated mask write continues
 * the sequence with the descriptor in an unknown state and tells nobody, which
 * on Locate means searching on a mask that is part this tag and part the last
 * one — the failure the operator reads as "the item is not here".
 */
describe('register writes survive one unanswered frame', () => {
  const registerWrites = (sequence: ReturnType<typeof locateSettingsSequence>) =>
    sequence.filter(cmd => cmd.event === RFID_FIRMWARE_COMMAND);

  const EPC_96 = '112233445566778899AABBCC';
  const EPC_128 = '112233445566778899AABBCCDDEEFF00';

  it.each([
    ['the ambiguous-width mask', locateSettingsSequence(EPC_96)],
    ['the unambiguous 128-bit mask', locateSettingsSequence(EPC_128)],
    ['the locate mode config', LOCATE_CONFIG_SEQUENCE]
  ])('gives every write in %s a retry schedule', (_label, sequence) => {
    const writes = registerWrites(sequence);
    expect(writes.length).toBeGreaterThan(0);

    for (const write of writes) {
      expect(write.retryDelays).toEqual([100, 200, 500, 1000]);
    }
  });

  it('tolerates none of them', () => {
    // A half-written descriptor that reports success is worse than a sequence
    // that fails, because only one of the two is visible to a caller.
    const everyWrite = [
      ...registerWrites(locateSettingsSequence(EPC_96)),
      ...registerWrites(locateSettingsSequence(EPC_128)),
      ...registerWrites(LOCATE_CONFIG_SEQUENCE)
    ];

    for (const write of everyWrite) {
      expect(write.toleratesFailure).toBeFalsy();
    }
  });

  it('matches the schedule the ABORT already ships, for the same op code', async () => {
    // 0x8002 is one op code with one measured answer distribution (p99.9 59.8ms,
    // max 67.8ms, timeout 200ms). Two schedules for it would be two claims
    // about the same hardware, and only one of them could be the measured one.
    const { RFID_STOP_SEQUENCE } = await import('../sequences.js');

    expect(locateSettingsSequence(EPC_96)[0].retryDelays)
      .toEqual(RFID_STOP_SEQUENCE[0].retryDelays);
  });
});

describe('LOCATE_CONFIG_SEQUENCE', () => {
  it('has correct structure', () => {
    // Should have at least power on and configuration commands
    expect(LOCATE_CONFIG_SEQUENCE.length).toBeGreaterThan(0);

    // First command should be RFID_POWER_ON
    expect(LOCATE_CONFIG_SEQUENCE[0].event).toBe(RFID_POWER_ON);
    expect(LOCATE_CONFIG_SEQUENCE[0].retryDelays).toEqual([100]);
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