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
import { ReaderState } from '../types/reader.js';

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
 * An INVARIANT VIOLATION: a command was dispatched while one was in flight.
 *
 * This used to be a routine outcome — the mutex rejecting a concurrent caller —
 * and `reader.ts` carried two branches treating it as benign, because on the
 * settings path it genuinely was (TRA-1091). Both are gone. Since the queue
 * landed, no reachable path can produce this: `dispatchCommand` runs only from
 * inside `runExclusive`, and `runExclusive` admits one operation at a time.
 *
 * It is kept, and kept loud, because "cannot happen" and "is not checked" are
 * different claims. If it ever fires, the queue has been bypassed and the wire
 * has two owners — which is not a condition any caller should absorb as benign.
 * A benign branch for an impossible error is the shape that swallows a real one.
 */
/**
 * Marks an error as having already been through the sequence retry.
 *
 * A PROPERTY, not a message suffix. This used to be the string
 * `' (already retried)'` appended to the message, tested with
 * `errorMessage.includes('(already retried)')` — a flag carried in prose, which
 * meant the only way to propagate it was to build a NEW `Error`, and building a
 * new Error discarded the class of the one being wrapped.
 *
 * That destroyed error identity on the retry path: a retried
 * `SequenceAbortedError` reached its consumer as a plain `Error` whose text
 * still contained "aborted". `reader.ts` matched on that text and therefore
 * still behaved correctly — the over-wide match was accidentally compensating
 * for the identity loss. Narrowing that match to `instanceof` exposed this, on
 * hardware, as a failing inventory sequence (TRA-1187).
 *
 * Two message-shaped mechanisms propping each other up: neither was visible
 * while both were in place.
 */
const ALREADY_RETRIED = Symbol.for('trakrf.cs108.alreadyRetried');

function wasAlreadyRetried(error: unknown): boolean {
  return typeof error === 'object' && error !== null && ALREADY_RETRIED in error;
}

/**
 * Flag the error and return IT — never a copy.
 *
 * Preserving the object preserves its class and its stack, which is the whole
 * point: every consumer downstream discriminates by class now.
 */
function markRetried(error: unknown): unknown {
  if (typeof error === 'object' && error !== null) {
    Object.defineProperty(error, ALREADY_RETRIED, { value: true, enumerable: false });
    return error;
  }
  const wrapped = new Error(String(error));
  Object.defineProperty(wrapped, ALREADY_RETRIED, { value: true, enumerable: false });
  return wrapped;
}

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

  // Configuration
  private readonly DEFAULT_TIMEOUT = 2500; // 2.5 seconds

  // Transport callback
  private sendToTransport: (data: Uint8Array) => void;

  // Notification handler callback for responses that need data emission
  private notificationHandler: ((packet: CS108Packet) => void) | null = null;

  // State context for managing reader state transitions
  private stateContext: StateContext | null = null;

  constructor(
    sendToTransport: (data: Uint8Array) => void,
    notificationHandler?: (packet: CS108Packet) => void,
    stateContext?: StateContext
  ) {
    this.sendToTransport = sendToTransport;
    this.packetHandler = new PacketHandler();
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
    quietPeriodAfter?: number
  ): Promise<unknown> {
    // Check if sequence was aborted
    if (this.isAborted) {
      throw new SequenceAbortedError('Command execution aborted');
    }

    // Honour any quiet window the previous command declared. Re-check the abort
    // flag afterwards: an abort landing during the wait must still take effect,
    // and the wait can be seconds long.
    await this.awaitQuietWindow();
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
      if (quietPeriodAfter) {
        this.quietUntil = Date.now() + quietPeriodAfter;
      }

      // Log and send
      logger.debug(`[CommandManager] Sending command: ${event.name} (0x${event.eventCode.toString(16)})`);
      this.sendToTransport(packet);
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
        // Map known error codes
        const errorMessages: Record<number, string> = {
          0x0000: 'Wrong header prefix',
          0x0001: 'Payload length too large',
          0x0002: 'Unknown target',
          0x0003: 'Unknown event'
        };
        const errorDesc = errorMessages[errorCode] || `Unknown error 0x${errorCode.toString(16).padStart(4, '0')}`;
        errorMessage = `Command rejected: ${errorDesc} (0x${errorCode.toString(16).padStart(4, '0')})`;

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
   */
  async executeSequence(sequence: CommandSequence): Promise<void> {
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

    // Set BUSY state before starting sequence (if we have state context)
    if (this.stateContext) {
      logger.debug(`[CommandManager] Setting BUSY state before sequence execution`);
      this.stateContext.setReaderState(ReaderState.BUSY);
    }

    for (let i = 0; i < sequence.length; i++) {
      const cmd = sequence[i];
      logger.debug(`[CommandManager] Sequence step ${i + 1}/${sequence.length}: ${cmd.event.name} (0x${cmd.event.eventCode.toString(16)})`)

      try {
        // dispatchCommand will throw SequenceAbortedError if aborted
        await this.dispatchCommand(cmd.event, cmd.payload, cmd.quietPeriodAfter);
        logger.debug(`[CommandManager] Sequence step ${i + 1}/${sequence.length} completed: ${cmd.event.name}`);
      } catch (error: unknown) {
        // Set ERROR state on failure (if we have state context)
        if (this.stateContext) {
          logger.debug(`[CommandManager] Setting ERROR state due to command failure`);
          this.stateContext.setReaderState(ReaderState.ERROR);
        }

        // Don't retry on sequence aborts
        if (error instanceof SequenceAbortedError) {
          throw error;
        }

        // If retryOnError is set and this is the first failure, retry once
        const errorMessage = error instanceof Error ? error.message : String(error);
        if (cmd.retryOnError && !wasAlreadyRetried(error)) {
          logger.debug(`[CommandManager] Command failed with: ${errorMessage}`);
          logger.debug(`[CommandManager] Retrying ${cmd.event.name} per sequence configuration`);
          await new Promise(resolve => setTimeout(resolve, 100)); // Brief delay
          try {
            await this.dispatchCommand(cmd.event, cmd.payload, cmd.quietPeriodAfter);
          } catch (retryError: unknown) {
            // Mark to prevent infinite retry, WITHOUT rebuilding the error —
            // see ALREADY_RETRIED. Rebuilding is what lost the class.
            throw markRetried(retryError);
          }
        } else {
          throw error; // Re-throw if no retry configured or already retried
        }
      }

      // Apply delay if specified (and not aborted)
      if (cmd.delay && !this.isAborted) {
        logger.debug(`[CommandManager] Applying ${cmd.delay}ms delay after ${cmd.event.name}`);
        await new Promise(resolve => setTimeout(resolve, cmd.delay));
      }
    }

    // Set final state on successful sequence completion
    if (this.stateContext) {
      logger.debug(`[CommandManager] Setting final state: ${finalState}`);
      this.stateContext.setReaderState(finalState);
    }

    logger.debug(`[CommandManager] Sequence completed successfully - all ${sequence.length} commands executed`);
  }
}