/**
 * Worker Proxy Types
 */

import { Remote } from 'comlink';
import type { DeviceState, StandardTag, BarcodeData } from './common';
import type { BatteryInfo } from '@/lib/device/types';

// Temporary stub interface for worker device
export interface WorkerDevice {
  connect(port: MessagePort): Promise<boolean>;
  disconnect(): Promise<void>;
  subscribe(callback: (state: DeviceState) => void): Promise<() => void>;
  rfid: {
    onTagRead(callback: (tag: StandardTag) => void): Promise<void>;
    startRead(): Promise<void>;
    stopRead(): Promise<void>;
  };
  barcode: {
    onBarcodeRead(callback: (barcode: BarcodeData) => void): Promise<void>;
    startDecode(): Promise<void>;
    stopDecode(): Promise<void>;
  };
  system: {
    getBattery(): Promise<number>;
    playBeep(duration?: number): Promise<void>;
    onBatteryUpdate(callback: (battery: BatteryInfo) => void): Promise<void>;
    onTriggerPress(callback: () => void): Promise<void>;
    onTriggerRelease(callback: () => void): Promise<void>;
  };
}

export type WorkerProxy<T> = Remote<T> & WorkerDevice;

export interface WorkerAPI {
  createDevice(deviceId: string): Promise<void>;
  destroyDevice(deviceId: string): Promise<void>;
  getDevice(deviceId: string): Promise<unknown>;
}