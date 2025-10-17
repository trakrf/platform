/**
 * Inventory Tag Handler for CS108 RFID Reader
 *
 * Implements NotificationHandler interface to process 0x8100 inventory notifications.
 * Uses RingBuffer and InventoryParser to handle raw CS108 packets with tags that
 * may span multiple BLE packets.
 *
 * Key responsibilities:
 * - Process raw 0x8100 packets through InventoryParser
 * - Batch tags using InventoryBatcher for efficient processing
 * - Emit TAG_BATCH events to application layer
 * - Monitor buffer health and performance metrics
 * - Handle both compact and normal inventory modes
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../notification/types';
import { logger } from '../../utils/logger.js';
import type { CS108Packet } from '../type';
import { WorkerEventType } from '../../types/events';
import { ReaderMode } from '../../types/reader';
import { InventoryParser, type ParsedTag, type InventoryMode } from './parser';
import { InventoryBatcher } from './inventory/batcher';
import type { BufferMetrics } from './ring-buffer';
import type { TagData } from './types';

export interface HandlerConfig {
  mode?: InventoryMode;
  batchSize?: number;
  batchTimeout?: number;
  bufferMonitoring?: boolean;
  debug?: boolean;
}

export interface HandlerStats {
  packetsProcessed: number;
  tagsExtracted: number;
  batchesEmitted: number;
  parseErrors: number;
  lastProcessTime: number;
  bufferMetrics: BufferMetrics;
}

export class InventoryTagHandler implements NotificationHandler {
  private parser: InventoryParser;
  private batcher: InventoryBatcher;
  private config: HandlerConfig;
  private stats = {
    packetsProcessed: 0,
    tagsExtracted: 0,
    batchesEmitted: 0,
    parseErrors: 0,
    lastProcessTime: 0
  };
  private monitoringInterval?: NodeJS.Timeout;
  private contextCallback?: (event: Omit<any, 'timestamp'>) => void;

  constructor(config: HandlerConfig = {}) {
    this.config = {
      mode: 'compact',
      batchSize: 50,
      batchTimeout: 100,
      bufferMonitoring: true,
      debug: false,
      ...config
    };

    // Initialize parser with configured mode
    this.parser = new InventoryParser(this.config.mode, this.config.debug);

    // Initialize batcher with configuration
    this.batcher = new InventoryBatcher({
      maxSize: this.config.batchSize!,
      timeWindowMs: this.config.batchTimeout!,
      flushOnModeChange: true
    });

    // Start buffer monitoring if enabled
    if (this.config.bufferMonitoring) {
      this.startBufferMonitoring();
    }

    logger.debug(`[InventoryTagHandler] Initialized with mode: ${this.config.mode}, batch: ${this.config.batchSize}/${this.config.batchTimeout}ms`);
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
   * Handle the inventory notification packet
   */
  handle(packet: CS108Packet, context: NotificationContext): void {
    try {
      // Store context callback if not set
      if (!this.contextCallback) {
        this.contextCallback = context.emitNotificationEvent;

        // Set up batcher flush callback
        this.batcher.onFlush((tags) => {
          this.emitTagBatch(tags as ParsedTag[], context);
          this.stats.batchesEmitted++;
        });
      }

      // Process the already-parsed payload directly
      // No need to reconstruct the packet - CS108Packet already did all the parsing!
      const tags = this.parser.processInventoryPayload(
        packet.rawPayload,
        packet.reserve // sequence number is in the reserve field
      );

      // Update statistics
      this.stats.packetsProcessed++;
      this.stats.tagsExtracted += tags.length;
      this.stats.lastProcessTime = Date.now();

      // Monitor buffer health if enabled
      if (this.config.bufferMonitoring) {
        this.checkBufferHealth(context);
      }

      // Process each tag through the batcher
      for (const tag of tags) {
        // Convert ParsedTag to TagData format expected by batcher
        const tagData: TagData = {
          epc: tag.epc,
          rssi: tag.rssi,
          pc: tag.pc,
          timestamp: tag.timestamp,
          antenna: tag.antennaPort
        };

        // Add tag to batcher
        this.batcher.add(tagData);

        // In LOCATE mode, also emit individual tag updates
        if (context.currentMode === ReaderMode.LOCATE) {
          this.emitLocateUpdate(tag, context);
        }
      }

    } catch (error) {
      this.stats.parseErrors++;
      logger.error('[InventoryTagHandler] Error processing packet:', error);

      // Emit error event
      context.emitNotificationEvent({
        type: WorkerEventType.ERROR,
        payload: {
          message: `Failed to process inventory packet: ${error instanceof Error ? error.message : String(error)}`,
          code: 'INVENTORY_PARSE_ERROR'
        }
      });
    }
  }

  /**
   * Emit TAG_BATCH event with accumulated tags
   */
  private emitTagBatch(tags: ParsedTag[], context: NotificationContext): void {
    if (tags.length === 0) return;

    // Transform tags to match expected format
    const tagData = tags.map(tag => ({
      epc: tag.epc,
      rssi: tag.rssi,
      pc: tag.pc,
      antennaPort: tag.antennaPort || 0,
      timestamp: tag.timestamp,
      count: 1 // Initial count for new tags
    }));

    context.emitNotificationEvent({
      type: WorkerEventType.TAG_BATCH,
      payload: {
        tags: tagData
      }
    });

    if (this.config.debug) {
      logger.debug(`[InventoryTagHandler] Emitted batch of ${tags.length} tags`);
    }
  }

  /**
   * Emit LOCATE_UPDATE event for individual tag in locate mode
   */
  private emitLocateUpdate(tag: ParsedTag, context: NotificationContext): void {
    context.emitNotificationEvent({
      type: WorkerEventType.LOCATE_UPDATE,
      payload: {
        epc: tag.epc,
        rssi: tag.rssi,
        timestamp: tag.timestamp
      }
    });
  }


  /**
   * Check buffer health and log warnings if needed
   */
  private checkBufferHealth(context: NotificationContext): void {
    const metrics = this.parser.getBufferMetrics();

    // Warn if buffer utilization is high
    if (metrics.utilizationPercent > 80) {
      logger.warn(`[InventoryTagHandler] Buffer utilization high: ${metrics.utilizationPercent}%`);
    }

    // Warn if buffer has grown multiple times
    if (metrics.growthCount > 2) {
      logger.warn(`[InventoryTagHandler] Buffer has grown ${metrics.growthCount} times, consider increasing initial size`);
    }

    // Check for overflow
    if (metrics.overflowCount > 0) {
      logger.error(`[InventoryTagHandler] Buffer overflow detected: ${metrics.overflowCount} times`);

      // Emit error event for overflow
      context.emitNotificationEvent({
        type: WorkerEventType.ERROR,
        payload: {
          message: `Ring buffer overflow detected: ${metrics.overflowCount} overflows, ${metrics.used}/${metrics.size} bytes used`,
          code: 'BUFFER_OVERFLOW'
        }
      });
    }

    // Trigger growth check
    this.parser.checkBufferHealth();
  }

  /**
   * Start periodic buffer monitoring
   */
  private startBufferMonitoring(): void {
    this.monitoringInterval = setInterval(() => {
      const metrics = this.parser.getBufferMetrics();
      const parserState = this.parser.getState();

      if (this.config.debug) {
        logger.debug('[InventoryTagHandler] Buffer metrics:', {
          utilization: `${metrics.utilizationPercent}%`,
          size: metrics.size,
          used: metrics.used,
          packetsProcessed: parserState.packetsProcessed,
          tagsExtracted: parserState.tagsExtracted,
          parseErrors: parserState.parseErrors
        });
      }
    }, 10000); // Every 10 seconds
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
   * Update handler configuration
   */
  updateConfig(config: Partial<HandlerConfig>): void {
    this.config = { ...this.config, ...config };

    // Update parser mode if changed
    if (config.mode) {
      this.parser.setMode(config.mode);
    }

    // Update batcher configuration
    if (config.batchSize !== undefined || config.batchTimeout !== undefined) {
      this.batcher.updateConfig({
        maxSize: config.batchSize || this.config.batchSize!,
        timeWindowMs: config.batchTimeout || this.config.batchTimeout!,
        flushOnModeChange: true
      });
    }

    // Handle monitoring changes
    if (config.bufferMonitoring !== undefined) {
      if (config.bufferMonitoring && !this.monitoringInterval) {
        this.startBufferMonitoring();
      } else if (!config.bufferMonitoring && this.monitoringInterval) {
        this.stopBufferMonitoring();
      }
    }
  }

  /**
   * Get current handler statistics
   */
  getStats(): HandlerStats {
    const parserState = this.parser.getState();

    return {
      packetsProcessed: this.stats.packetsProcessed,
      tagsExtracted: this.stats.tagsExtracted,
      batchesEmitted: this.stats.batchesEmitted,
      parseErrors: parserState.parseErrors,
      lastProcessTime: this.stats.lastProcessTime,
      bufferMetrics: this.parser.getBufferMetrics()
    };
  }

  /**
   * Cleanup handler resources
   */
  cleanup(): void {
    // Flush any remaining tags
    this.batcher.flush();

    // Clear batcher
    this.batcher.cleanup();

    // Reset parser
    this.parser.reset();

    // Stop monitoring
    this.stopBufferMonitoring();

    logger.debug('[InventoryTagHandler] Cleanup complete, stats:', {
      packetsProcessed: this.stats.packetsProcessed,
      tagsExtracted: this.stats.tagsExtracted,
      batchesEmitted: this.stats.batchesEmitted,
      parseErrors: this.stats.parseErrors
    });
  }
}

/**
 * Create inventory handler with proper context binding
 * This factory function allows pre-configuration before registration
 */
export function createInventoryHandler(
  config?: HandlerConfig
): InventoryTagHandler {
  return new InventoryTagHandler(config);
}