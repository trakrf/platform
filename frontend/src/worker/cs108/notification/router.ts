/**
 * NotificationRouter - Central routing for CS108 notifications
 *
 * This router replaces the massive switch statements with a clean,
 * extensible handler-based architecture. Each notification type
 * has its own handler that can be tested in isolation.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from './types';
import { logger } from '../../utils/logger.js';
import type { CS108Packet } from '../type';
import type { ReaderModeType, ReaderStateType } from '../../types/reader';
import type { WorkerEvent } from '../../types/events';

/**
 * Router configuration options
 */
export interface RouterConfig {
  /** Enable debug logging */
  debug?: boolean;

  /** Error handler for handler exceptions */
  onError?: (error: Error, packet: CS108Packet) => void;

  /** Get current reader mode */
  getCurrentMode: () => ReaderModeType;

  /** Get current reader state */
  getReaderState: () => ReaderStateType;
}

/**
 * Central router for CS108 notifications
 */
export class NotificationRouter {
  private handlers = new Map<number, NotificationHandler[]>();
  private config: RouterConfig;
  private emitNotificationEvent: (event: Omit<WorkerEvent, 'timestamp'>) => void;

  constructor(
    emitNotificationEvent: (event: Omit<WorkerEvent, 'timestamp'>) => void,
    config: RouterConfig
  ) {
    this.emitNotificationEvent = emitNotificationEvent;
    this.config = config;
  }

  /**
   * Register a handler for a specific event code
   * Multiple handlers can be registered for the same event code
   */
  register(eventCode: number, handler: NotificationHandler): void {
    const existing = this.handlers.get(eventCode) || [];
    existing.push(handler);
    this.handlers.set(eventCode, existing);

    if (this.config.debug) {
      logger.debug(
        `[NotificationRouter] Registered handler for event 0x${eventCode.toString(16)} ` +
        `(${existing.length} handler(s) total)`
      );
    }
  }

  /**
   * Unregister all handlers for an event code
   */
  unregister(eventCode: number): void {
    const handlers = this.handlers.get(eventCode);
    if (handlers) {
      // Call cleanup on all handlers
      for (const handler of handlers) {
        if (handler.cleanup) {
          try {
            handler.cleanup();
          } catch (error) {
            logger.error(
              `[NotificationRouter] Error during handler cleanup for 0x${eventCode.toString(16)}:`,
              error
            );
          }
        }
      }

      this.handlers.delete(eventCode);

      if (this.config.debug) {
        logger.debug(
          `[NotificationRouter] Unregistered ${handlers.length} handler(s) for event 0x${eventCode.toString(16)}`
        );
      }
    }
  }

  /**
   * Route a notification packet to the appropriate handler
   */
  handleNotification(packet: CS108Packet): void {
    const eventCode = packet.event.eventCode;
    const handlers = this.handlers.get(eventCode);

    if (!handlers || handlers.length === 0) {
      if (this.config.debug) {
        logger.debug(
          `[NotificationRouter] No handler for event 0x${eventCode.toString(16)} (${packet.event.name})`
        );
      }
      return;
    }

    // Build context for handlers
    const context: NotificationContext = {
      currentMode: this.config.getCurrentMode(),
      readerState: this.config.getReaderState(),
      emitNotificationEvent: this.emitNotificationEvent,
      metadata: {},
    };

    // Try each handler until one accepts the packet
    let handled = false;
    for (const handler of handlers) {
      // Check if handler can handle this packet
      try {
        if (!handler.canHandle(packet, context)) {
          continue;
        }
      } catch (error) {
        logger.error(
          `[NotificationRouter] Error in canHandle for event 0x${eventCode.toString(16)}:`,
          error
        );
        continue;
      }

      // Execute handler with error boundary
      try {
        handler.handle(packet, context);
        handled = true;

        if (this.config.debug) {
          logger.debug(
            `[NotificationRouter] Successfully handled event 0x${eventCode.toString(16)}`
          );
        }
      } catch (error) {
        const err = error instanceof Error ? error : new Error(String(error));
        logger.error(
          `[NotificationRouter] Error handling event 0x${eventCode.toString(16)}:`,
          err
        );

        // Call custom error handler if provided
        if (this.config.onError) {
          this.config.onError(err, packet);
        }
      }
    }

    if (!handled && this.config.debug) {
      logger.debug(
        `[NotificationRouter] No handler accepted packet for event 0x${eventCode.toString(16)}`
      );
    }
  }

  /**
   * Get list of registered event codes
   */
  getRegisteredEvents(): number[] {
    return Array.from(this.handlers.keys());
  }

  /**
   * Check if a handler is registered for an event
   */
  hasHandler(eventCode: number): boolean {
    return this.handlers.has(eventCode);
  }

  /**
   * Clear all handlers
   */
  clear(): void {
    // Cleanup all handlers
    this.handlers.forEach((handlers, eventCode) => {
      for (const handler of handlers) {
        if (handler.cleanup) {
          try {
            handler.cleanup();
          } catch (error) {
            logger.error(
              `[NotificationRouter] Error during cleanup for 0x${eventCode.toString(16)}:`,
              error
            );
          }
        }
      }
    });

    this.handlers.clear();

    if (this.config.debug) {
      logger.debug('[NotificationRouter] Cleared all handlers');
    }
  }

  /**
   * Get handler for a specific event code (mainly for testing)
   */
  getHandler(eventCode: number): NotificationHandler | undefined {
    const handlers = this.handlers.get(eventCode);
    return handlers ? handlers[0] : undefined;
  }

  /**
   * Batch register multiple handlers
   */
  registerAll(handlers: Map<number, NotificationHandler | NotificationHandler[]>): void {
    handlers.forEach((handler, eventCode) => {
      if (Array.isArray(handler)) {
        for (const h of handler) {
          this.register(eventCode, h);
        }
      } else {
        this.register(eventCode, handler);
      }
    });
  }
}