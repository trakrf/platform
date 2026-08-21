/**
 * Device Manager - Simplified with vite-plugin-comlink
 * Lifecycle = Connection: creation connects, destruction disconnects
 */

import * as Comlink from 'comlink';
import { endpointSymbol } from 'vite-plugin-comlink/symbol';
import type { StandardTag, BarcodeData } from './types';
import type { ReaderModeType, ReaderSettings } from '@/worker/types/reader.js';
import { ReaderMode, ReaderState } from '@/worker/types/reader.js';
import { TransportFactory, type TransportFactoryConfig } from './transport/transport-factory';
import type { Transport } from './transport/Transport';
import type { WorkerEvent } from '@/worker/types/events.js';
import { WorkerEventType } from '@/worker/types/events.js';
import { LogLevel } from '@/worker/utils/logger.js';

// Worker interface for type safety
interface CS108WorkerAPI {
  initialize(port: MessagePort): Promise<boolean>;
  disconnect(): Promise<void>;
  setMode(mode: ReaderModeType, settings?: ReaderSettings): Promise<void>;
  setSettings(settings: ReaderSettings): Promise<void>;
  startScanning(): Promise<void>;
  stopScanning(): Promise<void>;
  setLogLevel(level: LogLevel): void;
  setRssiDebug(enabled: boolean): void;
}

// Import stores for direct updates
import { useDeviceStore } from '../../stores/deviceStore';
import { useTagStore } from '../../stores/tagStore';
import { useLocateStore } from '../../stores/locateStore';
import type { ScanTabMode } from '../../stores/uiStore';
import { routeBarcodeRead } from './barcode-bridge';

export interface DeviceManagerConfig {
  transport?: TransportFactoryConfig;
}

/**
 * Simple tab-to-mode mapping
 */
const TAB_TO_MODE: Record<string, ReaderModeType> = {
  'scan': ReaderMode.INVENTORY,
  'locate': ReaderMode.LOCATE,
  // Kit flows are continuous inventory scans; tags land in tagStore with
  // asset/location classification (TRA-1033).
  'kits': ReaderMode.INVENTORY,
  'assets': ReaderMode.BARCODE,
  // Everything else gets IDLE
};

/**
 * Whether the Locate screen already knows what it is looking for. A blank or
 * whitespace-only target means the operator is still acquiring one (TRA-1121).
 */
export function hasLocateTarget(rfid?: { targetEPC?: string }): boolean {
  return Boolean(rfid?.targetEPC?.trim());
}

/**
 * Whether a settings change needs the reader mode reapplied. Only the Locate
 * tab cares, and only when the target appears or disappears — editing one EPC
 * into another must not churn the reader through a mode change mid-search.
 */
export function shouldReapplyModeForTarget(
  previousHasTarget: boolean,
  nextHasTarget: boolean,
  tab: string
): boolean {
  return tab === 'locate' && previousHasTarget !== nextHasTarget;
}

/**
 * Resolve the reader mode for a tab. The Scan tab is dual-mode (TRA-1031):
 * its RFID|Barcode toggle decides between INVENTORY and BARCODE. The Kits tab
 * is dual-mode per view (TRA-1033): commission defaults to barcode, verify to
 * RFID — kitsScanMode is the effective mode of the active kit view.
 *
 * The Locate tab is dual-mode by target (TRA-1121). The trigger means "do the
 * thing this screen is for", and that depends on whether the screen already
 * knows what to look for: with no target the operator is still acquiring one,
 * so park in BARCODE and let the trigger scan a label; once a target is set,
 * LOCATE so the trigger searches for it. Nothing binds the trigger to either
 * action — the worker already starts and stops whatever the current mode is.
 *
 * locateHasTarget defaults to true so a caller that says nothing about the
 * target gets the search behaviour rather than silently arming the scanner.
 */
