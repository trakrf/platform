import { describe, it, expect } from 'vitest';
// @ts-expect-error — .mjs instrument module, no types by design
import {
  parseFrame,
  classifyFirmwarePayload,
  analyseTransmissions,
  tallyByLabel,
  retryOutcome
} from '../../scripts/ring-unanswered-commands.mjs';

/**
 * The instrument that answers "which 0x8002, and did the retry land" (TRA-1239).
 *
 * `RFID_FIRMWARE_COMMAND` is one op code for every RFID firmware command, so the
 * soak log's `Command timeout: RFID_FIRMWARE_COMMAND` count names neither the
 * command nor whether it ultimately failed — the line fires per ATTEMPT. On the
 * 2026-09-01 arm it read 47, and the ring showed those were 45 ABORTs the retry
 * recovered plus 2 register writes out of 45,228.
 *
 * The cases below are the ones that produced a WRONG ANSWER during the
 * investigation, not a spread of the happy path.
 */

const hexFrame = (payload: number[]) =>
  ['A7', 'B3', '0A', 'C2', '82', '37', '00', '00', '80', '02']
    .concat(payload.map((b) => b.toString(16).padStart(2, '0').toUpperCase()))
    .join(' ');

const line = (direction: string, timestamp: string, text: string) =>
  JSON.stringify({ id: 1, timestamp, direction, text, size: text.split(' ').length, is_packet: true });

const at = (ms: number) => new Date(Date.UTC(2026, 8, 1, 5, 0, 0, 0) + ms).toISOString();

const WRITE_0706 = [0x70, 0x01, 0x06, 0x07, 0x1e, 0x00, 0x00, 0x00];
const ABORT = [0x40, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

const tx = (ms: number, payload: number[]) => parseFrame(line('TX', at(ms), hexFrame(payload)));
const rx = (ms: number) => parseFrame(line('RX', at(ms), hexFrame([0x00])));

describe('parseFrame', () => {
  it('rejects a continuation fragment instead of decoding it as a command', () => {
    // THE TRAP. At mtu=23 most RX lines are continuations. They carry no A7 B3
    // prefix, but bytes 8..9 of one still decode to a plausible event code — so
    // a parser that skips this check produces confident wrong counts rather
    // than an error. Found by an 0x8002 total that could not be reconciled with
    // the instrument's.
    const fragment = line('RX', at(0), '1E 00 00 00 80 02 70 01 06 07 00 00');
    expect(parseFrame(fragment)).toBeNull();
  });

  it('ignores a truncated final line rather than throwing', () => {
    // A ring is an append-only capture; the last line is routinely half-written.
    expect(parseFrame('{"is_packet": true, "text": "A7 B3')).toBeNull();
  });
});

describe('classifyFirmwarePayload', () => {
  it('decodes a register write LSB-first, as spec A.3 populates it', () => {
    const frame = tx(0, WRITE_0706)!;
    expect(classifyFirmwarePayload(frame.bytes)).toMatchObject({
      kind: 'WRITE',
      register: 0x0706,
      label: 'W 0x0706'
    });
  });

  it('names ABORT rather than reading its opcode as a register', () => {
    // `40 03` sits exactly where `70 <rw>` sits on a register command. Treating
    // every 0x8002 as a register write decodes ABORT as register 0x0000 and
    // buries the one command that actually goes unanswered.
    const frame = tx(0, ABORT)!;
    expect(classifyFirmwarePayload(frame.bytes)).toMatchObject({ kind: 'ABORT', register: null, label: 'ABORT' });
  });
});

describe('analyseTransmissions', () => {
  it('counts a transmission unanswered when the host moved on before the reply', () => {
    // 0x8002 is shared by every RFID firmware command, so once the host has sent
    // another one, a reply cannot be attributed. Waiting out the full budget
    // instead would credit the retry's answer to the attempt that missed.
    const frames = [tx(0, ABORT), tx(300, ABORT), rx(330)];
    const [first, second] = analyseTransmissions(frames);
    expect(first.answered).toBe(false);
    expect(second.answered).toBe(true);
    expect(second.latencyMs).toBe(30);
  });

  it('does not credit a reply that arrives after the budget', () => {
    const frames = [tx(0, ABORT), rx(2600)];
    expect(analyseTransmissions(frames)[0].answered).toBe(false);
  });
});

describe('retryOutcome', () => {
  it('reports a miss that the retry recovered', () => {
    const frames = [tx(0, ABORT), tx(300, ABORT), rx(330)];
    expect(retryOutcome(analyseTransmissions(frames))).toMatchObject({
      unanswered: 1,
      retriedAndAnswered: 1,
      retriedAndFailed: 0,
      neverRetried: 0
    });
  });

  it('does not report a clean recovery for a command that never retried', () => {
    // The row that is easiest to overlook and most misleading if it is missing.
    // A command with no retryDelays produces no follow-up at all, so reading
    // only the two retry rows shows 0 failures for a command that failed
    // outright. `writeRegister` has no retry schedule, which is exactly the
    // case this guards.
    const frames = [tx(0, WRITE_0706)];
    expect(retryOutcome(analyseTransmissions(frames))).toMatchObject({
      unanswered: 1,
      retriedAndAnswered: 0,
      retriedAndFailed: 0,
      neverRetried: 1
    });
  });

  it('does not treat two separate stops as one retried stop', () => {
    // THE OTHER TRAP. The first version grouped transmissions by proximity in a
    // 3s window and reported 241 retried stops on the 2026-09-01 arm — but 194
    // of its gaps were >=1600ms, which the schedule ([100,200,500,1000] behind a
    // 200ms timeout) cannot produce. It was counting distinct stops as retries.
    // Anchoring on "was the previous attempt unanswered" removes the heuristic.
    const frames = [tx(0, ABORT), rx(30), tx(2000, ABORT), rx(2030)];
    expect(retryOutcome(analyseTransmissions(frames))).toMatchObject({
      unanswered: 0,
      retriedAndAnswered: 0,
      neverRetried: 0
    });
  });
});

describe('tallyByLabel', () => {
  it('sorts the command that actually goes unanswered to the top', () => {
    const frames = [
      tx(0, WRITE_0706), rx(30),
      tx(100, WRITE_0706), rx(130),
      tx(200, ABORT), tx(500, ABORT), rx(530)
    ];
    const rows = tallyByLabel(analyseTransmissions(frames));
    expect(rows[0]).toMatchObject({ label: 'ABORT', sent: 2, unanswered: 1 });
    expect(rows[1]).toMatchObject({ label: 'W 0x0706', sent: 2, unanswered: 0 });
  });
});
