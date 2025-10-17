/**
 * Device-related types for lib/device
 * Minimal interfaces needed for device management
 */

// Basic device state type
export interface DeviceState {
  isConnected: boolean;
  deviceName: string | null;
  batteryPercentage: number | null;
  readerState: string;
}

// Standard RFID tag structure
export interface StandardTag {
  epc: string;
  rssi: number;
  timestamp: number;
  antenna?: number;
}

// Barcode data structure
export interface BarcodeData {
  barcode: string;
  symbology: string;
  timestamp: number;
}

// Battery information
export interface BatteryInfo {
  voltage?: number;
  percentage?: number;
  isCharging?: boolean;
}

// Minimal device interface for Comlink wrapping
export interface IHandheldDevice {
  // Connection
  connect(): Promise<boolean>;
  disconnect(): Promise<void>;
  
  // State
  getState(): Promise<DeviceState>;
  
  // Operations
  startInventory(): Promise<void>;
  stopInventory(): Promise<void>;
  startBarcodeScan(): Promise<void>;
  stopBarcodeScan(): Promise<void>;
  
  // Events (these would be implemented via MessagePort)
  onStateChange?(callback: (state: DeviceState) => void): void;
  onTagRead?(callback: (tag: StandardTag) => void): void;
  onBarcodeRead?(callback: (barcode: BarcodeData) => void): void;
  onBatteryUpdate?(callback: (battery: BatteryInfo) => void): void;
}