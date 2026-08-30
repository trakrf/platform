/**
 * The three contracts CommandManager owes its callers.
 *
 * Serialisation, response correlation and the inter-command quiet window are
 * one file because they are one object's contract, and because each of them was
 * previously "enforced" by something that could not go red — a throw the caller
 * swallowed, a match that compared nothing, and a branch whose condition was
 * unreachable. Every test here is written to fail against the behaviour that
 * shipped before it.
 *
 * Refs: TRA-1197 (the pass), TRA-1143 (serialise), TRA-1154 (correlate),
 * TRA-1185 (quiet window).
 */

import { describe, it, expect, beforeEach, afterEach, vi, type Mock } from 'vitest';
import { CommandManager } from './command.js';
import type { CS108Event, CS108Packet } from './type.js';

vi.mock('./packet.js', () => ({
  PacketHandler: vi.fn().mockImplementation(() => ({
    buildCommand: vi.fn((event: CS108Event) =>
      new Uint8Array([0xA7, 0xB3, 0x00, 0x01, event.eventCode >> 8, event.eventCode & 0xFF])
    ),
    getDebugReport: vi.fn(() => '')
  }))
}));

const event = (eventCode: number, name: string, extra: Partial<CS108Event> = {}): CS108Event => ({
  eventCode,
  name,
  isCommand: true,
  isNotification: false,
  description: name,
  module: 0,
  successByte: 0x00,
  ...extra
});

const FIRST = event(0x0001, 'FIRST');
const SECOND = event(0x0002, 'SECOND');
const ABORT = event(0x8002, 'RFID_FIRMWARE_COMMAND');

/** A success response for `sent`, as the packet router would deliver it. */
function responseFor(sent: CS108Event): CS108Packet {
  return {
    eventCode: sent.eventCode,
    event: { ...sent },
    rawPayload: new Uint8Array([0x00]),
    payload: undefined
  } as unknown as CS108Packet;
}

/**
 * Let the queue hand the wire over.
 *
 * `executeCommand()` no longer reaches the transport synchronously: it takes
 * its turn in the FIFO first, so the send lands a microtask later even when the
 * queue is empty. Every assertion about what is in flight has to be made after
 * this, and a test that forgets sees an idle manager rather than a wrong one.
 */
async function wireHandedOver(): Promise<void> {
  await vi.advanceTimersByTimeAsync(0);
}

/** The op code the transport was last handed, decoded back out of the frame. */
function opCodesSent(spy: Mock): number[] {
  return spy.mock.calls.map(([frame]: [Uint8Array]) => (frame[4] << 8) | frame[5]);
}

