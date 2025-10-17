/**
 * Generic RFID Reader Test Client
 * 
 * Device-agnostic client for testing any RFID reader hardware via MCP bridge.
 * Uses shared configuration from config/ble-test-config.ts for consistency.
 * Built on ble-mcp-test 0.7.3 with simplified sendCommandAsync API.
 * 
 * Supports:
 * - CS108 (current)
 * - Future: Zebra, TSL, etc. (just change .env.local UUIDs)
 */
import { NodeBleClient } from 'ble-mcp-test/node';
import { getIntegrationTestConfig } from '../../config/ble-bridge.config';

export class RfidReaderTestClient {
  private client: NodeBleClient;
  private notificationCallback?: (data: Uint8Array) => void;
  private readonly CONNECTION_COOLDOWN_MS = 1000; // 1 second cooldown between connections

  // Static tracking across all instances to prevent rapid reconnections between tests
  private static globalLastDisconnectTime: number = 0;

  constructor() {
    // Use shared configuration for integration tests
    const config = getIntegrationTestConfig();
    
    // Use shared config for all BLE settings
    this.client = new NodeBleClient({
      bridgeUrl: config.bridgeUrl,
      service: config.service,
      write: config.write,
      notify: config.notify,
      sessionId: config.sessionId,
      debug: false  // Disable verbose debug logging during tests
    });

    console.log(`[RfidReaderTestClient] Configured with:`, config);
  }

  /**
   * Connect to RFID reader via MCP bridge server
   * Enforces a 1-second cooldown between connections to prevent bridge server issues
   */
  async connect(): Promise<boolean> {
    try {
      // Enforce global cooldown across all instances
      const timeSinceGlobalDisconnect = Date.now() - RfidReaderTestClient.globalLastDisconnectTime;
      if (RfidReaderTestClient.globalLastDisconnectTime > 0 && timeSinceGlobalDisconnect < this.CONNECTION_COOLDOWN_MS) {
        const waitTime = this.CONNECTION_COOLDOWN_MS - timeSinceGlobalDisconnect;
        console.log(`[RfidReaderTestClient] Enforcing ${waitTime}ms cooldown before reconnecting (global)...`);
        await new Promise(resolve => setTimeout(resolve, waitTime));
      }

      console.log('[RfidReaderTestClient] Connecting to bridge server...');

      // First connect the NodeBleClient to the bridge server
      await this.client.connect();
      console.log('[RfidReaderTestClient] Connected to bridge server, requesting device...');

      // Set up notification handler for autonomous notifications
      this.client.onNotification((data: Uint8Array) => {
        const hex = Array.from(data).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ');
        console.log(`[RfidReaderTestClient] Received notification: ${hex}`);

        // Debug: What type of packet is this?
        if (data.length >= 11 && data[8] === 0xA0) {
          const cmd = data[9];
          console.log(`[RfidReaderTestClient] >>> This is 0xA0${cmd.toString(16).padStart(2, '0')} (${cmd === 0x00 ? 'BATTERY' : cmd === 0x01 ? 'TRIGGER' : '?'})`);
        }

        if (this.notificationCallback) {
          this.notificationCallback(data);
        }
      });

      // // Request device through NodeBleClient (Web Bluetooth API compatible)
      // this.device = await this.client.requestDevice({
      //   filters: [{
      //     services: [process.env.VITE_BLE_SERVICE_UUID || '9800']
      //   }]
      // });
      //
      // console.log('[RfidReaderTestClient] Device found, connecting to GATT server...');
      //
      // // Connect to GATT server
      // this.gattServer = await this.device.gatt!.connect();
      //
      // // Get primary service
      // this.service = await this.gattServer.getPrimaryService(
      //   process.env.VITE_BLE_SERVICE_UUID || '9800'
      // );
      //
      // // Get characteristics
      // this.writeCharacteristic = await this.service.getCharacteristic(
      //   process.env.VITE_BLE_WRITE_UUID || '9900'
      // );
      //
      // this.notifyCharacteristic = await this.service.getCharacteristic(
      //   process.env.VITE_BLE_NOTIFY_UUID || '9901'
      // );

      // Skip ABORT for now - bridge is having memory issues with inventory flood
      console.log('[RfidReaderTestClient] Skipping ABORT to avoid bridge memory issues');

      // TODO: Re-enable ABORT sequence once we figure out how to handle inventory flood
      // The old code sent: 0x40 0x03 0x00 0x00 0x00 0x00 0x00 0x00
      // With 500ms timeout, 100ms wait, one retry, then 200ms final delay

      console.log('[RfidReaderTestClient] Connected successfully');
      return true;
    } catch (error) {
      console.error('[RfidReaderTestClient] Connection failed:', error);
      return false;
    }
  }

  /**
   * Register a callback for autonomous notifications
   * @param callback Function to call when notifications arrive
   */
  onNotification(callback: (data: Uint8Array) => void): void {
    this.notificationCallback = callback;
  }


  /**
   * ONLY FOR SMOKE TESTING - NOT FOR INTEGRATION TESTS
   * This violates the bidirectional stream architecture on purpose
   * to verify basic hardware connectivity.
   */
  async smokeTestCommand(command: Uint8Array, timeoutMs: number = 5000): Promise<Uint8Array> {
    if (!this.client) {
      throw new Error('Not connected - call connect() first');
    }

    console.log(`[RfidReaderTestClient] SMOKE TEST - Sending: ${Array.from(command).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ')}`);
    const response = await this.client.sendCommandAsync(command, timeoutMs);
    console.log(`[RfidReaderTestClient] SMOKE TEST - Response: ${Array.from(response).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ')}`);
    return response;
  }

  /**
   * Send raw bytes without waiting for response (fire-and-forget)
   * This is the ONLY way to send commands in integration tests.
   * ALL responses come through the notification handler.
   * @param command Raw command bytes to send
   */
  async sendRawBytes(command: Uint8Array): Promise<void> {
    if (!this.client) {
      throw new Error('Not connected - call connect() first');
    }

    console.log(`[RfidReaderTestClient] Sending raw bytes (fire-and-forget): ${Array.from(command).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' ')}`);

    // Use writeValue for fire-and-forget send
    await this.client.writeValue(command);
  }

  /**
   * Check if client is connected to hardware
   */
  isConnected(): boolean {
    return this.client?.isConnected() ?? false;
  }

  async disconnect(): Promise<void> {
    try {
      console.log('[RfidReaderTestClient] Disconnecting...');

      // Simplified API - NodeBleClient handles all cleanup internally
      await this.client.disconnect();

      // Record disconnect time globally for cooldown enforcement across instances
      RfidReaderTestClient.globalLastDisconnectTime = Date.now();

      console.log('[RfidReaderTestClient] Disconnected successfully');
    } catch (error) {
      console.error('[RfidReaderTestClient] Disconnect error:', error);
      // Record disconnect time even on error to enforce cooldown
      RfidReaderTestClient.globalLastDisconnectTime = Date.now();
      throw error;
    }
  }
}