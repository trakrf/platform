/**
 * BLE MCP Test Client - Bridge Server Connectivity Test
 * 
 * Validates that we can connect to the MCP bridge server.
 * This is a basic connectivity test, not specific to CS108.
 * 
 * Success criteria:
 * - Connect to bridge server via WebSocket
 * - Send a basic command
 * - Receive a response
 * - Clean disconnect and resource cleanup
 */
import { describe, test, expect, beforeEach, afterEach } from 'vitest';
import { TransportCommandClient } from './transport-command-client';
import { cs108TestCommand } from '../../config/ble-bridge.config';

describe('BLE MCP Bridge Server Connection', () => {
  let client: TransportCommandClient;

  beforeEach(async () => {
    console.log('\n🧪 Setting up bridge server connection test...');
    client = new TransportCommandClient();
  });

  afterEach(async () => {
    if (client) {
      try {
        await client.disconnect();
      } catch (error) {
        console.error('Error during cleanup:', error);
      }
    }
  });

  test('should connect to bridge server and send/receive data', async () => {
    // Connect to bridge server
    console.log('📡 Connecting to bridge server...');
    try {
      await client.connect();
      console.log('✅ Connected successfully');
    } catch (error) {
      console.error('❌ Connection failed:', error);
      throw error;
    }

    // Use shared test command from config
    console.log('\n📤 Sending test command...');
    const response = await client.sendCommand(cs108TestCommand,10000);
    
    // Basic validation - we got some response
    expect(response).toBeDefined();
    // `ArrayBuffer.isView`, not `toBeInstanceOf(Uint8Array)`. The response has
    // crossed the transport's MessageChannel, and under jsdom a structured
    // clone is built with jsdom's constructors while this module's `Uint8Array`
    // binding is Node's — so `instanceof` is false for a genuine, correct
    // Uint8Array. See the note in src/worker/BaseReader.ts.
    expect(ArrayBuffer.isView(response)).toBe(true);
    expect(response.length).toBeGreaterThan(0);
    
    console.log('📥 Received response:', Array.from(response).map(b => '0x' + b.toString(16).padStart(2, '0')).join(' '));
    console.log('\n✅ Bridge server connection test passed!');
  }, 30000); // 15 second test timeout for hardware communication
});
