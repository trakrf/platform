/**
 * CS108 RFID Reader Implementation - WORKER THREAD ONLY
 *
 * ⚠️ STRICT ISOLATION: This code runs in a Web Worker thread.
 *
 * IMPORT RULES:
 * - ✅ ONLY DeviceFactory creates worker instance (via new Worker())
 * - ❌ UI components MUST NOT import from this directory
 * - ❌ Stores MUST NOT import from this directory
 * - ❌ Other workers MUST NOT import from this directory
 * - ❌ Even DeviceManager doesn't import - it uses message passing
 *
 * This isolation is critical because:
 * 1. Worker code runs in a separate thread with no DOM access
 * 2. Communication happens only through message passing (postMessage/onmessage)
 * 3. Direct imports would break the thread boundary
 *
 * Public types are exposed via worker/types/reader.ts
 * Worker instantiation happens via DeviceFactory.createWorker()
 */

import { BaseReader } from '../BaseReader.js';
import {
  ReaderState,
  ReaderMode,
  RainTarget,
  type ReaderModeType,
  type ReaderSettings
} from '../types/reader.js';
import { postWorkerEvent, WorkerEventType, type WorkerEvent } from '../types/events.js';
import { CommandManager, SequenceAbortedError } from './command.js';
import type { StateContext } from './state-context.js';
import { PacketHandler } from './packet.js';
import { NotificationManager } from './notification/manager.js';
import { NotificationRouter } from './notification/router.js';
import { logger, LogLevel } from '../utils/logger.js';
import type { CommandSequence } from './type.js';
import { IDLE_SEQUENCE, BATTERY_VOLTAGE_SEQUENCE } from './system/sequences.js';
import { INVENTORY_CONFIG_SEQUENCE } from './rfid/inventory/sequences.js';
import { BARCODE_CONFIG_SEQUENCE, BARCODE_START_SEQUENCE, BARCODE_STOP_SEQUENCE } from './barcode/sequences.js';
import { applyRfidSettings, type RfidSettings } from './rfid/firmware-command.js';
import { LOCATE_CONFIG_SEQUENCE, locateSettingsSequence } from './rfid/locate/sequences.js';
import { transmitPowerSequence, RFID_START_SEQUENCE, RFID_STOP_SEQUENCE } from './rfid/sequences.js';
import { RFID_FIRMWARE_COMMAND } from './event.js';

/**
 * CS108 RFID Reader
 * 
 * This class extends BaseReader with CS108-specific protocol details,
 * command sequences, and packet handling.
 */
class CS108Reader extends BaseReader {
  protected readerSettings: ReaderSettings = this.getDefaultSettings();
  private commandManager: CommandManager;
  private notificationManager: NotificationManager;
  private notificationRouter: NotificationRouter;
  private packetHandler: PacketHandler;
  private targetMode: ReaderModeType | null = null; // Track target mode for early exit
  private isStoppingScanning = false;
  private triggerState = false; // Track physical trigger state for reconciliation
  private lastTriggerTime = 0;
  private triggerDebounceMs = 100; // Minimum time between trigger events
  private batteryCheckTimer?: NodeJS.Timeout;
  private lastBatteryPercentage = -1;
  private scanningRequested = false; // Track if scanning was explicitly requested (button/trigger)

  constructor() {
    super();

    // Initialize packet handler for protocol parsing
    this.packetHandler = new PacketHandler();

    // Initialize notification manager with event emitter callback and context
    this.notificationManager = new NotificationManager(
      this.handleNotificationEvent.bind(this),
      {
        debug: false, // Could be made configurable
        getCurrentMode: () => this.readerMode || ReaderMode.IDLE,
        getReaderState: () => this.readerState
      }
    );

    // Get router for direct access (avoids pass-through overhead)
    this.notificationRouter = this.notificationManager.getRouter();

    // Create state context for CommandManager
    const stateContext: StateContext = {
      getReaderState: () => this.readerState,
      setReaderState: (state) => this.setReaderState(state)
    };

    // Initialize command manager with transport callback, notification handler, and state context
    this.commandManager = new CommandManager(
      this.sendCommand.bind(this),
      this.notificationRouter.handleNotification.bind(this.notificationRouter),
      stateContext
    );

    // Apply initial log level from settings if provided
    if (this.readerSettings?.system?.workerLogLevel) {
      const logLevelMap: Record<string, number> = {
        'error': 1, // LogLevel.ERROR
        'warn': 2, // LogLevel.WARN
        'info': 3, // LogLevel.INFO
        'debug': 4 // LogLevel.DEBUG
      };
      const enumLevel = logLevelMap[this.readerSettings.system.workerLogLevel] || 3;
      logger.setLevel(enumLevel);
    }

    // Emit initial state events when reader is created
    this.emitInitialState();
  }

