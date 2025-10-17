/**
 * CS108 Command Manager
 * 
 * Manages serial command execution for CS108 hardware:
 * - Strict serial execution (one command at a time)
 * - Command timeouts with configurable values
 * - Response matching via event codes
 * - Queue clearing on errors or mode switches
 * 
 * CS108 requires serial command execution with 2-3 second timeouts.
 * Commands are queued and executed one at a time, waiting for responses.
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

export class CommandManager {
  private packetHandler: PacketHandler;
  private currentCommandResolve: ((result: unknown) => void) | null = null;
  private currentCommandReject: ((error: Error) => void) | null = null;
  private currentCommandPromise: Promise<unknown> | null = null;
  private currentTimeout: NodeJS.Timeout | null = null;
  private isAborted = false;

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
   * Execute a single command and wait for response
   * No queue - commands executed serially via await in sequences
   */
  async executeCommand(event: CS108Event, payload?: Uint8Array): Promise<unknown> {
    // Check if sequence was aborted
    if (this.isAborted) {
      throw new SequenceAbortedError('Command execution aborted');
    }

    // Check if another command is already active
    if (this.currentCommandResolve) {
      throw new Error('Command already active - executeCommand called concurrently');
    }

    // Create and track the promise
    this.currentCommandPromise = new Promise((resolve, reject) => {
      // Build packet FIRST (before setting up handlers to avoid race)
      const packet = this.packetHandler.buildCommand(event, payload);

      // Set up timeout BEFORE setting resolve/reject to ensure it's ready
      const timeout = event.timeout || this.DEFAULT_TIMEOUT;
      this.currentTimeout = setTimeout(() => {
        logger.warn(`[CommandManager] Command timeout: ${event.name}`);
        this.handleTimeout();
      }, timeout);

      // NOW set up current command tracking (right before send)
      this.currentCommandResolve = resolve;
      this.currentCommandReject = reject;

      // Log and send
      logger.debug(`[CommandManager] Sending command: ${event.name} (0x${event.eventCode.toString(16)})`);
      this.sendToTransport(packet);
    });

    return this.currentCommandPromise;
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
    const isAutonomous = !this.currentCommandResolve;
    if (isAutonomous) {
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
    
    logger.debug(`[CommandManager] Response received: ${packet.event.name}`);
    
    // Clear timeout
    if (this.currentTimeout) {
      clearTimeout(this.currentTimeout);
      this.currentTimeout = null;
    }
    
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
    this.currentCommandResolve = null;
    this.currentCommandReject = null;
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
    this.currentCommandResolve = null;
    this.currentCommandReject = null;
    this.currentTimeout = null;

    // Reject with timeout error
    reject(new Error('Command timeout'));

    // DON'T set abort flag - let the sequence handle the error
    // Timeout of one command shouldn't abort the entire sequence
    // this.isAborted = true;
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
    return !this.currentCommandResolve;
  }

  /**
   * Check if we're waiting for a response
   * Used by Reader to determine if packet should be routed as command response
   */
  isWaitingForResponse(_packet: CS108Packet): boolean {
    // If no command is active, we're not waiting for anything
    return this.currentCommandResolve !== null;
  }

  /**
   * Reset abort flag - called when starting new sequence
   */
  resetAbortFlag(): void {
    this.isAborted = false;
  }
  
  /**
   * Execute a sequence of commands in order
   */
  async executeSequence(sequence: CommandSequence): Promise<void> {
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
        // executeCommand will throw SequenceAbortedError if aborted
        await this.executeCommand(cmd.event, cmd.payload);
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
        if (cmd.retryOnError && !errorMessage.includes('(already retried)')) {
          logger.debug(`[CommandManager] Command failed with: ${errorMessage}`);
          logger.debug(`[CommandManager] Retrying ${cmd.event.name} per sequence configuration`);
          await new Promise(resolve => setTimeout(resolve, 100)); // Brief delay
          try {
            await this.executeCommand(cmd.event, cmd.payload);
          } catch (retryError: unknown) {
            // Add marker to prevent infinite retry
            const message = retryError instanceof Error ? retryError.message : String(retryError);
            throw new Error(`${message} (already retried)`);
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