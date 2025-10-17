/**
 * RFID locate mode notification handler
 *
 * Handles RFID tag notifications in locate/search mode.
 * Provides real-time RSSI feedback without batching for immediate UI updates.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../../notification/types';
import { logger } from '../../../utils/logger.js';
import type { TagData } from '../types';
import type { CS108Packet } from '../../type';
import { ReaderMode } from '../../../types/reader';
import { postWorkerEvent, WorkerEventType } from '../../../types/events';
import { InventoryParser } from '../parser.js';

/**
 * Ring buffer for RSSI smoothing
 */
class RssiRingBuffer {
  private buffer: number[];
  private index = 0;
  private count = 0;

  constructor(private size: number = 10) {
    this.buffer = new Array(size).fill(-120); // Initialize with minimum RSSI
  }

  /**
   * Add RSSI value to buffer
   */
  add(rssi: number): void {
    this.buffer[this.index] = rssi;
    this.index = (this.index + 1) % this.size;
    this.count = Math.min(this.count + 1, this.size);
  }

  /**
   * Get average RSSI
   */
  getAverage(): number {
    if (this.count === 0) return -120;

    const sum = this.buffer
      .slice(0, this.count)
      .reduce((acc, val) => acc + val, 0);

    return Math.round(sum / this.count);
  }

  /**
   * Get smoothed RSSI (weighted average favoring recent values)
   */
  getSmoothed(): number {
    if (this.count === 0) return -120;

    let weightedSum = 0;
    let totalWeight = 0;

    for (let i = 0; i < this.count; i++) {
      const bufferIndex = (this.index - 1 - i + this.size) % this.size;
      const weight = this.count - i; // More recent = higher weight
      weightedSum += this.buffer[bufferIndex] * weight;
      totalWeight += weight;
    }

    return Math.round(weightedSum / totalWeight);
  }

  /**
   * Get latest RSSI value
   */
  getLatest(): number {
    if (this.count === 0) return -120;
    const lastIndex = (this.index - 1 + this.size) % this.size;
    return this.buffer[lastIndex];
  }

  /**
   * Clear buffer
   */
  clear(): void {
    this.buffer.fill(-120);
    this.index = 0;
    this.count = 0;
  }
}

/**
 * Handler for RFID locate mode notifications
 */
export class LocateTagHandler implements NotificationHandler {
  private rssiBuffer = new RssiRingBuffer(10);
  private parser = new InventoryParser();
  private lastUpdateTime = 0;
  private updateCount = 0;
  private readonly MIN_UPDATE_INTERVAL_MS = 50; // Throttle updates

  /**
   * Check if we should handle this packet
   * Only handle in LOCATE mode for inventory packets
   */
  canHandle(packet: CS108Packet, context: NotificationContext): boolean {
    // Must be in locate mode
    if (context.currentMode !== ReaderMode.LOCATE) {
      logger.debug(`[LocateHandler] Not in LOCATE mode (current: ${context.currentMode})`);
      return false;
    }

    // Must be an INVENTORY_TAG packet
    if (packet.event?.name !== 'INVENTORY_TAG') {
      logger.debug(`[LocateHandler] Not inventory tag packet (got: ${packet.event?.name || `0x${packet.eventCode.toString(16)}`})`);
      return false;
    }

    // Must have raw payload data to parse
    return !!packet.rawPayload && packet.rawPayload.length > 0;
  }

  /**
   * Handle locate tag notification
   * Provides real-time RSSI feedback for tag location
   */
  handle(packet: CS108Packet, context: NotificationContext): void {
    // Parse the inventory packet to get tag data
    let tags;
    try {
      tags = this.parser.processInventoryPayload(
        packet.rawPayload,
        packet.reserve // sequence number
      );
    } catch (error) {
      logger.error('[LocateTagHandler] Failed to parse inventory packet:', error);
      return;
    }

    // Process each tag (typically just one in locate mode due to filtering)
    for (const tag of tags) {
      const tagData: TagData = {
        epc: tag.epc,
        rssi: tag.rssi,
        wbRssi: tag.wbRssi,
        pc: tag.pc,
        timestamp: Date.now(),
        phase: tag.phase,
        antenna: tag.antennaPort,
      };

      // In locate mode, we process all tags
      // The application layer (locateStore) will filter for the target EPC
      // This keeps the handler decoupled from application state

      // Add to ring buffer for smoothing
      this.rssiBuffer.add(tagData.rssi);
      this.updateCount++;

      // Throttle updates to prevent UI overload
      const now = Date.now();
      if (now - this.lastUpdateTime < this.MIN_UPDATE_INTERVAL_MS) {
        continue; // Skip this tag update but continue processing others
      }
      this.lastUpdateTime = now;

      // Emit locate update with both raw and smoothed values
      postWorkerEvent({
        type: WorkerEventType.LOCATE_UPDATE,
        payload: {
          epc: tagData.epc,
          rssi: tagData.rssi,
          wbRssi: tagData.wbRssi,  // Wideband RSSI for better accuracy
          smoothedRssi: this.rssiBuffer.getSmoothed(),
          averageRssi: this.rssiBuffer.getAverage(),
          timestamp: tagData.timestamp,
          antennaPort: tagData.antenna
        },
      });
    } // End of for loop

    // Log periodically in debug mode
    if (context.metadata?.debug && this.updateCount % 20 === 0 && tags.length > 0) {
      const lastTag = tags[tags.length - 1];
      logger.debug(
        `[LocateHandler] Tag: ${lastTag.epc.slice(-8)}, ` +
        `RSSI: ${lastTag.rssi}, Smoothed: ${this.rssiBuffer.getSmoothed()}`
      );
    }
  }


  /**
   * Get handler statistics
   */
  getStats(): {
    updatesProcessed: number;
    currentRssi: number;
    smoothedRssi: number;
    averageRssi: number;
  } {
    return {
      updatesProcessed: this.updateCount,
      currentRssi: this.rssiBuffer.getLatest(),
      smoothedRssi: this.rssiBuffer.getSmoothed(),
      averageRssi: this.rssiBuffer.getAverage(),
    };
  }

  /**
   * Cleanup when handler is unregistered
   */
  cleanup(): void {
    this.rssiBuffer.clear();
    this.parser.reset();
    this.updateCount = 0;
    this.lastUpdateTime = 0;
  }
}