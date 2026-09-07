/**
 * A start whose START_INVENTORY goes unanswered is retried, not failed.
 *
 * The stop path got this in TRA-1197 and the locate mask write got it in
 * TRA-1239. `RFID_START_SEQUENCE` was the third instance and was left out:
 * `command.ts` reads `cmd.retryDelays ?? []`, so an omitted schedule means
 * exactly one attempt, and one unanswered firmware command wedged the reader in
 * ERROR until a page reload.
 *
 * Measured 2026-09-07 on a 30-rep Playwright arm: 3 failures, and
 * `[Reader] Failed to start scanning` discriminated them perfectly — present in
 * 3 of 3 failures and 0 of 27 passes. `Command timeout: RFID_FIRMWARE_COMMAND`
 * did NOT discriminate (11 of 15 reps on the earlier arm carried one and
 * passed), which is the whole point: the timeout is common and survivable
 * everywhere it can be retried, and lethal only here, where it could not be.
 *
 * WHY A SEPARATE FILE from `abort-retry-on-timeout.test.ts`: that one pins the
 * STOP path. The two sequences differ in a way that matters — the start
 * declares `ignoresQuietPeriod: true` — so the start needs its own coverage of
 * the interaction rather than inheriting confidence from its neighbour.
 *
 * Refs: TRA-1262.
 */

import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { CommandManager } from './command.js';
import type { CS108Packet } from './type.js';
import { RFID_START_SEQUENCE } from './rfid/sequences.js';
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

function accepted(): CS108Packet {
  return {
    header: { prefix: 0xB3A7, messageLength: 1, flags: 0, reserved: 0, crc: 0 },
    eventCode: RFID_FIRMWARE_COMMAND.eventCode,
    event: { ...RFID_FIRMWARE_COMMAND },
    rawPayload: new Uint8Array([0x00]), // status 0x00 = success
    payload: undefined
  };
}

describe('RFID_START_SEQUENCE, when the reader never answers START_INVENTORY', () => {
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

  it('has a retry schedule at all — one attempt was the defect', () => {
    const delays = RFID_START_SEQUENCE[0].retryDelays;

    expect(delays).toBeDefined();
    expect(delays!.length).toBeGreaterThanOrEqual(4);

    // Backoff, not a fixed interval.
    expect([...delays!]).toEqual([...delays!].sort((a, b) => a - b));

    // The first gap is a quarantine window, never zero: every RFID firmware
    // command shares downlink code 0x8002, so a reply that arrives after we
    // gave up must land with nothing in flight or it can settle a command it
    // does not belong to.
    expect(delays![0]).toBeGreaterThanOrEqual(100);
  });

  /**
   * The start is on the operator's trigger press, so the budget is theirs, not
   * a test's. The wedge it replaces was unbounded — ERROR until a page reload —
   * but a schedule long enough to feel like a hang would trade one bad
   * experience for another.
   */
  it('spends a budget an operator holding the trigger will tolerate', () => {
    const delays = RFID_START_SEQUENCE[0].retryDelays!;
    const worstCase = delays.reduce((a, b) => a + b, 0)
      + (delays.length + 1) * RFID_FIRMWARE_COMMAND.timeout!;

    expect(worstCase).toBeLessThan(5000);
  });

  /**
   * ⚠ `toleratesFailure` is the WRONG knob here and will look like the right
   * one — its docstring describes TRA-1217, a structurally identical failure
   * (a silent op code putting the reader in ERROR and costing 63 of 200 reps).
   *
   * Tolerating a failed START_INVENTORY would be worse than the wedge. The
   * wedge announces itself: ERROR state, a failing test, an operator who can
   * see something is broken. A tolerated failure completes the sequence and
   * publishes `finalState: SCANNING` — the reader reports it is scanning when
   * it is not, and an operator waves it at tags that never read.
   */
  it('retries rather than tolerating — a start that quietly fails is worse than one that wedges', () => {
    expect(RFID_START_SEQUENCE[0].toleratesFailure).toBeFalsy();
    expect(RFID_START_SEQUENCE[0].finalState).toBeDefined();
  });

  it('sends START_INVENTORY a second time instead of failing the start', async () => {
    const promise = commandManager.executeSequence(RFID_START_SEQUENCE);
    await wireHandedOver();

    expect(sendToTransport).toHaveBeenCalledTimes(1);

    // Say nothing at all — this is the hardware failure, an absent reply.
    const firstDelay = RFID_START_SEQUENCE[0].retryDelays![0];
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + firstDelay + 50);

    expect(sendToTransport).toHaveBeenCalledTimes(2);

    commandManager.handleCommandResponse(accepted());
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.settlingDelay! + 50);

    await expect(promise).resolves.toBeUndefined();
  });

  it('walks the whole schedule, then stops', async () => {
    const delays = RFID_START_SEQUENCE[0].retryDelays!;
    const promise = commandManager.executeSequence(RFID_START_SEQUENCE);
    const settled = promise.catch((e: unknown) => e);
    await wireHandedOver();

    for (let i = 0; i < delays.length; i++) {
      await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + delays[i] + 50);
      expect(sendToTransport).toHaveBeenCalledTimes(i + 2);
    }

    // One original plus one per delay, and NOT one more. A retry loop that
    // never terminates would be worse than the defect it fixes.
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.timeout! + 5000);
    expect(sendToTransport).toHaveBeenCalledTimes(delays.length + 1);

    await expect(settled).resolves.toBeInstanceOf(Error);
  });

  /**
   * THE INTERACTION THE TICKET ASKED FOR, and the reason this file exists apart
   * from the stop path's.
   *
   * `RFID_START_SEQUENCE` declares `ignoresQuietPeriod: true` with a long
   * justification: gating a restart on the post-ABORT window costs ~2s per
   * trigger cycle, measured on hardware, and that stall is not merely slow — it
   * is what starts the failure, because an operator who feels nothing happen
   * cycles the trigger harder and the extra edges are dropped while BUSY.
   *
   * A retry that re-honoured the window would reintroduce exactly that stall on
   * the attempt after the first. It does not, because the retry re-dispatches
   * with the same flag — but that is a property of the loop, so it is pinned
   * here rather than assumed.
   */
  it('does not sit out the quiet window on the retry', async () => {
    const QUIET_MS = 2000;
    const withQuietWindow = [
      // A predecessor that arms a long quiet window, as a mode change does.
      {
        event: RFID_FIRMWARE_COMMAND,
        payload: new Uint8Array([0x01]),
        quietPeriodAfter: QUIET_MS
      },
      RFID_START_SEQUENCE[0]
    ];

    const promise = commandManager.executeSequence(withQuietWindow);
    const settled = promise.catch((e: unknown) => e);
    await wireHandedOver();

    // Answer the predecessor so the sequence moves on to the start step.
    commandManager.handleCommandResponse(accepted());
    await vi.advanceTimersByTimeAsync(RFID_FIRMWARE_COMMAND.settlingDelay! + 50);

    expect(sendToTransport).toHaveBeenCalledTimes(2); // predecessor + first start attempt

    // Now say nothing. The retry must land on its schedule, NOT after the
    // 2000ms window — which is well beyond timeout + first delay.
    const firstDelay = RFID_START_SEQUENCE[0].retryDelays![0];
    const retryAt = RFID_FIRMWARE_COMMAND.timeout! + firstDelay + 50;
    expect(retryAt).toBeLessThan(QUIET_MS);

    await vi.advanceTimersByTimeAsync(retryAt);

    expect(sendToTransport).toHaveBeenCalledTimes(3);

    await vi.advanceTimersByTimeAsync(10_000);
    await settled;
  });
});