export function resolveModeForTab(
  tab: string,
  scanTabMode: ScanTabMode,
  kitsScanMode: ScanTabMode = 'rfid',
  locateHasTarget: boolean = true
): ReaderModeType {
  if (tab === 'scan' && scanTabMode === 'barcode') return ReaderMode.BARCODE;
  if (tab === 'kits' && kitsScanMode === 'barcode') return ReaderMode.BARCODE;
  if (tab === 'locate' && !locateHasTarget) return ReaderMode.BARCODE;
  return TAB_TO_MODE[tab] || ReaderMode.IDLE;
}

export class DeviceManager {
  private worker: CS108WorkerAPI;
  private transport: Transport;
  private static instance: DeviceManager | null = null;
  private settingsUnsubscribe?: () => void;
  private activeTabUnsubscribe?: () => void;
  private kitsModeUnsubscribe?: () => void;
  // Mode-resolution inputs. Instance fields rather than closure state because
  // setupSettingsSubscription needs them too.
  private previousTab = '';
  private previousScanMode: ScanTabMode = 'rfid';
  private previousKitsMode: ScanTabMode = 'rfid';
  private previousHasTarget = true;
  private scanButtonUnsubscribe?: () => void;

  /**
   * Constructor implements lifecycle = connection pattern
   * Creating the DeviceManager connects to the device
   */
  private constructor(transport: Transport, worker: CS108WorkerAPI) {
    this.transport = transport;
    this.worker = worker;
  }

  /**
   * Create and connect to device
   * This replaces getInstance + connect pattern
   */
  static async create(config: DeviceManagerConfig): Promise<DeviceManager> {
    // Prevent multiple instances
    if (DeviceManager.instance) {
      throw new Error('Device already connected. Call destroy() first.');
    }

    // Creating device manager

    // Create transport
    const transport = TransportFactory.create(config.transport);

    // Connect transport and get MessagePort
    // Connecting transport
    const port = await transport.connect();
    if (!port) {
      await transport.disconnect();
      throw new Error('Failed to establish transport connection');
    }

    // Create worker instance with Comlink auto-proxying
    // Creating worker with Comlink
    const worker = new ComlinkWorker<typeof import('../../worker/cs108-worker')>(
      new URL('../../worker/cs108-worker', import.meta.url),
      { type: 'module' }
    );

    // Create manager BEFORE initializing worker
    // This allows us to set up event handlers before any events are emitted
    DeviceManager.instance = new DeviceManager(transport, worker);

    // Set up event callback BEFORE initializing worker
    // This ensures we capture all events during initialization
    DeviceManager.instance.setupEventCallback();

    // NOW initialize worker with transport port - use Comlink.transfer for MessagePort
    // Initializing worker with transport
    const success = await worker.initialize(Comlink.transfer(port, [port]));
    if (!success) {
      await transport.disconnect();
      // ComlinkWorker cleanup is handled by the plugin
      throw new Error('Worker failed to initialize with transport');
    }

    // Set up settings subscription for live updates
    await DeviceManager.instance.setupSettingsSubscription();

    // Push initial settings to worker so it starts with current UI values
    // Pushing initial settings to worker
    const { useSettingsStore } = await import('../../stores/settingsStore');
    const initialState = useSettingsStore.getState();
    try {
      // Extract only the ReaderSettings portion (exclude functions)
      const initialSettings: ReaderSettings = {
        rfid: initialState.rfid,
        barcode: initialState.barcode,
        system: initialState.system
      };
      await worker.setSettings(initialSettings);
      // Initial settings pushed to worker
    } catch (error) {
      // Worker rejected initial settings
    }

    // Set up activeTab subscription for automatic mode switching
    // Setting up activeTab subscription
    await DeviceManager.instance.setupActiveTabSubscription();

    // Now that we're connected, set the mode based on current activeTab
    const { useUIStore } = await import('../../stores/uiStore');
    const { useKitStore, getKitsScanMode } = await import('../../stores/kitStore');
    const currentTab = useUIStore.getState().activeTab;
    // Current active tab at connection

    // Set initial mode based on current tab (including IDLE for home/settings).
    // Must include the kits view mode or connecting while on the Kits tab
    // configures INVENTORY under a Barcode toggle (TRA-1033).
    // Use already imported settings from above (line 125-127)
    const currentSettings = useSettingsStore.getState();
    const mode = resolveModeForTab(
      currentTab,
      useUIStore.getState().scanTabMode,
      getKitsScanMode(useKitStore.getState()),
      hasLocateTarget(currentSettings.rfid)
    );
    await DeviceManager.instance.setMode(mode, {
      rfid: currentSettings.rfid,
      barcode: currentSettings.barcode,
      system: currentSettings.system
    });

    // Expose for E2E testing
    if (typeof window !== 'undefined' && import.meta.env.MODE === 'test') {
      window.__DEVICE_MANAGER__ = DeviceManager.instance;
    }

    console.info('[DeviceManager] Device manager created successfully');
    return DeviceManager.instance;
  }

