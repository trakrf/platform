import { describe, it, expect, vi, type Mock } from 'vitest';
import { CommandManager } from './command.js';
import type { StateContext } from './state-context.js';
import type { CommandSequence } from './type.js';
import { ReaderState } from '../types/reader.js';
import { GET_BATTERY_VOLTAGE, ERROR_NOTIFICATION } from './event.js';

describe('CommandManager State Transitions', () => {
  it('should set BUSY state before sequence and finalState after success', async () => {
    // Mock state context
    const mockStateContext: StateContext = {
      getReaderState: vi.fn().mockReturnValue(ReaderState.CONNECTED),
      setReaderState: vi.fn()
    };

    // Mock transport
    const mockSendToTransport = vi.fn();

    // Create CommandManager with state context
    const manager = new CommandManager(
      mockSendToTransport,
      undefined,
      mockStateContext
    );

    // Create test sequence with finalState
    const sequence: CommandSequence = [{
      event: GET_BATTERY_VOLTAGE,
      finalState: ReaderState.SCANNING
    }];

    // Mock successful response
    const successResponse = {
      event: GET_BATTERY_VOLTAGE,
      rawPayload: new Uint8Array([0x00, 0x50]), // Success byte + battery value
      eventCode: 0xA000,
      payload: undefined
    };

    // Execute sequence and immediately provide response
    const executePromise = manager.executeSequence(sequence);

    // Simulate command response after a brief delay
    setTimeout(() => {
      manager.handleCommandResponse(successResponse as any);
    }, 10);

    await executePromise;

    // Verify state transitions
    expect(mockStateContext.setReaderState).toHaveBeenCalledTimes(2);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(1, ReaderState.BUSY);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(2, ReaderState.SCANNING);
  });

  it('should set ERROR state on command failure when finalState is specified', async () => {
    // Mock state context
    const mockStateContext: StateContext = {
      getReaderState: vi.fn().mockReturnValue(ReaderState.CONNECTED),
      setReaderState: vi.fn()
    };

    // Mock transport
    const mockSendToTransport = vi.fn();

    // Create CommandManager with state context
    const manager = new CommandManager(
      mockSendToTransport,
      undefined,
      mockStateContext
    );

    // Create test sequence with finalState
    const sequence: CommandSequence = [{
      event: GET_BATTERY_VOLTAGE,
      finalState: ReaderState.SCANNING
    }];

    // Mock error response
    const errorResponse = {
      event: ERROR_NOTIFICATION,
      rawPayload: new Uint8Array([0x00, 0x03]), // Unknown event error
      eventCode: 0xFFFF,
      payload: undefined
    };

    // Execute sequence and immediately provide error response
    const executePromise = manager.executeSequence(sequence);

    // Simulate error response after a brief delay
    setTimeout(() => {
      manager.handleCommandResponse(errorResponse as any);
    }, 10);

    // Expect the sequence to throw an error
    await expect(executePromise).rejects.toThrow();

    // Verify state transitions
    expect(mockStateContext.setReaderState).toHaveBeenCalledTimes(2);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(1, ReaderState.BUSY);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(2, ReaderState.ERROR);
  });

  it('should not set states if no StateContext is provided', async () => {
    // Mock transport
    const mockSendToTransport = vi.fn();

    // Create CommandManager WITHOUT state context
    const manager = new CommandManager(
      mockSendToTransport,
      undefined
    );

    // Create test sequence with finalState
    const sequence: CommandSequence = [{
      event: GET_BATTERY_VOLTAGE,
      finalState: ReaderState.SCANNING
    }];

    // Mock successful response
    const successResponse = {
      event: GET_BATTERY_VOLTAGE,
      rawPayload: new Uint8Array([0x00, 0x50]),
      eventCode: 0xA000,
      payload: undefined
    };

    // Execute sequence and immediately provide response
    const executePromise = manager.executeSequence(sequence);

    // Simulate command response after a brief delay
    setTimeout(() => {
      manager.handleCommandResponse(successResponse as any);
    }, 10);

    await executePromise;

    // Should complete without errors even without state context
    expect(mockSendToTransport).toHaveBeenCalled();
  });

  it('should default to READY state when no finalState is specified', async () => {
    // Mock state context
    const mockStateContext: StateContext = {
      getReaderState: vi.fn().mockReturnValue(ReaderState.CONNECTED),
      setReaderState: vi.fn()
    };

    // Mock transport
    const mockSendToTransport = vi.fn();

    // Create CommandManager with state context
    const manager = new CommandManager(
      mockSendToTransport,
      undefined,
      mockStateContext
    );

    // Create test sequence - no finalState
    const sequence: CommandSequence = [{
      event: GET_BATTERY_VOLTAGE
      // No finalState specified - should default to READY
    }];

    // Mock successful response
    const successResponse = {
      event: GET_BATTERY_VOLTAGE,
      rawPayload: new Uint8Array([0x00, 0x50]),
      eventCode: 0xA000,
      payload: undefined
    };

    // Execute sequence and immediately provide response
    const executePromise = manager.executeSequence(sequence);

    // Simulate command response after a brief delay
    setTimeout(() => {
      manager.handleCommandResponse(successResponse as any);
    }, 10);

    await executePromise;

    // Should have set BUSY then READY (default)
    expect(mockStateContext.setReaderState).toHaveBeenCalledTimes(2);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(1, ReaderState.BUSY);
    expect(mockStateContext.setReaderState).toHaveBeenNthCalledWith(2, ReaderState.CONNECTED);
  });

  /**
   * ERROR is a verdict on the SEQUENCE, never on one attempt of one step.
   *
   * It used to be published from inside the attempt loop, before `retryDelays`
   * were walked — so a command that failed once and succeeded on the retry
   * announced "the hardware is in an unknown condition" and then recovered from
   * it about two seconds later.
   *
   * That is not a cosmetic flicker. `Reader.waitForSettledState` treats ERROR as
   * a state a transition has SETTLED into, so a settings push parked on BUSY was
   * woken by it, found the reader not CONNECTED, and dropped the targetEPC it was
   * carrying — leaving Locate to search on the previous tag's mask. 27 of 33
   * failures in the 2026-09-01 200-rep arm trace to exactly that. TRA-1237.
   */
  it('does not publish ERROR for a step that fails once and succeeds on retry', async () => {
    const mockStateContext: StateContext = {
      getReaderState: vi.fn().mockReturnValue(ReaderState.CONNECTED),
      setReaderState: vi.fn()
    };
    const mockSendToTransport = vi.fn();
    const manager = new CommandManager(mockSendToTransport, undefined, mockStateContext);

    const sequence: CommandSequence = [{
      event: GET_BATTERY_VOLTAGE,
      retryDelays: [10],
      finalState: ReaderState.CONNECTED
    }];

    // ERROR_NOTIFICATION is what actually fails a step here. GET_BATTERY_VOLTAGE
    // declares no `successByte`, so handleCommandResponse resolves ANY payload
    // for it — an earlier draft of this test "failed" the first attempt with a
    // wrong success byte, retried nothing, and passed against the broken code.
    const failure = {
      event: ERROR_NOTIFICATION,
      rawPayload: new Uint8Array([0x00, 0x03]),
      eventCode: 0xFFFF,
      payload: undefined
    };
    const success = {
      event: GET_BATTERY_VOLTAGE,
      rawPayload: new Uint8Array([0x00, 0x50]),
      eventCode: 0xA000,
      payload: undefined
    };

    const executePromise = manager.executeSequence(sequence);
    setTimeout(() => manager.handleCommandResponse(failure as any), 10);
    setTimeout(() => manager.handleCommandResponse(success as any), 40);

    await executePromise;

    // The retry is the premise of the whole test. Without this the assertion
    // below is satisfied by a run that never failed an attempt at all.
    expect(mockSendToTransport).toHaveBeenCalledTimes(2);

    // The whole trace, not just the absence of ERROR: asserting only "no ERROR"
    // would also pass on a manager that published nothing at all.
    expect((mockStateContext.setReaderState as Mock).mock.calls.map(call => call[0]))
      .toEqual([ReaderState.BUSY, ReaderState.CONNECTED]);
  });
});
describe('CommandManager transport write failure', () => {
  it('fails the in-flight command immediately instead of waiting out its timeout', async () => {
    // A dropped write means no ACK will ever arrive. Without this the command sits
    // until its own timeout expires and reports "Command timeout", which hides the
    // real cause (queue full / not connected / retries exhausted).
    vi.useFakeTimers();
    try {
      const manager = new CommandManager(vi.fn());
      const pending = manager.executeCommand(GET_BATTERY_VOLTAGE);
      // The command reaches the transport a microtask later now that dispatch
      // goes through the queue; failing it before then would fail nothing.
      await vi.advanceTimersByTimeAsync(0);

      manager.failCurrentCommand('Command queue full, write dropped');

      // No timer advance: if this only resolved via the timeout, it would hang.
      await expect(pending).rejects.toThrow(/write dropped/);
      expect(manager.isIdle()).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });
});
