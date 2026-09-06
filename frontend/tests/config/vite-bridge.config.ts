/**
 * BLE Bridge Configuration for Vite
 *
 * This file contains only the configuration needed by vite.config.ts
 * Updated to import from lib/device/transport for consolidated transport layer
 */

import os from 'os';
import {
  CS108_BLE_SERVICE_UUID,
  CS108_BLE_WRITE_UUID,
  CS108_BLE_NOTIFY_UUID
} from '../../src/lib/device/transport/cs108-ble-transport';
import { resolveBridgePort } from './resolve-bridge-port';
import { loadRootEnv } from './load-root-env';

// Load environment variables once, from the repo root (TRA-1195).
//
// vite.config.ts imports this file, which is why `pnpm vite` printed
// "[dotenv] injecting env (0) from .env.local" on every start — a standing,
// visible symptom of the bug that nobody read as one, because vite's own
// loadEnv covered for it and the app resolved its API URL correctly anyway.
loadRootEnv();

// System hostname for unique session IDs
const systemHostname = os.hostname();

/**
 * Get config for Vite bridge injection
 * Minimal config with no dependencies on app code
 */
export function getViteBridgeConfig() {
  // Core bridge server settings
  const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
  const wsPort = resolveBridgePort();
  
  // BLE device settings - use constants from transport module
  const service = CS108_BLE_SERVICE_UUID;
  const write = CS108_BLE_WRITE_UUID;
  const notify = CS108_BLE_NOTIFY_UUID;
  
  // Session ID
  const sessionId = process.env.BLE_SESSION_ID || `trakrf-platform-dev-${systemHostname}`;
  
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