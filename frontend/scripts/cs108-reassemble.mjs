/**
 * Reassemble CS108 packets out of BLE frames, for offline analysis.
 *
 * `is_packet: true` in a bridge dump means "a BLE frame", not "a CS108 packet".
 * R2000 packets are split across the 20-byte MTU and reassembled in a ring
 * buffer, so anything counting packet contents has to reassemble first. In the
 * 20-rep diagnostic dump, 58% of RX frames carry no A7B3 header at all:
 *
 *   RX frames in the dump                             42,690
 *     complete CS108 packets                           6,237
 *     header present but truncated at MTU             11,812
 *     NO A7B3 header at all (continuation fragments)  24,641
 *
 * This exists because the arithmetic has already been got wrong twice, in both
 * directions, and both times the wrong number looked entirely reasonable:
 *
 *   - "224 aborts got a status but only 151 got a confirmation — 73 anomalies."
 *     There were no anomalies. Reassembling gives 224 confirmations: 230 aborts,
 *     224 both, 0 status-only, 6 neither.
 *
 *   - "The vendor app hits the same failure, 4 of 11 aborts unanswered." The
 *     true figure is 2 of 11. That conclusion came from a `grep a7b3` reduction
 *     step which keeps only frames that BEGIN a packet, so a response the CS108
 *     concatenated behind tag data simply vanished.
 *
 * Algorithm follows CSL's own (BTReceive.cs:82-122): find the A7B3 header, take
 * the length byte, accumulate until the packet is whole. Resynchronisation
 * matters as much as reassembly — one dropped frame must cost one packet, not
 * every packet after it.
 *
 * Deliberately not in src/worker/. This is analysis tooling for captures, not
 * part of the receive path; packet.ts owns the live one.
 *
 * Refs: TRA-1215.
 */

/** Header is 8 bytes; the length byte counts everything after it, event code included. */
const HEADER_SIZE = 8;
const PREFIX = 0xa7;
const TRANSPORT_BT = 0xb3;
const TRANSPORT_USB = 0xe6;

/** The largest payload the protocol allows, used to reject a bogus length byte. */
const MAX_DATA_SIZE = 120;

function isHeaderAt(bytes, i) {
  return (
    bytes[i] === PREFIX &&
    (bytes[i + 1] === TRANSPORT_BT || bytes[i + 1] === TRANSPORT_USB) &&
    bytes[i + 2] <= MAX_DATA_SIZE
  );
}

/**
 * Reassemble a frame stream into whole CS108 packets.
 *
 * Accepts `{ direction, data, timestamp? }` records, where `data` is a
 * Uint8Array (or any array-like of bytes) holding one BLE frame. Frames are
 * concatenated per direction before scanning, because a packet is split across
 * frames and a response may be concatenated behind an unrelated packet inside
 * one frame — both happen on real hardware.
 *
 * Returns `{ direction, timestamp, eventCode, sequence, payload }` per packet.
 * `sequence` is byte 4 verbatim: a constant 0x82 on every code except uplink
 * 0x8100, where it is a wrapping counter and the only means of detecting a
 * dropped frame. Normalising it away would remove the diagnostic.
 */
export function reassemble(records) {
  const byDirection = new Map();

  for (const record of records) {
    const direction = record.direction ?? 'rx';
    if (!byDirection.has(direction)) byDirection.set(direction, []);
    byDirection.get(direction).push(record);
  }

  const packets = [];

  for (const [direction, stream] of byDirection) {
    // Concatenate, keeping a timestamp for the frame each byte arrived in so a
    // reassembled packet can be dated by its FIRST frame.
    const bytes = [];
    const stamps = [];
    for (const record of stream) {
      for (const byte of record.data) {
        bytes.push(byte);
        stamps.push(record.timestamp);
      }
    }

    let i = 0;
    while (i < bytes.length) {
      if (!isHeaderAt(bytes, i)) {
        // Not a header. Advance one byte and look again — this is what makes a
        // dropped frame cost one packet rather than the remainder of the
        // stream. A parser that consumed to the end here would report every
        // later packet as garbage.
        i++;
        continue;
      }

      const length = bytes[i + 2];
      const total = HEADER_SIZE + length;

      if (i + total > bytes.length) {
        // The packet claims more bytes than remain. Two very different causes,
        // and treating them alike is what makes a dropped frame catastrophic:
        //
        //   - the capture simply ends here, which is normal and final
        //   - a frame was lost, so this packet is unrecoverable but the ones
        //     after it are perfectly readable
        //
        // Look for the next plausible header. If there is one, this packet is
        // the casualty and we resume there; if there is not, the capture really
        // did end. Breaking unconditionally reported every later packet as
        // absent, which reads in analysis as a link that died rather than one
        // that dropped a single frame.
        let next = -1;
        for (let j = i + 1; j < bytes.length; j++) {
          if (isHeaderAt(bytes, j)) {
            next = j;
            break;
          }
        }

        if (next === -1) break;
        i = next;
        continue;
      }

      const body = bytes.slice(i + HEADER_SIZE, i + total);
      packets.push({
        direction,
        timestamp: stamps[i],
        eventCode: (body[0] << 8) | body[1],
        sequence: bytes[i + 4],
        payload: Uint8Array.from(body.slice(2)),
      });

      i += total;
    }
  }

  return packets;
}

/** Count reassembled packets by event code. The number frames cannot give you. */
export function countByEventCode(packets) {
  const counts = new Map();
  for (const packet of packets) {
    counts.set(packet.eventCode, (counts.get(packet.eventCode) ?? 0) + 1);
  }
  return counts;
}

/**
 * Sequence-number discontinuities on uplink 0x8100, as dropped-frame evidence.
 *
 * CSL tracks the same byte and calls ClearBuffer on a discontinuity
 * (CSLibrary.cs:245-258). Two measurements for scale:
 *
 *   vendor capture     1054 uplink 0x8100   2 discontinuities   27 frames lost (2.6%)
 *   our 20-rep dump   11993 uplink 0x8100   2 discontinuities    6 frames lost (0.05%)
 *
 * ⚠ The counter is per-connection and wraps at 0xFF, so a reconnect legitimately
 * restarts it. Pass `sessionBoundaries` (indices at which a new connection
 * begins) to avoid counting a reconnect as a loss — that mistake turns a clean
 * link into an alarming one.
 */
export function sequenceGaps(packets, { eventCode = 0x8100, sessionBoundaries = [] } = {}) {
  const boundaries = new Set(sessionBoundaries);
  const tagPackets = packets.filter((p) => p.eventCode === eventCode);
  const gaps = [];

  for (let i = 1; i < tagPackets.length; i++) {
    if (boundaries.has(i)) continue;

    const previous = tagPackets[i - 1].sequence;
    const current = tagPackets[i].sequence;
    const expected = (previous + 1) & 0xff;

    if (current !== expected) {
      gaps.push({
        index: i,
        previous,
        current,
        lost: (current - expected + 256) & 0xff,
      });
    }
  }

  return gaps;
}