  /**
   * Get current instance
   */
  static getInstance(): DeviceManager | null {
    return DeviceManager.instance;
  }

  /**
   * Set up event listener for worker events
   * Events flow directly through postMessage
   */
  private setupEventCallback(): void {
    // Setting up event listener

    // Get the underlying Worker instance
    const rawWorker = (this.worker as unknown as { [key: symbol]: Worker })[endpointSymbol];

    // Set up message handler directly on the worker
    rawWorker.onmessage = (e: MessageEvent) => {
      // Raw message received

      // Check if this is a valid worker event
      if (!e.data || typeof e.data !== 'object' || !e.data.type) {
        console.warn('[DeviceManager] Received non-event message:', e.data);
        return;
      }

      const event = e.data as WorkerEvent;
      // Debug logging for all events
      // Received event via Comlink

      switch (event.type) {
        case WorkerEventType.READER_STATE_CHANGED: {
          const newState = event.payload.readerState;
          const prevState = useDeviceStore.getState().readerState;
          useDeviceStore.getState().setReaderState(newState);

          // If reader transitions from SCANNING to READY (scan completed)
          // and the scan button is still active, restart scanning
          // This keeps inventory/locate running when triggered by UI button
          if (prevState === ReaderState.SCANNING && newState === ReaderState.CONNECTED) {
            if (useDeviceStore.getState().scanButtonActive) {
              const currentMode = useDeviceStore.getState().readerMode;
              console.debug(`[DeviceManager] ${currentMode} scan completed, button still active - restarting`);
              // Restart scanning after a short delay to let state settle
              setTimeout(async () => {
                try {
                  // Double-check conditions before restarting to avoid race conditions
                  if (useDeviceStore.getState().scanButtonActive &&
                      useDeviceStore.getState().readerState === ReaderState.CONNECTED &&
                      !useDeviceStore.getState().triggerState) {
                    console.debug('[DeviceManager] Restarting scan for continuous button operation');
                    await this.worker.startScanning();
                  }
                } catch (error) {
                  console.error('[DeviceManager] Failed to restart scanning:', error);
                  useDeviceStore.setState({ scanButtonActive: false });
                }
              }, 100); // Slightly longer delay to avoid race conditions
            }
          }
          break;
        }

        case WorkerEventType.READER_MODE_CHANGED:
          useDeviceStore.getState().setReaderMode(event.payload.mode);
          break;

        case WorkerEventType.TAG_READ:
          // Handle array of tags from TAG_READ event
          event.payload.tags.forEach(tag => {
            useTagStore.getState().addTag({
              epc: tag.epc,
              rssi: tag.rssi,
              count: 1,
              antenna: tag.antennaPort ?? 1,
              timestamp: tag.timestamp ?? Date.now(),
              source: 'rfid'
            });
          });
          break;

        case WorkerEventType.BARCODE_READ:
          routeBarcodeRead(event.payload);
          break;

        case WorkerEventType.BATTERY_UPDATE:
          useDeviceStore.getState().setBatteryPercentage(event.payload.percentage);
          break;

        case WorkerEventType.LOCATE_UPDATE: {
          // Route locate updates to the locate store
          // Ignore readings older than 1 second (stale data)
          const locatePayload = event.payload;
          const now = Date.now();
          const age = now - locatePayload.timestamp;

          // Debug: log raw vs smoothed if enabled
          if ((window as unknown as Record<string, unknown>).__LOCATE_DEBUG_RAW) {
            console.log(`[RAW] raw=${locatePayload.rssi} smoothed=${locatePayload.smoothedRssi} wb=${locatePayload.wbRssi}`);
          }

          if (age <= 1000) {
            useLocateStore.getState().addRssiReading(
              locatePayload.smoothedRssi ?? locatePayload.rssi,
              locatePayload.wbRssi,
              undefined, // phase not in payload
              locatePayload._workerTimestamp, // for metrics
              locatePayload.epc // so the store can reject reads from other tags
            );
          } else {
            console.debug(`[DeviceManager] Ignoring stale locate update (${age}ms old)`);
          }
          break;
        }

        case WorkerEventType.TRIGGER_STATE_CHANGED: {
          // Handle trigger state from worker
          const triggerPayload = event.payload;
          useDeviceStore.getState().setTriggerState(triggerPayload.pressed);
          // Worker handles start/stop operations directly
          // Don't sync scanButtonActive - let trigger and button be independent
          break;
        }

        case WorkerEventType.DEBUG_LOG: {
          // Forward debug logs from worker to console
          const debugPayload = event.payload;
          const prefix = debugPayload.context ? `[Worker:${debugPayload.context}]` : '[Worker]';
          console.log(`${prefix} ${debugPayload.message}`, debugPayload.details || '');
          break;
        }

        case WorkerEventType.TRANSPORT_DISCONNECTED: {
          // Transport layer died unexpectedly - perform "honorable suicide"
          const transportPayload = event.payload;
          console.warn(`[DeviceManager] Transport disconnected: ${transportPayload.reason || 'Unknown reason'}`);

          // Destroy singleton to clean up state mismatch
          // This prevents "Device already connected" error when user tries to reconnect
          DeviceManager.instance?.destroy().catch((error: unknown) => {
            console.error('[DeviceManager] Failed to destroy singleton after transport disconnect:', error);
          });
          break;
        }

        default:
          // TypeScript will ensure this never happens with proper typing
          // Unknown event type
          break;
      }
    };

    // Event listener set up on worker
  }

