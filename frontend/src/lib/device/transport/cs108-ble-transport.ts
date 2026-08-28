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
    // NOT writeValueWithResponse / writeValueWithoutResponse. Real Web Bluetooth
    // has both and `src/types/web-bluetooth.d.ts` describes them correctly — but
    // this block is a `declare global` augmentation, and what it actually types
    // at the call site is ble-mcp-test's mock, where neither method exists.
    //
    // Declaring them here was TRA-1187's motivating example: a hand-written
    // interface asserting a shape nobody had checked, partially satisfied so it
    // read as validated. Nothing ever called them, so the fiction was harmless —
    // but a consumer switching to `writeValueWithResponse()` in good faith would
    // have thrown on the first write of every @hardware run.
    //
    // ble-mcp-test 0.9.0 makes the absence enforceable: an arm-A conformance
    // check asserts both are absent, deferred until TRA-1153 5b-client gives
    // them something to resolve on. Add them back when that lands, deliberately,
    // and not before.
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
    /** Wall-clock point past which this write must be abandoned, not retried. */
    deadline: number;
  }> = [];
  private commandInProgress = false;
  private readonly MAX_QUEUE_LENGTH = 5;
  
  // Bound event handlers
  private boundHandleNotifications: (event: Event) => void;
  private boundHandleDisconnect: (event: Event) => void;
  
  /**
   * How long a single write may spend retrying, in total.
   *
   * Deliberately under `CommandManager.DEFAULT_TIMEOUT` (2500 ms,
   * `worker/cs108/command.ts`). The retries live *inside* a command's lifetime,
   * so a retry budget larger than the command timeout means sleeping on behalf
   * of a command that has already rejected — and then issuing the write anyway.
   * A write nobody is waiting for is not a retry, it is an injection into the
   * command stream. The old defaults summed to 7000 ms against that 2500 ms
   * timeout (TRA-1179).
   */
  private readonly WRITE_BUDGET_MS = 2000;

  constructor(config: CS108BLETransportConfig = {}) {
    this.retryCount = config.retryCount || 3;
    // Sums to 1750 ms — inside WRITE_BUDGET_MS, which is inside the command timeout.
    this.retryDelays = config.retryDelays || [250, 500, 1000];
    
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
      } catch (e) {
        // Report, do not rethrow: teardown must still complete. Visibility is
        // the requirement, not propagation — the same trade #583 made for
        // writes. This catch was dead while stopNotifications() was a no-op;
        // TRA-1153 makes it a real gate that can reject (TRA-1179).
        this.reportTransportError(
          `stopNotifications failed: ${e instanceof Error ? e.message : String(e)}`
        );
      }

      // Unhook regardless — a listener left on a characteristic we are dropping
      // is exactly the orphan cleanup() exists to prevent.
      this.notifyCharacteristic.removeEventListener(
        'characteristicvaluechanged',
        this.boundHandleNotifications
      );
    }
    
    // Remove disconnect listener
    if (this.device && typeof this.device.removeEventListener === 'function') {
      this.device.removeEventListener(
        'gattserverdisconnected',
        this.boundHandleDisconnect
      );
    }
    
    // Disconnect GATT, and wait for it.
    //
    // Real Web Bluetooth returns void here, so this is a no-op await. The
    // ble-mcp-test mock returns a settleable value, and per TRA-1153 the
    // command-path release lands when the *server* processes the socket close —
    // not when `server.connected` flips, which happens synchronously before it.
    // Fire-and-forget lets the next connect race ahead of the release and be
    // refused as busy by our own previous session, which reads as an ownership
    // bug rather than a lifecycle one (TRA-1179).
    if (this.device?.gatt?.connected) {
      await Promise.resolve(this.device.gatt.disconnect());
    }
    
    // Notify worker and close port
    if (this.messagePort) {
      this.messagePort.postMessage({ 
        type: 'ble:disconnected' 
      } as BLEMessage);
      this.messagePort.close();
    }
    
    // Clean up — cleanup() owns the test exposure too, so an unexpected GATT
    // drop clears exactly as much as an explicit disconnect (TRA-1179).
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
        this.reportTransportError('Command queue full, write dropped');
        resolve(false);
        return;
      }
      
      this.commandQueue.push({
        data,
        resolve,
        retriesLeft: this.retryCount,
        deadline: Date.now() + this.WRITE_BUDGET_MS
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
        this.reportTransportError('Transport not connected, write dropped');
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
      
      const command_ = command;
      const delayIndex = this.retryCount - command_.retriesLeft;
      const delay = this.retryDelays[Math.min(delayIndex, this.retryDelays.length - 1)];

      // A retry is only worth attempting if the command that owns it will still
      // be listening when the write lands. Past the deadline it is not a retry,
      // it is a stale command injected into the stream (TRA-1179).
      const withinBudget = Date.now() + delay < command_.deadline;

      if (this.isRetryable(errorMessage) && command_.retriesLeft > 0 && withinBudget) {
        command_.retriesLeft--;
        const attempt = this.retryCount - command_.retriesLeft;

        // Leave a trace. This branch used to be silent — the only marker was a
        // comment — so a retry firing and a retry never firing looked identical
        // from outside. That is how `retryDelays` came to sum to 7000ms inside a
        // 2500ms command timeout without anyone noticing: the path had been
        // unobservable for as long as it had been wrong (TRA-1179).
        console.warn(
          `[CS108BLETransport] write retry ${attempt}/${this.retryCount} in ${delay}ms: ${errorMessage}`
        );

        await new Promise(r => setTimeout(r, delay));

        // Put command back at front of queue
        this.commandQueue.unshift(command_);
      } else {
        this.reportTransportError(errorMessage);
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
  /**
   * Which write failures are worth another attempt.
   *
   * `GATT Server is disconnected` is deliberately NOT here. A disconnected
   * server does not recover by waiting, so retrying spends the budget for
   * nothing — and if the link *does* come back, the retry lands a stale command
   * on a fresh connection, which is the most harmful outcome available
   * (TRA-1179).
   */
  private isRetryable(errorMessage: string): boolean {
    return (
      errorMessage.includes('GATT operation already in progress') ||
      errorMessage.includes('Device busy')
    );
  }

  private reportTransportError(error: string): void {
    // Not "write failed" any more — teardown reports through here too.
    console.error('[CS108BLETransport]', error);
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
  /**
   * Release everything this transport owns.
   *
   * This is the single teardown owner: both the explicit `disconnect()` and the
   * `gattserverdisconnected` handler route here, so they cannot clear different
   * amounts. They used to — `disconnect()` deleted `__TRANSPORT_MANAGER__` and
   * `handleDisconnect()` did not, which left the e2e trigger helpers injecting
   * into an orphaned characteristic after any unexpected drop. That surfaced as
   * `NOTIFY_CHAR_NOT_FOUND` on real hardware (TRA-1179).
   */
  private async cleanup(): Promise<void> {
    this.device = null;
    this.server = null;
    this.service = null;
    this.writeCharacteristic = null;
    this.notifyCharacteristic = null;

    // The test hook points at a characteristic that no longer receives anything.
    if (typeof window !== 'undefined') {
      const testWindow = window as TestWindow;
      if (testWindow.__TRANSPORT_MANAGER__) {
        delete testWindow.__TRANSPORT_MANAGER__;
      }
    }

    if (this.messagePort) {
      this.messagePort.close();
      this.messagePort = null;
    }
  }
}