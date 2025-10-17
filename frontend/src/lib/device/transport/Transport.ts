/**
 * Transport abstraction interface
 * Provides uniform interface for BLE, USB, Bridge, and Mock transports
 */

export interface Transport {
  /**
   * Connect to the device and return a MessagePort for communication
   * @returns MessagePort for bidirectional communication with worker
   */
  connect(): Promise<MessagePort>;
  
  /**
   * Disconnect from the device
   */
  disconnect(): Promise<void>;
  
  /**
   * Check if transport is connected
   */
  isConnected(): boolean;
  
  /**
   * Get transport type identifier
   */
  getType(): string;
}

/**
 * Message types for BLE communication over MessagePort
 */
export interface BLEMessage {
  type: 'ble:data' | 'ble:write' | 'ble:error' | 'ble:connected' | 'ble:disconnected';
  data?: Uint8Array;
  error?: string;
}