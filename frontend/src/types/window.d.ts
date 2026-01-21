/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Global window extensions for debugging and testing
 * These properties are attached to window for development/debugging purposes
 */

import type { DeviceManager } from '@/lib/device/device-manager';

// Define the shape of Zustand stores
interface ZustandStores {
  deviceStore?: any;
  tagStore?: any;
  locateStore?: any;
  barcodeStore?: any;
  settingsStore?: any;
  uiStore?: any;
  packetStore?: any;
}

// Transport manager interface
interface TransportManager {
  deviceManager?: DeviceManager;
  notifyCharacteristic?: {
    simulateNotification?: (packet: Uint8Array) => void;
  };
}

// BLE Mock interface
interface BleMock {
  simulateNotification?: (packet: Uint8Array) => void;
}

// WebBleMock interface for testing
interface WebBleMock {
  injectWebBluetoothMock: (config: any) => void;
}

// Extend the Window interface
declare global {
  interface Window {
    // Device management debugging
    __DEVICE_MANAGER__?: DeviceManager;
    __TRANSPORT_MANAGER__?: TransportManager;
    
    // Store debugging
    __ZUSTAND_STORES__?: ZustandStores;
    
    // Testing utilities
    __BLE_MOCK__?: BleMock;
    __webBluetoothBridged?: boolean;
    __testPage?: boolean;
    WebBleMock?: WebBleMock;
    
    // OpenReplay debugging
    __checkOpenReplay?: () => void;
    __testOpenReplay?: () => void;
    
    // Legacy test utilities (if needed)
    getRfidManager?: any;
  }
}

export {};