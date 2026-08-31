/**
 * A stop whose ABORT goes unanswered is retried, not failed.
 *
 * Measured 2026-08-30 on a 20-rep hardware arm with the bridge's packet ring
 * dumped and 0x8002 frames counted per rep, windowed by each rep's own
 * start/end:
 *
 *   15 passing reps   248 downlink / 248 uplink   deficit 0, every one
 *    5 failing reps   deficit 1, 1, 1, 1, 2
 *
 * All six gaps were the same frame — the ABORT:
 *
 *   A7 B3 0A C2 82 37 00 00 80 02 | 40 03 00 00 00 00 00 00
 *
 * The reply is absent, not mis-matched. Since every RFID firmware command
 * shares downlink code 0x8002, a correlation fault was the competing
 * hypothesis; it would have left the failing reps balanced, and it did not.
 *
 * WHY THIS TEST EXISTS SEPARATELY from `command.test.ts`'s retry coverage:
 * that test drives the *failed response* path (a wrong success byte). The
 * hardware failure is a **timeout** — no packet at all — which rejects from
 * `handleTimeout()` down a different branch. A `retryOnError` that covered only
 * the first would look correct, pass the existing suite, and do nothing for the
 * defect it was added for.
 *
 * Refs: TRA-1197.
 */

import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { CommandManager, SequenceAbortedError } from './command.js';
import type { CS108Packet } from './type.js';
import { RFID_STOP_SEQUENCE } from './rfid/sequences.js';
import { RFID_FIRMWARE_COMMAND } from './event.js';

vi.mock('./packet.js', () => ({
  PacketHandler: vi.fn().mockImplementation(() => ({
    buildCommand: vi.fn((event: { eventCode: number }) =>
      new Uint8Array([0xA7, 0xB3, 0x00, 0x01, event.eventCode >> 8, event.eventCode & 0xFF])
    )
  }))
}));

/** The queue hands the wire over a microtask after the call, not inside it. */
async function wireHandedOver(): Promise<void> {
  await vi.advanceTimersByTimeAsync(0);
}

function abortAccepted(): CS108Packet {
  return {
    header: { prefix: 0xB3A7, messageLength: 1, flags: 0, reserved: 0, crc: 0 },
    eventCode: RFID_FIRMWARE_COMMAND.eventCode,
    event: { ...RFID_FIRMWARE_COMMAND },
    rawPayload: new Uint8Array([0x00]), // status 0x00 = success
    payload: undefined
  };
}

describe('RFID_STOP_SEQUENCE, when the reader never answers the ABORT', () => {
  let commandManager: CommandManager;
  let sendToTransport: Mock;

  beforeEach(() => {
    vi.useFakeTimers();
    sendToTransport = vi.fn();
    commandManager = new CommandManager(sendToTransport, vi.fn());
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it('keeps a backoff schedule with room to survive a cancelled retry', () => {
    const delays = RFID_STOP_SEQUENCE[0].retryDelays;

    // Deliberately properties, not the literal [100, 200, 500, 1000] — tuning
    // should not require editing a test that merely restates the constant. What
    // is pinned is the REASONING, so a silent narrowing gets caught:
    expect(delays).toBeDefined();

    // More than one retry. On hardware, 13 of 13 first retries were answered,
    // but the one rep that still failed was a retry CANCELLED mid-window by an
    // unrelated abortSequence(). Extra attempts are the margin against that,
    // and attempt count is the vendor's entire margin too (21 to our 5).
    expect(delays!.length).toBeGreaterThanOrEqual(4);

    // Backoff, not a fixed interval.
    expect([...delays!]).toEqual([...delays!].sort((a, b) => a - b));

    // The first gap is a quarantine window, never zero: a response that arrives
    // after we gave up must land with nothing in flight, or it can settle a
    // command it does not belong to (all firmware commands share 0x8002).
    expect(delays![0]).toBeGreaterThanOrEqual(100);

    // The whole schedule has to fit inside a stop an operator will tolerate.
    const worstCase = delays!.reduce((a, b) => a + b, 0)
      + (delays!.length + 1) * RFID_FIRMWARE_COMMAND.timeout!;
    expect(worstCase).toBeLessThan(5000);
  });

  it('sends the ABORT a second time instead of failing the stop', async () => {
    const promise = commandManager.executeSequence(RFID_STOP_SEQUENCE);
    await wireHandedOver();

    expect(sendToTransport).toHaveBeenCalledTimes(1);

    // Say nothing at all — this is the hardware failure, an absent reply.
    // The retry lands at timeout + the first delay in the schedule.
    const firstDelay = RFID_STOP_SEQUENCE[0].retryDelays![0];
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + firstDelay + 50);

    expect(sendToTransport).toHaveBeenCalledTimes(2);

    // The reader answers the second one, as it did for every other ABORT in the
    // same reps on hardware.
    commandManager.handleCommandResponse(abortAccepted());

    // RFID_FIRMWARE_COMMAND carries settlingDelay: 100, so a successful reply
    // resolves inside a timer rather than on the spot. Awaiting the promise
    // without advancing past it hangs until vitest's own test timeout, which
    // reports as a failure of the retry rather than of the wait.
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.settlingDelay! + 50);

    await expect(promise).resolves.toBeUndefined();
  });

  it('walks the whole backoff schedule, then stops', async () => {
    const delays = RFID_STOP_SEQUENCE[0].retryDelays!;
    const promise = commandManager.executeSequence(RFID_STOP_SEQUENCE);
    const settled = promise.catch((e: unknown) => e);
    await wireHandedOver();

    // Answer nothing, ever. Each attempt costs its timeout plus its gap.
    for (let i = 0; i < delays.length; i++) {
      await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + delays[i] + 50);
      expect(sendToTransport).toHaveBeenCalledTimes(i + 2);
    }

    // Schedule spent: one original + one per delay, and NOT one more. A retry
    // loop that never terminates would be worse than the defect it fixes.
    expect(sendToTransport).toHaveBeenCalledTimes(delays.length + 1);
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + 2000);
    expect(sendToTransport).toHaveBeenCalledTimes(delays.length + 1);

    await expect(settled).resolves.toBeInstanceOf(Error);
  });

  it('does not retry through a SequenceAbortedError', async () => {
    // The residual failure in the 20-rep arm: a teardown called abortSequence()
    // inside the retry window, the re-dispatch hit `isAborted`, and the stop
    // failed. Retrying through an abort would be wrong — an abort is a
    // decision — so this pins the behaviour rather than the bug.
    const promise = commandManager.executeSequence(RFID_STOP_SEQUENCE);
    const settled = promise.catch((e: unknown) => e);
    await wireHandedOver();
    expect(sendToTransport).toHaveBeenCalledTimes(1);

    // NOT awaited before advancing: abortSequence() sets the flag synchronously
    // and then waits for the in-flight command, which under fake timers cannot
    // complete until we advance them. Awaiting first deadlocks the test.
    const aborting = commandManager.abortSequence('teardown');
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + 2000);
    await aborting;

    const err = await settled;
    expect(err).toBeInstanceOf(SequenceAbortedError);
    // Its class survived the retry path — rebuilding it is what cost TRA-1187.
  });
});