  /**
   * Set up subscription to activeTab changes for automatic mode switching
   */
  private async setupActiveTabSubscription(): Promise<void> {
    // setupActiveTabSubscription called

    // Import UI store dynamically
    const { useUIStore } = await import('../../stores/uiStore');
    const { useSettingsStore } = await import('../../stores/settingsStore');
    const { useKitStore, getKitsScanMode } = await import('../../stores/kitStore');
    // UI store imported

    // Get initial tab
    const initialTab = useUIStore.getState().activeTab;
    // Initial tab determined

    // Subscribe to activeTab AND scanTabMode changes (TRA-1031), plus the
    // Kits tab's per-view RFID|Barcode mode (TRA-1033). The Locate tab's target
    // (TRA-1121) is watched in setupSettingsSubscription instead: it lives in
    // settingsStore, and a second subscriber there would race the settings push
    // for the worker's command mutex.
    this.previousTab = initialTab;
    this.previousScanMode = useUIStore.getState().scanTabMode;
    this.previousKitsMode = getKitsScanMode(useKitStore.getState());
    this.previousHasTarget = hasLocateTarget(useSettingsStore.getState().rfid);

    this.activeTabUnsubscribe = useUIStore.subscribe(
      async (state) => {
        const { activeTab, scanTabMode } = state;

        // Only process if the resolved mode inputs actually changed
        if (activeTab === this.previousTab && scanTabMode === this.previousScanMode) return;
        this.previousTab = activeTab;
        this.previousScanMode = scanTabMode;
        await this.applyResolvedMode();
      }
    );

    this.kitsModeUnsubscribe = useKitStore.subscribe(
      async (state) => {
        const kitsMode = getKitsScanMode(state);
        if (kitsMode === this.previousKitsMode) return;
        this.previousKitsMode = kitsMode;
        // Only reconfigure the reader while the Kits tab drives it
        if (this.previousTab !== 'kits') return;
        await this.applyResolvedMode();
      }
    );

    // ActiveTab subscription set up
    // URL parameters are handled in App.tsx BEFORE initial tab is set
    // Set initial mode for current tab
    await this.applyResolvedMode();
  }