describe('CommandManager contract', () => {
  let commandManager: CommandManager;
  let sendToTransport: Mock;
  let notificationHandler: Mock;

  beforeEach(() => {
    vi.useFakeTimers();
    sendToTransport = vi.fn();
    notificationHandler = vi.fn();
    commandManager = new CommandManager(sendToTransport, notificationHandler);
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  // ==========================================================================
  // TRA-1143 — a second caller waits its turn; it is not thrown at.
  // ==========================================================================
  describe('serialised execution', () => {
    it('queues a concurrent command instead of throwing at its caller', async () => {
      const first = commandManager.executeCommand(FIRST);
      const second = commandManager.executeCommand(SECOND);

      // The queue must not run ahead: SECOND has not reached the transport.
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await expect(first).resolves.toEqual(new Uint8Array([0x00]));

      // Only now does the second command go out.
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await expect(second).resolves.toEqual(new Uint8Array([0x00]));
    });

    it('does not interleave two sequences issued concurrently', async () => {
      // The TRA-1143 collision in miniature: the worker's own auto-stop and a
      // caller's setMode(), both driving a multi-step sequence at once. The
      // failure to prevent is not just the throw — it is two sequences' steps
      // arriving at the hardware shuffled together.
      const alpha = commandManager.executeSequence([{ event: FIRST }, { event: SECOND }]);
      const beta = commandManager.executeSequence([{ event: ABORT }]);

      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await alpha;

      // beta's step runs only after alpha has finished both of its own.
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0002, 0x8002]);

      commandManager.handleCommandResponse(responseFor(ABORT));
      await beta;
    });

    it('lets the next queued caller run after the one ahead of it fails', async () => {
      // A poisoned chain would be worse than the race it replaces: one failure
      // would mute every command behind it for the rest of the session.
      const doomed = commandManager.executeCommand(event(0x0003, 'DOOMED', { timeout: 100 }));
      const next = commandManager.executeCommand(SECOND);

      // Attach the expectation BEFORE the timers that trigger the rejection.
      // Advancing first leaves `doomed` rejected with no handler for a tick,
      // which vitest reports as an unhandled rejection and fails the whole run
      // on — a green test inside a red file.
      const doomedRejects = expect(doomed).rejects.toThrow('Command timeout');
      await vi.advanceTimersByTimeAsync(100);
      await doomedRejects;

      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0003, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await expect(next).resolves.toEqual(new Uint8Array([0x00]));
    });
  });

  // ==========================================================================
  // TRA-1154 — the response has to be the response to the command that was sent.
  // ==========================================================================
  describe('response correlation by op code', () => {
    it('does not settle the pending command with a mismatched op code', async () => {
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      let outcome: string | null = null;
      pending.then(() => { outcome = 'resolved'; }, () => { outcome = 'rejected'; });

      // A battery packet arrives mid-command. It is command-class, so it used
      // to settle whatever was in flight.
      commandManager.handleCommandResponse(
        responseFor(event(0xA000, 'GET_BATTERY_VOLTAGE', { isNotification: true }))
      );

      // Promise adoption costs several ticks — flush generously, or this
      // assertion passes for the wrong reason.
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(0);
      expect(outcome).toBeNull();

      // ...and the real response still settles it.
      commandManager.handleCommandResponse(responseFor(FIRST));
      await expect(pending).resolves.toEqual(new Uint8Array([0x00]));
    });

    it('retains the identity of the command in flight', async () => {
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      expect(commandManager.activeCommand?.eventCode).toBe(0x0001);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await pending;

      expect(commandManager.activeCommand).toBeNull();
    });

    it('discriminates by packet in isWaitingForResponse()', async () => {
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      expect(commandManager.isWaitingForResponse(responseFor(FIRST))).toBe(true);
      expect(commandManager.isWaitingForResponse(responseFor(SECOND))).toBe(false);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await pending;

      expect(commandManager.isWaitingForResponse(responseFor(FIRST))).toBe(false);
    });

    it('still settles on ERROR_NOTIFICATION, which carries its own op code', async () => {
      // The one deliberate exception. A rejection reports failure under a
      // different code by design, so op-code equality must not be the whole
      // rule — an exception written down beats an exception discovered later.
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      const error = responseFor(event(0xA101, 'ERROR_NOTIFICATION', { isNotification: true }));
      (error as { rawPayload: Uint8Array }).rawPayload = new Uint8Array([0x00, 0x03]);
      commandManager.handleCommandResponse(error);

      await expect(pending).rejects.toThrow('Command rejected');
    });

    it('forwards a mismatched packet that carries data rather than dropping it', async () => {
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      const battery = responseFor(event(0xA000, 'GET_BATTERY_VOLTAGE', { isNotification: true }));
      (battery as { payload?: unknown }).payload = { percentage: 82 };
      commandManager.handleCommandResponse(battery);

      expect(notificationHandler).toHaveBeenCalledWith(battery);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await pending;
    });
  });

  // ==========================================================================
  // TRA-1185 — the vendor's 2s post-ABORT window, as a mechanism.
  // ==========================================================================
  describe('inter-command quiet window', () => {
    it('holds the next dispatch for the whole window, measured from the send', async () => {
      const stop = commandManager.executeSequence([
        { event: ABORT, quietPeriodAfter: 2000 }
      ]);

      await vi.advanceTimersByTimeAsync(0);
      commandManager.handleCommandResponse(responseFor(ABORT));
      await stop;

      const next = commandManager.executeCommand(FIRST);
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002]);

      await vi.advanceTimersByTimeAsync(1999);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002]);

      await vi.advanceTimersByTimeAsync(1);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002, 0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await next;
    });

    it('does not make the aborting caller wait out the window', async () => {
      // The whole point of moving the wait here: the operator releases the
      // trigger and the UI is told at once, while the hardware constraint is
      // paid by whoever dispatches next.
      let stopped = false;
      const stop = commandManager
        .executeSequence([{ event: ABORT, quietPeriodAfter: 2000 }])
        .then(() => { stopped = true; });

      await vi.advanceTimersByTimeAsync(0);
      commandManager.handleCommandResponse(responseFor(ABORT));
      await stop;

      // No timer was advanced by 2000, so this could only be true if the
      // window did not block the caller.
      expect(stopped).toBe(true);
    });

    it('charges the command\'s own round trip against the window', async () => {
      // Armed at send, per the vendor text: "after the ABORT command, a 2
      // seconds delay is required". Time the ack already spent is time the
      // reader spent clearing, so it counts.
      const stop = commandManager.executeSequence([
        { event: ABORT, quietPeriodAfter: 2000 }
      ]);

      await vi.advanceTimersByTimeAsync(500);
      commandManager.handleCommandResponse(responseFor(ABORT));
      await stop;

      const next = commandManager.executeCommand(FIRST);
      await vi.advanceTimersByTimeAsync(1499);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002]);

      await vi.advanceTimersByTimeAsync(1);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002, 0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await next;
    });

    it('leaves commands that declare no window undelayed', async () => {
      const first = commandManager.executeCommand(FIRST);
      await vi.advanceTimersByTimeAsync(0);
      commandManager.handleCommandResponse(responseFor(FIRST));
      await first;

      const second = commandManager.executeCommand(SECOND);
      await vi.advanceTimersByTimeAsync(0);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await second;
    });
  });
});
