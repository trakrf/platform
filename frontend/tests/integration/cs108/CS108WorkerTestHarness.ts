/**
 * CS108 Worker Test Harness - BIDIRECTIONAL BYTE PIPE ARCHITECTURE
 *
 * This harness implements a pure bidirectional byte pipe for integration testing:
 * - Commands are sent fire-and-forget via sendRawBytes()
 * - ALL responses flow through the notification handler
 * - NO request-response patterns at the transport layer
 * - Intelligence exists ONLY in the worker's protocol layer
 *
 * INTEGRATION TEST RULES:
 *
 * ✅ ALLOWED - Public API Only:
 * - connect(), disconnect()
 * - setMode(), setSettings()
 * - startScanning(), stopScanning()
 * - waitForEvent(), getEvents()
 * - getReaderState(), getReaderMode() [read-only observation]
 *
 * ❌ FORBIDDEN - No Internal Access:
 * - Direct worker property access
 * - Private method calls
 * - State manipulation
 * - Request-response patterns in transport
 *
 * The harness bridges between:
 * - Test environment (public API calls, event assertions)
 * - CS108 Worker (production code, untouched)
 * - Real hardware (CS108 device via BLE bridge)
 *
 * If a test needs internal access to pass, the production code is broken.
 */

import { getEventByCode } from '@/worker/cs108/event';
import { CS108Reader } from '@/worker/cs108/reader';
import { ReaderMode, ReaderState } from '@/worker/types/reader';
import { WorkerEventType } from '@/worker/types/events';

import { RfidReaderTestClient } from '../ble-mcp-test/rfid-reader-test-client';
import type { ReaderSettings } from '@/worker/types/reader';

// Domain event types from spec
export type DataReadEvent = 
  | { type: 'INVENTORY'; data: InventoryRead[] }
  | { type: 'LOCATE'; data: LocateRead[] }
  | { type: 'BARCODE'; data: BarcodeRead[] };

export type SystemEvent =
  | { type: 'READER_STATE_CHANGED'; payload: { readerState: ReaderState } }
  | { type: 'READER_MODE_CHANGED'; payload: { mode: ReaderMode } }
  | { type: 'BATTERY_UPDATE'; payload: { percentage: number } }
  | { type: 'TRIGGER_STATE_CHANGED'; payload: { pressed: boolean } }
  | { type: 'SETTINGS_UPDATED'; payload: { settings: Partial<ReaderSettings> } }
  | { type: 'CONFIGURATION_COMPLETE'; payload: { mode: ReaderMode; duration: number } }
  | { type: 'CONFIGURATION_FAILED'; payload: { mode: ReaderMode; error: string } };

export type NotificationEvent = DataReadEvent | SystemEvent;

interface InventoryRead {
  epc: string;
  rssi: number;
  timestamp: number;
}

interface LocateRead {
  nbRssi: number;
  wbRssi?: number;
  phase?: number;
  timestamp: number;
}

interface BarcodeRead {
  symbology: string;
  data: string;
  timestamp: number;
}

/**
 * Mock MessagePort for BLE transport simulation
 */
class MockMessagePort implements MessagePort {
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onmessageerror: ((ev: MessageEvent) => void) | null = null;
  
  private otherPort?: MockMessagePort;
  
  postMessage(message: { type: string; data?: Uint8Array } | NotificationEvent): void {
    // Simulate async message delivery to other port
    if (this.otherPort?.onmessage) {
      setTimeout(() => {
        this.otherPort!.onmessage!(new MessageEvent('message', { data: message }));
      }, 0);
    }
  }
  
  start(): void {}
  close(): void {}
  
  addEventListener(): void {}
  removeEventListener(): void {}
  dispatchEvent(): boolean { return true; }
  
  // Test helper to link ports
  linkTo(other: MockMessagePort): void {
    this.otherPort = other;
    other.otherPort = this;
  }
  
  // Test helper to simulate incoming BLE data
  simulateMessage(data: { type: string; data?: Uint8Array }): void {
    if (this.onmessage) {
      this.onmessage(new MessageEvent('message', { data }));
    }
  }
}

