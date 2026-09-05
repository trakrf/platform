/**
 * CS108 Command Manager
 *
 * The single owner of the wire to the reader. It guarantees three things, and
 * each of them replaces something that used to be asserted rather than enforced:
 *
 * - **Serial execution by QUEUEING.** A second caller waits; it is not thrown
 *   at. The throw used to be the mechanism, which made every caller responsible
 *   for a race it could not see — and the losing call was simply dropped, so a
 *   mode change nobody reapplied left the reader doing what it was doing before
 *   (TRA-1143).
 * - **Responses matched to the command that was sent**, by op code. Any
 *   command-class packet used to settle whatever was pending (TRA-1154).
 * - **Inter-command quiet windows**, declared by the sequence and paid by the
 *   NEXT dispatch rather than by the caller that armed them (TRA-1185).
 *
 * The queue is why the other two can be simple. Because exactly one operation
 * holds the wire at a time, `activeCommand` is a single slot rather than a map,
 * and the quiet window is a single deadline rather than a per-command schedule.
 */

import type { CS108Event, CS108Packet } from './type.js';
import type { CommandSequence } from './type.js';
import type { StateContext } from './state-context.js';
import { PacketHandler } from './packet.js';
import { logger } from '../utils/logger.js';
import { describeErrorCode } from './system/error.js';
import { ReaderState, type ReaderStateType } from '../types/reader.js';

/**
 * Error thrown when a command sequence is aborted due to mode change
 */
export class SequenceAbortedError extends Error {
  constructor(reason: string) {
    super(`Sequence aborted: ${reason}`);
    this.name = 'SequenceAbortedError';
  }
}

/**
 * The retry path used to carry an ALREADY_RETRIED symbol so a nested,
 * recursive re-dispatch could tell "first failure" from "already retried".
 * Before that it was the string ' (already retried)' appended to the message,
 * which forced building a NEW Error to propagate — and that discarded the class
 * of the error being wrapped, so a retried SequenceAbortedError arrived as a
 * plain Error. reader.ts matched on message text and accidentally compensated;
 * narrowing that match to instanceof exposed it on hardware as a failing
 * inventory sequence (TRA-1187).
 *
 * Both are gone. runSequence() now retries in a LOOP, holds the original error
 * object in a local, and rethrows THAT. There is no second entry point that
 * needs to be told what happened, so there is nothing to mark. The invariant
 * "never rebuild the error" is now a property of the shape rather than a rule
 * a flag reminds you to follow.
 */

/**
 * An INVARIANT VIOLATION: a command was dispatched while one was in flight.
 *
 * This used to be a routine outcome — the mutex rejecting a concurrent caller —
 * and `reader.ts` carried two branches treating it as benign, because on the
 * settings path it genuinely was (TRA-1091). Both are gone. The queue makes it
 * unreachable from CONCURRENCY: `dispatchCommand` runs only from inside
 * `runExclusive`, and `runExclusive` admits one operation at a time.
 *
 * It is kept, and kept loud, because "cannot happen" and "is not checked" are
 * different claims. A benign branch for an impossible error is the shape that
 * swallows a real one — and this check has now earned its keep once. It used to
 * say "no reachable path can produce this", and one did: a synchronous throw
 * from `sendToTransport` left `inFlight` claimed by a command that never
 * reached the wire, so the next dispatch met this error and reported a
 * two-owner wire when the real fault was a dead transport (TRA-1239, fixed in
 * `dispatchCommand`).
 *
 * The lesson is the general one, not the specific leak. This fires when the
 * slot is OCCUPIED, which the queue guarantees only for commands that are
 * genuinely in flight; anything that claims the slot and then fails to release
 * it presents here, wearing a message about concurrency. Read it as "the slot
 * was not free", and go looking for who did not give it back.
 */
export class CommandInFlightError extends Error {
  constructor(message = 'Command already active - executeCommand called concurrently') {
    super(message);
    this.name = 'CommandInFlightError';
  }
}

/**
 * The command-issuing surface handed to a `runExclusive()` body.
 *
 * A caller that needs several commands to reach the hardware with nothing
 * interleaved between them takes the wire once and drives it through this,
 * rather than making N separate public calls that the queue is free to
 * interleave. Handing out a scoped runner — rather than letting the public
 * methods detect re-entrancy from a flag — is what keeps that correct: while an
 * exclusive body awaits, unrelated code (a notification handler, say) really can
 * run, and a flag cannot tell that call apart from the body's own.
 */
