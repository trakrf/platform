/**
 * BLE Transport implementation using Web Bluetooth API
 * Based on proven patterns from lib/rfid/cs108/transportManager.ts
 */

import type { Transport, BLEMessage } from './Transport';

// Type for test environment window extensions
interface TestWindow extends Window {
  __TRANSPORT_MANAGER__?: {
    notifyCharacteristic: BluetoothRemoteGATTCharacteristic | null;
  };
}

// CS108 BLE Service and Characteristic UUIDs
// Using full 128-bit UUIDs for clarity and Web Bluetooth compatibility
// Bridge server will normalize these as needed
export const CS108_BLE_SERVICE_UUID = '00009800-0000-1000-8000-00805f9b34fb';
export const CS108_BLE_WRITE_UUID = '00009900-0000-1000-8000-00805f9b34fb';
export const CS108_BLE_NOTIFY_UUID = '00009901-0000-1000-8000-00805f9b34fb';

// Web Bluetooth API type declarations
declare global {
  interface BluetoothDevice {
    id: string;
    name?: string;
    gatt?: BluetoothRemoteGATTServer;
    addEventListener(type: string, listener: EventListener): void;
    removeEventListener(type: string, listener: EventListener): void;
  }

  interface BluetoothRemoteGATTServer {
    connected: boolean;
    device: BluetoothDevice;
    connect(): Promise<BluetoothRemoteGATTServer>;
    disconnect(): void;
    getPrimaryService(service: string | number): Promise<BluetoothRemoteGATTService>;
  }

  interface BluetoothRemoteGATTService {
    uuid: string;
    device: BluetoothDevice;
    getCharacteristic(characteristic: string | number): Promise<BluetoothRemoteGATTCharacteristic>;
  }

  interface BluetoothRemoteGATTCharacteristic {
    uuid: string;
    service: BluetoothRemoteGATTService;
    value?: DataView;
    readValue(): Promise<DataView>;
    writeValue(value: BufferSource): Promise<void>;
    writeValueWithResponse(value: BufferSource): Promise<void>;
    writeValueWithoutResponse(value: BufferSource): Promise<void>;
    startNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
    stopNotifications(): Promise<BluetoothRemoteGATTCharacteristic>;
    addEventListener(type: 'characteristicvaluechanged', listener: (event: Event) => void): void;
    removeEventListener(type: 'characteristicvaluechanged', listener: (event: Event) => void): void;
  }

  interface BluetoothLEScanFilter {
    services?: Array<string | number>;
    name?: string;
    namePrefix?: string;
  }
}

// Minimal config - CS108 UUIDs are hardcoded
export interface CS108BLETransportConfig {
  retryCount?: number;
  retryDelays?: number[];
}

export class CS108BLETransport implements Transport {
  // CS108 UUIDs - hardcoded, not configurable
  private readonly serviceUUID = CS108_BLE_SERVICE_UUID;
  private readonly writeUUID = CS108_BLE_WRITE_UUID;
  private readonly notifyUUID = CS108_BLE_NOTIFY_UUID;
  
  // Configurable options
  private readonly retryCount: number;
  private readonly retryDelays: number[];
  private device: BluetoothDevice | null = null;
  private server: BluetoothRemoteGATTServer | null = null;
  private service: BluetoothRemoteGATTService | null = null;
  private writeCharacteristic: BluetoothRemoteGATTCharacteristic | null = null;
  private notifyCharacteristic: BluetoothRemoteGATTCharacteristic | null = null;
  private messagePort: MessagePort | null = null;
  
  // Command queue for reliable writes
  private commandQueue: Array<{
    data: Uint8Array;
    resolve: (success: boolean) => void;
    retriesLeft: number;
  }> = [];
  private commandInProgress = false;
  private readonly MAX_QUEUE_LENGTH = 5;
  
  // Bound event handlers
  private boundHandleNotifications: (event: Event) => void;
  private boundHandleDisconnect: (event: Event) => void;
  
  constructor(config: CS108BLETransportConfig = {}) {
    this.retryCount = config.retryCount || 3;
    this.retryDelays = config.retryDelays || [500, 1500, 5000];
    
    // Bind event handlers
    this.boundHandleNotifications = this.handleNotifications.bind(this);
    this.boundHandleDisconnect = this.handleDisconnect.bind(this);
  }
  
