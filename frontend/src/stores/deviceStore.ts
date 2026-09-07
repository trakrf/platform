/**
 * Device Store - Manages device connection state and device information
 */
import { create } from 'zustand';
import { ReaderState, type ReaderStateType, type ReaderModeType, type ReaderDetails } from '@/worker/types/reader';
import { trackRFIDOperation } from '@/lib/openreplay';
import { createStoreWithTracking } from './createStore';
import { DeviceManager } from '@/lib/device/device-manager';

// Device store interface
interface DeviceState {
  // Connection state
  readerState: ReaderStateType;
  readerMode: ReaderModeType | null;
  deviceName: string | null;
  /**
   * What the connected reader turned out to be — firmware versions, serial,
   * MAC error. `null` until the reader answers, and null again on disconnect.
   *
   * Fields inside it are individually optional for the same reason the object
   * is nullable: the five values are read at two different moments, and an
   * absent one means "no answer", never a value. TRA-1232.
   */
  readerDetails: ReaderDetails | null;
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
  setReaderDetails: (details: ReaderDetails | null) => void;
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
  readerDetails: null,
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

    /**
     * The mirror of the warning above, and it exists because its absence cost a
     * whole bench arm.
     *
     * TRA-1259: `hold-sweep` found the reader `Disconnected` three seconds after
     * its own `beforeAll` had confirmed `Connected`, with the link demonstrably
     * intact. Reconstructing WHY meant reading every route to DISCONNECTED after
     * the fact, because the transition itself left no record — the worker logs
     * its own transitions at `logger.debug`, which an e2e run does not forward.
     *
     * A stack trace names the caller at the moment it happens, which is the
     * difference between one reproduction being enough and needing another arm.
     * Deliberately only on the way OUT of a connected state: DISCONNECTED →
     * DISCONNECTED is the ordinary idempotent teardown and says nothing.
     */
    if (state === ReaderState.DISCONNECTED && prevState.readerState !== ReaderState.DISCONNECTED) {
      console.warn(
        `[DeviceStore] Reader lost CONNECTED: ${prevState.readerState} -> Disconnected. ` +
        'Trace names the caller — TRA-1259.'
      );
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
  setReaderDetails: (details) => set({ readerDetails: details }),
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
    // Held so the guard-refusal path below can put it back. `CONNECTING` is
    // published before we know whether there is anything to connect, and it is
    // a TRANSIENT state — `waitForSettledState` and the e2e trigger helpers
    // both park on it — so leaving it behind is worse than the disconnect it
    // replaced, not better.
    const stateBeforeConnect = get().readerState;
    set({ readerState: ReaderState.CONNECTING });

    try {
      // Use new simplified DeviceManager.create pattern
      // This creates and connects in one step, and subscriptions are handled internally
      //
      // No transport mode to select any more: there is one transport, and a
      // browser without Web Bluetooth is an error rather than a silent switch
      // to fabricated data (TRA-1177 §5).
      await DeviceManager.create({
        transport: {}
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

      /**
       * Publish DISCONNECTED only when there is in fact nothing connected.
       *
       * `create()` has two failure shapes and they need opposite treatment.
       * When construction fails it destroys its own half-built singleton (
       * TRA-1250), so `getInstance()` is null and DISCONNECTED is the truth.
       * But the guard at the top of `create()` throws `Device already
       * connected. Call destroy() first.` BEFORE touching anything — the
       * existing manager, its worker and its transport are all alive and
       * working. Publishing DISCONNECTED there tells the UI the reader is gone
       * while it is sitting there connected, and it does it WITHOUT a transport
       * event, so no `link-close` and no `link-teardown` accompanies it.
       *
       * That is exactly TRA-1259's signature — store loses CONNECTED, link
       * intact, nothing in the log — which is why this route is closed rather
       * than merely instrumented. It is NOT a claim that this is what happened
       * in the observed failure; nothing yet establishes that. It is a route to
       * the signature that should not exist either way.
       */
      const stillConnected = DeviceManager.getInstance() !== null;
      if (stillConnected) {
        console.warn(
          '[DeviceStore] connect() failed but a live DeviceManager remains — ' +
          'restoring reader state rather than reporting a disconnect that did ' +
          'not happen. TRA-1259.'
        );
        // Only if nothing newer has landed. The surviving manager's worker is
        // still publishing, so a state that has moved on since is the truth and
        // this one is stale.
        if (get().readerState === ReaderState.CONNECTING) {
          get().setReaderState(stateBeforeConnect);
        }
      } else {
        set({
          readerState: ReaderState.DISCONNECTED,
          isConnected: false
        });
      }

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
        // These described the reader that just went away. Carrying them would
        // open the next connection showing the previous device's firmware,
        // which is worse than showing nothing because it looks read.
        readerDetails: null,
        batteryPercentage: null,
        triggerState: false,
        isConnected: false,
        isScanning: false,
        scanButtonActive: false
      });
    }
  },
}), 'DeviceStore'));