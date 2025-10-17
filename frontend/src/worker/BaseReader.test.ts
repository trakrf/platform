import { describe, it, expect, beforeEach, vi, type Mock } from 'vitest';
import { BaseReader } from './BaseReader.js';
import { ReaderState, ReaderMode, type ReaderModeType, type ReaderSettings } from './types/reader.js';
import { postWorkerEvent, WorkerEventType } from './types/events.js';

// Create a concrete test implementation of BaseReader
class TestReader extends BaseReader {
  public onConnectMock: Mock;
  public onDisconnectMock: Mock;
  public handleBleDataMock: Mock;
  public setModeMock: Mock;
  public setSettingsMock: Mock;
  public startScanningMock: Mock;
  public stopScanningMock: Mock;

  constructor() {
    super();
    this.onConnectMock = vi.fn();
    this.onDisconnectMock = vi.fn();
    this.handleBleDataMock = vi.fn();
    this.setModeMock = vi.fn(async (mode: ReaderModeType) => {
      this.readerMode = mode;
      this.readerState = ReaderState.CONNECTED;
      postWorkerEvent({
        type: WorkerEventType.READER_STATE_CHANGED,
        payload: { readerState: ReaderState.CONNECTED }
      });
    });
    this.setSettingsMock = vi.fn();
    this.startScanningMock = vi.fn(async () => {
      this.readerState = ReaderState.SCANNING;
    });
    this.stopScanningMock = vi.fn(async () => {
      this.readerState = ReaderState.CONNECTED;
    });
  }

  protected async onConnect(): Promise<void> {
    return this.onConnectMock();
  }

  protected async onDisconnect(): Promise<void> {
    return this.onDisconnectMock();
  }

  protected handleBleData(data: Uint8Array): void {
    this.handleBleDataMock(data);
  }

  async setMode(mode: ReaderModeType): Promise<void> {
    return this.setModeMock(mode);
  }

  async setSettings(settings: ReaderSettings): Promise<void> {
    return this.setSettingsMock(settings);
  }

  async startScanning(): Promise<void> {
    return this.startScanningMock();
  }

  async stopScanning(): Promise<void> {
    return this.stopScanningMock();
  }

  // Expose protected methods for testing
  public testSendCommand(data: Uint8Array): void {
    this.sendCommand(data);
  }
}