  /**
   * Connect to BLE device and set up MessagePort communication
   */
  async connect(): Promise<MessagePort> {
    if (!this.isSupported()) {
      throw new Error('Web Bluetooth is not supported in this browser');
    }
    
    try {
      // Request device selection
      // Selection is by service UUID only. Web Bluetooth ORs the filters array,
      // so a second { name } filter would WIDEN the match rather than narrow it.
      const filters: BluetoothLEScanFilter[] = [
        { services: [this.serviceUUID] }
      ];

      this.device = await navigator.bluetooth.requestDevice({
        filters,
        optionalServices: [this.serviceUUID]
      });
      
      if (!this.device) {
        throw new Error('No device selected');
      }
      
      // Set up disconnect listener (if supported by the device implementation)
      if (typeof this.device.addEventListener === 'function') {
        this.device.addEventListener('gattserverdisconnected', this.boundHandleDisconnect);
      }
      
      // Connect to GATT server
      this.server = await this.device.gatt!.connect();
      
      // Get service
      this.service = await this.server.getPrimaryService(this.serviceUUID);
      
      // Get characteristics
      this.writeCharacteristic = await this.service.getCharacteristic(this.writeUUID);
      this.notifyCharacteristic = await this.service.getCharacteristic(this.notifyUUID);
      
      // Subscribe to notifications
      await this.notifyCharacteristic.startNotifications();
      this.notifyCharacteristic.addEventListener('characteristicvaluechanged', this.boundHandleNotifications);

      // Expose for E2E testing - allows simulateTriggerPress to inject notifications
      if (typeof window !== 'undefined' && (import.meta.env.MODE === 'test' || import.meta.env.MODE === 'development')) {
        const testWindow = window as TestWindow;
        testWindow.__TRANSPORT_MANAGER__ = {
          notifyCharacteristic: this.notifyCharacteristic
        };
        // Exposed __TRANSPORT_MANAGER__ for testing
      } else {
        // Not exposing __TRANSPORT_MANAGER__ (not in test mode)
      }

      // Create MessageChannel for worker communication
      const channel = new MessageChannel();
      this.messagePort = channel.port1;
      
      // Set up message handling from worker
      this.messagePort.onmessage = (event) => {
        // Received message from worker
        const message = event.data as BLEMessage;
        if (message.type === 'ble:write' && message.data) {
          // Queueing write command
          this.queueWrite(message.data);
        } else {
          // Ignoring non-write message
        }
      };
      
      // Notify worker of connection
      this.messagePort.postMessage({ 
        type: 'ble:connected' 
      } as BLEMessage);
      
      console.info(`Connected to ${this.device.name || 'BLE Device'}`);
      
      // Return port2 for worker
      return channel.port2;
      
    } catch (error) {
      // Clean up on error
      await this.cleanup();
      throw error;
    }
  }
  
  /**
   * Disconnect from BLE device
   */
  async disconnect(): Promise<void> {
    // Clear command queue
    this.clearCommandQueue('Device disconnecting');
    
    // Stop notifications
    if (this.notifyCharacteristic) {
      try {
        await this.notifyCharacteristic.stopNotifications();
        this.notifyCharacteristic.removeEventListener(
          'characteristicvaluechanged',
          this.boundHandleNotifications
        );
      } catch (e) {
        // Error stopping notifications
      }
    }
    
    // Remove disconnect listener
    if (this.device && typeof this.device.removeEventListener === 'function') {
      this.device.removeEventListener(
        'gattserverdisconnected',
        this.boundHandleDisconnect
      );
    }
    
    // Disconnect GATT
    if (this.device?.gatt?.connected) {
      this.device.gatt.disconnect();
    }
    
    // Notify worker and close port
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
      this.messagePort.close();
    }
    
    // Clean up test exposure
    if (typeof window !== 'undefined') {
      const testWindow = window as TestWindow;
      if (testWindow.__TRANSPORT_MANAGER__) {
        // Clearing __TRANSPORT_MANAGER__ on disconnect
        delete testWindow.__TRANSPORT_MANAGER__;
      }
    }