  /**
   * Handle domain events from notification handlers
   * Intercepts auto-stop, vibrator requests, and trigger events before emitting
   */
  private async handleNotificationEvent(event: Omit<WorkerEvent, 'timestamp'>): Promise<void> {
    // Handle internal control events
    switch (event.type) {
      case 'TRIGGER_STATE_CHANGED': {
        // Handle trigger press/release based on current mode
        logger.debug(`Trigger state changed event received: ${JSON.stringify(event.payload)}`);

        // Always update trigger state immediately for reconciliation
        const payload = event.payload as { pressed?: boolean };
        this.triggerState = payload?.pressed ?? false;
        logger.debug(`Trigger state updated: ${this.triggerState}`);

        // Check if we're in a scanning mode
        if (this.readerMode === ReaderMode.BARCODE ||
            this.readerMode === ReaderMode.INVENTORY ||
            this.readerMode === ReaderMode.LOCATE) {

          // Apply debouncing to prevent command hammering
          const now = Date.now();
          if (now - this.lastTriggerTime < this.triggerDebounceMs) {
            logger.debug('Trigger event debounced, but state tracked for reconciliation');
            // Still emit for UI feedback even if debounced
            break;
          }
          this.lastTriggerTime = now;

          if (this.triggerState) {
            // Trigger pressed - start scanning if ready
            if (this.readerState === ReaderState.CONNECTED) {
              logger.debug(`Trigger pressed - starting ${this.readerMode} scan`);
              await this.startScanning();
            } else {
              logger.debug(`Trigger pressed ignored - reader state is ${this.readerState}`);
            }
          } else {
            // Trigger released - stop scanning immediately
            if (this.readerState === ReaderState.SCANNING) {
              logger.debug(`Trigger released - stopping ${this.readerMode} scan`);
              await this.stopScanning();
            } else {
              logger.debug(`Trigger released ignored - reader state is ${this.readerState}`);
            }
          }
        } else {
          logger.debug(`Trigger event ignored - mode is ${this.readerMode}`);
        }
        // Post event to DeviceManager for UI sync
        postWorkerEvent(event);
        return; // Don't fall through to postWorkerEvent at end
      }

      case 'BARCODE_AUTO_STOP_REQUEST':
        // Auto-stop scanning after successful barcode read
        await this.stopScanning();
        // Don't emit this internal control event
        return;
    }

    // Pass through all other events
    postWorkerEvent(event);
  }
  
  /**
   * Emit initial state events on instantiation
   */
  private emitInitialState(): void {
    // Emit initial reader state (DISCONNECTED)
    postWorkerEvent({
      type: WorkerEventType.READER_STATE_CHANGED,
      payload: { readerState: this.readerState }
    });
    
    // Emit initial reader mode (null - not initialized)
    postWorkerEvent({
      type: WorkerEventType.READER_MODE_CHANGED,
      payload: { mode: this.readerMode }
    });
  }
  
  /**
   * Get default CS108 settings
   */
  private getDefaultSettings(): ReaderSettings {
    return {
      rfid: {
        transmitPower: 30,  // Maximum power default
        session: 1,         // S1 default
        target: RainTarget.A,  // Target A default
        qValue: 4           // Mid-range Q value
      },
      barcode: {
        continuous: false,
        timeout: 5000,
        illumination: true,
        aimPattern: true
      }
    };
  }
  
  // BaseReader Abstract Method Implementations
  
  /**
   * CS108-specific connection logic
   */
  protected async onConnect(): Promise<void> {
    logger.debug('[Reader] Connecting...');
    
    // Initialize default settings
    this.readerSettings = this.getDefaultSettings();
    
    // BaseReader.connect() will call setMode(IDLE) after this returns
    // So we don't need to do it here
    
    logger.debug('[Reader] Connected successfully');
  }
  
