#!/usr/bin/env node

/**
 * Development server with BLE mock support
 * 
 * This script:
 * 1. Checks if BLE bridge server is configured and available
 * 2. Health checks the mock server
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

// Build URLs from standard ble-mcp-test env vars
const host = process.env.BLE_MCP_HOST || process.env.BLE_MCP_WS_HOST || 'localhost';
const wsPort = process.env.BLE_MCP_WS_PORT || '8080';
const httpPort = process.env.BLE_MCP_HTTP_PORT || '8081';
const BLE_BRIDGE_WS_URL = `ws://${host}:${wsPort}`;
const BLE_BRIDGE_HTTP_URL = `http://${host}:${httpPort}`;
const BLE_MCP_TOKEN = process.env.BLE_MCP_HTTP_TOKEN;

// Parse URL to get host
const wsUrl = new URL(BLE_BRIDGE_WS_URL);
const isLocalhost = wsUrl.hostname === 'localhost' || wsUrl.hostname === '127.0.0.1';

console.log('🔌 BLE Mock Configuration:');
console.log(`   WebSocket URL: ${BLE_BRIDGE_WS_URL}`);
console.log(`   HTTP URL: ${BLE_BRIDGE_HTTP_URL}`);
console.log(`   Host: ${wsUrl.hostname}:${wsUrl.port}`);
console.log(`   Auth: ${BLE_MCP_TOKEN ? 'Token configured' : 'No auth token'}`);
console.log('');

/**
 * Check if the mock server is available via HTTP health endpoint
 */
async function checkMockServer() {
  try {
    console.log(`🔍 Checking mock server at ${BLE_BRIDGE_HTTP_URL}/health...`);
    
    const headers = {};
    if (BLE_MCP_TOKEN) {
      headers['Authorization'] = `Bearer ${BLE_MCP_TOKEN}`;
    }
    
    const response = await fetch(`${BLE_BRIDGE_HTTP_URL}/health`, {
      method: 'GET',
      headers,
      signal: AbortSignal.timeout(3000)
    });
    
    if (response.ok) {
      const health = await response.json();
      console.log('✅ Mock server is available');
      console.log(`   Version: ${health.version || 'unknown'}`);
      console.log(`   Status: ${health.status || 'unknown'}`);
      return true;
    } else {
      console.log(`❌ Mock server health check failed: HTTP ${response.status}`);
      return false;
    }
  } catch (err) {
    console.log(`❌ Mock server connection failed: ${err.message}`);
    return false;
  }
}

/**
 * Show MCP configuration instructions
 */
function showMcpInstructions() {
  console.log('\n📋 MCP Configuration Instructions:');
  console.log('   For better development experience, configure Claude with MCP:');
  console.log('');
  
  const httpUrl = BLE_BRIDGE_HTTP_URL.replace(/\/$/, ''); // Remove trailing slash
  
  if (BLE_MCP_TOKEN) {
    console.log(`   claude mcp add ble-mcp-test --transport http ${httpUrl} --header "Authorization: Bearer $BLE_MCP_HTTP_TOKEN"`);
  } else {
    console.log(`   claude mcp add ble-mcp-test --transport http ${httpUrl}`);
  }
  
  console.log('');
  console.log('   This enables real-time BLE monitoring in Claude while developing.');
  console.log('');
}

/**
 * Start the development server
 */
function startDevServer() {
  console.log('🚀 Starting development server...\n');
  
  // Use vite directly with environment to enable mock
  // Run in test mode so DeviceManager is exposed for E2E tests
  const vite = spawn('pnpm', ['vite', '--mode', 'test', '--port', '5173'], {
    stdio: 'inherit',
    shell: true,
    env: {
      ...process.env,
      VITE_BLE_MOCK_ENABLED: 'true',
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
  // Check if mock server is available
  const serverAvailable = await checkMockServer();
  
  if (!serverAvailable) {
    console.error('\n❌ BLE mock server is not available!');
    console.error('');
    
    if (isLocalhost) {
      console.error('   The mock server needs to be running on localhost.');
      console.error('   You can start it with:');
      console.error('');
      console.error('   pnpm dlx ble-mcp-test serve --port ' + wsUrl.port);
      console.error('');
      console.error('   Or use the test:e2e:with-app command which starts it automatically.');
    } else {
      console.error(`   The configured mock server at ${wsUrl.hostname}:${wsUrl.port} is not responding.`);
      console.error('   Please ensure the remote server is running and accessible.');
      
      if (BLE_MCP_TOKEN) {
        console.error('   Note: Auth token is configured, make sure it\'s correct.');
      }
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