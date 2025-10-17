/**
 * Centralized BLE Bridge Configuration
 * 
 * Single source of truth for all BLE bridge server and device settings
 * Used by: vite.config.ts (mock injection), integration tests, and E2E tests
 * 
 * Environment Variables:
 * - BLE_MCP_HOST: Bridge server hostname (default: localhost)
 * - BLE_MCP_WS_PORT: WebSocket port (default: 8080)
 * - BLE_MCP_HTTP_PORT: HTTP/MCP port (default: 8081)
 * - BLE_MCP_HTTP_TOKEN: Auth token for MCP
 * - BLE_DEVICE_NAME: Device name filter (default: CS108)
 * - BLE_SERVICE_UUID: BLE service UUID (default: 9800)
 * - BLE_WRITE_UUID: Write characteristic UUID (default: 9900)
 * - BLE_NOTIFY_UUID: Notify characteristic UUID (default: 9901)
 * - BLE_SESSION_ID: Optional session ID override (computed if not set)
 */

import os from 'os';
import * as dotenv from 'dotenv';
import {
  CS108_BLE_SERVICE_UUID,
  CS108_BLE_WRITE_UUID,
  CS108_BLE_NOTIFY_UUID,
  CS108_DEVICE_NAME
} from '../../src/lib/device/transport/cs108-ble-transport';

// Load environment variables once
dotenv.config({ path: '.env.local' });

// System hostname for unique session IDs
const systemHostname = os.hostname();

/**
 * Core BLE bridge configuration
 * All other configs derive from this
 */
export interface BleBridgeConfig {
  // Bridge server settings
  bridge: {
    host: string;
    wsPort: string;
    httpPort: string;
    wsUrl: string;
    httpUrl: string;
    token?: string;
  };
  
  // BLE device settings
  device: {
    name: string;
    service: string;
    write: string;
    notify: string;
  };
  
  // Session management
  session: {
    id: string;
    hostname: string;
  };
}

/**
 * Get the complete BLE bridge configuration
 * This is the single source of truth - no duplication
 */
export function getBleBridgeConfig(): BleBridgeConfig {
  // Core bridge server settings (BLE_MCP_* prefix for bridge server vars)
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const wsPort = process.env.BLE_MCP_WS_PORT || '8080';
  const httpPort = process.env.BLE_MCP_HTTP_PORT || '8081';
  const token = process.env.BLE_MCP_HTTP_TOKEN;
  
  // BLE device settings - use constants from transport module
  const deviceName = process.env.BLE_DEVICE_NAME || process.env.VITE_DEVICE_NAME || CS108_DEVICE_NAME;
  const service = CS108_BLE_SERVICE_UUID;
  const write = CS108_BLE_WRITE_UUID;
  const notify = CS108_BLE_NOTIFY_UUID;
  
  // Session ID - always the same for connection pool reuse
  const sessionId = process.env.BLE_SESSION_ID || `trakrf-handheld-dev-${systemHostname}`;
  
  // Build URLs from components (no more VITE_BLE_BRIDGE_URL duplication!)
  const wsUrl = `ws://${host}:${wsPort}`;
  const httpUrl = `http://${host}:${httpPort}`;
  
  return {
    bridge: {
      host,
      wsPort,
      httpPort,
      wsUrl,
      httpUrl,
      token
    },
    device: {
      name: deviceName,
      service,
      write,
      notify
    },
    session: {
      id: sessionId,
      hostname: systemHostname
    }
  };
}

/**
 * Get config for Vite mock injection
 */
export function getViteMockConfig() {
  const config = getBleBridgeConfig();
  return {
    sessionId: config.session.id,
    serverUrl: config.bridge.wsUrl,
    service: config.device.service,
    write: config.device.write,
    notify: config.device.notify
  };
}

/**
 * Get config for integration tests (NodeBleClient)
 */
export function getIntegrationTestConfig() {
  const config = getBleBridgeConfig();
  return {
    bridgeUrl: config.bridge.wsUrl,
    service: config.device.service,
    write: config.device.write,
    notify: config.device.notify,
    sessionId: config.session.id,
    // Include extra metadata for debugging
    host: config.bridge.host,
    port: config.bridge.wsPort,
    systemHostname: config.session.hostname,
    deviceName: config.device.name
  };
}

/**
 * Get config for E2E tests
 */
export function getE2EBridgeConfig() {
  const config = getBleBridgeConfig();
  return {
    bridge: {
      wsUrl: config.bridge.wsUrl,
      httpUrl: config.bridge.httpUrl
    },
    device: {
      name: config.device.name,
      serviceUuid: config.device.service,
      writeUuid: config.device.write,
      notifyUuid: config.device.notify
    },
    sessionId: config.session.id
  };
}

/**
 * Build a bridge URL with query parameters
 * Used by E2E tests for specific test scenarios
 */
export function buildBridgeUrl(options?: { 
  deviceAvailability?: 'available' | 'none' | 'timeout' | 'mock' 
}): string {
  const config = getBleBridgeConfig();
  const url = new URL(config.bridge.wsUrl);
  
  // Add device parameters
  url.searchParams.set('device', config.device.name);
  url.searchParams.set('service', config.device.service);
  url.searchParams.set('write', config.device.write);
  url.searchParams.set('notify', config.device.notify);
  url.searchParams.set('sessionId', config.session.id);
  
  // Add optional test parameters
  if (options?.deviceAvailability) {
    url.searchParams.set('availability', options.deviceAvailability);
  }
  
  return url.toString();
}


// Re-export general utilities for convenience
export { bytesToHex } from './utils.config';

// Re-export CS108 test commands and validation for backward compatibility
export { 
  cs108TestCommand, 
  cs108TestResponse,
  isValidTriggerStateResponse,
  getTriggerState
} from './cs108.config';
