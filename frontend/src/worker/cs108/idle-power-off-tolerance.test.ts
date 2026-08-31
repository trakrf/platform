/**
 * A mode change survives a reader that will not answer RFID_POWER_OFF.
 *
 * Measured 2026-08-31 from the archived bridge capture of a 200-rep arm
 * (`~/soak-archives/2026-08-31-tra1197-overnight-backoff/`). Inside the failing
 * block — reps 46-108, contiguous, 03:23:32 → 04:45:55 UTC — packets counted by
 * event code at bytes [8..9]:
 *
 *   0x8002 RFID firmware command   TX  421   RX  421   answered one-for-one
 *   0x8001 RFID_POWER_OFF          TX 1257   RX   12   99% unanswered
 *   0x8100 tag notification        TX    0   RX 1071   streaming throughout
 *
 * The reader was alive, streaming, and answering every other RFID command. It
 * stopped answering exactly one op code, held that for 82 minutes across 63
 * reps, and then resumed on its own. The `0x8001` deficit is exactly 0 in every
 * 10-minute bucket outside that window, across all 137 clean reps on both
 * sides — which is the control the failing reps alone could not supply.
 *
 * WHY THE MODE CHANGE FAILED, rather than merely logging it: `IDLE_SEQUENCE`
 * opens with RFID_POWER_OFF and `buildModeSequences()` prefixes IDLE_SEQUENCE to
 * every mode, so an unanswered power-off failed EVERY mode change. `setMode()`
 * then set ERROR mode, and the integration teardown that called it skipped its
 * own `cleanup()`, leaving the link claimed — which is where the `DEVICE_BUSY`
 * the ticket was originally filed about comes from. 63 of 200 reps, 31%.
 *
 * Tolerating the unanswered power-off is worth doing whatever the device turns
 * out to be doing, because the alternative is not a working power-off — it is a
 * reader in ERROR for the rest of the session.
 *
 * Refs: TRA-1217.
 */

import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { CommandManager, SequenceAbortedError } from './command.js';
import type { CS108Event, CS108Packet } from './type.js';
import type { StateContext } from './state-context.js';
import { IDLE_SEQUENCE } from './system/sequences.js';
import { RFID_POWER_OFF, BARCODE_POWER_OFF, GET_TRIGGER_STATE, GET_BATTERY_VOLTAGE } from './event.js';
import { ReaderState, type ReaderStateType } from '../types/reader.js';

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

/** A success reply to `event`, as the reader sends one. */
function accepted(event: CS108Event): CS108Packet {
  return {
    prefix: 0xA7,
    transport: 0xB3,
    length: 1,
    module: event.module,
    reserve: 0x82,
    direction: 0x9E,
    crc: 0,
    eventCode: event.eventCode,
    event,
    rawPayload: new Uint8Array([0x00]),
    totalExpected: 9,
    isComplete: true
  };
}

/** Every attempt this command gets: the original, plus one per retry delay. */
function attemptsFor(event: CS108Event, retryDelays: number[] = []): number {
  return retryDelays.length + 1;
}

