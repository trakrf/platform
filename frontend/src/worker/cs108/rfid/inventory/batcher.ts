/**
 * Inventory tag batcher
 *
 * Batches RFID tag reads for efficient processing and UI updates.
 * Implements configurable batching strategies with time and size triggers.
 */

import type { Batcher, BatchingConfig } from '../../notification/types';
import type { TagData } from '../types';

/**
 * Default batching configuration for inventory mode
 */
export const DEFAULT_INVENTORY_CONFIG: BatchingConfig = {
  maxSize: 50,                    // Flush after 50 tags
  timeWindowMs: 100,               // Flush every 100ms
  flushOnModeChange: true,        // Flush when switching modes
  deduplicationWindowMs: 1000,    // Dedupe tags within 1 second
};

/**
 * Configuration for locate mode (more frequent updates)
 */
export const LOCATE_MODE_CONFIG: BatchingConfig = {
  maxSize: 5,                     // Smaller batches for real-time feedback
  timeWindowMs: 50,                // More frequent updates
  flushOnModeChange: true,
  deduplicationWindowMs: 500,     // Shorter dedupe window
};

/**
 * Tag with metadata for deduplication
 */
interface BatchedTag extends TagData {
  /** First time this tag was seen in current batch */
  firstSeen: number;
  /** Number of times seen in current batch */
  count: number;
  /** Best (highest) RSSI value seen */
  bestRssi: number;
}

/**
 * Batcher for inventory tag data
 */
export class InventoryBatcher implements Batcher<TagData> {
  private batch = new Map<string, BatchedTag>();
  private config: BatchingConfig;
  private flushCallback?: (items: TagData[]) => void;
  private flushTimer?: NodeJS.Timeout;
  private lastFlush = Date.now();

  constructor(config: BatchingConfig = DEFAULT_INVENTORY_CONFIG) {
    this.config = config;
  }

  /**
   * Add tag to batch
   * Handles deduplication and triggers flush if needed
   */
  add(tag: TagData): void {
    const existing = this.batch.get(tag.epc);

    if (existing) {
      // Update existing tag
      existing.count++;
      existing.timestamp = tag.timestamp;

      // Keep best RSSI
      if (tag.rssi > existing.bestRssi) {
        existing.bestRssi = tag.rssi;
        existing.rssi = tag.rssi;
      }
    } else {
      // Add new tag
      this.batch.set(tag.epc, {
        ...tag,
        firstSeen: tag.timestamp,
        count: 1,
        bestRssi: tag.rssi,
      });
    }

    // Check size trigger
    if (this.batch.size >= this.config.maxSize) {
      this.flush();
    } else {
      // Set up time trigger if not already running
      this.scheduleFlush();
    }
  }

  /**
   * Force flush of current batch
   */
  flush(): void {
    // Cancel any pending flush timer
    if (this.flushTimer) {
      clearTimeout(this.flushTimer);
      this.flushTimer = undefined;
    }

    if (this.batch.size === 0) {
      return;
    }

    // Convert batch to array
    const tags: TagData[] = [];
    for (const batchedTag of this.batch.values()) {
      // Convert back to TagData (remove internal tracking fields)
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { firstSeen, count, bestRssi, ...tagData } = batchedTag;
      tags.push(tagData);
    }

    // Clear batch
    this.batch.clear();
    this.lastFlush = Date.now();

    // Emit if we have tags and a callback
    if (tags.length > 0 && this.flushCallback) {
      this.flushCallback(tags);
    }
  }

  /**
   * Clear batch without emitting
   */
  clear(): void {
    if (this.flushTimer) {
      clearTimeout(this.flushTimer);
      this.flushTimer = undefined;
    }
    this.batch.clear();
  }

  /**
   * Get current batch size
   */
  size(): number {
    return this.batch.size;
  }

  /**
   * Set flush callback
   */
  onFlush(callback: (items: TagData[]) => void): void {
    this.flushCallback = callback;
  }

  /**
   * Update batching configuration
   */
  updateConfig(config: Partial<BatchingConfig>): void {
    this.config = { ...this.config, ...config };

    // If reducing time window, reschedule flush
    if (config.timeWindowMs !== undefined && this.flushTimer) {
      this.scheduleFlush();
    }
  }

  /**
   * Get current configuration
   */
  getConfig(): BatchingConfig {
    return { ...this.config };
  }

  /**
   * Schedule a flush based on time window
   */
  private scheduleFlush(): void {
    // Don't schedule if already scheduled
    if (this.flushTimer) {
      return;
    }

    // Calculate time until next flush
    const timeSinceLastFlush = Date.now() - this.lastFlush;
    const timeUntilFlush = Math.max(
      0,
      this.config.timeWindowMs - timeSinceLastFlush
    );

    this.flushTimer = setTimeout(() => {
      this.flushTimer = undefined;
      this.flush();
    }, timeUntilFlush);
  }

  /**
   * Get statistics about current batch
   */
  getStats(): {
    size: number;
    uniqueTags: number;
    totalReads: number;
    timeSinceLastFlush: number;
  } {
    let totalReads = 0;
    for (const tag of this.batch.values()) {
      totalReads += tag.count;
    }

    return {
      size: this.batch.size,
      uniqueTags: this.batch.size,
      totalReads,
      timeSinceLastFlush: Date.now() - this.lastFlush,
    };
  }

  /**
   * Cleanup when batcher is no longer needed
   */
  cleanup(): void {
    this.clear();
    this.flushCallback = undefined;
  }
}