  /**
   * CS108-specific disconnection logic
   */
  protected async onDisconnect(): Promise<void> {
    logger.debug('[Reader] Disconnecting...');

    // Clear battery check timer
    if (this.batteryCheckTimer) {
      clearTimeout(this.batteryCheckTimer);
      this.batteryCheckTimer = undefined;
      logger.debug('[Reader] Battery check timer cleared');
    }

    // Reset trigger state on disconnect
    this.triggerState = false;

    // Abort any running sequence on disconnect
    this.commandManager.abortSequence('Disconnect requested');

    logger.debug('[Reader] Disconnected successfully');
  }
  
  /**
   * Handle incoming CS108 packets
   * Reader owns packet parsing and routing
   */
  protected handleBleData(data: Uint8Array): void {
    // Log incoming BLE data
    logger.trace('Received BLE data', data);
    logger.hexDump('BLE Data', data);

    // Reader owns packet parsing
    const packets = this.packetHandler.processIncomingData(data);
    logger.debug(`Parsed ${packets.length} packets from BLE data`);

    // Debug: log if we got inventory packets
    const inventoryPackets = packets.filter(p => p.eventCode === 0x8100);
    if (inventoryPackets.length > 0) {
      logger.debug(`Got ${inventoryPackets.length} inventory packets from BLE data`);
    }

    // Route each packet based on event flags
    for (const packet of packets) {
      logger.debug(`Packet: ${packet.event.name} (0x${packet.eventCode.toString(16)}), isCommand: ${packet.event.isCommand}, isNotification: ${packet.event.isNotification}`);

      // Special handling for ERROR_NOTIFICATION with "Wrong header prefix" error
      // This error occurs when the CS108 hardware misinterprets fragmented inventory packets
      // as malformed commands. This is a known hardware issue that occurs during RFID operations.
      // Since inventory packets are streaming and non-critical, we can safely ignore these
      // specific errors. They don't indicate actual communication problems.
      if (packet.event.name === 'ERROR_NOTIFICATION' && packet.rawPayload.length >= 2) {
        const errorCode = (packet.rawPayload[0] << 8) | packet.rawPayload[1];
        if (errorCode === 0x0000) {
          // Always ignore "Wrong header prefix" errors - they're spurious from the hardware
          // The CS108 firmware incorrectly interprets its own fragmented packets as commands
          logger.debug('[Reader] Ignoring spurious "Wrong header prefix" error from CS108 hardware');
          continue; // Skip this packet entirely
        }
      }

      // First check if this is a command response we're waiting for
      if (packet.event.isCommand && this.commandManager.isWaitingForResponse(packet)) {
        // This is a response to a pending command - handle ONLY as command
        logger.debug(`Routing command response to manager: ${packet.event.name} (0x${packet.eventCode.toString(16)})`);
        this.commandManager.handleCommandResponse(packet);
      } else if (packet.event.isNotification) {
        // Not a pending command response - handle as notification
        logger.debug(`Routing as notification: ${packet.event.name}`);


        this.notificationRouter.handleNotification(packet);
      }
    }
  }
  
  // IReader Interface - CS108-Specific Implementation
  
