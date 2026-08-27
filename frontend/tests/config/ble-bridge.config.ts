/**
 * Centralized BLE Bridge Configuration
 * 
 * Single source of truth for all BLE bridge server and device settings
 * Used by: vite.config.ts (mock injection), integration tests, and E2E tests
 * 
 * Environment Variables:
 * - BLE_MCP_HOST: Bridge server hostname (default: localhost)
 * - BLE_MCP_WS_PORT: WebSocket port (REQUIRED — no default; see TRA-1179)
 * - BLE_SERVICE_UUID: BLE service UUID (default: 9800)
 * - BLE_WRITE_UUID: Write characteristic UUID (default: 9900)
 * - BLE_NOTIFY_UUID: Notify characteristic UUID (default: 9901)
 * - BLE_SESSION_ID: Optional session ID override (computed if not set)
 */

import os from 'os';
import { resolveBridgePort } from './resolve-bridge-port';
import * as dotenv from 'dotenv';
import {
  CS108_BLE_SERVICE_UUID,
  CS108_BLE_WRITE_UUID,
  CS108_BLE_NOTIFY_UUID
} from '../../src/lib/device/transport/cs108-ble-transport';

// Load environment variables once
dotenv.config({ path: '.env.local' });

// System hostname for unique session IDs
const systemHostname = os.hostname();

/**
 * Core BLE bridge configuration
 * All other configs derive from this
 *
 * There is no HTTP surface. TRA-1161 replaced ble-mcp-test's HTTP/MCP server
 * with a PEP 723 stdio shim over a unix socket, so BLE_MCP_HTTP_PORT and
 * BLE_MCP_HTTP_TOKEN no longer exist anywhere — see the guard in
 * no-dead-http-config.test.ts before adding either back (TRA-1177 row H).
 */
export interface BleBridgeConfig {
  // Bridge server settings
  bridge: {
    host: string;
    wsPort: string;
    wsUrl: string;
  };
  
  // BLE device settings — selection is by service UUID only, never by name
  device: {
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
  const wsPort = resolveBridgePort();

  // BLE device settings - use constants from transport module
  const service = CS108_BLE_SERVICE_UUID;
  const write = CS108_BLE_WRITE_UUID;
  const notify = CS108_BLE_NOTIFY_UUID;
  
  // Session ID - always the same for connection pool reuse
  const sessionId = process.env.BLE_SESSION_ID || `trakrf-handheld-dev-${systemHostname}`;
  
  // Build URLs from components (no more VITE_BLE_BRIDGE_URL duplication!)
  const wsUrl = `ws://${host}:${wsPort}`;

  return {
    bridge: {
      host,
      wsPort,
      wsUrl
    },
    device: {
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
    systemHostname: config.session.hostname
  };
}

/**
 * Get config for E2E tests
 */
export function getE2EBridgeConfig() {
  const config = getBleBridgeConfig();
  return {
    bridge: {
      wsUrl: config.bridge.wsUrl
    },
    device: {
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
  
  // Add device parameters — service UUID identifies the device, not its name
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
