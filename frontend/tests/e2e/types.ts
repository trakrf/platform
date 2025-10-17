/**
 * Type definitions for e2e tests
 */

// Store types for window.__ZUSTAND_STORES__
export interface DeviceStoreState {
  isConnected: boolean;
  deviceName: string | null;
  batteryLevel: number;
  triggerState: boolean;
  readerState: number;
  setTriggerState: (state: boolean) => void;
  setDeviceName: (name: string | null) => void;
  setBatteryLevel: (level: number) => void;
  setReaderState: (state: number) => void;
  reset: () => void;
}

export interface BarcodeStoreState {
  scanning: boolean;
  barcodes: Array<{
    data: string;
    type: string;
    timestamp: number;
  }>;
  addBarcode: (barcode: { data: string; type: string; timestamp: number }) => void;
  clearBarcodes: () => void;
  setScanning: (scanning: boolean) => void;
}

export interface TagStoreState {
  tags: Array<{
    epc: string;
    rssi: number;
    count: number;
    timestamp: number;
    firstSeen: number;
    lastSeen: number;
  }>;
  inventoryRunning: boolean;
  searchRSSI: number;
  searchTargetEPC: string;
  lastRSSIUpdateTime: number;
  setTags: (tags: Array<{
    epc: string;
    rssi: number;
    count: number;
    timestamp: number;
    firstSeen: number;
    lastSeen: number;
  }>) => void;
  clearTags: () => void;
  setInventoryRunning: (running: boolean) => void;
  updateSearchRSSI: (rssi: number) => void;
  setSearchTargetEPC: (epc: string) => void;
}

export interface LocateStoreState {
  isLocating: boolean;
  targetEPC: string;
  currentRSSI: number;
  averageRSSI: number;
  peakRSSI: number;
  updateRate: number;
  rssiBuffer: Array<{
    timestamp: number;
    nb_rssi: number;
    wb_rssi?: number;
    phase?: number;
  }>;
  setTargetEPC: (epc: string) => void;
  startLocate: () => void;
  stopLocate: () => void;
  addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number) => void;
  getFilteredRSSI: () => number;
}

export interface ZustandStores {
  deviceStore: {
    getState: () => DeviceStoreState;
  };
  barcodeStore: {
    getState: () => BarcodeStoreState;
  };
  tagStore: {
    getState: () => TagStoreState;
  };
  locateStore: {
    getState: () => LocateStoreState;
  };
  tabStore?: {
    getState: () => {
      activeTab: string;
      setActiveTab: (tab: string) => void;
    };
  };
}

export interface WindowWithStores extends Window {
  __ZUSTAND_STORES__?: ZustandStores;
  WebBleMock?: {
    requestDevice?: (options?: RequestDeviceOptions) => Promise<BluetoothDevice>;
    getDevices?: () => Promise<BluetoothDevice[]>;
  };
  __TRANSPORT_MANAGER__?: {
    notifyCharacteristic?: { simulateNotification?: (data: Uint8Array) => void };
    emit?: (event: string, data: Uint8Array) => void;
    device?: BluetoothDevice;
  };
  __DEVICE_MANAGER__?: {
    transportManager?: {
      characteristic?: { simulateNotification?: (data: Uint8Array) => void };
      emit?: (event: string, data: Uint8Array) => void;
    };
  };
  __webBluetoothMocked?: boolean;
  getRfidManager?: () => Promise<{
    startInventory?: () => Promise<boolean>;
    stopInventory?: () => Promise<boolean>;
    isInventoryRunning?: () => boolean;
  }>;
}

// Type guard for checking if stores exist
export function hasZustandStores(window: Window): window is WindowWithStores {
  return '__ZUSTAND_STORES__' in window;
}