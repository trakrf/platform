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
import { useBarcodeStore } from '../../stores/barcodeStore';
import { useLocateStore } from '../../stores/locateStore';

export interface DeviceManagerConfig {
  transport?: TransportFactoryConfig;
}

export class DeviceManager {
  private worker: CS108WorkerAPI;
  private transport: Transport;
  private static instance: DeviceManager | null = null;
  private settingsUnsubscribe?: () => void;
  private activeTabUnsubscribe?: () => void;
  private scanButtonUnsubscribe?: () => void;

  /**
   * Simple tab-to-mode mapping
   */
  private static readonly TAB_TO_MODE: Record<string, ReaderModeType> = {
    'inventory': ReaderMode.INVENTORY,
    'locate': ReaderMode.LOCATE,
    'barcode': ReaderMode.BARCODE,
    // Everything else gets IDLE
  };

  /**
   * Switch to the appropriate mode for a tab
   */

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
    const currentTab = useUIStore.getState().activeTab;
    // Current active tab at connection

    // Set initial mode based on current tab (including IDLE for home/settings)
    const mode = DeviceManager.TAB_TO_MODE[currentTab] || ReaderMode.IDLE;
    // Use already imported settings from above (line 125-127)
    const currentSettings = useSettingsStore.getState();
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
          useBarcodeStore.getState().addBarcode({
            data: event.payload.barcode,
            type: event.payload.symbology || 'Unknown',
            timestamp: event.payload.timestamp ?? Date.now()
          });
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
              locatePayload._workerTimestamp // for metrics
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
    // UI store imported

    // Get initial tab
    const initialTab = useUIStore.getState().activeTab;
    // Initial tab determined

    // Subscribe to activeTab changes
    let previousTab = initialTab;
    this.activeTabUnsubscribe = useUIStore.subscribe(
      async (state) => {
        const activeTab = state.activeTab;

        // Only process if tab actually changed
        if (activeTab === previousTab) return;
        previousTab = activeTab;

        // Active tab changed
        // URL parameters are now handled in App.tsx BEFORE tab change
        // This ensures settings are updated before we snapshot them
        const mode = DeviceManager.TAB_TO_MODE[activeTab] || ReaderMode.IDLE;
        const settings = useSettingsStore.getState();
        await this.setMode(mode, {
          rfid: settings.rfid,
          barcode: settings.barcode,
          system: settings.system
        });
      }
    );

    // ActiveTab subscription set up
    // URL parameters are handled in App.tsx BEFORE initial tab is set
    // Set initial mode for current tab
    const initialMode = DeviceManager.TAB_TO_MODE[initialTab] || ReaderMode.IDLE;
    const settings = useSettingsStore.getState();
    await this.setMode(initialMode, {
      rfid: settings.rfid,
      barcode: settings.barcode,
      system: settings.system
    });
  }

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