export interface CommandRunner {
  command(event: CS108Event, payload?: Uint8Array): Promise<unknown>;
  sequence(sequence: CommandSequence): Promise<void>;
}

export class CommandManager {
  private packetHandler: PacketHandler;
  private currentCommandResolve: ((result: unknown) => void) | null = null;
  private currentCommandReject: ((error: Error) => void) | null = null;
  private currentCommandPromise: Promise<unknown> | null = null;
  private currentTimeout: NodeJS.Timeout | null = null;
  private isAborted = false;

  /**
   * The command awaiting a response, or null. The identity TRA-1154 found
   * missing: without it `handleCommandResponse()` had nothing to compare an
   * arriving packet against, so it compared nothing.
   */
  private inFlight: CS108Event | null = null;

  /**
   * FIFO tail. Chained through `.then(fn, fn)` so a rejection cannot strand
   * everything queued behind it — a poisoned chain would be worse than the race
   * it replaces, because one failure would mute the reader for the rest of the
   * session.
   */
  private tail: Promise<unknown> = Promise.resolve();

  /**
   * Wall-clock deadline before which no command may be SENT, from
   * `SequenceCommand.quietPeriodAfter`. 0 means the wire is free.
   */
  private quietUntil = 0;

  /**
   * Whether BUSY has been published and not yet superseded by a terminal state.
   *
   * Tracked here rather than read back through `stateContext.getReaderState()`
   * because the reader publishes many states this class does not author, and
   * because what needs suppressing is a DUPLICATE announcement, not a state.
   * Queueing a second sequence behind a first must not re-emit BUSY — the
   * reader is already busy, and a caller gating on that reads it correctly.
   */
  private busyAnnounced = false;

  // Configuration
  private readonly DEFAULT_TIMEOUT = 2500; // 2.5 seconds

  // Transport callback
  private sendToTransport: (data: Uint8Array) => void;

  // Notification handler callback for responses that need data emission
  private notificationHandler: ((packet: CS108Packet) => void) | null = null;

  // State context for managing reader state transitions
  private stateContext: StateContext | null = null;

  /**
   * @param packetHandler - THE handler for this link, not a private one.
   *
   * This class needs a handler to build commands, and that half works with any
   * instance because `buildCommand()` touches none of the reassembly state.
   * The other half does not: `getDebugReport()` prints the inbound ring
   * buffer, and only `processIncomingData()` ever fills it. A handler that has
   * only built commands reports `Recent BLE packets (0 captured)` and always
   * will, so the caller that feeds the inbound bytes must pass the same
   * instance. TRA-1250.
   *
   * The default exists for tests and standalone use, where nothing is being
   * reassembled and the report is meaningless anyway.
   */
  constructor(
    sendToTransport: (data: Uint8Array) => void,
    notificationHandler?: (packet: CS108Packet) => void,
    stateContext?: StateContext,
    packetHandler: PacketHandler = new PacketHandler()
  ) {
    this.sendToTransport = sendToTransport;
    this.packetHandler = packetHandler;
    this.notificationHandler = notificationHandler || null;
    this.stateContext = stateContext || null;
  }
  
  /**
   * The command in flight, for callers that need to reason about correlation.
   */
  get activeCommand(): CS108Event | null {
    return this.inFlight;
  }

  /**
   * Take the wire for the duration of `body`, which may issue any number of
   * commands and sequences through the runner it is given without anything
   * interleaving between them.
   *
   * Waiting rather than throwing is the whole of TRA-1143. Ordering is FIFO;
   * a body that fails reports to its own caller and releases the wire.
   */
  async runExclusive<T>(body: (run: CommandRunner) => Promise<T>): Promise<T> {
    const runner: CommandRunner = {
      command: (event, payload) => this.dispatchCommand(event, payload),
      sequence: (sequence) => this.runSequence(sequence)
    };
    const result = this.tail.then(() => body(runner), () => body(runner));
    this.tail = result.then(() => undefined, () => undefined);
    return result;
  }

  /**
   * Execute a single command and wait for its response.
   *
   * Queues behind whatever holds the wire. Use `runExclusive()` instead when
   * several commands must reach the hardware as one unit.
   */
  async executeCommand(event: CS108Event, payload?: Uint8Array): Promise<unknown> {
    return this.runExclusive(run => run.command(event, payload));
  }

