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

  /**
   * Whether notifications reach us over a network hop rather than straight from
   * the local BLE stack.
   *
   * Native BLE delivers the 2-7 notifications that make up one CS108 packet
   * ~1-3 ms apart. A networked transport (the ble-mcp-test bridge, an ESPHome
   * proxy) adds line buffering, a WebSocket, JSON parsing and the browser task
   * queue on top, which stretches the *tail* of the inter-fragment gap far
   * beyond that even when the typical case stays fast.
   *
   * Consumers use this to size fragment-reassembly timeouts. It says nothing
   * about throughput or reliability, only about latency shape.
   */
  isNetworked(): boolean;
}

/**
 * Message types for BLE communication over MessagePort
 */
export interface BLEMessage {
  type: 'ble:data' | 'ble:write' | 'ble:error' | 'ble:connected' | 'ble:disconnected';
  data?: Uint8Array;
  error?: string;
}