/**
 * Test harness for CS108 worker implementation
 */
// Transport message types
export interface TransportMessage {
  type: 'command' | 'response' | 'notification';
  bytes: Uint8Array;
  timestamp: number;
  direction: 'outbound' | 'inbound';
  commandName?: string; // Parsed command name
}

export class CS108WorkerTestHarness {
  private rfidClient: RfidReaderTestClient;
  private worker: CS108Reader; // CS108Reader instance
  private blePort: MockMessagePort;
  private workerPort: MockMessagePort;
  private domainEvents: NotificationEvent[] = [];
  private eventWaiters: Map<string, (event: NotificationEvent) => void> = new Map();
  private transportMessages: TransportMessage[] = [];

  // Simple raw traffic capture with timestamps
  public traffic: Array<{
    direction: 'TX' | 'RX',
    bytes: Uint8Array,
    timestamp: number
  }> = [];
  
  constructor() {
    this.rfidClient = new RfidReaderTestClient();
    this.blePort = new MockMessagePort();
    this.workerPort = new MockMessagePort();
    this.blePort.linkTo(this.workerPort);
  }
  
  /**
   * Initialize harness and optionally connect to real CS108 hardware
   */
  async initialize(connectToHardware: boolean = false): Promise<void> {
    // Only connect to real CS108 if needed
    if (connectToHardware) {
      await this.rfidClient.connect();
      
      // Set up notification handler for autonomous notifications
      this.rfidClient.onNotification((data: Uint8Array) => {
        // Capture traffic with timestamp
        this.traffic.push({
          direction: 'RX',
          bytes: new Uint8Array(data),
          timestamp: Date.now()
        });

        // Only log received data in verbose mode (uncomment for debugging)
        // const hexDump = Array.from(data).map(b => b.toString(16).padStart(2, '0').toUpperCase()).join(' ');
        // console.log(`[Harness] RX from bridge:`, hexDump);

        // Debug: Decode the packet (uncomment for debugging)
        // if (data.length >= 11 && data[8] === 0xa0) {
        //   const cmdCode = data[9];
        //   const cmdName = cmdCode === 0x00 ? 'BATTERY_VOLTAGE' : cmdCode === 0x01 ? 'TRIGGER_STATE' : cmdCode === 0x02 ? 'BATTERY_REPORTING' : 'UNKNOWN';
        //   console.log(`[Harness] >>> This is 0xA0${cmdCode.toString(16).padStart(2, '0')} (${cmdName})`);

        //   if (cmdCode === 0x00 && data.length >= 12) {
        //     const voltage = (data[10] << 8) | data[11];
        //     console.log(`[Harness] >>> Battery voltage: 0x${voltage.toString(16).toUpperCase()} = ${voltage}mV`);
        //   } else if (cmdCode === 0x01 && data.length >= 11) {
        //     const state = data[10];
        //     console.log(`[Harness] >>> Trigger state: 0x${state.toString(16).padStart(2, '0')} = ${state === 0 ? 'RELEASED' : 'PRESSED'}`);
        //   }
        // }

        // Log notification
        this.transportMessages.push({
          type: 'notification',
          bytes: data,
          timestamp: Date.now(),
          direction: 'inbound'
        });

        // Deliver notification to worker as unsolicited BLE data
        if (this.workerPort.onmessage) {
          console.log('[Harness] Delivering notification to worker');
          const event = new MessageEvent('message', {
            data: { type: 'ble:data', data }
          });
          this.workerPort.onmessage(event);
        } else {
          console.error('[Harness] No onmessage handler on workerPort!');
        }
      });
    }
    
    // Wire up domain event capture BEFORE creating worker
    this.setupEventCapture();
    
    // Import and instantiate CS108Reader
    this.worker = new CS108Reader();
    
    // Setup transport message capture BEFORE injecting port
    this.setupTransportCapture();
    
    // Inject BLE transport port using BaseReader method
    this.worker.setTransportPort(this.workerPort);
  }
  