  /**
   * Put one command on the wire. Callers reach this only through
   * `runExclusive()`, which is what makes the single-slot state below safe.
   */
  private async dispatchCommand(
    event: CS108Event,
    payload?: Uint8Array,
    quietPeriodAfter?: number,
    ignoresQuietPeriod?: boolean
  ): Promise<unknown> {
    // Check if sequence was aborted
    if (this.isAborted) {
      throw new SequenceAbortedError('Command execution aborted');
    }

    // Honour any quiet window the previous command declared, unless this command
    // declares itself safe to issue inside one. Re-check the abort flag
    // afterwards: an abort landing during the wait must still take effect, and
    // the wait can be seconds long.
    if (!ignoresQuietPeriod) {
      await this.awaitQuietWindow();
    }
    if (this.isAborted) {
      throw new SequenceAbortedError('Command execution aborted');
    }

    if (this.inFlight) {
      throw new CommandInFlightError();
    }

    // Create and track the promise
    const commandPromise = new Promise((resolve, reject) => {
      // Build packet FIRST (before setting up handlers to avoid race)
      const packet = this.packetHandler.buildCommand(event, payload);

      // Set up timeout BEFORE setting resolve/reject to ensure it's ready
      const timeout = event.timeout || this.DEFAULT_TIMEOUT;
      this.currentTimeout = setTimeout(() => {
        logger.warn(`[CommandManager] Command timeout: ${event.name}`);
        this.handleTimeout();
      }, timeout);

      // NOW set up current command tracking (right before send)
      this.inFlight = event;
      this.currentCommandResolve = resolve;
      this.currentCommandReject = reject;

      // Arm the quiet window at SEND, not at ack. The vendor requirement is
      // worded "after the ABORT command", so the ack round trip is time the
      // reader has already spent clearing its buffer and counts toward it.
      const quietUntilBefore = this.quietUntil;
      if (quietPeriodAfter) {
        this.quietUntil = Date.now() + quietPeriodAfter;
      }

      // Log and send
      logger.debug(`[CommandManager] Sending command: ${event.name} (0x${event.eventCode.toString(16)})`);

      // A send can fail SYNCHRONOUSLY, and everything above has already been
      // claimed on behalf of a command that is about to not exist.
      //
      // `BaseReader.sendCommand` throws `Transport port not initialized` the
      // moment `disconnect()` clears the port, and `postMessage` on a closed
      // MessagePort throws too. Without this catch the executor's throw rejects
      // `commandPromise` and skips `clearInFlight()` entirely: `inFlight` stays
      // pointing at a command that never reached the wire, and the next
      // dispatch — the retry, usually — meets `CommandInFlightError`. The
      // caller is then told the WIRE HAS TWO OWNERS when what actually happened
      // is that it has none.
      //
      // Every `Command already active` in the 2026-09-01 200-rep arm is this:
      // 13 occurrences (26 log lines — each raises one WARN from the tolerated
      // RFID_POWER_OFF and one ERROR from the setMode step behind it), ALL of
      // them in reps 137-143 and none in the other 193. That window is the
      // teardown/wedge one, which is the distribution a transport cause
      // predicts and a caller race does not.
      //
      // TRA-1239 pre-registered this as "6 per 200, in reps 5/6/39, where the
      // device was refusing", inherited from TRA-1143. Both halves are wrong
      // against the archived per-rep logs, and the location is the half that
      // mattered: refusals are answered commands, and no refusal is involved.
      // Refs TRA-1239, TRA-1143.
      //
      // The orphaned timeout eventually cleared the slot on its own, so the
      // reader recovered and this read as intermittent. Recovering from a leak
      // is not the same as not leaking: until it fired, every command on this
      // manager failed for a reason that named the wrong culprit.
      //
      // The quiet window goes back too. It is a claim about what the DEVICE is
      // busy doing after receiving something; a frame that never left the host
      // gives it nothing to be busy with, and a spurious 2s hold on the next
      // dispatch is the vendor's ABORT window charged for an ABORT that was
      // never sent.
      try {
        this.sendToTransport(packet);
      } catch (error) {
        this.quietUntil = quietUntilBefore;
        this.clearInFlight();
        reject(error instanceof Error ? error : new Error(String(error)));
      }
    });

    this.currentCommandPromise = commandPromise;
    return commandPromise;
  }

