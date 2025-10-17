import { describe, it, expect, vi } from 'vitest';
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
});