  /**
   * Push the mode the current tab resolves to. The single place that decides a
   * mode, so every caller agrees on what the reader should be doing.
   */
  private applyResolvedMode = async (): Promise<void> => {
    const { useSettingsStore } = await import('../../stores/settingsStore');
    const settings = useSettingsStore.getState();
    const mode = resolveModeForTab(
      this.previousTab,
      this.previousScanMode,
      this.previousKitsMode,
      this.previousHasTarget
    );
    await this.setMode(mode, {
      rfid: settings.rfid,
      barcode: settings.barcode,
      system: settings.system
    });
  };

  /**
   * Set up subscription to settings store for live updates
   * Simple dumb pipe - just pass ALL settings through to worker
   * The worker has all the logic to filter based on mode and state
   */
  private async setupSettingsSubscription(): Promise<void> {
    // Setting up settings subscription

    // Import settings store dynamically
    const { useSettingsStore } = await import('../../stores/settingsStore');

    // Subscribe to ALL settings changes and pass them through
    // No filtering, no state checking - that's the worker's job
    this.settingsUnsubscribe = useSettingsStore.subscribe(
      async (state) => {
        // The Locate target lives in these settings, and gaining or losing one
        // flips the tab between acquiring (BARCODE) and searching (LOCATE)
        // (TRA-1121). Decided here, and applied *after* the settings push,
        // because both are commands into a non-re-entrant CommandManager: two
        // subscribers issuing them back to back means the second loses the
        // mutex with "Command already active". Losing setSettings is benign —
        // reader.ts:652 says why — but a lost setMode is never reapplied, and
        // the reader silently stays in the mode it was already in.
        const nextHasTarget = hasLocateTarget(state.rfid);
        const modeNeedsReapplying = shouldReapplyModeForTarget(
          this.previousHasTarget,
          nextHasTarget,
          this.previousTab
        );
        this.previousHasTarget = nextHasTarget;

        try {
          // Extract only the ReaderSettings portion (exclude functions)
          const settings: ReaderSettings = {
            rfid: state.rfid,
            barcode: state.barcode,
            system: state.system
          };

          // Pass the serializable settings - worker decides what to use
          await this.worker.setSettings(settings);
        } catch (error) {
          // Worker will throw if not in READY state or settings are invalid for mode
          // Worker rejected settings
        }

        if (!modeNeedsReapplying) return;
        try {
          await this.applyResolvedMode();
        } catch (error) {
          // Never swallow this one silently: an unapplied mode change is why
          // a cleared Locate target left the reader still hunting the old EPC.
          console.error('[DeviceManager] Failed to apply mode after target change:', error);
        }
      }
    );

    // Settings subscription set up

    // Subscribe to scanButtonActive changes
    // When UI toggles the scan button, sync the reader state accordingly
    this.scanButtonUnsubscribe = useDeviceStore.subscribe(
      async (state, prevState) => {
        // Only react to scanButtonActive changes
        if (state.scanButtonActive === prevState.scanButtonActive) return;

        try {
          if (state.scanButtonActive) {
            console.debug('[DeviceManager] Scan button activated - starting scanning');
            await this.worker.startScanning();
          } else {
            console.debug('[DeviceManager] Scan button deactivated - stopping scanning');
            await this.worker.stopScanning();
          }
        } catch (error) {
          console.error('[DeviceManager] Failed to sync scanning state:', error);
          // Reset the button state on error
          useDeviceStore.setState({ scanButtonActive: false });
        }
      }
    );
  }

