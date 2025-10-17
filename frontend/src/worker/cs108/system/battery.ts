/**
 * Battery notification handler
 *
 * Handles battery voltage notifications (0xA000) from the CS108 device.
 * Parser already converts voltage to percentage, so we just emit it.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../notification/types';
import { logger } from '../../utils/logger.js';
import type { CS108Packet } from '../type';
import { isScalarPayload } from '../payload-types';
import { postWorkerEvent, WorkerEventType } from '../../types/events';

/**
 * Handler for battery voltage notifications
 */
export class BatteryHandler implements NotificationHandler {
  /**
   * Check if we can handle this packet
   * Battery notifications should have a scalar payload (percentage)
   */
  canHandle(packet: CS108Packet, _context: NotificationContext): boolean {
    return isScalarPayload(packet.payload);
  }

  /**
   * Handle battery notification
   * Payload is already the percentage (0-100) from the parser
   */
  handle(packet: CS108Packet, context: NotificationContext): void {
    if (!isScalarPayload(packet.payload)) {
      return;
    }

    const percentage = packet.payload;

    // Always log battery updates during debugging
    logger.debug(`[BatteryHandler] Battery update: ${percentage}%`);

    // Emit domain event with battery percentage
    logger.debug(`[BatteryHandler] Posting BATTERY_UPDATE event with percentage: ${percentage}`);
    postWorkerEvent({
      type: WorkerEventType.BATTERY_UPDATE,
      payload: {
        percentage
      },
    });
    logger.debug(`[BatteryHandler] Posted BATTERY_UPDATE event`);

    // Log if debug mode
    if (context.metadata?.debug) {
      logger.debug(`[BatteryHandler] Battery update: ${percentage}% (debug mode)`);
    }
  }
}