  /**
   * Sleep out whatever remains of the current quiet window.
   */
  private async awaitQuietWindow(): Promise<void> {
    const remaining = this.quietUntil - Date.now();
    if (remaining <= 0) return;

    logger.debug(`[CommandManager] Holding ${remaining}ms for the device's quiet window`);
    await new Promise(resolve => setTimeout(resolve, remaining));
  }

  /**
   * Publish BUSY once, until something publishes a terminal state over it.
   */
  private announceBusy(): void {
    if (!this.stateContext || this.busyAnnounced) return;
    this.busyAnnounced = true;
    this.stateContext.setReaderState(ReaderState.BUSY);
  }

  /**
   * Publish a state that ends the current unit of work, re-arming `announceBusy`.
   */
  private announceSettled(state: ReaderStateType): void {
    if (!this.stateContext) return;
    this.busyAnnounced = false;
    this.stateContext.setReaderState(state);
  }

  /**
   * Forget the command in flight and its timeout, whatever settled it.
   */
  private clearInFlight(): void {
    if (this.currentTimeout) {
      clearTimeout(this.currentTimeout);
      this.currentTimeout = null;
    }
    this.inFlight = null;
    this.currentCommandResolve = null;
    this.currentCommandReject = null;
  }


  /**
   * Handle command response packet
   * Called by Reader after routing parsed packet
   */
  handleCommandResponse(packet: CS108Packet): void {
    logger.debug(`[handleCommandResponse] Received response for ${packet.event.name} (0x${packet.eventCode.toString(16)})`);

    // This should only receive command responses
    if (!packet.event.isCommand) {
      logger.error('[CommandManager] Received non-command packet in handleCommandResponse');
      return;
    }

    // Check if this is an autonomous notification (no command waiting)
    const inFlight = this.inFlight;
    if (!inFlight) {
      logger.debug(`[CommandManager] Received autonomous notification ${packet.event.name} (0x${packet.eventCode.toString(16)}) - no command waiting, will be handled by notification router`);
      // Autonomous notifications will be handled by the notification router
      return;
    }

    // Forward certain command responses to notification handler for data emission
    // These responses contain data that needs to be pushed to stores
    if (this.notificationHandler) {
      const requiresDataEmission = [
        0xA000,  // Battery voltage (GET_BATTERY_VOLTAGE response)
        0xA001,  // Trigger state (GET_TRIGGER_STATE response)
        // Add other command responses that need data emission here
      ].includes(packet.event.eventCode);

      if (requiresDataEmission && packet.payload !== undefined) {
        logger.debug(`[CommandManager] Forwarding ${packet.event.name} (0x${packet.eventCode.toString(16)}) to notification handler, payload:`, packet.payload);
        // Forward to notification handler which will emit to stores
        this.notificationHandler(packet);
      }
    }
    
    // Does this packet actually answer the command in flight? Until TRA-1197
    // nothing asked: any command-class packet settled whatever was pending, so
    // a battery reading could resolve a STOP_INVENTORY.
    //
    // ERROR_NOTIFICATION is the one deliberate exception, and it is an exception
    // by protocol rather than by convenience: a rejection is reported under
    // 0xA101 and never under the op code it is rejecting, so op-code equality
    // cannot be the whole rule. Written down here because an exception nobody
    // wrote down is rediscovered as a bug.
    const isErrorResponse = packet.event.name === 'ERROR_NOTIFICATION';
    if (!isErrorResponse && packet.eventCode !== inFlight.eventCode) {
      logger.debug(
        `[CommandManager] ${packet.event.name} (0x${packet.eventCode.toString(16)}) does not answer ` +
        `${inFlight.name} (0x${inFlight.eventCode.toString(16)}) - left for the notification router`
      );
      return;
    }

    logger.debug(`[CommandManager] Response received: ${packet.event.name}`);

    // Use parsed payload if available, otherwise raw payload
    const result = packet.payload ?? packet.rawPayload;
    
    // Check for error response first
    let success = true;
    if (packet.event.name === 'ERROR_NOTIFICATION') {
      // ERROR_NOTIFICATION is always a failure
      success = false;
    } else if (packet.event.successByte !== undefined) {
      // Check success byte if specified
      success = packet.rawPayload.length > 0 && packet.rawPayload[0] === packet.event.successByte;
    }
    
    // Store resolve/reject for potential settling delay
    const resolve = this.currentCommandResolve;
    const reject = this.currentCommandReject;

    // Clear current command
    this.clearInFlight();
    this.currentCommandPromise = null;

    if (success) {
      // Apply settling delay if specified
      if (packet.event.settlingDelay) {
        logger.debug(`[CommandManager] Applying ${packet.event.settlingDelay}ms settling delay`);
        setTimeout(() => {
          resolve?.(result);
        }, packet.event.settlingDelay);
      } else {
        resolve?.(result);
      }
    } else {
      // Build error message
      let errorMessage = `Command failed: ${packet.event.name}`;
      if (packet.event.name === 'ERROR_NOTIFICATION' && packet.rawPayload.length >= 2) {
        const errorCode = (packet.rawPayload[0] << 8) | packet.rawPayload[1];
        // One table, in system/error.ts. It used to be duplicated here, and the
        // two copies disagreed: this one matched the spec while the other
        // numbered every code one higher, so the same wire bytes were named
        // correctly on this path and as "Unknown error" on the notification
        // path. Refs TRA-1229.
        const errorDesc = describeErrorCode(errorCode);
        errorMessage = `Command rejected: ${errorDesc} (0x${errorCode.toString(16).padStart(4, '0')})`;

        // Counted per op by the soak instrument, exactly as `Command timeout:`
        // is. It has to be its own line: TRA-1229 settles a refused command
        // from its 0xA101 reply in ~34ms, which CLEARS the timeout, so the
        // timeout line never fires and every needle keyed to it reads zero
        // through a fault storm. Keep the wording in step with
        // COMMAND_REJECTION_PREFIX in scripts/suite-run-signals.mjs.
        // Refs TRA-1230.
        logger.warn(
          `[CommandManager] Command rejected: ${inFlight.name} — ${errorDesc} ` +
          `(0x${errorCode.toString(16).padStart(4, '0')})`
        );

        // If this is a "Wrong header prefix" error, log packet history for debugging
        if (errorCode === 0x0000) {
          const debugReport = this.packetHandler.getDebugReport('Wrong header prefix (0x0000)');
          logger.error(debugReport);
        }
      }
      reject?.(new Error(errorMessage));
    }
  }
  
