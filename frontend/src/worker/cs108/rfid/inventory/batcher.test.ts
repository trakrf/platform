/**
 * Tests for InventoryBatcher
 */

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
  InventoryBatcher,
  DEFAULT_INVENTORY_CONFIG,
  LOCATE_MODE_CONFIG,
} from './batcher';
import type { TagData } from '../types';

// YAGNI: Keeping batcher code for future use but not using it yet
// We're using stream-as-we-parse approach for now
describe.skip('InventoryBatcher', () => {
  let batcher: InventoryBatcher;
  let flushCallback: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.useFakeTimers();
    flushCallback = vi.fn();
    batcher = new InventoryBatcher({
      maxSize: 3,
      timeWindowMs: 100,
      flushOnModeChange: true,
      deduplicationWindowMs: 1000,
    });
    batcher.onFlush(flushCallback);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('size-based batching', () => {
    it('should flush when batch reaches max size', () => {
      const tag1: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag2: TagData = {
        epc: 'E2001234567890AC',
        rssi: -55,
        timestamp: Date.now(),
      };
      const tag3: TagData = {
        epc: 'E2001234567890AD',
        rssi: -60,
        timestamp: Date.now(),
      };

      batcher.add(tag1);
      batcher.add(tag2);
      expect(flushCallback).not.toHaveBeenCalled();

      batcher.add(tag3); // This should trigger flush
      expect(flushCallback).toHaveBeenCalledOnce();
      expect(flushCallback).toHaveBeenCalledWith([tag1, tag2, tag3]);
    });

    it('should clear batch after flush', () => {
      const tag1: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag2: TagData = {
        epc: 'E2001234567890AC',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag3: TagData = {
        epc: 'E2001234567890AD',
        rssi: -50,
        timestamp: Date.now(),
      };

      batcher.add(tag1);
      batcher.add(tag2);
      batcher.add(tag3); // Triggers flush at maxSize=3

      expect(batcher.size()).toBe(0);
    });
  });

  describe('time-based batching', () => {
    it('should flush after time window expires', () => {
      const tag: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };

      batcher.add(tag);
      expect(flushCallback).not.toHaveBeenCalled();

      vi.advanceTimersByTime(100); // Advance to time window
      expect(flushCallback).toHaveBeenCalledOnce();
      expect(flushCallback).toHaveBeenCalledWith([tag]);
    });

    it('should not schedule multiple timers', () => {
      const tag1: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag2: TagData = {
        epc: 'E2001234567890AC',
        rssi: -55,
        timestamp: Date.now(),
      };

      batcher.add(tag1);
      batcher.add(tag2);

      vi.advanceTimersByTime(100);
      expect(flushCallback).toHaveBeenCalledOnce();
      expect(flushCallback).toHaveBeenCalledWith([tag1, tag2]);
    });
  });

  describe('deduplication', () => {
    it('should merge duplicate tags', () => {
      const tag1: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag2: TagData = {
        epc: 'E2001234567890AB', // Same EPC
        rssi: -45, // Better RSSI
        timestamp: Date.now() + 10,
      };

      batcher.add(tag1);
      batcher.add(tag2);
      batcher.flush();

      expect(flushCallback).toHaveBeenCalledWith([
        expect.objectContaining({
          epc: 'E2001234567890AB',
          rssi: -45, // Should keep best RSSI
          timestamp: tag2.timestamp, // Should update timestamp
        }),
      ]);
    });

    it('should track best RSSI for duplicates', () => {
      const epc = 'E2001234567890AB';

      batcher.add({ epc, rssi: -50, timestamp: Date.now() });
      batcher.add({ epc, rssi: -55, timestamp: Date.now() + 10 }); // Worse
      batcher.add({ epc, rssi: -45, timestamp: Date.now() + 20 }); // Better

      batcher.flush();

      expect(flushCallback).toHaveBeenCalledWith([
        expect.objectContaining({
          epc,
          rssi: -45, // Best RSSI
        }),
      ]);
    });
  });

  describe('manual operations', () => {
    it('should flush on demand', () => {
      const tag: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };

      batcher.add(tag);
      batcher.flush();

      expect(flushCallback).toHaveBeenCalledWith([tag]);
    });

    it('should clear without emitting', () => {
      const tag: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };

      batcher.add(tag);
      batcher.clear();

      expect(flushCallback).not.toHaveBeenCalled();
      expect(batcher.size()).toBe(0);
    });

    it('should handle empty flush gracefully', () => {
      batcher.flush();
      expect(flushCallback).not.toHaveBeenCalled();
    });
  });

  describe('configuration', () => {
    it('should use default inventory config', () => {
      const defaultBatcher = new InventoryBatcher();
      const config = defaultBatcher.getConfig();

      expect(config.maxSize).toBe(DEFAULT_INVENTORY_CONFIG.maxSize);
      expect(config.timeWindowMs).toBe(DEFAULT_INVENTORY_CONFIG.timeWindowMs);
    });

    it('should use locate mode config', () => {
      const locateBatcher = new InventoryBatcher(LOCATE_MODE_CONFIG);
      const config = locateBatcher.getConfig();

      expect(config.maxSize).toBe(LOCATE_MODE_CONFIG.maxSize);
      expect(config.timeWindowMs).toBe(LOCATE_MODE_CONFIG.timeWindowMs);
    });

    it('should update configuration', () => {
      batcher.updateConfig({
        maxSize: 10,
        timeWindowMs: 200,
      });

      const config = batcher.getConfig();
      expect(config.maxSize).toBe(10);
      expect(config.timeWindowMs).toBe(200);
    });
  });

  describe('statistics', () => {
    it('should provide accurate stats', () => {
      const tag1: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };
      const tag2: TagData = {
        epc: 'E2001234567890AC',
        rssi: -55,
        timestamp: Date.now(),
      };

      batcher.add(tag1);
      batcher.add(tag2);
      batcher.add(tag1); // Duplicate

      const stats = batcher.getStats();
      expect(stats.size).toBe(2); // 2 unique tags
      expect(stats.uniqueTags).toBe(2);
      expect(stats.totalReads).toBe(3); // 3 total reads
      expect(stats.timeSinceLastFlush).toBeGreaterThanOrEqual(0);
    });
  });

  describe('cleanup', () => {
    it('should cleanup properly', () => {
      const tag: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        timestamp: Date.now(),
      };

      batcher.add(tag);
      batcher.cleanup();

      expect(batcher.size()).toBe(0);

      // Should not emit after cleanup
      batcher.add(tag);
      batcher.flush();
      expect(flushCallback).not.toHaveBeenCalled();
    });
  });

  describe('edge cases', () => {
    it('should handle tags with optional fields', () => {
      const tag: TagData = {
        epc: 'E2001234567890AB',
        rssi: -50,
        pc: 0x3000,
        timestamp: Date.now(),
        phase: 45,
        antenna: 1,
      };

      batcher.add(tag);
      batcher.flush();

      expect(flushCallback).toHaveBeenCalledWith([
        expect.objectContaining({
          epc: tag.epc,
          rssi: tag.rssi,
          pc: tag.pc,
          phase: tag.phase,
          antenna: tag.antenna,
        }),
      ]);
    });

    it('should handle rapid additions', () => {
      const tags: TagData[] = [];
      for (let i = 0; i < 100; i++) {
        tags.push({
          epc: `E200123456789${i.toString(16).padStart(3, '0')}`,
          rssi: -50 - i,
          timestamp: Date.now() + i,
        });
      }

      tags.forEach(tag => batcher.add(tag));

      // Should have flushed multiple times due to size limit
      expect(flushCallback).toHaveBeenCalled();

      // All tags should have been processed
      const allFlushedTags = flushCallback.mock.calls.flat(1);
      expect(allFlushedTags.length).toBeGreaterThan(0);
    });
  });
});