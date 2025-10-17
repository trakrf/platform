/**
 * Worker API Interface
 * Defines the flat API exposed by the worker via Comlink
 */

import type { ReaderModeType, ReaderStateType, ReaderSettings } from '../../worker/types/reader.js';
import type { StandardTag, BarcodeData } from './types';

/**
 * Flat API exposed by the CS108 worker
 * All methods are at the top level for Comlink serialization
 */
export interface WorkerAPI {
  // Connection methods
  connect(port: MessagePort): Promise<boolean>;
  disconnect(): Promise<void>;

  // Configuration methods
  setMode(mode: ReaderModeType): Promise<void>;
  setSettings(settings: ReaderSettings): Promise<void>;

  // Scanning control
  startScanning(): Promise<void>;
  stopScanning(): Promise<void>;

  // Event subscriptions
  onStateChanged(callback: (state: ReaderStateType) => void): void;
  onModeChanged(callback: (mode: ReaderModeType | null) => void): void;
  onTagRead(callback: (tag: StandardTag) => void): void;
  onBarcodeRead(callback: (barcode: BarcodeData) => void): void;
  onBatteryUpdate(callback: (percentage: number) => void): void;
  onTriggerChanged(callback: (pressed: boolean) => void): void;
}