  /**
   * Handle command timeout
   */
  private handleTimeout(): void {
    if (!this.currentCommandReject) return;

    const reject = this.currentCommandReject;

    // Clear current command
    this.clearInFlight();

    // Reject with timeout error
    reject(new Error('Command timeout'));

    // DON'T set abort flag - let the sequence handle the error
    // Timeout of one command shouldn't abort the entire sequence
    // this.isAborted = true;
  }
  
  /**
   * Fail the in-flight command because its write never reached the device.
   *
   * A dropped write means no ACK will ever arrive, so waiting out the command's
   * own timeout only delays the failure and reports "Command timeout" instead of
   * the real cause.
   */
  failCurrentCommand(reason: string): void {
    if (!this.currentCommandReject) return;

    const reject = this.currentCommandReject;

    this.clearInFlight();

    logger.warn(`[CommandManager] Command failed before send: ${reason}`);
    reject(new Error(reason));
  }
  
  /**
   * Abort current sequence execution
   * Waits for current command to complete (including settling delay)
   * then prevents any further commands in the sequence
   */
  async abortSequence(reason: string): Promise<void> {
    logger.debug(`[CommandManager] Aborting sequence: ${reason}`);

    // Set abort flag - prevents NEXT command from starting
    this.isAborted = true;

    // If a command is currently executing, wait for it to complete
    if (this.currentCommandPromise) {
      logger.debug('[CommandManager] Waiting for current command to complete...');
      try {
        // Wait for current command + any settling delay
        await this.currentCommandPromise;
        logger.debug('[CommandManager] Current command completed normally');
      } catch (error) {
        // Command might fail, that's ok
        logger.debug('[CommandManager] Current command failed during abort:', error);
      }
    }

    // Clear the promise tracker
    this.currentCommandPromise = null;

    logger.debug('[CommandManager] Sequence aborted cleanly');
  }
  
