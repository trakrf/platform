/**
 * Device Store - Manages device connection state and device information
 */
import { create } from 'zustand';
import { ReaderState, type ReaderStateType, type ReaderModeType } from '@/worker/types/reader';
import { trackRFIDOperation } from '@/lib/openreplay';
import { createStoreWithTracking } from './createStore';
import { DeviceManager } from '@/lib/device/device-manager';

// Device store interface
interface DeviceState {
  // Connection state
  readerState: ReaderStateType;
  readerMode: ReaderModeType | null;
  deviceName: string | null;
  batteryPercentage: number | null;
  triggerState: boolean;

  // UI state
  scanButtonActive: boolean;  // UI toggle button state - triggers scanning when true

  // Computed properties
  isConnected: boolean;
  isScanning: boolean;  // Computed from readerState === SCANNING

  // Actions
  setReaderState: (state: ReaderStateType) => void;
  setReaderMode: (mode: ReaderModeType | null) => void;
  setDeviceName: (name: string | null) => void;
  setBatteryPercentage: (percentage: number | null) => void;
  setTriggerState: (isDown: boolean) => Promise<void>;

  // UI Scanning control - sets the button state, DeviceManager reacts to this
  toggleScanButton: () => void;

  // Connection methods
  connect: () => Promise<void>;
  disconnect: () => Promise<void>;
}

export const useDeviceStore = create<DeviceState>(createStoreWithTracking((set, get) => ({
  // Initial state
  readerState: ReaderState.DISCONNECTED,
  readerMode: null,
  deviceName: null,
  batteryPercentage: null,
  triggerState: false,

  // UI state
  scanButtonActive: false,

  // Computed properties - these are derived from readerState
  isConnected: false,
  isScanning: false,
  
  // Actions
  setReaderState: (state) => set((prevState) => {
    // Log warning for suspicious state transitions (DISCONNECTED -> CONNECTED without CONNECTING)
    if (prevState.readerState === ReaderState.DISCONNECTED && state === ReaderState.CONNECTED) {
      console.warn('[DeviceStore] WARNING: Setting CONNECTED after DISCONNECTED - this is likely a bug');
      console.trace();
    }

    const isConnected = state !== ReaderState.DISCONNECTED;
    const isScanning = state === ReaderState.SCANNING;

    // Sync scan button state with actual reader state
    // If reader stops scanning (goes to READY), turn off the button
    // If reader disconnects or errors, also turn off the button
    let scanButtonActive = prevState.scanButtonActive;
    if (state === ReaderState.CONNECTED && prevState.readerState === ReaderState.SCANNING) {
      scanButtonActive = false;
    } else if (state === ReaderState.DISCONNECTED) {
      scanButtonActive = false;
    } else if (state === ReaderState.ERROR) {
      scanButtonActive = false;
    }

    return { readerState: state, isConnected, isScanning, scanButtonActive };
  }),
  setReaderMode: (mode) => set(() => {
    return { readerMode: mode };
  }),
  setDeviceName: (name) => set({ deviceName: name }),
  setBatteryPercentage: (percentage) => set({ batteryPercentage: percentage }),
  setTriggerState: async (isDown) => {
    set({ triggerState: isDown });
    // This just tracks UI state - actual trigger handling happens in the worker
    // when it receives 0xA102/0xA103 notifications from hardware or mock
  },

  // UI Scanning control - toggles the button state
  // DeviceManager subscribes to this and reacts by calling startScanning/stopScanning
  toggleScanButton: () => set((state) => {
    return { scanButtonActive: !state.scanButtonActive };
  }),

  // Connection methods
  connect: async () => {
    set({ readerState: ReaderState.CONNECTING });

    try {
      // Use new simplified DeviceManager.create pattern
      // This creates and connects in one step, and subscriptions are handled internally
      await DeviceManager.create({
        transport: { mode: 'auto' }
      });

      // Connection successful (create throws on failure)
      set({
        deviceName: 'CS108',
        isConnected: true
      });
      // Don't set readerState here - it's handled by internal subscriptions
      
      trackRFIDOperation('connect', { 
        deviceName: 'CS108'
      });
    } catch (error) {
      console.error('Connection failed:', error);
      set({ 
        readerState: ReaderState.DISCONNECTED,
        isConnected: false
      });
      trackRFIDOperation('error', { 
        operation: 'connect',
        error: error instanceof Error ? error.message : String(error) 
      });
      throw error;
    }
  },
  
  disconnect: async () => {
    try {
      const deviceManager = DeviceManager.getInstance();
      if (deviceManager) {
        await deviceManager.destroy();
      }

      trackRFIDOperation('disconnect', {
        deviceName: get().deviceName
      });
    } catch (error) {
      console.error('Failed to disconnect:', error);
      trackRFIDOperation('error', { 
        operation: 'disconnect',
        error: error instanceof Error ? error.message : String(error) 
      });
      throw error;
    } finally {
      set({
        readerState: ReaderState.DISCONNECTED,
        readerMode: null,
        deviceName: null,
        batteryPercentage: null,
        triggerState: false,
        isConnected: false,
        isScanning: false,
        scanButtonActive: false
      });
    }
  },
}), 'DeviceStore'));