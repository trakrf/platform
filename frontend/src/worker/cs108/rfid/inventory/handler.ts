/**
 * Simplified Inventory Tag Handler for CS108 RFID Reader
 *
 * YAGNI: No batching - stream tags as we parse them
 * This is a simpler implementation that emits tags immediately
 * without the complexity of batching and deduplication.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../../notification/types';
import { logger } from '../../../utils/logger.js';
import type { CS108Packet } from '../../type';
import { WorkerEventType, postWorkerEvent } from '../../../types/events';
import { ReaderMode } from '../../../types/reader';
import { InventoryParser, type ParsedTag, type InventoryMode } from '../parser';
import type { BufferMetrics } from '../ring-buffer';

export interface HandlerConfig {
  mode?: InventoryMode;
  bufferMonitoring?: boolean;
  debug?: boolean;
}

export interface HandlerStats {
  packetsProcessed: number;
  tagsExtracted: number;
  parseErrors: number;
  lastProcessTime: number;
  bufferMetrics: BufferMetrics;
}

export class InventoryTagHandler implements NotificationHandler {
  private parser: InventoryParser;
  private config: HandlerConfig;
  private stats = {
    packetsProcessed: 0,
    tagsExtracted: 0,
    parseErrors: 0,
    lastProcessTime: 0
  };
  private monitoringInterval?: NodeJS.Timeout;

  constructor(config: HandlerConfig = {}) {
    this.config = {
      mode: 'compact',
      bufferMonitoring: false,
      debug: false,
      ...config
    };

    // Initialize parser with configured mode
    this.parser = new InventoryParser(this.config.mode, this.config.debug);

    // Start buffer monitoring if enabled
    if (this.config.bufferMonitoring) {
      this.startBufferMonitoring();
    }

    logger.debug(`[InventoryTagHandler] Initialized with mode: ${this.config.mode}, stream-as-we-parse`);
  }

  /**
   * Check if this handler can process the given packet
   * Only handles 0x8100 in INVENTORY or LOCATE modes
   */
  canHandle(packet: CS108Packet, context: NotificationContext): boolean {
    // Check for 0x8100 event code
    if (packet.eventCode !== 0x8100) {
      return false;
    }

    // Only handle in INVENTORY or LOCATE modes
    const validModes = [ReaderMode.INVENTORY, ReaderMode.LOCATE];
    return validModes.includes(context.currentMode as any);
  }

  /**
   * Handle inventory notification packet
   */
  async handle(packet: CS108Packet, context: NotificationContext): Promise<void> {
    const startTime = Date.now();
    this.stats.packetsProcessed++;

    try {
      // Parse tags from the raw payload (already stripped of headers by packet.ts)
      const tags = this.parser.processInventoryPayload(
        packet.rawPayload,
        packet.reserve // sequence number
      );

      this.stats.tagsExtracted += tags.length;
      this.stats.lastProcessTime = Date.now() - startTime;

      // Stream tags immediately - no batching
      if (tags.length > 0) {
        this.emitTags(tags, context);
      }

      // Check buffer health periodically
      if (this.stats.packetsProcessed % 100 === 0) {
        this.checkBufferHealth();
      }

    } catch (error) {
      this.stats.parseErrors++;
      logger.error('[InventoryTagHandler] Parse error:', error);

      // Emit error event for debugging
      if (this.config.debug) {
        this.emitError(error as Error, packet);
      }
    }
  }

  /**
   * Emit tags immediately (stream-as-we-parse)
   * Tags from a single parse cycle are emitted together to reduce overhead
   */
  private emitTags(tags: ParsedTag[], context: NotificationContext): void {
    // In LOCATE mode, emit special event
    if (context.currentMode === ReaderMode.LOCATE && tags.length > 0) {
      // Find strongest signal for locate mode
      // Remember: RSSI is negative, so -30 dBm is stronger than -80 dBm
      const strongestTag = tags.reduce((best, tag) =>
        tag.rssi > best.rssi ? tag : best
      );

      // Use postWorkerEvent instead of globalThis.postMessage
      postWorkerEvent({
        type: WorkerEventType.LOCATE_UPDATE,
        payload: {
          epc: strongestTag.epc,
          rssi: strongestTag.rssi,
          timestamp: Date.now()
        }
      });
    } else {
      // Normal inventory mode - emit array of tags from single parse
      // This avoids overhead of individual postMessage calls while
      // still streaming results as soon as they're parsed
      postWorkerEvent({
        type: WorkerEventType.TAG_READ,
        payload: {
          tags,  // Natural grouping from single packet parse
          timestamp: Date.now()
        }
      });
    }
  }

  /**
   * Check buffer health and emit warning if needed
   */
  private checkBufferHealth(): void {
    const metrics = this.parser.getBufferMetrics();

    // Exactly > 80%, not >= 80%
    if (metrics.utilizationPercent > 80) {
      logger.warn(`[InventoryTagHandler] Buffer utilization high: ${metrics.utilizationPercent}%`);

      // Use postWorkerEvent instead of globalThis.postMessage
      postWorkerEvent({
        type: WorkerEventType.BUFFER_WARNING,
        payload: {
          utilizationPercent: metrics.utilizationPercent,
          used: metrics.used,
          size: metrics.size
        }
      });
    }
  }

  /**
   * Start periodic buffer monitoring
   */
  private startBufferMonitoring(): void {
    if (this.monitoringInterval) return;

    this.monitoringInterval = setInterval(() => {
      const metrics = this.parser.getBufferMetrics();

      if (this.config.debug) {
        logger.debug('[InventoryTagHandler] Buffer metrics:', metrics);
      }

      // Emit metrics for monitoring using postWorkerEvent
      postWorkerEvent({
        type: WorkerEventType.BUFFER_METRICS,
        payload: metrics
      });
    }, 5000); // Every 5 seconds
  }

  /**
   * Stop buffer monitoring
   */
  private stopBufferMonitoring(): void {
    if (this.monitoringInterval) {
      clearInterval(this.monitoringInterval);
      this.monitoringInterval = undefined;
    }
  }

  /**
   * Emit error event
   */
  private emitError(error: Error, packet: CS108Packet): void {
    // Use postWorkerEvent instead of globalThis.postMessage
    postWorkerEvent({
      type: WorkerEventType.PARSE_ERROR,
      payload: {
        error: error.message,
        packet: {
          eventCode: packet.eventCode,
          payloadLength: packet.rawPayload.length,
          reserve: packet.reserve
        }
      }
    });
  }

  /**
   * Update handler configuration
   */
  updateConfig(config: Partial<HandlerConfig>): void {
    this.config = { ...this.config, ...config };

    // Update parser mode if changed
    if (config.mode) {
      this.parser = new InventoryParser(config.mode, this.config.debug);
    }

    // Update monitoring
    if (config.bufferMonitoring !== undefined) {
      if (config.bufferMonitoring) {
        this.startBufferMonitoring();
      } else {
        this.stopBufferMonitoring();
      }
    }

    logger.debug('[InventoryTagHandler] Configuration updated:', this.config);
  }

  /**
   * Get handler statistics
   */
  getStats(): HandlerStats {
    return {
      ...this.stats,
      bufferMetrics: this.parser.getBufferMetrics()
    };
  }

  /**
   * Cleanup handler resources
   */
  cleanup(): void {
    logger.debug('[InventoryTagHandler] Cleaning up...');

    // Stop monitoring
    this.stopBufferMonitoring();

    // Reset parser
    this.parser.reset();

    // Clear stats
    this.stats = {
      packetsProcessed: 0,
      tagsExtracted: 0,
      parseErrors: 0,
      lastProcessTime: 0
    };
  }
}

/**
 * Factory function to create inventory handler
 */
export function createInventoryHandler(config?: HandlerConfig): InventoryTagHandler {
  return new InventoryTagHandler(config);
}