/**
 * Settings Store - Manages reader settings with localStorage persistence
 * Uses the same ReaderSettings shape as the worker API for consistency
 */
import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';
import { validateEPC, validateAndNormalize } from '../utils/settingsValidation';
import type { ReaderSettings } from '@/worker/types/reader';

// Check for browser environment with safer localStorage access
const isBrowser = typeof window !== 'undefined';

// Safe localStorage access function
const safeLocalStorage = {
  getItem: (key: string): string | null => {
    if (!isBrowser) return null;
    try {
      return localStorage.getItem(key);
    } catch (error) {
      console.warn('Unable to access localStorage:', error);
      return null;
    }
  },
  setItem: (key: string, value: string): void => {
    if (!isBrowser) return;
    try {
      localStorage.setItem(key, value);
    } catch (error) {
      console.warn('Unable to access localStorage:', error);
    }
  }
};

// Note: EPC validation now handled by shared utility

// Settings Store interface - extends ReaderSettings with UI-specific settings and actions
interface SettingsState extends ReaderSettings {
  // UI-specific settings (not sent to worker)
  showDebugInfo: boolean;
  showLeadingZeros: boolean;

  // Actions for updating settings
  setTransmitPower: (power: number) => void;
  setSession: (session: number) => void;
  setTargetEPC: (epc: string) => boolean; // Returns true if valid and applied
  setShowDebugInfo: (show: boolean) => void;
  setShowLeadingZeros: (show: boolean) => void;
  setBatteryCheckInterval: (interval: number) => void;
  setWorkerLogLevel: (level: 'error' | 'warn' | 'info' | 'debug') => void;
}

// Get initial values from localStorage or use defaults
const savedRfPower = safeLocalStorage.getItem('rfid_power');
const initialTransmitPower = savedRfPower ? parseFloat(savedRfPower) : 30;

const savedSession = safeLocalStorage.getItem('rfid_session');
const initialSession = savedSession ? parseInt(savedSession, 10) : 1;

const savedShowDebugInfo = safeLocalStorage.getItem('rfid_show_debug');
const initialShowDebugInfo = savedShowDebugInfo === 'true';

const savedShowLeadingZeros = safeLocalStorage.getItem('rfid_show_leading_zeros');
const initialShowLeadingZeros = savedShowLeadingZeros === 'true';

const savedTargetEPC = safeLocalStorage.getItem('locate_epc'); // Keep localStorage key for backward compatibility
const initialTargetEPC = savedTargetEPC || '';

const savedBatteryInterval = safeLocalStorage.getItem('batteryCheckInterval');
const initialBatteryInterval = savedBatteryInterval ? parseInt(savedBatteryInterval, 10) : 60;

const savedLogLevel = safeLocalStorage.getItem('workerLogLevel');
const initialLogLevel = (savedLogLevel as 'error' | 'warn' | 'info' | 'debug') || 'info';

export const useSettingsStore = create<SettingsState>(createStoreWithTracking((set) => ({
  // Initial state following ReaderSettings structure
  rfid: {
    transmitPower: initialTransmitPower,
    session: initialSession,
    targetEPC: initialTargetEPC,
  },
  // barcode section will be added as needed
  system: {
    batteryCheckInterval: initialBatteryInterval,
    workerLogLevel: initialLogLevel,
  },

  // UI-specific settings
  showDebugInfo: initialShowDebugInfo,
  showLeadingZeros: initialShowLeadingZeros,

  // Actions with localStorage persistence
  setTransmitPower: (power) => {
    // Save to localStorage
    safeLocalStorage.setItem('rfid_power', power.toString());
    // Update state in the nested structure
    set((state) => ({
      rfid: { ...state.rfid, transmitPower: power }
    }));
  },
  setSession: (session) => {
    // Save to localStorage
    safeLocalStorage.setItem('rfid_session', session.toString());
    // Update state in the nested structure
    set((state) => ({
      rfid: { ...state.rfid, session }
    }));
  },
  setTargetEPC: (epc) => {
    try {
      const normalizedEPC = validateAndNormalize(epc, validateEPC, 'targetEPC');

      safeLocalStorage.setItem('locate_epc', normalizedEPC); // Keep localStorage key for backward compatibility
      set((state) => ({
        rfid: { ...state.rfid, targetEPC: normalizedEPC }
      }));

      return true;
    } catch (error) {
      console.warn('[SettingsStore] EPC validation failed:', error instanceof Error ? error.message : String(error));
      return false;
    }
  },
  setShowDebugInfo: (show) => {
    // Save to localStorage
    safeLocalStorage.setItem('rfid_show_debug', show.toString());
    // Update state
    set({ showDebugInfo: show });
  },
  setShowLeadingZeros: (show) => {
    // Save to localStorage
    safeLocalStorage.setItem('rfid_show_leading_zeros', show.toString());
    // Update state
    set({ showLeadingZeros: show });
  },
  setBatteryCheckInterval: (interval) => {
    // Save to localStorage
    safeLocalStorage.setItem('batteryCheckInterval', String(interval));
    // Update state in the nested structure
    set((state) => ({
      system: { ...state.system, batteryCheckInterval: interval }
    }));
    // DeviceManager will automatically pick up this change via its settings subscription
  },
  setWorkerLogLevel: (level) => {
    // Save to localStorage
    safeLocalStorage.setItem('workerLogLevel', level);
    // Update state in the nested structure
    set((state) => ({
      system: { ...state.system, workerLogLevel: level }
    }));
    // DeviceManager will automatically pick up this change via its settings subscription
  },
}), 'SettingsStore'));