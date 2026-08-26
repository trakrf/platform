#!/usr/bin/env node

/**
 * Development server with BLE bridge support
 *
 * This script:
 * 1. Checks if BLE bridge server is configured and available
 * 2. Health checks the bridge server
 * 3. Provides MCP configuration instructions
 * 4. Starts the development server
 */

import { spawn } from 'child_process';
import dotenv from 'dotenv';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { validateBleEnvironment } from './validate-url.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Load environment variables
dotenv.config({ path: join(__dirname, '..', '.env.local') });

// Validate environment variables
const envValidation = validateBleEnvironment(process.env);
if (!envValidation.isValid) {
  console.error('❌ Invalid environment configuration:');
  envValidation.errors.forEach(error => console.error(`   ${error}`));
  process.exit(1);
}

// Build URLs from standard ble-mcp-test env vars.
//
// There is no HTTP surface any more: TRA-1161 replaced it with a stdio MCP
// shim over a unix socket, so the HTTP port and token variables were deleted
// everywhere (TRA-1177 row H).
const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
const wsPort = process.env.BLE_MCP_WS_PORT || '8080';
const BLE_BRIDGE_WS_URL = `ws://${host}:${wsPort}`;

// Parse URL to get host
const wsUrl = new URL(BLE_BRIDGE_WS_URL);
const isLocalhost = wsUrl.hostname === 'localhost' || wsUrl.hostname === '127.0.0.1';

console.log('🔌 BLE Bridge Configuration:');
console.log(`   WebSocket URL: ${BLE_BRIDGE_WS_URL}`);
console.log(`   Host: ${wsUrl.hostname}:${wsUrl.port}`);
console.log('');

/**
 * Check whether the bridge is listening.
 *
 * There is no /health endpoint any more. This function used to fetch it on the
 * HTTP port and exit(1) when it failed — which it always did, because TRA-1161
 * deleted that server. So `pnpm dev:bridge` was broken outright rather than
 * merely printing stale advice: it reported "bridge server is not available"
 * with a healthy bridge running right there on the WebSocket port
 * (TRA-1177 row H).
 *
 * The WebSocket port answers a plain HTTP request with 426 Upgrade Required.
 * Any HTTP status proves something is listening and speaking the upgrade
 * protocol; a connection error is the real "not running" signal.
 */
async function checkBridgeServer() {
  const probeUrl = `http://${host}:${wsPort}/`;

  try {
    console.log(`🔍 Checking bridge server at ${probeUrl} ...`);

    const response = await fetch(probeUrl, {
      method: 'GET',
      signal: AbortSignal.timeout(3000)
    });

    // 426 Upgrade Required is the expected answer from a WebSocket listener.
    console.log(`✅ Bridge server is listening (HTTP ${response.status})`);
    return true;
  } catch (err) {
    console.log(`❌ Bridge server not reachable at ${probeUrl}: ${err.message}`);
    return false;
  }
}

/**
 * Show MCP configuration instructions.
 *
 * The MCP server is a PEP 723 stdio shim that connects to a unix socket the
 * bridge listens on ($BLE_MCP_SOCKET_PATH, else $XDG_RUNTIME_DIR/ble-bridge.sock,
 * else /tmp/ble-bridge-$UID.sock, mode 0600). The `--transport http` line that
 * used to be printed here registered against a port nothing serves, generating
 * a dead endpoint fresh on every run (TRA-1177 rows F and H). ble-mcp-test's
 * docs/MCP-SERVER.md is the reference for the tools and the socket contract.
 */
function showMcpInstructions() {
  console.log('\n📋 MCP Configuration Instructions:');
  console.log('   For real-time BLE monitoring in Claude while developing:');
  console.log('');
  console.log('   claude mcp add ble-mcp-test -- uv run --script /path/to/ble-mcp-test/mcp-server/ble_mcp.py');
  console.log('');
  console.log('   Replace /path/to/ble-mcp-test with your checkout. The shim talks to');
  console.log('   the bridge over a unix socket — there is no HTTP port to configure.');
  console.log('');
}

/**
 * Start the development server
 */
function startDevServer() {
  console.log('🚀 Starting development server...\n');

  // Use vite directly with environment to enable bridge
  // Run in test mode so DeviceManager is exposed for E2E tests
  const vite = spawn('pnpm', ['vite', '--mode', 'test', '--port', '5173'], {
    stdio: 'inherit',
    shell: true,
    env: {
      ...process.env,
      VITE_BLE_BRIDGE_ENABLED: 'true',
      VITE_BLE_BRIDGE_URL: BLE_BRIDGE_WS_URL
    }
  });
  
  vite.on('error', (err) => {
    console.error('Failed to start dev server:', err);
    process.exit(1);
  });
  
  vite.on('exit', (code) => {
    process.exit(code || 0);
  });
}

/**
 * Main function
 */
async function main() {
  // Check if bridge server is available
  const serverAvailable = await checkBridgeServer();

  if (!serverAvailable) {
    console.error('\n❌ BLE bridge server is not available!');
    console.error('');

    if (isLocalhost) {
      console.error('   The bridge server needs to be running on localhost.');
      console.error('   Start it from your ble-mcp-test checkout with:');
      console.error('');
      console.error('   just bridge     # or: uv run bridge/main.py');
      console.error('');
      console.error('   The bridge is a Python server now — there is no npm bin to run.');
    } else {
      console.error(`   The configured bridge server at ${wsUrl.hostname}:${wsUrl.port} is not responding.`);
      console.error('   Please ensure the remote server is running and accessible.');
    }

    process.exit(1);
  }

  // Show MCP instructions
  showMcpInstructions();

  // Start the dev server
  startDevServer();
}

// Run main
main().catch(console.error);