  /**
   * Setup transport message capture at MessagePort boundary
   * 
   * Flow:
   * 1. Worker sends command via workerPort.postMessage
   * 2. We intercept and forward to hardware
   * 3. Hardware responds asynchronously 
   * 4. We deliver response to workerPort.onmessage (set by BaseReader)
   */
  private setupTransportCapture(): void {
    // Intercept outbound commands from worker
    const originalWorkerPost = this.workerPort.postMessage.bind(this.workerPort);
    this.workerPort.postMessage = (message: { type: string; data?: Uint8Array } | NotificationEvent) => {
      if (message.type === 'ble:write' && message.data instanceof Uint8Array) {
        // Log command for debugging
        const commandName = this.identifyCommand(message.data);
        this.transportMessages.push({
          type: 'command',
          bytes: message.data,
          timestamp: Date.now(),
          direction: 'outbound',
          commandName
        });

        // Forward to hardware WITHOUT waiting for response
        // ALL responses (ACKs and notifications) come through the notification handler
        if (this.rfidClient.isConnected()) {
          // Capture traffic with timestamp
          this.traffic.push({
            direction: 'TX',
            bytes: new Uint8Array(message.data),
            timestamp: Date.now()
          });

          const hexDump = Array.from(message.data).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ');
          console.log(`[Harness] Sending command to hardware:`, hexDump);

          // Debug: Log what command we're sending
          if (message.data.length >= 10 && message.data[8] === 0xa0) {
            const cmdCode = message.data[9];
            console.log(`[Harness] >>> OUTBOUND: 0xA0${cmdCode.toString(16).padStart(2, '0')} (${cmdCode === 0x00 ? 'GET_BATTERY_VOLTAGE' : cmdCode === 0x01 ? 'GET_TRIGGER_STATE' : cmdCode === 0x02 ? 'START_BATTERY_REPORTING' : 'UNKNOWN'})`);
          }

          // Use fire-and-forget send so we don't block on ONE response
          // This allows streaming responses (like inventory tags) to flow through notifications
          this.rfidClient.sendRawBytes(message.data).catch(error => {
            console.error('[Harness] Hardware command failed:', error);
            console.error('[Harness] Error details:', error.message || error);
          });
        } else {
          console.error('[Harness] Cannot send command - not connected to hardware');
        }
      }
      // Don't call original for commands - we handle them
      if (message.type !== 'command') {
        originalWorkerPost(message);
      }
    };
    
    // We don't need to override blePort since we handle responses directly above
  }
  
  /**
   * Identify command from bytes using CS108 metadata
   * Header is always 8 bytes, followed by 2-byte event code
   */
  private identifyCommand(bytes: Uint8Array): string {
    if (bytes.length < 10) return 'UNKNOWN';
    
    // Extract event code (bytes 8-9 after 8-byte header)
    const eventCode = (bytes[8] << 8) | bytes[9];
    
    // Use the single source of truth from events.ts
    const event = getEventByCode(eventCode);
    return event?.name || `CMD_0x${eventCode.toString(16).toUpperCase()}`;
  }
  
  /**
   * Setup domain event capture from worker
   */
  private setupEventCapture(): void {
    // Override postMessage to capture domain events
    // Save original for potential restoration (not currently used)
    // const _originalPostMessage = globalThis.postMessage;
    globalThis.postMessage = (message: NotificationEvent) => {
      console.log(`[Harness] Captured event: ${message.type}`);
      this.domainEvents.push(message);

      // Notify any waiters - but let the waiter decide if it matches
      const waiter = this.eventWaiters.get(message.type);
      if (waiter) {
        console.log(`[Harness] Notifying waiter for ${message.type}`);
        waiter(message);
        // Don't delete here - let the waiter itself decide when to remove
      }

      // Don't call original in test environment - it causes jsdom errors
    };
  }
  
  /**
   * Forward BLE data to worker
   */
  async forwardBleData(data: Uint8Array): Promise<void> {
    console.log('[Harness] Forwarding BLE data to worker:', Array.from(data).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));

