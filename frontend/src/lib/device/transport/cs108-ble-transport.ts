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

// Default device name for CS108
export const CS108_DEVICE_NAME = 'CS108';

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
  deviceNameFilter?: string;
  retryCount?: number;
  retryDelays?: number[];
}

export class CS108BLETransport implements Transport {
  // CS108 UUIDs - hardcoded, not configurable
  private readonly serviceUUID = CS108_BLE_SERVICE_UUID;
  private readonly writeUUID = CS108_BLE_WRITE_UUID;
  private readonly notifyUUID = CS108_BLE_NOTIFY_UUID;
  
  // Configurable options
  private readonly deviceNameFilter: string;
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
    this.deviceNameFilter = config.deviceNameFilter || '';
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
      const filters: BluetoothLEScanFilter[] = [
        { services: [this.serviceUUID] }
      ];
      
      if (this.deviceNameFilter) {
        filters.push({ name: this.deviceNameFilter });
      }
      
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
    return !!(this.device && this.server && this.writeCharacteristic);
  }
  
  /**
   * Get transport type
   */
  getType(): string {
    return 'ble';
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
    
    // Clone data immediately to avoid DataView detachment
    const data = new Uint8Array(value.buffer.slice(0));
    
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
  private async queueWrite(data: Uint8Array): Promise<void> {
    return new Promise<void>((resolve) => {
      if (this.commandQueue.length >= this.MAX_QUEUE_LENGTH) {
        console.warn('Command queue full, rejecting write');
        resolve();
        return;
      }
      
      this.commandQueue.push({
        data,
        resolve: () => resolve(),
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
        // Not connected, skipping command
        command.resolve(false);
        return;
      }

      // Writing to BLE characteristic
      await this.writeCharacteristic!.writeValue(command.data);
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
        console.error('Write failed:', errorMessage);
        command.resolve(false);
        
        // Send error to worker
        if (this.messagePort) {
          this.messagePort.postMessage({
            type: 'ble:error',
            error: errorMessage
          } as BLEMessage);
        }
      }
    } finally {
      this.commandInProgress = false;
      this.processNextCommand();
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