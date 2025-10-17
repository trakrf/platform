/**
 * Centralized E2E test configuration
 * Merges all test configurations into a single source of truth
 * 
 * 🚀 IMPORTANT: When writing E2E tests, please refer to:
 * tests/e2e/BLE-ENHANCED-TESTING-STRATEGY.md
 * 
 * This document explains how to leverage BLE packet monitoring capabilities
 * for deeper test coverage, including:
 * - Real-time packet inspection with ble-mcp-test
 * - Protocol-level assertions using helpers/ble-protocol.ts
 * - Performance metrics and timing analysis
 * - Examples in inventory-protocol.spec.ts
 */

import { getE2EBridgeConfig } from '../config/ble-bridge.config';

export interface E2EConfig {
  bridge: {
    wsUrl: string;
    httpUrl: string;
  };
  device: {
    name: string;
    serviceUuid: string;
    writeUuid: string;
    notifyUuid: string;
  };
  timeouts: {
    connect: number;
    command: number;
    ui: number;
    scan: number;
  };
  selectors: {
    connectButton: string;
    disconnectButton: string;
    batteryIndicator: string;
    tagCount: string;
    connectionStatus: string;
    inventoryTable: string;
  };
  expected: {
    battery: {
      min: number;
      max: number;
    };
  };
}

export function getE2EConfig(): E2EConfig {
  // Use shared configuration for consistency
  const bleConfig = getE2EBridgeConfig();
  
  return {
    bridge: {
      wsUrl: bleConfig.bridge.wsUrl,
      httpUrl: bleConfig.bridge.httpUrl,
    },
    device: {
      name: bleConfig.device.name,
      serviceUuid: bleConfig.device.serviceUuid,
      writeUuid: bleConfig.device.writeUuid,
      notifyUuid: bleConfig.device.notifyUuid
    },
    timeouts: {
      connect: parseInt(process.env.BRIDGE_CONNECT_TIMEOUT || '30000'), // 30s for connection (mock retries up to 10 times)
      command: parseInt(process.env.BRIDGE_COMMAND_TIMEOUT || '5000'),  // 5s for commands
      ui: parseInt(process.env.BRIDGE_UI_TIMEOUT || '5000'),           // 5s for UI
      scan: parseInt(process.env.BRIDGE_SCAN_TIMEOUT || '15000')       // 15s for scan
    },
    selectors: {
      connectButton: 'button[data-testid="connect-button"]',
      disconnectButton: 'button[data-testid="disconnect-button"]',
      batteryIndicator: '[title*="Battery:"] span',
      tagCount: 'text=/Tags: \\d+/',
      connectionStatus: '[data-testid="connection-status"]',
      inventoryTable: 'table tbody'
    },
    expected: {
      battery: {
        min: 3000, // 3.0V in millivolts
        max: 5000  // 5.0V in millivolts (though actual max is 4.2V)
      }
    }
  };
}

export type DeviceAvailability = 'available' | 'none' | 'timeout' | 'mock';

// Re-export buildBridgeUrl from the shared config
export { buildBridgeUrl } from '../config/ble-bridge.config';

// Helper to get bridge WS URL
export function getBridgeWsUrl(): string {
  return getE2EConfig().bridge.wsUrl;
}

// Helper to get bridge HTTP URL  
export function getBridgeHttpUrl(): string {
  return getE2EConfig().bridge.httpUrl;
}

// Legacy helper for compatibility during migration
export function getDeviceConfig() {
  const config = getE2EConfig().device;
  return {
    namePrefix: config.name,
    serviceUUID: config.serviceUuid,
    writeUUID: config.writeUuid,
    notifyUUID: config.notifyUuid
  };
}