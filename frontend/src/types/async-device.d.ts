/**
 * Async Device Types
 */

export interface AsyncDevice {
  connect(): Promise<void>;
  disconnect(): Promise<void>;
  isConnected(): boolean;
  getStatus(): DeviceStatus;
}

export interface DeviceStatus {
  connected: boolean;
  deviceName?: string;
  batteryPercentage?: number;
  readerState?: number;
}

export interface WorkerDevice extends AsyncDevice {
  startInventory(): Promise<void>;
  stopInventory(): Promise<void>;
  setRfPower(power: number): Promise<void>;
  getRfPower(): Promise<number>;
}