  /**
   * Check if manager is idle
   */
  isIdle(): boolean {
    return this.inFlight === null;
  }

  /**
   * Is THIS packet the response the manager is waiting for?
   *
   * The parameter used to be `_packet` — literally ignored — so the caller in
   * reader.ts read as per-packet matching and meant "is anything pending".
   * It now answers the question its name asks.
   */
  isWaitingForResponse(packet: CS108Packet): boolean {
    if (!this.inFlight) return false;
    if (packet.event.name === 'ERROR_NOTIFICATION') return true;
    return packet.eventCode === this.inFlight.eventCode;
  }

  /**
   * Reset abort flag - called when starting new sequence
   */
  resetAbortFlag(): void {
    this.isAborted = false;
  }
  
  /**
   * Execute a sequence of commands in order.
   *
   * Queues behind whatever holds the wire, and holds it for the whole sequence:
   * two sequences issued at once run one after the other rather than having
   * their steps shuffled together.
   *
   * ⚠ BUSY is published HERE, synchronously, before anything is queued — and
   * that is load-bearing rather than cosmetic.
   *
   * Callers gate on reader state to decide whether to issue work at all. The
   * trigger path in reader.ts is the one that matters:
   *
   *     if (this.readerState === ReaderState.CONNECTED) await this.startScanning();
   *     else logger.debug('Trigger pressed ignored - reader state is ...');
   *
   * Before this class queued, `executeSequence` set BUSY synchronously before
   * its first await, so that guard saw BUSY the instant work was requested and
   * dropped every further press. **That transition — not the throw — is what
   * kept trigger events from stacking.** Publishing it from inside the queued
   * body instead would leave the reader reading CONNECTED for as long as
   * anything sat ahead in the queue, and each press past the 100ms debounce
   * would enqueue another start, to be drained long after the operator let go.
   *
   * It is the same shape as a guard evaluated at schedule time protecting work
   * that begins later. The queue exists to stop CONCURRENT callers colliding
   * (TRA-1143); it must not become somewhere repeated requests accumulate.
   * Caught in review of #621 by Mike, who removed the original queue for
   * exactly this reason.
   */
  async executeSequence(sequence: CommandSequence): Promise<void> {
    // Announce before enqueueing, so a caller checking state cannot see an idle
    // reader with work already pending.
    this.announceBusy();
    return this.runExclusive(run => run.sequence(sequence));
  }

