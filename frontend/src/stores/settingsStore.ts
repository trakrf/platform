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
  autoClearOnSave: boolean;

  // Actions for updating settings
  setTransmitPower: (power: number) => void;
  setSession: (session: number) => void;
  setTargetEPC: (epc: string) => boolean; // Returns true if valid and applied
  setShowDebugInfo: (show: boolean) => void;
  setShowLeadingZeros: (show: boolean) => void;
  setAutoClearOnSave: (enabled: boolean) => void;
  setBatteryCheckInterval: (interval: number) => void;
  setWorkerLogLevel: (level: 'error' | 'warn' | 'info' | 'debug') => void;

  // Tag data capture (TRA-1251)
  setCaptureAllTagData: (enabled: boolean) => void;
  setTidWords: (words: number) => void;
  setUserOffset: (offset: number) => void;
  setUserWords: (words: number) => void;
}

/**
 * Clamp a register field to the width the hardware actually gives it.
 *
 * TAGACC_CNT allots 8 bits to each word count and TAGACC_PTR 16 bits to each
 * offset. An out-of-range value does not fail — it gets masked, which means the
 * radio quietly reads a different number of words from a different place than
 * the operator asked for. Clamping here makes the limit visible in the UI
 * instead.
 *
 * A non-numeric input keeps the previous value rather than writing NaN into a
 * register.
 */
function clampSetting(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.trunc(value)));
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

const savedAutoClearOnSave = safeLocalStorage.getItem('inventory_auto_clear_on_save');
const initialAutoClearOnSave = savedAutoClearOnSave === 'true';

const savedTargetEPC = safeLocalStorage.getItem('locate_epc'); // Keep localStorage key for backward compatibility
const initialTargetEPC = savedTargetEPC || '';

const savedBatteryInterval = safeLocalStorage.getItem('batteryCheckInterval');
const initialBatteryInterval = savedBatteryInterval ? parseInt(savedBatteryInterval, 10) : 60;

const savedLogLevel = safeLocalStorage.getItem('workerLogLevel');
const initialLogLevel = (savedLogLevel as 'error' | 'warn' | 'info' | 'debug') || 'info';

// Tag data capture (TRA-1251). Off unless explicitly stored — capture drops
// inventory into normal mode with a tag_delay of 30, which is a real
// throughput cost, so it is never the default.
const savedCaptureAll = safeLocalStorage.getItem('rfid_capture_all_tag_data');
const initialCaptureAll = savedCaptureAll === 'true';

const savedTidWords = safeLocalStorage.getItem('rfid_tid_words');
const initialTidWords = savedTidWords ? parseInt(savedTidWords, 10) : 6;

const savedUserOffset = safeLocalStorage.getItem('rfid_user_offset');
const initialUserOffset = savedUserOffset ? parseInt(savedUserOffset, 10) : 0;

// parseInt is used rather than `|| 4` because 0 is a meaningful stored value
// here: it means "read TID only".
const savedUserWords = safeLocalStorage.getItem('rfid_user_words');
const initialUserWords = savedUserWords !== null ? parseInt(savedUserWords, 10) : 4;

export const useSettingsStore = create<SettingsState>(createStoreWithTracking((set) => ({
  // Initial state following ReaderSettings structure
  rfid: {
    transmitPower: initialTransmitPower,
    session: initialSession,
    targetEPC: initialTargetEPC,
    captureAllTagData: initialCaptureAll,
    tidWords: initialTidWords,
    userOffset: initialUserOffset,
    userWords: initialUserWords,
  },
  // barcode section will be added as needed
  system: {
    batteryCheckInterval: initialBatteryInterval,
    workerLogLevel: initialLogLevel,
  },

  // UI-specific settings
  showDebugInfo: initialShowDebugInfo,
  showLeadingZeros: initialShowLeadingZeros,
  autoClearOnSave: initialAutoClearOnSave,

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
  setAutoClearOnSave: (enabled) => {
    safeLocalStorage.setItem('inventory_auto_clear_on_save', enabled.toString());
    set({ autoClearOnSave: enabled });
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

  // Tag data capture (TRA-1251). DeviceManager picks these up through the same
  // settings subscription as everything else; MODE_SETTINGS.INVENTORY is what
  // lets them reach the radio.
  setCaptureAllTagData: (enabled) => {
    safeLocalStorage.setItem('rfid_capture_all_tag_data', enabled.toString());
    set((state) => ({
      rfid: { ...state.rfid, captureAllTagData: enabled }
    }));
  },
  setTidWords: (words) => {
    set((state) => {
      // Zero words of TID is not a shorter read, it is no read — and the flag
      // that turns the whole feature off already exists.
      const tidWords = clampSetting(words, 1, 255, state.rfid?.tidWords ?? 6);
      safeLocalStorage.setItem('rfid_tid_words', String(tidWords));
      return { rfid: { ...state.rfid, tidWords } };
    });
  },
  setUserOffset: (offset) => {
    set((state) => {
      const userOffset = clampSetting(offset, 0, 65535, state.rfid?.userOffset ?? 0);
      safeLocalStorage.setItem('rfid_user_offset', String(userOffset));
      return { rfid: { ...state.rfid, userOffset } };
    });
  },
  setUserWords: (words) => {
    set((state) => {
      // 0 is legal and load-bearing here: it drops the two-bank read to a
      // TID-only read, which is the way out when a chip has no USER bank.
      const userWords = clampSetting(words, 0, 255, state.rfid?.userWords ?? 4);
      safeLocalStorage.setItem('rfid_user_words', String(userWords));
      return { rfid: { ...state.rfid, userWords } };
    });
  },
}), 'SettingsStore'));