  /**
   * Set reader mode with complete cleanup of previous mode
   *
   * This method performs COMPLETE cleanup when switching modes, ensuring:
   * - All running command sequences are aborted
   * - All notification handlers are cleared and their cleanup() called
   * - Parser buffers and state are reset
   * - Hardware modules (RFID/Barcode) are powered off
   * - Fresh state is initialized for the new mode
   *
   * This mirrors real-world usage where users switch between tabs in the app.
   * Each tab navigation should give users a completely clean slate, with no
   * remnants from the previous mode affecting the new mode.
   *
   * Cleanup sequence:
   * 1. Abort any running sequences (stops in-flight commands)
   * 2. Clear notification router (calls cleanup() on all handlers)
   * 3. Execute IDLE sequence (powers off hardware modules)
   * 4. Configure hardware for new mode
   * 5. Re-initialize handlers with fresh state
   *
   * @param mode - The reader mode to switch to (IDLE, INVENTORY, BARCODE, LOCATE)
   * @param options - Optional parameters (e.g., targetEPC for LOCATE mode)
   */
  async setMode(mode: ReaderModeType, settings?: ReaderSettings): Promise<void> {
    logger.debug(`[setMode] Request to set mode to ${mode}`, settings);

    // If settings provided, update them BEFORE mode change
    // This ensures the mode configuration uses the latest values
    if (settings) {
      logger.debug(`[setMode] Updating settings before mode change`);
      this.readerSettings = { ...this.readerSettings, ...settings };
    }

    // Early exit if already targeting this mode
    if (mode === this.targetMode) {
      logger.debug(`[setMode] Already targeting ${mode} mode, early exit`);
      return;
    }

    // Set target immediately - this blocks concurrent calls!
    this.targetMode = mode;
    logger.debug(`[setMode] Target mode set to ${mode}, starting mode change`);

    try {
      // Stop scanning if we're currently scanning (no active sequence in SCANNING state)
      if (this.readerState === ReaderState.SCANNING) {
        logger.debug('[setMode] Currently scanning, stopping first...');
        await this.stopScanning();
      }

      // Abort any active command sequence (e.g., if we're BUSY with another operation)
      // This respects current command completion but prevents further commands
      await this.commandManager.abortSequence('Mode change requested');

      // Build and execute mode sequences
      const sequences = this.buildModeSequences(mode);
      logger.debug(`[setMode] Executing ${sequences.length} commands for ${mode} mode`);
      await this.commandManager.executeSequence(sequences);

      // SUCCESS - update actual mode
      this.readerMode = mode;
      logger.debug(`[setMode] Successfully changed to ${mode} mode`);

      // Re-initialize notification handlers for new mode
      // IDLE mode needs notifications for battery updates
      if (mode !== ReaderMode.ERROR) {
        logger.debug(`[Reader] Re-initializing notification manager for ${mode} mode...`);
        this.notificationManager = new NotificationManager(
          this.handleNotificationEvent.bind(this),
          {
            getCurrentMode: () => this.readerMode || ReaderMode.IDLE,
            getReaderState: () => this.readerState
          }
        );
        this.notificationRouter = this.notificationManager.getRouter();
      }

      // Emit mode change event
      postWorkerEvent({
        type: WorkerEventType.READER_MODE_CHANGED,
        payload: { mode: this.readerMode }
      });

      // Schedule battery checks in IDLE mode
      if (mode === ReaderMode.IDLE) {
        this.scheduleBatteryCheck();
      }

    } catch (error) {
      logger.error(`[setMode] Failed to set ${mode} mode:`, error);

      // Handle sequence abortion gracefully
      if (error instanceof SequenceAbortedError) {
        logger.debug(`[setMode] Mode change aborted (another mode change in progress)`);
        // Don't set ERROR - another setMode is taking over
        return;
      }

      // Set ERROR mode - hardware is in unknown state
      this.readerMode = ReaderMode.ERROR;
      this.targetMode = ReaderMode.ERROR; // Sync target with actual

      // Clear handlers - we don't know what state we're in
      this.notificationRouter.clear();

      // Emit ERROR mode
      postWorkerEvent({
        type: WorkerEventType.READER_MODE_CHANGED,
        payload: { mode: ReaderMode.ERROR }
      });

      throw error;
    } finally {
      // Always sync targetMode with final mode
      this.targetMode = this.readerMode || ReaderMode.IDLE;
    }
  }

  /**
   * Internal method that performs the actual mode change
   * This is wrapped by the mutex logic in setMode()
   */
  private buildModeSequences(mode: ReaderModeType): CommandSequence {
    switch (mode) {
      case ReaderMode.IDLE:
        logger.debug('[Reader] Building IDLE sequence');
        return [...IDLE_SEQUENCE];

      case ReaderMode.INVENTORY:
        logger.debug('[Reader] Building INVENTORY sequence');
        return [
          ...IDLE_SEQUENCE,
          ...INVENTORY_CONFIG_SEQUENCE,
          ...transmitPowerSequence(this.readerSettings.rfid?.transmitPower)
        ];

      case ReaderMode.LOCATE: {
        logger.debug('[Reader] Building LOCATE sequence');

        // Use the targetEPC from settings
        const targetEPC = this.readerSettings.rfid?.targetEPC || '';
        logger.info(`[Reader] Building LOCATE with targetEPC: ${targetEPC || 'none'}`);

        return [
          ...IDLE_SEQUENCE,
          ...LOCATE_CONFIG_SEQUENCE,
          ...transmitPowerSequence(this.readerSettings.rfid?.transmitPower),
          ...locateSettingsSequence(targetEPC)
        ];
      }

      case ReaderMode.BARCODE:
        logger.debug('[Reader] Building BARCODE sequence');
        return [
          ...IDLE_SEQUENCE,
          ...BARCODE_CONFIG_SEQUENCE
        ];

      case ReaderMode.ERROR:
        // ERROR mode doesn't need configuration
        logger.debug('[Reader] ERROR mode - no sequences needed');
        return [];

      default:
        throw new Error(`Unsupported reader mode: ${mode}`);
    }
  }
  
