/**
 * NotificationManager - Simplified notification handling
 *
 * This manager replaces the massive 1400+ line switch statement
 * with a clean router-based architecture. All notification logic
 * is delegated to specific handlers.
 */

import type { CS108Packet } from '../type';
import { logger } from '../../utils/logger.js';
import type { ReaderModeType, ReaderStateType } from '../../types/reader';
import type { WorkerEvent } from '../../types/events';
import { NotificationRouter, type RouterConfig } from './router';

// Import handlers
import { BatteryHandler } from '../system/battery';
import {
  TriggerPressedHandler,
  TriggerReleasedHandler,
  TriggerStateHandler,
} from '../system/trigger';
import { ErrorNotificationHandler } from '../system/error';
import { createInventoryHandler } from '../rfid/inventory/handler';
import { LocateTagHandler } from '../rfid/locate/handler';
import {
  BarcodeDataHandler,
  BarcodeGoodReadHandler,
} from '../barcode/scan-handler';

/**
 * CS108 event codes
 */
const EventCodes = {
  // System notifications
  BATTERY_VOLTAGE: 0xA000,
  TRIGGER_STATE: 0xA001,
  TRIGGER_PRESSED: 0xA102,
  TRIGGER_RELEASED: 0xA103,
  ERROR_NOTIFICATION: 0xA101,

  // RFID notifications
  INVENTORY_TAG: 0x8100,

  // Barcode notifications
  BARCODE_DATA: 0x9100,
  BARCODE_GOOD_READ: 0x9101,
} as const;

/**
 * Configuration for NotificationManager
 */
export interface NotificationManagerConfig {
  /** Enable debug logging */
  debug?: boolean;

  /** Get current reader mode */
  getCurrentMode: () => ReaderModeType;

  /** Get current reader state */
  getReaderState: () => ReaderStateType;

  /** Optional error handler */
  onError?: (error: Error, packet: CS108Packet) => void;
}

/**
 * Simplified notification manager using router pattern
 */
export class NotificationManager {
  private router: NotificationRouter;
  private config: NotificationManagerConfig;

  constructor(
    emitNotificationEvent: (event: Omit<WorkerEvent, 'timestamp'>) => void,
    config: NotificationManagerConfig
  ) {
    this.config = config;

    // Create router with configuration
    const routerConfig: RouterConfig = {
      debug: config.debug,
      onError: config.onError,
      getCurrentMode: config.getCurrentMode,
      getReaderState: config.getReaderState,
    };

    this.router = new NotificationRouter(emitNotificationEvent, routerConfig);

    // Register all handlers
    this.registerHandlers();

    logger.debug('[NotificationManager] Initialized with router-based architecture');
  }

  /**
   * Register all notification handlers with the router
   */
  private registerHandlers(): void {
    // System handlers
    this.router.register(EventCodes.BATTERY_VOLTAGE, new BatteryHandler());
    this.router.register(EventCodes.TRIGGER_STATE, new TriggerStateHandler());
    this.router.register(EventCodes.TRIGGER_PRESSED, new TriggerPressedHandler());
    this.router.register(EventCodes.TRIGGER_RELEASED, new TriggerReleasedHandler());
    this.router.register(EventCodes.ERROR_NOTIFICATION, new ErrorNotificationHandler());

    // RFID handlers
    // Create inventory handler with default configuration
    const inventoryHandler = createInventoryHandler();
    this.router.register(EventCodes.INVENTORY_TAG, inventoryHandler);

    // Locate handler uses same event code but different mode
    // Router will call both, but canHandle will filter
    this.router.register(EventCodes.INVENTORY_TAG, new LocateTagHandler());

    // Barcode handlers
    this.router.register(EventCodes.BARCODE_DATA, new BarcodeDataHandler());
    this.router.register(EventCodes.BARCODE_GOOD_READ, new BarcodeGoodReadHandler());

    if (this.config.debug) {
      const registered = this.router.getRegisteredEvents();
      logger.debug(
        `[NotificationManager] Registered ${registered.length} handlers: ` +
        registered.map((code) => `0x${code.toString(16)}`).join(', ')
      );
    }
  }

  /**
   * Get the router for direct packet handling
   * This avoids unnecessary pass-through methods
   */
  getRouter(): NotificationRouter {
    return this.router;
  }


  /**
   * Get statistics from all handlers
   */
  getStats(): Record<string, unknown> {
    const stats: Record<string, unknown> = {
      registeredHandlers: this.router.getRegisteredEvents().length,
    };

    // Get stats from specific handlers if needed
    // This could be expanded to query all handlers

    return stats;
  }

  /**
   * Cleanup when manager is destroyed
   */
  cleanup(): void {
    logger.debug('[NotificationManager] Cleaning up');
    this.router.clear();
  }

  /**
   * Update debug mode
   */
  setDebugMode(debug: boolean): void {
    this.config.debug = debug;
    // Could propagate to router if needed
  }

  /**
   * Check if handler is registered for an event
   */
  hasHandlerFor(eventCode: number): boolean {
    return this.router.hasHandler(eventCode);
  }
}