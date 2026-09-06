import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import { reassemble, countByEventCode } from '../../scripts/cs108-reassemble.mjs';

/**
 * The trap this exists to close, and why counting frames is not counting packets.
 *
 * A round of TRA-1197 analysis on the 20-rep diagnostic dump reported "224
 * aborts got a status but only 151 got a confirmation — 73 anomalies". There
 * were no anomalies. CS108 packets are split across the 20-byte BLE MTU, and
 * the confirmations were being counted per frame:
 *
 *   RX frames in the dump                             42,690
 *     complete CS108 packets                           6,237
 *     header present but truncated at MTU             11,812
 *     NO A7B3 header at all (continuation fragments)  24,641   <- 58%
 *
 * `is_packet: true` in a bridge dump means "a BLE frame", not "a CS108 packet".
 * Reassembling per CSL's own algorithm (BTReceive.cs:82-122) gives 224
 * confirmations and the correct picture: 230 aborts, 224 both, 0 status-only,
 * 6 neither.
 *
 * The identical bug lived in the Android extract path, where a `grep a7b3` step
 * kept only frames that BEGIN a packet and dropped every continuation. That
 * produced "the vendor app hits the same failure, 4 of 11 aborts unanswered"
 * when the true figure is 2 of 11 — a wrong conclusion about the vendor, drawn
 * from our own tooling.
 *
 * Both failures are silent and both produce a plausible number, which is why
 * the cases below assert the shapes that make the number wrong rather than
 * merely that a well-formed packet round-trips.
 */

/** Build a CS108 packet: 8-byte header + 2-byte event code + payload. */
function packet(eventCode: number, payload: number[] = [], sequence = 0x82): Uint8Array {
  const body = [(eventCode >> 8) & 0xff, eventCode & 0xff, ...payload];
  return new Uint8Array([
    0xa7, 0xb3, body.length, 0xc2, sequence, 0x9e, 0x00, 0x00, ...body,
  ]);
}

/** Split into BLE MTU-sized frames, exactly as the link does. */
function fragment(bytes: Uint8Array, mtu = 20): Uint8Array[] {
  const out: Uint8Array[] = [];
  for (let i = 0; i < bytes.length; i += mtu) out.push(bytes.slice(i, i + mtu));
  return out;
}

describe('CS108 packet reassembly (TRA-1215)', () => {
  it('reassembles one packet from several frames', () => {
    const original = packet(0x8100, Array.from({ length: 40 }, (_, i) => i));
    const frames = fragment(original);

    // Precondition: this is genuinely a multi-frame packet, or the test proves
    // nothing about reassembly.
    expect(frames.length).toBeGreaterThan(1);

    const packets = reassemble(frames.map((data) => ({ direction: 'rx', data })));

    expect(packets).toHaveLength(1);
    expect(packets[0].eventCode).toBe(0x8100);
    expect(Array.from(packets[0].payload)).toEqual(Array.from({ length: 40 }, (_, i) => i));
  });

  it('does not drop continuation frames that carry no A7B3 header', () => {
    // The `grep a7b3` defect, reproduced: every frame after the first has no
    // header, so a header-matching filter keeps 1 of 3 and silently truncates.
    const original = packet(0x8100, Array.from({ length: 40 }, (_, i) => i));
    const frames = fragment(original);

    const headerBearing = frames.filter((f) => f[0] === 0xa7 && f[1] === 0xb3);
    expect(headerBearing).toHaveLength(1);
    expect(frames.length).toBe(3);

    const packets = reassemble(frames.map((data) => ({ direction: 'rx', data })));
    expect(packets).toHaveLength(1);
    expect(packets[0].payload).toHaveLength(40);
  });

  it('recovers a second packet concatenated behind the first', () => {
    // This is the abort-response case from the surviving btsnoop capture: two
    // ABORTs, both answered, and the second response invisible to the lossy
    // pipeline because the CS108 concatenated it behind tag data.
    const tagData = packet(0x8100, Array.from({ length: 30 }, () => 0xaa));
    const abortResponse = packet(0x8002, [0x00, 0x00]);

    const joined = new Uint8Array([...tagData, ...abortResponse]);
    const frames = fragment(joined);

    const packets = reassemble(frames.map((data) => ({ direction: 'rx', data })));

    expect(packets).toHaveLength(2);
    expect(packets.map((p: { eventCode: number }) => p.eventCode)).toEqual([0x8100, 0x8002]);
  });

  it('counts packets, not frames — the 151-vs-224 shape', () => {
    // Ten multi-frame confirmations. Counting frames that look like packets
    // undercounts; counting reassembled packets does not.
    const frames = [];
    for (let i = 0; i < 10; i++) {
      frames.push(...fragment(packet(0x8002, Array.from({ length: 25 }, () => i))));
    }

    const framesThatBeginAPacket = frames.filter((f) => f[0] === 0xa7 && f[1] === 0xb3).length;
    const counts = countByEventCode(reassemble(frames.map((data) => ({ direction: 'rx', data }))));

    expect(framesThatBeginAPacket).toBe(10);
    expect(frames.length).toBe(20); // twice as many frames as packets
    expect(counts.get(0x8002)).toBe(10);
  });

  it('resynchronises after a dropped frame instead of consuming the rest', () => {
    // A dropped frame desynchronises the ring buffer, and the failure mode that
    // matters is a parser that never recovers — every later packet becomes
    // garbage. Dropping the middle frame must cost one packet, not all of them.
    const first = fragment(packet(0x8100, Array.from({ length: 40 }, () => 0x11)));
    const second = packet(0x8002, [0x00, 0x00]);

    const withHole = [first[0], first[2], second]; // first[1] lost in transit

    const packets = reassemble(withHole.map((data) => ({ direction: 'rx', data })));

    expect(packets.map((p: { eventCode: number }) => p.eventCode)).toContain(0x8002);
  });

  it('keeps the sequence byte rather than assuming it is 0x82', () => {
    // Byte 4 is a wrapping counter on 0x8100, and it is the only means of
    // detecting a dropped frame. A reassembler that normalises it away removes
    // the diagnostic this tool exists to enable.
    const frames = fragment(packet(0x8100, [0x01, 0x02], 0x17));
    const packets = reassemble(frames.map((data) => ({ direction: 'rx', data })));

    expect(packets[0].sequence).toBe(0x17);
  });
});