  /**
   * Get current reader settings
   * Returns a deep copy to prevent external mutations
   */
  getSettings(): ReaderSettings {
    // Return a deep copy using native structuredClone (Node 18+, modern browsers)
    return structuredClone(this.readerSettings);
  }

  /**
   * Update reader settings - simplified version
   * Just stores settings and applies hardware changes when READY
   * No mode-based filtering, no complex validation
   *
   * @param settings - Reader settings to update
   */
  async setSettings(settings: ReaderSettings): Promise<void> {
    logger.debug('[Reader] setSettings called with:', JSON.stringify(settings));

    // Handle system settings (like log level) immediately - no state required
    if (settings.system?.workerLogLevel) {
      const logLevelMap: Record<string, LogLevel> = {
        'error': LogLevel.ERROR,
        'warn': LogLevel.WARN,
        'info': LogLevel.INFO,
        'debug': LogLevel.DEBUG
      };
      const newLevel = logLevelMap[settings.system.workerLogLevel] || LogLevel.INFO;
      logger.setLevel(newLevel);
      logger.debug(`[Reader] Log level changed to: ${settings.system.workerLogLevel}`);
    }

    // Always store all settings - they'll be used by setMode
    this.readerSettings = { ...this.readerSettings, ...settings };
    logger.debug('[Reader] Stored settings for future use');

    // Check if we need to apply hardware settings
    const hasHardwareSettings =
      settings.rfid?.transmitPower !== undefined ||
      settings.rfid?.session !== undefined ||
      settings.rfid?.algorithm !== undefined ||
      settings.rfid?.inventoryMode !== undefined ||
      settings.barcode !== undefined;

    // If we have hardware settings and we're READY, apply them
    if (hasHardwareSettings && this.readerState === ReaderState.CONNECTED) {
      try {
        // Apply transmit power if changed
        if (settings.rfid?.transmitPower !== undefined) {
          await this.commandManager.executeSequence(transmitPowerSequence(settings.rfid.transmitPower));
          logger.debug('[Reader] Applied transmit power');
        }

        // Apply session/algorithm settings if in INVENTORY mode
        if (this.readerMode === ReaderMode.INVENTORY) {
          const rfidSettings = settings.rfid;
          if (rfidSettings?.session !== undefined ||
              rfidSettings?.algorithm !== undefined ||
              rfidSettings?.inventoryMode !== undefined) {

            // Convert session to string format if provided
            let sessionString: 'S0' | 'S1' | 'S2' | 'S3' | undefined;
            if (rfidSettings.session !== undefined) {
              const sessionNum = Number(rfidSettings.session);
              sessionString = `S${sessionNum}` as 'S0' | 'S1' | 'S2' | 'S3';
            }

            // Build settings object for firmware commands
            const firmwareSettings: RfidSettings = {
              ...(sessionString !== undefined && { session: sessionString }),
              ...(rfidSettings.algorithm !== undefined && { algorithm: rfidSettings.algorithm }),
              ...(rfidSettings.inventoryMode !== undefined && { inventoryMode: rfidSettings.inventoryMode })
            };

            if (Object.keys(firmwareSettings).length > 0) {
              const commands = applyRfidSettings(firmwareSettings);
              for (const payload of commands) {
                await this.commandManager.executeCommand(RFID_FIRMWARE_COMMAND, payload);
              }
              logger.debug(`[Reader] Applied ${commands.length} RFID settings to hardware`);
            }
          }
        }

        // Apply targetEPC immediately if we're in LOCATE mode
        if (this.readerMode === ReaderMode.LOCATE && settings.rfid?.targetEPC !== undefined) {
          const epcValue = settings.rfid.targetEPC || '';
          if (epcValue) {
            logger.debug('[Reader] Applying EPC tag mask in LOCATE mode');
            await this.commandManager.executeSequence(locateSettingsSequence(epcValue));
            logger.debug('[Reader] EPC tag mask applied successfully');
          } else {
            logger.warn('[Reader] LOCATE mode without targetEPC - will receive all tags');
          }
        }

        // TODO: Apply barcode settings when implemented
        if (settings.barcode) {
          logger.debug('[Reader] Barcode settings received (implementation pending)');
        }

      } catch (error) {
        // Handle sequence abortion gracefully
        if (error instanceof Error && error.message.includes('aborted')) {
          logger.debug('[Reader] Settings application aborted (mode change in progress)');
          // Settings are already stored, will be used next time
          return;
        }
        logger.error('[Reader] Failed to apply hardware settings:', error);
        throw error;
      }
    } else if (hasHardwareSettings && this.readerState !== ReaderState.CONNECTED) {
      // We have hardware settings but can't apply them now - just log
      logger.debug('[Reader] Hardware settings stored but not applied (reader not READY)');
    }

    // Emit settings updated event
    postWorkerEvent({
      type: WorkerEventType.SETTINGS_UPDATED,
      payload: { settings }
    });

    logger.debug('[Reader] Settings processing complete');
  }
  