describe('BaseReader', () => {
  let reader: TestReader;
  let postMessageSpy: Mock;

  beforeEach(() => {
    reader = new TestReader();
    postMessageSpy = vi.fn();
    globalThis.postMessage = postMessageSpy;
  });

  describe('connect()', () => {
    it('should transition through connection states correctly', async () => {
      const result = await reader.connect();

      expect(result).toBe(true);
      expect(reader.onConnectMock).toHaveBeenCalledOnce();
      // BaseReader no longer calls setMode internally - DeviceManager handles that
      expect(reader.setModeMock).not.toHaveBeenCalled();
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
      // Mode is not set during connection anymore
      expect(reader.getMode()).toBeNull();
    });

    it('should emit READER_STATE_CHANGED events during connection', async () => {
      await reader.connect();

      // Should emit CONNECTING state first
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: WorkerEventType.READER_STATE_CHANGED,
        payload: { readerState: ReaderState.CONNECTING }
      }));

      // Then READY state after setMode completes
      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: WorkerEventType.READER_STATE_CHANGED,
        payload: { readerState: ReaderState.CONNECTED }
      }));
    });

    it('should handle connection failure and reset state', async () => {
      const error = new Error('Connection failed');
      reader.onConnectMock.mockRejectedValueOnce(error);

      await expect(reader.connect()).rejects.toThrow('Connection failed');
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);

      // Should emit DISCONNECTED state on failure
      expect(postMessageSpy).toHaveBeenLastCalledWith(expect.objectContaining({
        type: WorkerEventType.READER_STATE_CHANGED,
        payload: { readerState: ReaderState.DISCONNECTED }
      }));
    });
  });

  describe('disconnect()', () => {
    beforeEach(async () => {
      await reader.connect();
      postMessageSpy.mockClear();
    });

    it('should stop scanning if active before disconnecting', async () => {
      // Start scanning first
      await reader.startScanning();
      expect(reader.getState()).toBe(ReaderState.SCANNING);

      await reader.disconnect();

      expect(reader.stopScanningMock).toHaveBeenCalledOnce();
      expect(reader.onDisconnectMock).toHaveBeenCalledOnce();
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
      expect(reader.getMode()).toBeNull();
    });

    it('should disconnect directly if not scanning', async () => {
      await reader.disconnect();

      expect(reader.stopScanningMock).not.toHaveBeenCalled();
      expect(reader.onDisconnectMock).toHaveBeenCalledOnce();
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
    });

    it('should close MessagePort on disconnect', async () => {
      const mockPort = {
        close: vi.fn(),
        postMessage: vi.fn(),
        onmessage: null as any
      } as unknown as MessagePort;

      reader.setTransportPort(mockPort);
      await reader.disconnect();

      expect(mockPort.close).toHaveBeenCalledOnce();
    });

    it('should emit READER_STATE_CHANGED event', async () => {
      await reader.disconnect();

      expect(postMessageSpy).toHaveBeenCalledWith(expect.objectContaining({
        type: WorkerEventType.READER_STATE_CHANGED,
        payload: { readerState: ReaderState.DISCONNECTED }
      }));
    });

    it('should always end in DISCONNECTED state even on error', async () => {
      const error = new Error('Disconnect failed');
      reader.onDisconnectMock.mockRejectedValueOnce(error);

      await expect(reader.disconnect()).rejects.toThrow('Disconnect failed');
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
      expect(reader.getMode()).toBeNull();
    });
  });

  describe('getters', () => {
    it('should return current state', () => {
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
    });

    it('should return current mode', async () => {
      expect(reader.getMode()).toBeNull();

      await reader.connect();
      // Mode is not set during connection - remains null until explicitly set
      expect(reader.getMode()).toBeNull();

      // Now explicitly set mode
      await reader.setMode(ReaderMode.IDLE);
      expect(reader.getMode()).toBe(ReaderMode.IDLE);
    });

    it('should return copy of settings', () => {
      const settings = reader.getSettings();
      expect(settings).toEqual({});

      // Verify it's a copy, not the original
      settings.session = 1;
      expect(reader.getSettings()).toEqual({});
    });
  });

  describe('setTransportPort()', () => {
    it('should set up MessagePort with message handler', () => {
      const mockPort = {
        close: vi.fn(),
        postMessage: vi.fn(),
        onmessage: null as any
      } as unknown as MessagePort;

      reader.setTransportPort(mockPort);

      expect(mockPort.onmessage).toBeDefined();
      expect(typeof mockPort.onmessage).toBe('function');
    });

    it('should handle incoming BLE data messages', () => {
      const mockPort = {
        close: vi.fn(),
        postMessage: vi.fn(),
        onmessage: null as any
      } as unknown as MessagePort;

      reader.setTransportPort(mockPort);

      const testData = new Uint8Array([0x01, 0x02, 0x03]);
      const event = new MessageEvent('message', {
        data: {
          type: 'ble:data',
          data: testData
        }
      });

      mockPort.onmessage!(event);

      expect(reader.handleBleDataMock).toHaveBeenCalledWith(testData);
    });

    it('should ignore non-BLE data messages', () => {
      const mockPort = {
        close: vi.fn(),
        postMessage: vi.fn(),
        onmessage: null as any
      } as unknown as MessagePort;

      reader.setTransportPort(mockPort);

      const event = new MessageEvent('message', {
        data: {
          type: 'other:message',
          data: 'test'
        }
      });

      mockPort.onmessage!(event);

      expect(reader.handleBleDataMock).not.toHaveBeenCalled();
    });
  });

  describe('sendCommand()', () => {
    it('should send command through MessagePort', () => {
      const mockPort = {
        close: vi.fn(),
        postMessage: vi.fn(),
        onmessage: null as any
      } as unknown as MessagePort;

      reader.setTransportPort(mockPort);

      const testData = new Uint8Array([0xA7, 0xB3, 0x00, 0x01]);
      reader.testSendCommand(testData);

      expect(mockPort.postMessage).toHaveBeenCalledWith({
        type: 'ble:write',
        data: testData
      });
    });

    it('should throw error if port not initialized', () => {
      const testData = new Uint8Array([0xA7, 0xB3, 0x00, 0x01]);

      expect(() => reader.testSendCommand(testData)).toThrow('Transport port not initialized');
    });
  });


  describe('lifecycle integration', () => {
    it('should handle full connect -> scan -> stop -> disconnect cycle', async () => {
      // Connect
      await reader.connect();
      expect(reader.getState()).toBe(ReaderState.CONNECTED);
      // Mode is not set during connection anymore
      expect(reader.getMode()).toBeNull();

      // Set mode explicitly (as DeviceManager would)
      await reader.setMode(ReaderMode.INVENTORY);
      expect(reader.getMode()).toBe(ReaderMode.INVENTORY);

      // Start scanning
      await reader.startScanning();
      expect(reader.getState()).toBe(ReaderState.SCANNING);

      // Stop scanning
      await reader.stopScanning();
      expect(reader.getState()).toBe(ReaderState.CONNECTED);

      // Disconnect
      await reader.disconnect();
      expect(reader.getState()).toBe(ReaderState.DISCONNECTED);
      expect(reader.getMode()).toBeNull();

      // Verify all lifecycle methods were called
      expect(reader.onConnectMock).toHaveBeenCalledOnce();
      expect(reader.setModeMock).toHaveBeenCalledOnce(); // Called explicitly now
      expect(reader.startScanningMock).toHaveBeenCalledOnce();
      expect(reader.stopScanningMock).toHaveBeenCalledOnce(); // Only called once manually, disconnect checks state first
      expect(reader.onDisconnectMock).toHaveBeenCalledOnce();
    });
  });
});