describe('IDLE_SEQUENCE, when the reader never answers RFID_POWER_OFF', () => {
  let commandManager: CommandManager;
  let sendToTransport: Mock;
  let states: ReaderStateType[];
  let stateContext: StateContext;

  const powerOffStep = IDLE_SEQUENCE[0];
  const powerOffAttempts = attemptsFor(RFID_POWER_OFF, powerOffStep.retryDelays);

  /** Burn the whole power-off schedule without ever answering it. */
  async function ignorePowerOffEntirely(): Promise<void> {
    for (const delay of powerOffStep.retryDelays ?? []) {
      await vi.advanceTimersByTimeAsync(RFID_POWER_OFF.timeout! + delay + 50);
    }
    await vi.advanceTimersByTimeAsync(RFID_POWER_OFF.timeout! + 50);
  }

  /** The commands actually put on the wire, in order, by op code. */
  function sentOpCodes(): number[] {
    return sendToTransport.mock.calls.map(([packet]: [Uint8Array]) =>
      (packet[4] << 8) | packet[5]
    );
  }

  beforeEach(() => {
    vi.useFakeTimers();
    sendToTransport = vi.fn();
    states = [];
    stateContext = {
      getReaderState: () => states[states.length - 1] ?? ReaderState.CONNECTED,
      setReaderState: (state) => { states.push(state); }
    };
    commandManager = new CommandManager(sendToTransport, vi.fn(), stateContext);
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it('declares the power-off tolerant, and the barcode power-off not', () => {
    // The asymmetry is deliberate, and it is the same discipline
    // `ignoresQuietPeriod` already documents: tolerance is a claim about what
    // this device does with THIS op code, and only 0x8001 has been watched
    // going silent on a reader. Nothing analogous has been observed for
    // BARCODE_POWER_OFF (0x9001), so it stays strict.
    expect(IDLE_SEQUENCE[0].event).toBe(RFID_POWER_OFF);
    expect(IDLE_SEQUENCE[0].toleratesFailure).toBe(true);

    expect(IDLE_SEQUENCE[1].event).toBe(BARCODE_POWER_OFF);
    expect(IDLE_SEQUENCE[1].toleratesFailure).toBeFalsy();
  });

  it('still walks the power-off retry schedule before tolerating anything', async () => {
    // Tolerance must not swallow the retry. The reader answered 12 of 1257
    // downlinks inside the window, so a re-send is not free of value — it is
    // the cheapest chance the mode change has of a real power-off.
    void commandManager.executeSequence(IDLE_SEQUENCE);
    await wireHandedOver();
    expect(sendToTransport).toHaveBeenCalledTimes(1);

    await ignorePowerOffEntirely();

    expect(sentOpCodes().filter(code => code === RFID_POWER_OFF.eventCode))
      .toHaveLength(powerOffAttempts);
  });

  it('carries on to the rest of the sequence instead of failing the mode change', async () => {
    const promise = commandManager.executeSequence(IDLE_SEQUENCE);
    await wireHandedOver();

    await ignorePowerOffEntirely();

    // The step after the tolerated one is on the wire — the sequence did not
    // stop at the power-off.
    expect(sentOpCodes()).toContain(BARCODE_POWER_OFF.eventCode);

    commandManager.handleCommandResponse(accepted(BARCODE_POWER_OFF));
    await vi.advanceTimersByTimeAsync(BARCODE_POWER_OFF.settlingDelay! + 50);

    commandManager.handleCommandResponse(accepted(GET_TRIGGER_STATE));
    await wireHandedOver();

    commandManager.handleCommandResponse(accepted(GET_BATTERY_VOLTAGE));

    await expect(promise).resolves.toBeUndefined();
  });

  it('leaves the reader CONNECTED, never ERROR', async () => {
    // This is the whole point. ERROR is what `setMode()` reads to decide the
    // hardware is in an unknown state, and once it publishes ERROR mode the
    // session is over — 63 reps died here, not at the power-off itself.
    const promise = commandManager.executeSequence(IDLE_SEQUENCE);
    await wireHandedOver();

    await ignorePowerOffEntirely();

    commandManager.handleCommandResponse(accepted(BARCODE_POWER_OFF));
    await vi.advanceTimersByTimeAsync(BARCODE_POWER_OFF.settlingDelay! + 50);
    commandManager.handleCommandResponse(accepted(GET_TRIGGER_STATE));
    await wireHandedOver();
    commandManager.handleCommandResponse(accepted(GET_BATTERY_VOLTAGE));
    await promise;

    expect(states).not.toContain(ReaderState.ERROR);
    expect(states[states.length - 1]).toBe(ReaderState.CONNECTED);
  });

  it('does not tolerate a SequenceAbortedError', async () => {
    // An abort is a decision, not a fault. Tolerating one would let a mode
    // change that has been superseded keep issuing commands at the reader.
    const promise = commandManager.executeSequence(IDLE_SEQUENCE);
    const settled = promise.catch((e: unknown) => e);
    await wireHandedOver();

    // Not awaited before advancing: abortSequence() sets its flag synchronously
    // and then waits on the in-flight command, which under fake timers cannot
    // settle until we advance them.
    const aborting = commandManager.abortSequence('mode change requested');
    await vi.advanceTimersByTimeAsync(RFID_POWER_OFF.timeout! + 2000);
    await aborting;

    expect(await settled).toBeInstanceOf(SequenceAbortedError);
  });
});
