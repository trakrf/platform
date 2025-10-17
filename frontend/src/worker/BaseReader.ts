/**
 * BaseReader - Common reader base implementation
 * 
 * Provides functionality shared by all RFID readers:
 * - BLE transport layer (MessagePort communication)
 * - State management (ReaderState, ReaderMode)
 * - RAIN RFID settings (session, target, Q value)
 * - Domain event emission (postMessage)
 * - Standard lifecycle (connect/disconnect)
 * 
 * Vendor-specific implementations handle:
 * - Command byte construction
 * - Response byte parsing
 * - Protocol-specific sequences
 * - Hardware-specific constants
 */

import {
  ReaderState,
  type IReader,
  type ReaderModeType,
  type ReaderStateType,
  type ReaderSettings
} from './types/reader.js';
import { postWorkerEvent, WorkerEventType } from './types/events.js';
import { logger } from './utils/logger.js';


/**
 * Base implementation for RFID readers
 * 
 * This base class provides common transport and state management.
 * Vendor-specific implementations (CS108Reader, ZebraReader, etc.)
 * extend this class and implement their protocol-specific logic.
 */
export abstract class BaseReader implements IReader {
  protected readerState: ReaderStateType = ReaderState.DISCONNECTED;
  protected readerMode: ReaderModeType | null = null;
  protected readerSettings: ReaderSettings = {};
  protected port?: MessagePort;

  /**
   * Internal method to update reader state and always emit the event
   * This ensures we never forget to notify the UI of state changes
   */
  protected setReaderState(newState: ReaderStateType): void {
    const previousState = this.readerState;
    this.readerState = newState;

    logger.debug(`[BaseReader] State transition: ${previousState} → ${newState}`);

    postWorkerEvent({
      type: WorkerEventType.READER_STATE_CHANGED,
      payload: { readerState: newState }
    });
  }

  /**
   * Internal method to update reader mode and always emit the event
   * This ensures we never forget to notify the UI of mode changes
   */
  protected setReaderMode(newMode: ReaderModeType | null): void {
    const previousMode = this.readerMode;
    this.readerMode = newMode;

    logger.debug(`[BaseReader] Mode transition: ${previousMode} → ${newMode}`);

    postWorkerEvent({
      type: WorkerEventType.READER_MODE_CHANGED,
      payload: { mode: newMode }
    });
  }
  
  // IReader Interface - Common Implementation
  
  async connect(): Promise<boolean> {
    logger.debug('[BaseReader] connect() - Starting connection sequence');
    try {
      this.setReaderState(ReaderState.CONNECTING);

      // Let child class perform vendor-specific connection
      logger.debug('[BaseReader] Calling onConnect() for vendor-specific initialization');
      await this.onConnect();
      logger.debug('[BaseReader] onConnect() completed successfully');

      // DeviceManager will handle setting initial mode
      // But we need to transition out of CONNECTING state
      this.setReaderState(ReaderState.CONNECTED);

      logger.debug(`[BaseReader] connect() - Connection sequence complete. State: ${this.readerState}, Mode: ${this.readerMode}`);

      return true;
    } catch (error) {
      logger.error('[BaseReader] connect() - Connection failed:', error);
      this.setReaderState(ReaderState.DISCONNECTED);
      throw error;
    }
  }
  
  async disconnect(): Promise<void> {
    try {
      // Stop any active scanning
      if (this.readerState === ReaderState.SCANNING) {
        await this.stopScanning();
      }

      // Let child class perform vendor-specific disconnection
      await this.onDisconnect();

      this.setReaderState(ReaderState.DISCONNECTED);
      this.setReaderMode(null);
      
      // Clean up MessagePort
      if (this.port) {
        this.port.close();
        this.port = undefined;
      }
    } catch (error) {
      // Always end in DISCONNECTED state
      this.setReaderState(ReaderState.DISCONNECTED);
      this.setReaderMode(null);
      throw error;
    }
  }
  
  getMode(): ReaderModeType | null {
    return this.readerMode;
  }
  
  getState(): ReaderStateType {
    return this.readerState;
  }
  
  getSettings(): ReaderSettings {
    return { ...this.readerSettings };
  }
  
  
  // Transport Management
  
  /**
   * Set the MessagePort for BLE communication
   * All readers use the same transport mechanism
   */
  public setTransportPort(port: MessagePort): void {
    this.port = port;
    this.port.onmessage = (event) => {
      if (event.data?.type === 'ble:data' && event.data.data instanceof Uint8Array) {
        // Delegate protocol-specific parsing to child class
        this.handleBleData(event.data.data);
      } else if (event.data?.type === 'ble:disconnected') {
        // Handle unexpected transport disconnection
        this.handleTransportDisconnect();
      }
    };
  }
  
  /**
   * Send raw bytes to hardware via MessagePort
   * Child classes build protocol-specific bytes
   */
  protected sendCommand(data: Uint8Array): void {
    if (!this.port) {
      throw new Error('Transport port not initialized');
    }

    this.port.postMessage({
      type: 'ble:write',
      data
    });
  }
  
  
  // Abstract Methods - Must be implemented by vendor-specific classes
  
  /**
   * Perform vendor-specific connection logic
   */
  protected abstract onConnect(): Promise<void>;
  
  /**
   * Perform vendor-specific disconnection logic
   */
  protected abstract onDisconnect(): Promise<void>;
  
  /**
   * Handle incoming bytes from hardware
   * Child classes parse protocol-specific formats
   */
  protected abstract handleBleData(data: Uint8Array): void;

  /**
   * Handle transport disconnection (BLE connection lost)
   * Notifies DeviceManager that transport is gone and singleton should be cleaned up
   */
  protected handleTransportDisconnect(): void {
    logger.warn('[BaseReader] Transport disconnected unexpectedly');

    // Set internal state to disconnected
    this.setReaderState(ReaderState.DISCONNECTED);
    this.setReaderMode(null);

    // Notify DeviceManager to destroy singleton (the "suicide" pattern)
    postWorkerEvent({
      type: WorkerEventType.TRANSPORT_DISCONNECTED,
      payload: { reason: 'BLE connection lost' }
    });
  }
  
  /**
   * Set reader mode with vendor-specific configuration
   */
  abstract setMode(mode: ReaderModeType, settings?: ReaderSettings): Promise<void>;
  
  /**
   * Update reader settings with vendor-specific commands
   */
  abstract setSettings(settings: ReaderSettings): Promise<void>;
  
  /**
   * Start scanning in current mode
   */
  abstract startScanning(): Promise<void>;
  
  /**
   * Stop scanning in current mode
   */
  abstract stopScanning(): Promise<void>;
}