  /**
   * Direct proxy methods - pass through to worker
   */
  setMode = async (mode: ReaderModeType, settings?: ReaderSettings) => {
    // Just pass through to worker - it handles duplicate calls efficiently
    await this.worker.setMode(mode, settings);
  };

  setSettings = (settings: ReaderSettings) => this.worker.setSettings(settings);
  startScanning = () => this.worker.startScanning();
  stopScanning = () => this.worker.stopScanning();

  /**
   * Check if connected
   */
  isConnected(): boolean {
    return this.transport?.isConnected() || false;
  }

  /**
   * Destroy the manager and clean up resources
   * This replaces the disconnect() method
   */
  async destroy(): Promise<void> {
    // Destroying device manager

    try {
      // Clean up subscriptions
      if (this.settingsUnsubscribe) {
        this.settingsUnsubscribe();
        this.settingsUnsubscribe = undefined;
      }

      if (this.activeTabUnsubscribe) {
        this.activeTabUnsubscribe();
        this.activeTabUnsubscribe = undefined;
      }

      if (this.kitsModeUnsubscribe) {
        this.kitsModeUnsubscribe();
        this.kitsModeUnsubscribe = undefined;
      }

      if (this.scanButtonUnsubscribe) {
        this.scanButtonUnsubscribe();
        this.scanButtonUnsubscribe = undefined;
      }

      // Disconnect worker from transport
      if (this.worker) {
        await this.worker.disconnect();
        // ComlinkWorker cleanup is handled by the plugin
      }

      // Disconnect transport
      if (this.transport) {
        await this.transport.disconnect();
      }
    } finally {
      // Clear singleton
      DeviceManager.instance = null;

      // Clear test exposure
      if (typeof window !== 'undefined' && import.meta.env.MODE === 'test') {
        delete window.__DEVICE_MANAGER__;
      }

      console.info('[DeviceManager] Device manager destroyed');
    }
  }

  /**
   * Backward compatibility - redirect to destroy
   * @deprecated Use destroy() instead
   */
  async disconnect(): Promise<void> {
    await this.destroy();
  }

  /**
   * Legacy subscription methods for backward compatibility
   * These now do nothing as events are handled via native message listening
   */
  onStateChange(_callback: (state: unknown) => void): void {
    console.warn('[DeviceManager] onStateChange is deprecated - events flow through native postMessage');
  }

  onModeChange(_callback: (mode: unknown) => void): void {
    console.warn('[DeviceManager] onModeChange is deprecated - events flow through native postMessage');
  }

  onTagRead(_callback: (tag: StandardTag) => void): void {
    console.warn('[DeviceManager] onTagRead is deprecated - events flow through native postMessage');
  }

  onBarcodeRead(_callback: (barcode: BarcodeData) => void): void {
    console.warn('[DeviceManager] onBarcodeRead is deprecated - events flow through native postMessage');
  }

  onBatteryUpdate(_callback: (battery: unknown) => void): void {
    console.warn('[DeviceManager] onBatteryUpdate is deprecated - events flow through native postMessage');
  }

  onTriggerChanged(_callback: (pressed: boolean) => void): void {
    console.warn('[DeviceManager] onTriggerChanged is deprecated - events flow through native postMessage');
  }

  /**
   * Enable/disable RSSI debug logging in the worker
   * Shows raw byte values and both formula results for calibration
   * Usage: DeviceManager.getInstance()?.setRssiDebug(true)
   *   or: window.__enableRssiDebug(true)
   */
  setRssiDebug(enabled: boolean): void {
    this.worker.setRssiDebug(enabled);
    console.info(`[DeviceManager] RSSI debug ${enabled ? 'enabled' : 'disabled'}`);
  }
}

// Expose RSSI debug toggle on window for easy console access
if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).__enableRssiDebug = (enabled: boolean) => {
    const manager = DeviceManager.getInstance();
    if (manager) {
      manager.setRssiDebug(enabled);
    } else {
      console.warn('[DeviceManager] No device connected. Connect first, then call __enableRssiDebug(true)');
    }
  };
}