  async startScanning(): Promise<void> {
    logger.debug(`[Reader] Starting scan in ${this.readerMode} mode, state=${this.readerState}`);

    // Mark that scanning was explicitly requested
    this.scanningRequested = true;

    // Validate we're in a ready state
    if (this.readerState !== ReaderState.CONNECTED) {
      throw new Error(`Cannot start scanning from state ${this.readerState}`);
    }

    // Note: LOCATE mode without EPC filter will return all tags like INVENTORY
    // The UI layer should prevent this scenario for better UX
    if (this.readerMode === ReaderMode.LOCATE && !this.readerSettings.rfid?.targetEPC) {
      logger.warn('[Reader] Starting LOCATE mode without targetEPC - will receive all tags');
    }

    try {
      // Send appropriate start command based on mode
      switch (this.readerMode) {
        case ReaderMode.INVENTORY:
        case ReaderMode.LOCATE: {
          // Start RFID inventory using START_INVENTORY command
          logger.debug(`[Reader] Starting RFID inventory for mode: ${this.readerMode}`);
          logger.debug(`[Reader] Executing RFID_START_SEQUENCE...`);

          await this.commandManager.executeSequence(RFID_START_SEQUENCE);

          logger.debug('[Reader] RFID inventory started successfully');
          break;
        }

        case ReaderMode.BARCODE: {
          // Send barcode trigger command
          await this.commandManager.executeSequence(BARCODE_START_SEQUENCE);
          logger.debug('[Reader] Sent barcode trigger command');
          break;
        }

        default:
          throw new Error(`Cannot start scanning in ${this.readerMode} mode`);
      }

      // CommandManager already set state to SCANNING
      logger.debug('[Reader] Scan started successfully');

      // Reconciliation: Check if trigger was released while we were starting
      // Only stop if BOTH trigger is released AND button is not active
      if (!this.triggerState && !this.scanningRequested) {
        logger.debug('[Reader] Trigger released during start, reconciling by stopping');
        await this.stopScanning();
      }
    } catch (error) {
      logger.error('[Reader] Failed to start scanning:', error);
      // CommandManager already set state to ERROR
      throw error;
    }
  }
  
