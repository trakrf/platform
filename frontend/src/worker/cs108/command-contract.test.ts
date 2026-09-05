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
  let stateTransitions: string[];
  /** What a caller gating on reader state would actually read. */
  let readerState: string;

  beforeEach(() => {
    vi.useFakeTimers();
    sendToTransport = vi.fn();
    notificationHandler = vi.fn();
    stateTransitions = [];
    readerState = 'Connected';
    commandManager = new CommandManager(sendToTransport, notificationHandler, {
      getReaderState: () => readerState,
      // The context has to REFLECT what it is told. A stub returning a constant
      // makes every "what would the caller see?" assertion vacuous, which is
      // exactly the question these tests exist to ask.
      setReaderState: (state: string) => { readerState = state; stateTransitions.push(state); }
    } as never);
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
  // TRA-1239 — a send that throws must give the wire back.
  //
  // `BaseReader.sendCommand` throws SYNCHRONOUSLY once the port is gone:
  //
  //     if (!this.port) throw new Error('Transport port not initialized');
  //     this.port.postMessage({ type: 'ble:write', data });
  //
  // and `disconnect()` sets `this.port = undefined`. dispatchCommand claims the
  // slot — `inFlight`, the timeout, the quiet window — and sends LAST, inside a
  // Promise executor. A throw there rejects the promise and skips every
  // release, so the slot stays claimed by a command that never reached the
  // wire, and the next dispatch meets `CommandInFlightError` instead.
  //
  // That is not hypothetical. In the 2026-09-01 200-rep arm every one of the 26
  // `Command already active` lines is in reps 137-143, the teardown/wedge
  // window, and every one is the same shape:
  //
  //   WARN  RFID_POWER_OFF (0x8001) went unanswered after 2 attempt(s):
  //         Command already active - executeCommand called concurrently
  //   ERROR [setMode] Failed to set Idle mode: CommandInFlightError
  //           at CommandManager.dispatchCommand (command.ts:219)
  //           at CommandManager.runSequence (command.ts:591)
  //
  // — attempt 1 threw at the send, attempt 2 met the slot it left behind. The
  // orphaned timeout is what eventually clears it, which is why the reader
  // recovers rather than staying dead, and why this reads as intermittent.
  //
  // The guard at :219 is NOT what these tests are arguing with. It is a real
  // invariant and it stays loud; the defect is that a failed send violates it.
  // ==========================================================================
  describe('a send that throws releases the wire', () => {
    const portGone = () => { throw new Error('Transport port not initialized'); };

    it('reports the transport failure to the caller, not a timeout', async () => {
      sendToTransport.mockImplementationOnce(portGone);

      await expect(commandManager.executeCommand(FIRST))
        .rejects.toThrow('Transport port not initialized');
    });

    it('leaves nothing in flight', async () => {
      sendToTransport.mockImplementationOnce(portGone);

      await expect(commandManager.executeCommand(FIRST)).rejects.toThrow();

      expect(commandManager.isIdle()).toBe(true);
      expect(commandManager.activeCommand).toBeNull();
    });

    it('lets the NEXT command reach the wire', async () => {
      sendToTransport.mockImplementationOnce(portGone);

      await expect(commandManager.executeCommand(FIRST)).rejects.toThrow();

      const next = commandManager.executeCommand(SECOND);
      await wireHandedOver();
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await expect(next).resolves.toEqual(new Uint8Array([0x00]));
    });

    it('retries the step rather than failing it as already active', async () => {
      // The arm's exact shape: one step, a retry schedule, and a first attempt
      // that never left the host. Before the fix the retry met the slot attempt
      // 1 abandoned, and the sequence failed with CommandInFlightError — a
      // message that blames the caller for a transport fault.
      sendToTransport.mockImplementationOnce(portGone);

      const sequence = commandManager.executeSequence([
        { event: FIRST, retryDelays: [100] }
      ]);

      await vi.advanceTimersByTimeAsync(100);
      expect(opCodesSent(sendToTransport)).toEqual([0x0001, 0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await expect(sequence).resolves.toBeUndefined();
    });

    it('disarms the timeout the abandoned attempt armed', async () => {
      // The orphan is what made this look intermittent instead of fatal: it
      // fires at the command's own timeout and clears the slot, so the damage
      // is bounded to whatever dispatches inside that window. Bounded is not
      // fixed, and a timer nobody owns can settle a command it never sent.
      sendToTransport.mockImplementationOnce(portGone);

      await expect(commandManager.executeCommand(FIRST)).rejects.toThrow();

      expect(vi.getTimerCount()).toBe(0);
    });

    it('does not charge the next command a quiet window it never armed', async () => {
      // The window says the DEVICE is busy clearing its buffer. A frame that
      // never left the host leaves it with nothing to clear, so holding the
      // next dispatch for two seconds is the vendor's ABORT constraint billed
      // for an ABORT that was never sent.
      sendToTransport.mockImplementationOnce(portGone);

      await expect(
        commandManager.executeSequence([{ event: ABORT, quietPeriodAfter: 2000 }])
      ).rejects.toThrow('Transport port not initialized');

      const next = commandManager.executeCommand(SECOND);
      await wireHandedOver();
      expect(opCodesSent(sendToTransport)).toEqual([0x8002, 0x0002]);

      commandManager.handleCommandResponse(responseFor(SECOND));
      await expect(next).resolves.toEqual(new Uint8Array([0x00]));
    });
  });

  // ==========================================================================
  // The state transition callers gate on must happen when work is REQUESTED.
  // ==========================================================================
  describe('BUSY is published at enqueue, not at dequeue', () => {
    // Review catch (Mike, 2026-08-30): "i actually did away with command
    // queueing to avoid stacking responses to trigger events... we want to be
    // careful about blindly stacking commands on a queue."
    //
    // The thing that used to prevent that stacking was NOT the throw. It was
    // that executeSequence() set BUSY synchronously, before its first await, so
    // reader.ts's trigger guard —
    //
    //     if (this.readerState === ReaderState.CONNECTED) await this.startScanning();
    //     else logger.debug('Trigger pressed ignored - reader state is ...');
    //
    // — saw BUSY the instant work was requested and dropped every further press.
    // Moving the BUSY transition behind the queue left the reader reading
    // CONNECTED for as long as anything was ahead in the queue, so each press
    // past the 100ms debounce enqueued another start. Same failure shape as a
    // guard evaluated at schedule time protecting work that begins later.
    //
    // These assertions are deliberately SYNCHRONOUS. Awaiting anything here
    // would hide the defect, because the transition does eventually happen —
    // "eventually" is the bug.
    it('publishes BUSY synchronously when a sequence is requested', () => {
      commandManager.executeSequence([{ event: FIRST }]);

      expect(readerState).toBe('Busy');
      expect(stateTransitions).toEqual(['Busy']);
    });

    it('still reads BUSY when a second request queues behind the first', () => {
      // The case that actually bites: something is ahead in the queue, so the
      // second request will not run for a while. A caller gating on reader
      // state must read BUSY the whole time, or it will keep enqueueing.
      commandManager.executeSequence([{ event: FIRST }]);
      stateTransitions.length = 0;

      commandManager.executeSequence([{ event: SECOND }]);

      expect(readerState).toBe('Busy');
      // ...and it does NOT re-announce. The reader was already busy; a second
      // identical event is noise, and the guard reads state, not events.
      expect(stateTransitions).toEqual([]);
    });

    it('still reports the final state once the sequence completes', async () => {
      // The enqueue-time signal must not replace the dequeue-time one.
      const done = commandManager.executeSequence([{ event: FIRST }]);
      await wireHandedOver();
      commandManager.handleCommandResponse(responseFor(FIRST));
      await done;

      expect(stateTransitions[stateTransitions.length - 1]).toBe('Connected');
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

    it('lets an exempted command through a window that has not expired', async () => {
      // A same-mode inventory restart. The window is armed and unexpired; the
      // command declares itself safe inside it and must not wait.
      const stop = commandManager.executeSequence([
        { event: ABORT, quietPeriodAfter: 2000 }
      ]);
      await wireHandedOver();
      commandManager.handleCommandResponse(responseFor(ABORT));
      await stop;

      const restart = commandManager.executeSequence([
        { event: FIRST, ignoresQuietPeriod: true }
      ]);
      await wireHandedOver();

      // No timers advanced: it went out immediately.
      expect(opCodesSent(sendToTransport)).toEqual([0x8002, 0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await restart;
    });

    it('still holds a NON-exempt command queued behind an exempt one', async () => {
      // The exemption is per-command, not a general disarm. Anything that has
      // not made the claim still waits out the original deadline.
      const stop = commandManager.executeSequence([
        { event: ABORT, quietPeriodAfter: 2000 }
      ]);
      await wireHandedOver();
      commandManager.handleCommandResponse(responseFor(ABORT));
      await stop;

      const restart = commandManager.executeCommand(FIRST);
      await wireHandedOver();
      // FIRST is not exempt here, so it waits.
      expect(opCodesSent(sendToTransport)).toEqual([0x8002]);

      await vi.advanceTimersByTimeAsync(2000);
      expect(opCodesSent(sendToTransport)).toEqual([0x8002, 0x0001]);

      commandManager.handleCommandResponse(responseFor(FIRST));
      await restart;
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

  // ==========================================================================
  // TRA-1247 — a command response can ALSO be data, and two op codes are.
  // ==========================================================================
  describe('command responses that carry state, not just an ack', () => {
    /**
     * `0xA000` and `0xA001` answer a command AND report device state, so
     * `handleCommandResponse` forwards them to the notification handler as well
     * as settling the command. Everything else settles the command only.
     *
     * This is the link two sessions have now misread in opposite directions —
     * once as "the poll's answer is relied on" (from the comment above the
     * command), once as "the answer is consumed as a command response and never
     * reaches the level" (from the routing in `reader.handleBleData`, which
     * only tells half the story). It is load-bearing: it is what makes a mode
     * change re-read the physical trigger and overwrite the host latch, which
     * is the whole mechanism behind ADR 0016.
     */
    const TRIGGER_STATE = event(0xA001, 'GET_TRIGGER_STATE');
    const BATTERY = event(0xA000, 'GET_BATTERY_VOLTAGE');

    /** A response carrying a parsed payload, as the packet handler delivers it. */
    function dataResponseFor(sent: CS108Event, payload: unknown): CS108Packet {
      return { ...responseFor(sent), payload } as unknown as CS108Packet;
    }

    it.each([
      ['GET_TRIGGER_STATE', TRIGGER_STATE, 1],
      ['GET_BATTERY_VOLTAGE', BATTERY, 58],
    ])('forwards a %s reply to the notification handler as well', async (_name, sent, payload) => {
      const pending = commandManager.executeCommand(sent as CS108Event);
      await wireHandedOver();

      commandManager.handleCommandResponse(dataResponseFor(sent as CS108Event, payload));
      await pending;

      expect(notificationHandler).toHaveBeenCalledWith(
        expect.objectContaining({ eventCode: (sent as CS108Event).eventCode, payload })
      );
    });

    it('does not forward an ordinary command ack', async () => {
      // The other half. Forwarding everything would push acks at the stores and
      // make the assertion above pass for the wrong reason.
      const pending = commandManager.executeCommand(FIRST);
      await wireHandedOver();

      commandManager.handleCommandResponse(dataResponseFor(FIRST, 0));
      await pending;

      expect(notificationHandler).not.toHaveBeenCalled();
    });
  });
});