    // Inject the data to the worker port's onmessage handler
    if (this.workerPort.onmessage) {
      const event = new MessageEvent('message', {
        data: { type: 'ble:data', data }
      });
      this.workerPort.onmessage(event);
    } else {
      console.error('[Harness] No onmessage handler on workerPort!');
    }

    // Give worker time to process
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  
  /**
   * Wait for specific domain event type with optional filter
   */
  async waitForEvent(
    eventType: string,
    filter?: (event: NotificationEvent) => boolean,
    timeoutMs: number = 5000
  ): Promise<NotificationEvent> {
    // Simple, clear parameters - no overloading needed
    console.log(`[Harness] Waiting for event: ${eventType}`);

    // Check if event already captured
    const existing = this.domainEvents.find(e => {
      if (e.type !== eventType) return false;
      if (filter) return filter(e);
      return true;
    });
    if (existing) {
      console.log(`[Harness] Found existing event: ${eventType}`);
      return existing;
    }

    console.log(`[Harness] Setting up waiter for event: ${eventType}`);
    // Wait for future event
    return new Promise((resolve, reject) => {
      const timeoutHandle = setTimeout(() => {
        this.eventWaiters.delete(eventType);
        console.log(`[Harness] Timeout waiting for event: ${eventType}`);
        reject(new Error(`Timeout waiting for event: ${eventType}`));
      }, timeoutMs);
      
      this.eventWaiters.set(eventType, (event) => {
        // Check if filter matches (if provided)
        if (filter && !filter(event)) {
          // Don't resolve yet, wait for matching event
          return;
        }
        clearTimeout(timeoutHandle);
        this.eventWaiters.delete(eventType);
        resolve(event);
      });
    });
  }
  
  /**
   * Get all captured domain events
   */
  getEvents(): NotificationEvent[] {
    return [...this.domainEvents];
  }
  
  /**
   * Get all captured events
   */
  getAllEvents(): NotificationEvent[] {
    return [...this.domainEvents];
  }

  /**
   * Get events of specific type
   */
  getEventsByType(type: string): NotificationEvent[] {
    return this.domainEvents.filter(e => e.type === type);
  }

  /**
   * Clear captured events
   */
  clearEvents(): void {
    this.domainEvents = [];
    this.transportMessages = [];
  }
  
  /**
   * Get transport messages (commands sent to hardware)
   */
  getTransportMessages(): TransportMessage[] {
    return [...this.transportMessages];
  }
  
  /**
   * Get outbound commands sent to hardware
   */
  getOutboundCommands(): string[] {
    return this.transportMessages
      .filter(m => m.direction === 'outbound')
      .map(m => m.commandName || 'UNKNOWN');
  }
  
  /**
   * Verify command was sent to hardware
   */
  wasCommandSent(commandName: string): boolean {
    return this.getOutboundCommands().includes(commandName);
  }
  
  /**
   * Get last transport message bytes (for debugging)
   */
  getLastTransportBytes(): Uint8Array | undefined {
    return this.transportMessages.at(-1)?.bytes;
  }
  
  /**
   * INTERNAL: Get the worker instance for API calls
   * Tests should NEVER call this directly - use the public API methods
   */
  private getWorker(): CS108Reader {
    if (!this.worker) {
      throw new Error('Worker not initialized - call initialize() first');
    }
    return this.worker;
  }
  
  /**
   * INTERNAL: Call worker method through public API
   * This is just a helper to reduce duplication - it ONLY calls public methods
   */
  private async callWorkerMethod<T>(method: string, ...args: unknown[]): Promise<T> {
    const worker = this.getWorker();
    if (typeof worker[method] !== 'function') {
      throw new Error(`Worker method not found: ${method}`);
    }
    return worker[method](...args);
  }

  /**
   * Connect to device - delegates to worker
   */
  async connect(): Promise<boolean> {
    console.log('[Harness] Connecting to device');
    return this.callWorkerMethod('connect');
  }

  /**
   * Disconnect from device - delegates to worker
   */
  async disconnect(): Promise<void> {
    console.log('[Harness] Disconnecting from device');
    return this.callWorkerMethod('disconnect');
  }

