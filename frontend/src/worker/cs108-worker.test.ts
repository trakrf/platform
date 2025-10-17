/**
 * Unit tests for CS108 Worker flat API
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ReaderMode } from './types/reader';
import type { WorkerEvent } from './types/events';

// Mock CS108Reader
vi.mock('./cs108/reader.js', () => ({
  CS108Reader: vi.fn().mockImplementation(() => ({
    setTransportPort: vi.fn(),
    connect: vi.fn().mockResolvedValue(true),
    disconnect: vi.fn().mockResolvedValue(undefined),
    setMode: vi.fn().mockResolvedValue(undefined),
    setSettings: vi.fn().mockResolvedValue(undefined),
    startScanning: vi.fn().mockResolvedValue(undefined),
    stopScanning: vi.fn().mockResolvedValue(undefined)
  }))
}));

describe('CS108 Worker', () => {
  let originalPostMessage: typeof globalThis.postMessage;
  let mockPostMessage: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    // Mock globalThis.postMessage for worker context
    originalPostMessage = globalThis.postMessage;
    mockPostMessage = vi.fn();
    globalThis.postMessage = mockPostMessage;

    vi.clearAllMocks();
  });

  afterEach(() => {
    // Restore original postMessage
    globalThis.postMessage = originalPostMessage;
  });

  it('should handle initialize with MessagePort', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort; // Mock MessagePort

    const result = await workerModule.initialize(mockPort);

    expect(result).toBe(true);
  });

  it('should handle disconnect', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort; // Mock MessagePort

    // Initialize first
    await workerModule.initialize(mockPort);

    // Then disconnect
    await expect(workerModule.disconnect()).resolves.toBeUndefined();
  });

  it('should handle setMode', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort;

    // Initialize first
    await workerModule.initialize(mockPort);

    // Set mode
    await expect(workerModule.setMode(ReaderMode.INVENTORY)).resolves.toBeUndefined();
  });

  it('should handle setSettings', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort;

    // Initialize first
    await workerModule.initialize(mockPort);

    // Set settings
    const settings = { power: 300, session: 1 };
    await expect(workerModule.setSettings(settings)).resolves.toBeUndefined();
  });

  it('should handle startScanning', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort;

    // Initialize first
    await workerModule.initialize(mockPort);

    // Start scanning
    await expect(workerModule.startScanning()).resolves.toBeUndefined();
  });

  it('should handle stopScanning', async () => {
    const workerModule = await import('./cs108-worker.js');
    const mockPort = {} as MessagePort;

    // Initialize first
    await workerModule.initialize(mockPort);

    // Stop scanning
    await expect(workerModule.stopScanning()).resolves.toBeUndefined();
  });

  describe('Event Handling', () => {
    it('should post events directly through postMessage', async () => {
      const workerModule = await import('./cs108-worker.js');
      const mockPort = {} as MessagePort;

      // Initialize
      await workerModule.initialize(mockPort);

      // Events now flow directly through postMessage
      // No callback needed - events go straight to main thread
      const testEvent = {
        type: 'READER_STATE_CHANGED',
        payload: { readerState: 4 },
        timestamp: Date.now()
      };

      // Simulate the reader posting an event
      globalThis.postMessage(testEvent);

      // Verify postMessage was called
      expect(mockPostMessage).toHaveBeenCalledWith(testEvent);
    });

    it('should handle TAG_BATCH event', async () => {
      const workerModule = await import('./cs108-worker.js');
      const mockPort = {} as MessagePort;

      await workerModule.initialize(mockPort);

      const testEvent = {
        type: 'TAG_BATCH',
        payload: {
          tags: [{
            epc: 'E28011700000020D8B0C7E6C',
            rssi: -45,
            timestamp: Date.now()
          }]
        },
        timestamp: Date.now()
      };

      globalThis.postMessage(testEvent);
      expect(mockPostMessage).toHaveBeenCalledWith(testEvent);
    });

    it('should handle BARCODE_READ event', async () => {
      const workerModule = await import('./cs108-worker.js');
      const mockPort = {} as MessagePort;

      await workerModule.initialize(mockPort);

      const testEvent = {
        type: 'BARCODE_READ',
        payload: {
          barcode: '123456789',
          symbology: 'Code 128',
          timestamp: Date.now()
        },
        timestamp: Date.now()
      };

      globalThis.postMessage(testEvent);
      expect(mockPostMessage).toHaveBeenCalledWith(testEvent);
    });

    it('should handle BATTERY_UPDATE event', async () => {
      const workerModule = await import('./cs108-worker.js');
      const mockPort = {} as MessagePort;

      await workerModule.initialize(mockPort);

      const testEvent = {
        type: 'BATTERY_UPDATE',
        payload: {
          percentage: 85
        },
        timestamp: Date.now()
      };

      globalThis.postMessage(testEvent);
      expect(mockPostMessage).toHaveBeenCalledWith(testEvent);
    });

    it('should handle TRIGGER_STATE_CHANGED events', async () => {
      const workerModule = await import('./cs108-worker.js');
      const mockPort = {} as MessagePort;

      await workerModule.initialize(mockPort);

      const pressEvent = {
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: true },
        timestamp: Date.now()
      };

      const releaseEvent = {
        type: 'TRIGGER_STATE_CHANGED',
        payload: { pressed: false },
        timestamp: Date.now()
      };

      globalThis.postMessage(pressEvent);
      expect(mockPostMessage).toHaveBeenCalledWith(pressEvent);

      mockPostMessage.mockClear();

      globalThis.postMessage(releaseEvent);
      expect(mockPostMessage).toHaveBeenCalledWith(releaseEvent);
    });
  });
});