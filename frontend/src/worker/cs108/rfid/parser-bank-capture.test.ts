/**
 * Slicing memory-bank data off a normal-mode inventory response.
 *
 * ## Why this is a separate file from parser.test.ts
 *
 * `parser.test.ts` is named in vitest.config.ts's exclude list and does not
 * run. It imports three fixture modules under `frontend/tests/data/` —
 * `test-tags`, `inventory-by-mode`, `inventory-all-mixed.json5` — that were
 * never written, so it cannot even be collected. Adding these cases there would
 * have produced tests that silently never execute, which is worse than not
 * writing them. Rehabilitating that file is its own piece of work.
 *
 * ## What is being tested
 *
 * With INV_CFG's tag_read set, the normal-mode inventory response carries
 * `PC + EPC + DATA1 [+ DATA2] + CRC16`, and bytes 16 and 17 report how many
 * 16-bit words each bank actually returned. The counts come from the READER,
 * not from what we requested — a bank that refused the read reports zero. So
 * the slice is driven by the packet, never by the settings, and a short or
 * refused read is visible rather than silently blank.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { InventoryParser } from './parser';

describe('normal-mode bank capture', () => {
  let parser: InventoryParser;

  /**
   * Build a normal-mode inventory response (pkt_ver 0x03, pkt_type 0x8005).
   *
   * Layout, from the vendor spec's Inventory-Response Packet Fields:
   *   0     pkt_ver = 0x03
   *   1     flags
   *   3:2   pkt_type = 0x8005
   *   5:4   pkt_len, in 4-BYTE WORDS (total = pkt_len * 4 + 8)
   *   11:8  ms_ctr
   *   12    wb_rssi        13  nb_rssi
   *   14    phase          15  chidx
   *   16    data1_count    17  data2_count   <- words returned, per bank
   *   19:18 port
   *   20+   inv_data = PC + EPC + DATA1 + DATA2 + CRC16
   */
  function normalModePacket({
    epc,
    data1 = new Uint8Array(0),
    data2 = new Uint8Array(0),
    declaredData1Words,
    declaredData2Words
  }: {
    epc: Uint8Array;
    data1?: Uint8Array;
    data2?: Uint8Array;
    declaredData1Words?: number;
    declaredData2Words?: number;
  }): Uint8Array {
    const pc = (epc.length / 2) << 11;       // EPC length in words, bits 15:11
    const invData = new Uint8Array([
      (pc >> 8) & 0xFF, pc & 0xFF,           // PC, big-endian on the wire
      ...epc,
      ...data1,
      ...data2,
      0xAB, 0xCD                             // CRC16
    ]);

    const total = 20 + invData.length;
    const packet = new Uint8Array(total);
    packet[0] = 0x03;
    packet[2] = 0x05;
    packet[3] = 0x80;
    const pktLen = (total - 8) / 4;
    packet[4] = pktLen & 0xFF;
    packet[5] = (pktLen >> 8) & 0xFF;
    packet[12] = 0x48;                        // wb_rssi
    packet[13] = 0x48;                        // nb_rssi
    packet[14] = 0x10;                        // phase
    packet[16] = declaredData1Words ?? data1.length / 2;
    packet[17] = declaredData2Words ?? data2.length / 2;
    packet[18] = 0x01;                        // port
    packet.set(invData, 20);
    return packet;
  }

  const EPC_96 = new Uint8Array([
    0x00, 0x01, 0x05, 0x00, 0x0F, 0x0E,
    0x01, 0x00, 0x19, 0x01, 0x00, 0x7D
  ]);
  const TID_6_WORDS = new Uint8Array([
    0xE2, 0x80, 0x11, 0x60, 0x60, 0x00,
    0x02, 0x07, 0x1D, 0x3C, 0x0B, 0x9A
  ]);
  const USER_4_WORDS = new Uint8Array([
    0xDE, 0xAD, 0xBE, 0xEF, 0x12, 0x34, 0x56, 0x78
  ]);

  beforeEach(() => {
    parser = new InventoryParser('normal', false);
  });

  it('leaves a no-capture packet exactly as it was', () => {
    // The LOCATE regression guard. Locate runs tag_read 0, so both counts are
    // zero and nothing about its result may change.
    const tags = parser.processInventoryPayload(
      normalModePacket({ epc: EPC_96 })
    );

    expect(tags).toHaveLength(1);
    expect(tags[0].epc).toBe('000105000F0E01001901007D');
    expect(tags[0].mode).toBe('normal');
    expect(tags[0].pc).toBe(0x3000);
    expect(tags[0].rssi).toBeLessThan(0);
    expect(tags[0].tid).toBeUndefined();
    expect(tags[0].userData).toBeUndefined();
  });

  it('slices TID off a single-bank read', () => {
    const tags = parser.processInventoryPayload(
      normalModePacket({ epc: EPC_96, data1: TID_6_WORDS })
    );

    expect(tags).toHaveLength(1);
    expect(tags[0].epc).toBe('000105000F0E01001901007D');
    expect(tags[0].tid).toBe('E2801160600002071D3C0B9A');
    expect(tags[0].userData).toBeUndefined();
  });

  it('slices TID and USER off a two-bank read', () => {
    const tags = parser.processInventoryPayload(
      normalModePacket({
        epc: EPC_96,
        data1: TID_6_WORDS,
        data2: USER_4_WORDS
      })
    );

    expect(tags).toHaveLength(1);
    // The EPC must still be the EPC. A slice that ran long would swallow TID
    // into it, and every consumer downstream would believe the result.
    expect(tags[0].epc).toBe('000105000F0E01001901007D');
    expect(tags[0].tid).toBe('E2801160600002071D3C0B9A');
    expect(tags[0].userData).toBe('DEADBEEF12345678');
  });

  it('reports a refused first bank as absent, not as empty USER data', () => {
    // data1_count 0 with data2 present is what a chip that refused the TID read
    // but answered the USER read looks like. The USER bytes must not slide up
    // into the TID slot.
    const tags = parser.processInventoryPayload(
      normalModePacket({
        epc: EPC_96,
        data1: new Uint8Array(0),
        data2: USER_4_WORDS
      })
    );

    expect(tags).toHaveLength(1);
    expect(tags[0].tid).toBeUndefined();
    expect(tags[0].userData).toBe('DEADBEEF12345678');
  });

  it('does not read past the end when the counts overstate the packet', () => {
    // A truncated or malformed packet must not produce hex built from whatever
    // bytes happen to follow in the ring buffer.
    const tags = parser.processInventoryPayload(
      normalModePacket({
        epc: EPC_96,
        data1: TID_6_WORDS,
        declaredData1Words: 64        // claims 128 bytes; 12 are present
      })
    );

    expect(tags).toHaveLength(1);
    expect(tags[0].epc).toBe('000105000F0E01001901007D');
    // Whatever comes back cannot be longer than the bytes that exist after the
    // EPC — TID plus the two CRC bytes, at two hex characters each.
    expect((tags[0].tid ?? '').length).toBeLessThanOrEqual(
      (TID_6_WORDS.length + 2) * 2
    );
  });

  it('still parses a 128-bit EPC with banks attached', () => {
    // 96-bit is what the sampled tags use, but EPC length comes from the PC
    // word and the bank offsets shift with it. A hardcoded 12 would put the
    // TID slice 4 bytes early here.
    const epc128 = new Uint8Array([
      0x00, 0x01, 0x05, 0x00, 0x0F, 0x0E, 0x01, 0x00,
      0x19, 0x01, 0x00, 0x7D, 0xAA, 0xBB, 0xCC, 0xDD
    ]);

    const tags = parser.processInventoryPayload(
      normalModePacket({ epc: epc128, data1: TID_6_WORDS })
    );

    expect(tags).toHaveLength(1);
    expect(tags[0].pc).toBe(0x4000);
    expect(tags[0].epc).toBe('000105000F0E01001901007DAABBCCDD');
    expect(tags[0].tid).toBe('E2801160600002071D3C0B9A');
  });
});