  /**
   * Set reader mode - delegates to worker
   * @param mode - Reader mode to set
   * @param options - Optional parameters (e.g., targetEPC for LOCATE mode)
   */
  async setMode(mode: string, options?: { targetEPC?: string }): Promise<void> {
    console.log(`[Harness] Setting mode to ${mode}`, options ? `with options: ${JSON.stringify(options)}` : '');
    return this.callWorkerMethod('setMode', mode, options);
  }

  /**
   * Set reader settings - delegates to worker
   */
  async setSettings(settings: any): Promise<void> {
    console.log('[Harness] Setting reader settings:', settings);
    return this.callWorkerMethod('setSettings', settings);
  }

  /**
   * Get current reader settings - delegates to worker
   */
  getSettings(): any {
    console.log('[Harness] Getting reader settings');
    return this.callWorkerMethod('getSettings');
  }

  /**
   * Start scanning - delegates to worker
   */
  async startScanning(): Promise<void> {
    console.log('[Harness] Starting scanning');
    return this.callWorkerMethod('startScanning');
  }

  /**
   * Stop scanning - delegates to worker
   */
  async stopScanning(): Promise<void> {
    console.log('[Harness] Stopping scanning');
    return this.callWorkerMethod('stopScanning');
  }

  /**
   * Wait for reader state
   */
  async waitForState(state: string, timeoutMs: number = 5000): Promise<void> {
    console.log(`[Harness] Waiting for state: ${state}`);

    // Check current state first
    const worker = this.getWorker();
    if ((worker as any).readerState === state) {
      return;
    }

    // Wait for state change event
    await this.waitForEvent('READER_STATE_CHANGED',
      (event) => event.payload?.readerState === state,
      timeoutMs
    );
  }

  /**
   * OBSERVE: Get current reader state (read-only)
   * This is OK for assertions but NEVER modify the state directly
   */
  getReaderState(): string {
    const worker = this.getWorker();
    return (worker as any).readerState;
  }

  /**
   * OBSERVE: Get current reader mode (read-only)
   * This is OK for assertions but NEVER modify the mode directly
   */
  getReaderMode(): string | null {
    const worker = this.getWorker();
    return (worker as any).readerMode;
  }

  /**
   * OBSERVE: Get current battery percentage (read-only)
   * Returns the last battery percentage from BATTERY_UPDATE events
   */
  getBatteryPercentage(): number | null {
    const batteryEvents = this.getEventsByType(WorkerEventType.BATTERY_UPDATE);
    if (batteryEvents.length > 0) {
      const lastEvent = batteryEvents[batteryEvents.length - 1];
      return lastEvent.payload.percentage;
    }
    return null;
  }

  /**
   * Dump all traffic for debugging
   */
  dumpTraffic(): void {
    console.log('\n========== TRAFFIC DUMP ==========');
    console.log(`Total messages: ${this.transportMessages.length}`);

    this.transportMessages.forEach((msg, i) => {
      const hexDump = Array.from(msg.bytes).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ');
      const direction = msg.direction === 'outbound' ? '>>>' : '<<<';
      console.log(`${i}: ${direction} ${msg.type} [${msg.bytes.length} bytes]`);
      console.log(`    ${hexDump}`);

      // Decode if it's a CS108 packet
      if (msg.bytes.length >= 10 && msg.bytes[8] === 0xa0) {
        const cmdCode = msg.bytes[9];
        const cmdName = cmdCode === 0x00 ? 'BATTERY' : cmdCode === 0x01 ? 'TRIGGER' : cmdCode === 0x02 ? 'START_BATTERY' : 'UNKNOWN';
        console.log(`    Command: 0xA0${cmdCode.toString(16).padStart(2, '0')} (${cmdName})`);

        if (cmdCode === 0x00 && msg.bytes.length >= 12) {
          const voltage = (msg.bytes[10] << 8) | msg.bytes[11];
          console.log(`    Battery voltage: 0x${voltage.toString(16)} (${voltage}mV)`);
        }
      }
    });
    console.log('========== END TRAFFIC DUMP ==========\n');
  }

