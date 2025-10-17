/**
 * Minimal BLE Mock Configuration for Vite
 *
 * This file contains only the configuration needed by vite.config.ts
 * Updated to import from lib/device/transport for consolidated transport layer
 */

import os from 'os';
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
 * Get config for Vite mock injection
 * Minimal config with no dependencies on app code
 */
export function getViteMockConfig() {
  // Core bridge server settings
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const wsPort = process.env.BLE_MCP_WS_PORT || '8080';
  
  // BLE device settings - use constants from transport module
  const service = CS108_BLE_SERVICE_UUID;
  const write = CS108_BLE_WRITE_UUID;
  const notify = CS108_BLE_NOTIFY_UUID;
  
  // Session ID
  const sessionId = process.env.BLE_SESSION_ID || `trakrf-handheld-dev-${systemHostname}`;
  
  // Build WebSocket URL
  const serverUrl = `ws://${host}:${wsPort}`;
  
  return {
    sessionId,
    serverUrl,
    service,
    write,
    notify
  };
}