    // Clean up
    await this.cleanup();
  }
  
  /**
   * Check if connected
   */
  isConnected(): boolean {
    // server.connected matters: between a GATT drop and handleDisconnect running,
    // the objects are all still here while the link is gone.
    return !!(this.device && this.server?.connected && this.writeCharacteristic);
  }
  
  /**
   * Get transport type
   */
  getType(): string {
    return 'ble';
  }

  /**
   * Real Web Bluetooth is local; the ble-mcp-test mock is not.
   *
   * The mock replaces navigator.bluetooth in place and flags itself with
   * __webBluetoothBridged, so getType() still reports 'ble' while every
   * notification is in fact arriving over a WebSocket from the bridge host.
   * That flag is the only thing that distinguishes the two here.
   */
  isNetworked(): boolean {
    return typeof window !== 'undefined' && !!window.__webBluetoothBridged;
  }
  
  /**
   * Check if Web Bluetooth is supported
   */
  private isSupported(): boolean {
    return 'bluetooth' in navigator;
  }
  
  /**
   * Handle BLE notifications
   */
  private handleNotifications(event: Event): void {
    const characteristic = event.target as unknown as BluetoothRemoteGATTCharacteristic;
    const value = characteristic.value;
    
    if (!value) return;

    // Clone data immediately to avoid DataView detachment.
    //
    // Honour byteOffset/byteLength: a DataView is a *view* onto its backing
    // ArrayBuffer, and that buffer may be larger than the notification (a pooled
    // allocator, a Node Buffer, or a binary/base64 WebSocket frame decoded into a
    // shared buffer). Copying `value.buffer` wholesale would hand the worker the
    // whole pool starting at offset 0 — garbage packets rather than a clean error.
    // Exact-size buffers at offset 0 (today's JSON text frames) are unaffected.
    const data = new Uint8Array(value.buffer, value.byteOffset, value.byteLength).slice();

    // Send to worker via MessagePort
    if (this.messagePort) {
      this.messagePort.postMessage({
        type: 'ble:data',
        data
      } as BLEMessage);
    }
  }
  
  /**
   * Handle disconnection
   */
  private handleDisconnect(): void {
    // BLE device disconnected
    
    // Clear command queue
    this.clearCommandQueue('Device disconnected');
    
    // Notify worker
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
    }
    
    // Clean up
    this.cleanup();
  }
  
  /**
   * Queue a write operation with retry logic
   */
  private async queueWrite(data: Uint8Array): Promise<boolean> {
    return new Promise<boolean>((resolve) => {
      if (this.commandQueue.length >= this.MAX_QUEUE_LENGTH) {
        this.reportWriteFailure('Command queue full, write dropped');
        resolve(false);
        return;
      }
      
      this.commandQueue.push({
        data,
        resolve,
        retriesLeft: this.retryCount
      });
      
      this.processNextCommand();
    });
  }
  
  /**
   * Process next command in queue
   */
  private async processNextCommand(): Promise<void> {
    if (this.commandInProgress || this.commandQueue.length === 0) {
      return;
    }
    
    const command = this.commandQueue.shift()!;
    this.commandInProgress = true;
    
    try {
      if (!this.isConnected()) {
        this.reportWriteFailure('Transport not connected, write dropped');
        command.resolve(false);
        return;
      }

      // Writing to BLE characteristic
      // Create a new Uint8Array to ensure proper ArrayBuffer type
      await this.writeCharacteristic!.writeValue(new Uint8Array(command.data));
      // BLE write completed successfully
      command.resolve(true);
      
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      
      // Check if we should retry
      const shouldRetry = 
        errorMessage.includes('GATT operation already in progress') ||
        errorMessage.includes('Device busy') ||
        errorMessage.includes('GATT Server is disconnected');
      
      if (shouldRetry && command.retriesLeft > 0) {
        command.retriesLeft--;
        const delayIndex = this.retryCount - command.retriesLeft - 1;
        const delay = this.retryDelays[Math.min(delayIndex, this.retryDelays.length - 1)];
        
        // Retrying write after delay
        await new Promise(r => setTimeout(r, delay));
        
        // Put command back at front of queue
        this.commandQueue.unshift(command);
      } else {
        this.reportWriteFailure(errorMessage);
        command.resolve(false);
      }
    } finally {
      this.commandInProgress = false;
      this.processNextCommand();
    }
  }
  
  /**
   * Tell the worker a write did not reach the device.
   *
   * Without this the worker cannot distinguish "command sent, awaiting ACK" from
   * "command never left", so every dropped write surfaced only as the command's
   * own 5s timeout. Two of the three failure paths previously reported nothing.
   */
  private reportWriteFailure(error: string): void {
    console.error('Write failed:', error);
    if (this.messagePort) {
      this.messagePort.postMessage({ type: 'ble:error', error } as BLEMessage);
    }
  }
  
  /**
   * Clear command queue
   */
  private clearCommandQueue(_reason: string): void {
    if (this.commandQueue.length > 0) {
      // Clearing queued commands
      this.commandQueue.forEach(cmd => cmd.resolve(false));
      this.commandQueue = [];
    }
    this.commandInProgress = false;
  }
  
  /**
   * Clean up resources
   */
  private async cleanup(): Promise<void> {
    this.device = null;
    this.server = null;
    this.service = null;
    this.writeCharacteristic = null;
    this.notifyCharacteristic = null;
    
    if (this.messagePort) {
      this.messagePort.close();
      this.messagePort = null;
    }
  }
}