  /**
   * Check if connected
   */
  isConnected(): boolean {
    const worker = this.getWorker();
    return (worker as any).readerState !== 'Disconnected';
  }

  /**
   * Register a callback for worker messages
   */
  onWorkerMessage(callback: (event: any) => void): void {
    // Listen to domain events by intercepting postMessage
    const originalPostMessage = globalThis.postMessage;
    globalThis.postMessage = (message: NotificationEvent) => {
      // Capture event in our list
      this.domainEvents.push(message);

      // Notify any waiters
      const waiter = this.eventWaiters.get(message.type);
      if (waiter) {
        waiter(message);
      }

      // Call the callback for this specific listener
      callback(message);
    };
  }

  /**
   * Simulate trigger press by injecting notification packet
   */
  async simulateTriggerPress(): Promise<void> {
    console.log('[Harness] Simulating trigger press');

    // Import packet builder from test utilities
    const { TestPackets } = await import('../../config/cs108-packet-builder');
    const packet = TestPackets.triggerPress();

    console.log('[Harness] Trigger press packet:', Array.from(packet).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));

    // Inject the packet as if it came from BLE - MUST AWAIT
    await this.forwardBleData(packet);
  }

  /**
   * Simulate trigger release by injecting notification packet
   */
  async simulateTriggerRelease(): Promise<void> {
    console.log('[Harness] Simulating trigger release');

    // Import packet builder from test utilities
    const { TestPackets } = await import('../../config/cs108-packet-builder');
    const packet = TestPackets.triggerRelease();

    console.log('[Harness] Trigger release packet:', Array.from(packet).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));

    // Inject the packet as if it came from BLE - MUST AWAIT
    await this.forwardBleData(packet);
  }

  /**
   * Simulate barcode scan by injecting barcode data notification
   */
  async simulateBarcodeRead(barcode: string, symbology: number = 0x03): Promise<void> {
    console.log(`[Harness] Simulating barcode read: ${barcode}`);

    // Build barcode data packet
    // Format: [symbology(1 byte), barcode data(ASCII), null terminator]
    const barcodeBytes = new TextEncoder().encode(barcode);
    const payload = new Uint8Array(1 + barcodeBytes.length + 1);

    // Symbology (1 byte) - 0x03 = CODE128
    payload[0] = symbology & 0xFF;

    // Barcode data (ASCII)
    payload.set(barcodeBytes, 1);
    
    // Null terminator (optional but good practice)
    payload[1 + barcodeBytes.length] = 0x00;

    // Import packet builder
    const { buildNotification } = await import('../../config/cs108-packet-builder');
    const { BARCODE_DATA_NOTIFICATION } = await import('@/worker/cs108/event');

    const packet = buildNotification(BARCODE_DATA_NOTIFICATION, payload);
    
    console.log('[Harness] Barcode packet payload:', Array.from(payload).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));

    // Inject the packet - MUST AWAIT
    await this.forwardBleData(packet);
  }

  /**
   * Cleanup resources
   */
  async cleanup(): Promise<void> {
    try {
      // First, ensure the reader is stopped and in a safe state
      if (this.worker) {
        try {
          // Stop any active scanning
          await this.worker.stopScanning();
        } catch {
          // Ignore errors - might not be scanning
        }

        try {
          // Return to IDLE mode to ensure vibration is off
          await this.worker.setMode(ReaderMode.IDLE);
        } catch {
          // Ignore errors - might already be disconnected
        }
      }

      // Now disconnect from hardware
      await this.rfidClient.disconnect();
    } catch {
      // Ignore disconnect errors - may not be connected
    }

    this.blePort.close();
    this.workerPort.close();
    this.domainEvents = [];
    this.eventWaiters.clear();
  }

  // Convenience getters
  get client(): RfidReaderTestClient {
    return this.rfidClient;
  }

  get events(): NotificationEvent[] {
    return this.domainEvents;
  }
}