  /**
   * The body of a sequence. Reached only from inside `runExclusive()`.
   */
  private async runSequence(sequence: CommandSequence): Promise<void> {
    logger.debug(`[CommandManager] Executing sequence of ${sequence.length} commands`);

    // Reset abort flag for new sequence
    this.resetAbortFlag();

    // Get finalState from the last command (default to READY)
    const lastCommand = sequence[sequence.length - 1];
    const finalState = lastCommand?.finalState || ReaderState.CONNECTED;

    // Set BUSY state before starting sequence. Usually already published at
    // enqueue by executeSequence(); this covers a sequence driven straight
    // through runExclusive(), and is a no-op otherwise.
    logger.debug(`[CommandManager] Setting BUSY state before sequence execution`);
    this.announceBusy();

    for (let i = 0; i < sequence.length; i++) {
      const cmd = sequence[i];
      logger.debug(`[CommandManager] Sequence step ${i + 1}/${sequence.length}: ${cmd.event.name} (0x${cmd.event.eventCode.toString(16)})`)

      // Attempt the command, then walk `retryDelays` until it lands or the
      // schedule is spent.
      //
      // A LOOP, not a recursive re-dispatch. The previous shape retried exactly
      // once and had to mark the error so the nested call could tell "first
      // failure" from "already retried" — the mechanism ALREADY_RETRIED existed
      // for. Here the attempt count is a loop variable, the original error
      // object is simply held and rethrown, and error identity is preserved
      // structurally rather than by a flag. That is what TRA-1187 actually
      // needed.
      const delays = cmd.retryDelays ?? [];
      let lastError: unknown;

      for (let attempt = 0; ; attempt++) {
        try {
          // dispatchCommand will throw SequenceAbortedError if aborted
          await this.dispatchCommand(cmd.event, cmd.payload, cmd.quietPeriodAfter, cmd.ignoresQuietPeriod);
          logger.debug(`[CommandManager] Sequence step ${i + 1}/${sequence.length} completed: ${cmd.event.name}`);
          lastError = undefined;
          break;
        } catch (error: unknown) {
          // An abort is a decision, not a fault — never retry through one, and
          // never tolerate one. It publishes NO state on its way past: the
          // operation taking over owns the state from here.
          //
          // This branch used to announce ERROR, which contradicted the sentence
          // above it and cost the same thing the retry case did. A mode change
          // taking the wire is not a fault, and announcing a terminal state on
          // one woke a settings push parked on BUSY, which then found the reader
          // not CONNECTED and dropped its targetEPC. Same defect, second route.
          //
          // Announcing nothing also leaves `busyAnnounced` set, so the reader
          // stays BUSY across the handover rather than flashing a state nothing
          // has actually reached, and the incoming operation's own
          // announceSettled re-arms it. TRA-1237.
          if (error instanceof SequenceAbortedError) {
            throw error;
          }

          // NO STATE IS PUBLISHED HERE. An attempt is not the sequence.
          //
          // This used to announce ERROR from inside the loop, before
          // `retryDelays` were walked, and the argument against it is the one
          // the tolerated case already made: that state is what callers read to
          // conclude the hardware is in an unknown condition. A step with
          // retries left is not that either — it is a step that is about to be
          // tried again, and usually succeeds.
          //
          // The cost was not cosmetic. Reader.waitForSettledState treats ERROR
          // as a state a transition has SETTLED into, so a settings push parked
          // on BUSY woke on it, found the reader not CONNECTED, and dropped the
          // targetEPC — leaving Locate searching on the previous tag's mask
          // while the reader recovered two seconds later. 27 of 33 failures in
          // the 2026-09-01 200-rep arm are that, and none of them involved a
          // sequence that actually failed. TRA-1237.
          lastError = error;
          if (attempt >= delays.length) break;

          const wait = delays[attempt];
          const errorMessage = error instanceof Error ? error.message : String(error);
          logger.debug(`[CommandManager] Command failed with: ${errorMessage}`);
          logger.debug(
            `[CommandManager] Retrying ${cmd.event.name} in ${wait}ms ` +
            `(attempt ${attempt + 2}/${delays.length + 1})`
          );
          await new Promise(resolve => setTimeout(resolve, wait));
        }
      }

      // The ORIGINAL error object, never a rebuild — consumers discriminate by
      // class, and rebuilding is what lost it before (TRA-1187).
      if (lastError !== undefined) {
        if (!cmd.toleratesFailure) {
          // The sequence has genuinely failed: the retry schedule is spent and
          // this step is not tolerated. THIS is the moment ERROR means what its
          // readers take it to mean, so it is published here and nowhere else.
          // TRA-1237.
          logger.debug(`[CommandManager] Setting ERROR state due to command failure`);
          this.announceSettled(ReaderState.ERROR);
          throw lastError;
        }

        // Loud on purpose. This is the one path where the reader did not do
        // what it was told and nobody is informed by an exception, so the log
        // line is the whole record — and it is what a soak report greps for.
        const errorMessage = lastError instanceof Error ? lastError.message : String(lastError);
        // "went unanswered" was true when the only way to fail was a timeout.
        // Since TRA-1229 a command can also be ANSWERED with a refusal, and
        // calling that unanswered is the same category error the whole
        // 0xA101 investigation turned on. Refs TRA-1230.
        const refused = errorMessage.startsWith('Command rejected:');
        logger.warn(
          `[CommandManager] ${cmd.event.name} (0x${cmd.event.eventCode.toString(16)}) ` +
          `${refused ? 'was refused' : 'went unanswered'} after ${delays.length + 1} attempt(s): ` +
          `${errorMessage} — tolerated, continuing the sequence`
        );
      }

      // Apply delay if specified (and not aborted)
      if (cmd.delay && !this.isAborted) {
        logger.debug(`[CommandManager] Applying ${cmd.delay}ms delay after ${cmd.event.name}`);
        await new Promise(resolve => setTimeout(resolve, cmd.delay));
      }
    }

    // Set final state on successful sequence completion
    logger.debug(`[CommandManager] Setting final state: ${finalState}`);
    this.announceSettled(finalState);

    logger.debug(`[CommandManager] Sequence completed successfully - all ${sequence.length} commands executed`);
  }
}