  async stopScanning(): Promise<void> {
    logger.debug(`[Reader] Stopping scan in ${this.readerMode} mode`);

    // Clear the scanning requested flag immediately
    this.scanningRequested = false;

    // Validate we're in a scanning state
    if (this.readerState !== ReaderState.SCANNING) {
      logger.warn(`[Reader] Not scanning, current state: ${this.readerState}`);
      return;
    }

    // Prevent concurrent stop operations
    if (this.isStoppingScanning) {
      logger.debug('[Reader] Stop already in progress, skipping');
      return;
    }

    this.isStoppingScanning = true;
    try {
      // Send appropriate stop command based on mode
      switch (this.readerMode) {
        case ReaderMode.INVENTORY:
        case ReaderMode.LOCATE: {
          // Stop RFID inventory using ABORT command
          logger.debug('[Reader] Stopping RFID inventory with ABORT command');

          await this.commandManager.executeSequence(RFID_STOP_SEQUENCE);

          // Monitor for packet streaming to verify stop worked
          logger.debug('[Reader] Monitoring packet stream to verify stop...');
          const packetMonitorStart = Date.now();
          const maxWaitTime = 2000; // 2 seconds per API documentation

          // Wait and check if packets are still streaming
          await new Promise(resolve => setTimeout(resolve, 1000)); // Initial wait

          const elapsedTime = Date.now() - packetMonitorStart;

          // Check if we need to force RFID power off
          // Note: In a real implementation, we'd monitor actual packet flow
          // For now, we implement the safety mechanism structure
          if (elapsedTime >= maxWaitTime) {
            logger.warn('[Reader] Packets may still be streaming after ABORT - forcing RFID power off');

            // Import and execute RFID power off command
            const { RFID_POWER_OFF } = await import('./event');
            await this.commandManager.executeCommand(RFID_POWER_OFF);

            logger.debug('[Reader] RFID power forced off to stop packet streaming');
          } else {
            logger.debug('[Reader] RFID inventory stopped successfully with ABORT');
          }
          break;
        }

        case ReaderMode.BARCODE: {
          // Send barcode stop command
          await this.commandManager.executeSequence(BARCODE_STOP_SEQUENCE);
          logger.debug('[Reader] Sent barcode stop command');
          break;
        }

        default:
          logger.warn(`[Reader] Cannot stop scanning in ${this.readerMode} mode`);
          return;
      }

      // CommandManager already set state to READY
      logger.debug('[Reader] Scan stopped successfully');

      // Reconciliation: Check if trigger was pressed while we were stopping
      // Only reconcile if we're in a scanning mode (not IDLE)
      if (this.triggerState &&
          (this.readerMode === ReaderMode.INVENTORY ||
           this.readerMode === ReaderMode.LOCATE ||
           this.readerMode === ReaderMode.BARCODE)) {
        logger.debug('[Reader] Trigger pressed during stop, reconciling by starting');
        // Reset the flag before recursing
        this.isStoppingScanning = false;
        await this.startScanning();
        return; // Exit early to avoid clearing flag twice
      }
    } catch (error) {
      logger.error('[Reader] Failed to stop scanning:', error);
      // CommandManager already set state to ERROR
      throw error;
    } finally {
      this.isStoppingScanning = false;
    }
  }


  /**
   * Get current battery percentage
   * Return percentage from 0-100 or -1 if unknown
   */
  async getBatteryPercentage(): Promise<number> {
    try {
      await this.commandManager.executeSequence(BATTERY_VOLTAGE_SEQUENCE);

      // TODO: Parse actual battery value from response
      // For now, return -1 to indicate unknown
      return -1;

    } catch (error) {
      logger.error('[Reader] Failed to get battery percentage:', error);
      return -1;
    }
  }

  /**
   * Schedule battery check at configured interval
   * Frequency doubles when battery < 20%
   */
  private scheduleBatteryCheck(): void {
    // Clear any existing timer
    if (this.batteryCheckTimer) {
      clearTimeout(this.batteryCheckTimer);
    }

    // Skip if scanning or busy
    if (this.readerState === ReaderState.SCANNING || this.readerState === ReaderState.BUSY) {
      logger.debug('[Reader] Skipping battery check - reader is busy');
      return;
    }

    const interval = this.readerSettings.system?.batteryCheckInterval || 60;
    if (interval <= 0) return; // 0 disables battery checks

    // Double frequency when battery is low
    const effectiveInterval = (this.lastBatteryPercentage > 0 && this.lastBatteryPercentage < 20)
      ? interval * 500  // Half the interval (in ms)
      : interval * 1000; // Normal interval (in ms)

    this.batteryCheckTimer = setTimeout(async () => {
      try {
        const percentage = await this.getBatteryPercentage();

        // Only emit if percentage changed
        if (percentage !== this.lastBatteryPercentage) {
          this.lastBatteryPercentage = percentage;
          postWorkerEvent({
            type: WorkerEventType.BATTERY_UPDATE,
            payload: { percentage }
          });
        }

        // Schedule next check
        this.scheduleBatteryCheck();
      } catch (error) {
        logger.error('[Reader] Battery check failed:', error);
        // Continue checking despite errors
        this.scheduleBatteryCheck();
      }
    }, effectiveInterval);

    logger.debug(`[Reader] Battery check scheduled in ${effectiveInterval}ms`);
  }

  /**
   * Get reader firmware version
   * TODO: Implement proper version command when available
   */
  async getFirmwareVersion(): Promise<string> {
    // TODO: Implement version query command when CS108 protocol spec is available
    return 'Unknown';
  }
}

export { CS108Reader };