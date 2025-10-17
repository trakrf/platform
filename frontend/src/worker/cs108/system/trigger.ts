/**
 * Trigger notification handlers
 *
 * Handles trigger button press/release notifications (0xA102/0xA103)
 * from the CS108 device. Emits TRIGGER_STATE_CHANGED events.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../notification/types';
import { logger } from '../../utils/logger.js';
import type { CS108Packet } from '../type';
import { isScalarPayload } from '../payload-types';
import { WorkerEventType } from '../../types/events';

/**
 * Base class for trigger handlers
 * Contains shared logic for trigger state management
 */
abstract class TriggerHandler implements NotificationHandler {
  protected abstract readonly pressed: boolean;
  protected abstract readonly eventName: string;

  /**
   * Trigger notifications are always valid
   */
  canHandle(_packet: CS108Packet, _context: NotificationContext): boolean {
    return true;
  }

  /**
   * Handle trigger notification
   * Emits TRIGGER_STATE_CHANGED with appropriate pressed state
   */
  handle(_packet: CS108Packet, context: NotificationContext): void {
    // Always log for debugging
    logger.debug(`[TriggerHandler] ${this.eventName} - emitting TRIGGER_STATE_CHANGED`);

    // Use callback so it can be intercepted by CS108Reader
    context.emitNotificationEvent({
      type: WorkerEventType.TRIGGER_STATE_CHANGED,
      payload: {
        pressed: this.pressed,
      },
    });

    // Additional debug log
    if (context.metadata?.debug) {
      logger.debug(`[TriggerHandler] ${this.eventName} (debug mode)`);
    }
  }
}

/**
 * Handler for trigger pressed notification (0xA102)
 */
export class TriggerPressedHandler extends TriggerHandler {
  protected readonly pressed = true;
  protected readonly eventName = 'Trigger PRESSED';
}

/**
 * Handler for trigger released notification (0xA103)
 */
export class TriggerReleasedHandler extends TriggerHandler {
  protected readonly pressed = false;
  protected readonly eventName = 'Trigger RELEASED';
}

/**
 * Handler for trigger state query response (0xA001)
 * This is a command response, not a notification, but follows similar pattern
 */
export class TriggerStateHandler implements NotificationHandler {
  /**
   * Check if packet has trigger state data
   */
  canHandle(packet: CS108Packet, _context: NotificationContext): boolean {
    return isScalarPayload(packet.payload);
  }

  /**
   * Handle trigger state response
   * Parses state value and emits TRIGGER_STATE_CHANGED
   */
  handle(packet: CS108Packet, context: NotificationContext): void {
    if (!isScalarPayload(packet.payload)) {
      logger.warn('[TriggerStateHandler] Invalid payload format');
      return;
    }

    // Extract state value (0 = released, 1 = pressed)
    const state = packet.payload;

    const pressed = state === 1;

    // Use callback so it can be intercepted by CS108Reader
    context.emitNotificationEvent({
      type: WorkerEventType.TRIGGER_STATE_CHANGED,
      payload: {
        pressed,
      },
    });

    // Also emit as command response for pending queries
    context.emitNotificationEvent({
      type: WorkerEventType.COMMAND_RESPONSE,
      payload: {
        command: 'GET_TRIGGER_STATE',
        response: { pressed },
      },
    });

    if (context.metadata?.debug) {
      logger.debug(`[TriggerStateHandler] Trigger state: ${pressed ? 'PRESSED' : 'RELEASED'}`);
    }
  }
}