import { describe, it, expect, beforeEach } from 'vitest';
import { RingBuffer } from './ring-buffer';

describe('RingBuffer', () => {
  describe('basic operations', () => {
    let buffer: RingBuffer;

    beforeEach(() => {
      buffer = new RingBuffer(100);
    });

    it('initializes with correct size', () => {
      expect(buffer.length).toBe(0);
      expect(buffer.capacity).toBe(100);

      const metrics = buffer.getMetrics();
      expect(metrics.size).toBe(100);
      expect(metrics.used).toBe(0);
      expect(metrics.available).toBe(100);
    });

    it('handles basic write and read operations', () => {
      const data = new Uint8Array([1, 2, 3, 4, 5]);

      expect(buffer.dataIn(data)).toBe(true);
      expect(buffer.length).toBe(5);

      const out = buffer.dataOut(3);
      expect(out).toEqual(new Uint8Array([1, 2, 3]));
      expect(buffer.length).toBe(2);

      const remaining = buffer.dataOut(2);
      expect(remaining).toEqual(new Uint8Array([4, 5]));
      expect(buffer.length).toBe(0);
    });

    it('handles peek without removing data', () => {
      const data = new Uint8Array([10, 20, 30, 40, 50]);

      buffer.dataIn(data);

      const peeked = buffer.dataPreOut(3);
      expect(peeked).toEqual(new Uint8Array([10, 20, 30]));
      expect(buffer.length).toBe(5); // Data still in buffer

      const read = buffer.dataOut(3);
      expect(read).toEqual(new Uint8Array([10, 20, 30]));
      expect(buffer.length).toBe(2); // Data removed
    });

    it('returns null when reading more than available', () => {
      const data = new Uint8Array([1, 2, 3]);
      buffer.dataIn(data);

      expect(buffer.dataPreOut(5)).toBeNull();
      expect(buffer.dataOut(5)).toBeNull();
      expect(buffer.length).toBe(3); // No data removed
    });

    it('handles empty writes gracefully', () => {
      expect(buffer.dataIn(new Uint8Array(0))).toBe(true);
      expect(buffer.length).toBe(0);

      expect(buffer.dataIn(new Uint8Array(5), 0, 0)).toBe(true);
      expect(buffer.length).toBe(0);
    });

    it('handles dataDel to remove data without returning', () => {
      const data = new Uint8Array([1, 2, 3, 4, 5]);
      buffer.dataIn(data);

      expect(buffer.dataDel(2)).toBe(true);
      expect(buffer.length).toBe(3);

      const remaining = buffer.dataOut(3);
      expect(remaining).toEqual(new Uint8Array([3, 4, 5]));
    });

    it('clear() resets buffer state', () => {
      buffer.dataIn(new Uint8Array([1, 2, 3, 4, 5]));
      expect(buffer.length).toBe(5);

      buffer.clear();
      expect(buffer.length).toBe(0);

      // Can write after clear
      buffer.dataIn(new Uint8Array([10, 20]));
      expect(buffer.dataOut(2)).toEqual(new Uint8Array([10, 20]));
    });
  });

  describe('wrap-around scenarios', () => {
    let buffer: RingBuffer;

    beforeEach(() => {
      buffer = new RingBuffer(10);
    });

    it('handles wrap-around on write correctly', () => {
      // Fill buffer almost to end
      buffer.dataIn(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]));
      expect(buffer.length).toBe(8);

      // Remove first 5 bytes
      const removed = buffer.dataOut(5);
      expect(removed).toEqual(new Uint8Array([1, 2, 3, 4, 5]));
      expect(buffer.length).toBe(3);

      // Add data that wraps around
      buffer.dataIn(new Uint8Array([9, 10, 11, 12, 13]));
      expect(buffer.length).toBe(8);

      // Should read correctly across wrap
      const out = buffer.dataOut(8);
      expect(out).toEqual(new Uint8Array([6, 7, 8, 9, 10, 11, 12, 13]));
    });

    it('handles wrap-around on read correctly', () => {
      // Create wrap scenario
      buffer.dataIn(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]));
      buffer.dataOut(6); // Remove [1,2,3,4,5,6], leaves [7,8]
      buffer.dataIn(new Uint8Array([9, 10, 11, 12])); // Wraps to beginning

      // Peek across wrap boundary
      const peeked = buffer.dataPreOut(6);
      expect(peeked).toEqual(new Uint8Array([7, 8, 9, 10, 11, 12]));

      // Read across wrap boundary
      const read = buffer.dataOut(6);
      expect(read).toEqual(new Uint8Array([7, 8, 9, 10, 11, 12]));
      expect(buffer.length).toBe(0);
    });

    it('handles multiple wrap-arounds', () => {
      // Simulate continuous read/write with wrapping
      for (let cycle = 0; cycle < 5; cycle++) {
        const writeData = new Uint8Array([cycle * 10, cycle * 10 + 1, cycle * 10 + 2]);
        buffer.dataIn(writeData);

        const readData = buffer.dataOut(3);
        expect(readData).toEqual(writeData);
      }

      expect(buffer.length).toBe(0);
    });

    it('handles exact buffer boundary writes', () => {
      // Fill exactly to capacity
      const data = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
      expect(buffer.dataIn(data)).toBe(true);
      expect(buffer.length).toBe(10);

      // Read all
      const out = buffer.dataOut(10);
      expect(out).toEqual(data);
      expect(buffer.length).toBe(0);
    });
  });

  describe('buffer growth', () => {
    it('grows buffer when needed', () => {
      const buffer = new RingBuffer(10, 100);
      const metrics1 = buffer.getMetrics();
      expect(metrics1.size).toBe(10);
      expect(metrics1.growthCount).toBe(0);

      // Try to add more than capacity
      const largeData = new Uint8Array(15).fill(42);
      expect(buffer.dataIn(largeData)).toBe(true);
      expect(buffer.length).toBe(15);

      const metrics2 = buffer.getMetrics();
      expect(metrics2.size).toBe(20); // Should have doubled
      expect(metrics2.growthCount).toBe(1);
      expect(metrics2.lastGrowth).toBeDefined();

      // Verify data integrity
      const out = buffer.dataOut(15);
      expect(out).toEqual(largeData);
    });

    it('preserves data during growth', () => {
      const buffer = new RingBuffer(10, 100);

      // Add initial data
      buffer.dataIn(new Uint8Array([1, 2, 3, 4, 5]));

      // Force growth
      const moreData = new Uint8Array([6, 7, 8, 9, 10, 11]);
      expect(buffer.dataIn(moreData)).toBe(true);

      // All data should be preserved
      const allData = buffer.dataOut(11);
      expect(allData).toEqual(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]));
    });

    it('preserves wrapped data during growth', () => {
      const buffer = new RingBuffer(10, 100);

      // Create wrapped scenario
      buffer.dataIn(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]));
      buffer.dataOut(5); // Remove first 5, leaves [6,7,8]
      buffer.dataIn(new Uint8Array([9, 10, 11, 12])); // Wraps, now [6,7,8,9,10,11,12]

      // Force growth with more data
      const moreData = new Uint8Array([13, 14, 15, 16]);
      expect(buffer.dataIn(moreData)).toBe(true);

      // Verify all data preserved in order
      const allData = buffer.dataOut(11);
      expect(allData).toEqual(new Uint8Array([6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]));
    });

    it('respects maximum size limit', () => {
      const buffer = new RingBuffer(10, 20);

      // Fill to near max
      buffer.dataIn(new Uint8Array(18));
      expect(buffer.capacity).toBe(20); // Should have grown

      // Try to exceed max
      const tooMuch = new Uint8Array(5);
      expect(buffer.dataIn(tooMuch)).toBe(false);
      expect(buffer.length).toBe(18); // No data added

      const metrics = buffer.getMetrics();
      expect(metrics.overflowCount).toBe(1);
    });

    it('checkGrowth() preemptively grows buffer', () => {
      const buffer = new RingBuffer(100, 1000);

      // Fill to 85%
      buffer.dataIn(new Uint8Array(85));
      expect(buffer.capacity).toBe(100);

      // Check growth with default 80% threshold
      buffer.checkGrowth();
      expect(buffer.capacity).toBe(200); // Should have doubled

      // Data should be preserved
      expect(buffer.length).toBe(85);
      const data = buffer.dataOut(85);
      expect(data).toHaveLength(85);
    });

    it('grows to exact needed size when doubling insufficient', () => {
      const buffer = new RingBuffer(10, 100);

      // Try to add 30 bytes (doubling to 20 wouldn't be enough)
      const data = new Uint8Array(30).fill(99);
      expect(buffer.dataIn(data)).toBe(true);

      // Should grow to at least 30
      expect(buffer.capacity).toBeGreaterThanOrEqual(30);
      expect(buffer.length).toBe(30);

      // Verify data
      expect(buffer.dataOut(30)).toEqual(data);
    });
  });

  describe('metrics tracking', () => {
    it('tracks peak usage correctly', () => {
      const buffer = new RingBuffer(100);
      const metrics1 = buffer.getMetrics();
      expect(metrics1.peakUsage).toBe(0);

      // Add and remove data
      buffer.dataIn(new Uint8Array(50));
      const metrics2 = buffer.getMetrics();
      expect(metrics2.peakUsage).toBe(50);

      buffer.dataOut(30);
      const metrics3 = buffer.getMetrics();
      expect(metrics3.peakUsage).toBe(50); // Peak doesn't decrease
      expect(metrics3.used).toBe(20);

      buffer.dataIn(new Uint8Array(60));
      const metrics4 = buffer.getMetrics();
      expect(metrics4.peakUsage).toBe(80); // New peak
    });

    it('calculates utilization percentage correctly', () => {
      const buffer = new RingBuffer(100);

      buffer.dataIn(new Uint8Array(25));
      let metrics = buffer.getMetrics();
      expect(metrics.utilizationPercent).toBe(25);

      buffer.dataIn(new Uint8Array(25));
      metrics = buffer.getMetrics();
      expect(metrics.utilizationPercent).toBe(50);

      buffer.dataIn(new Uint8Array(30));
      metrics = buffer.getMetrics();
      expect(metrics.utilizationPercent).toBe(80);
    });

    it('tracks overflow attempts', () => {
      const buffer = new RingBuffer(10, 10); // No growth allowed

      buffer.dataIn(new Uint8Array(8));
      const metrics1 = buffer.getMetrics();
      expect(metrics1.overflowCount).toBe(0);

      // Try to overflow
      expect(buffer.dataIn(new Uint8Array(5))).toBe(false);
      const metrics2 = buffer.getMetrics();
      expect(metrics2.overflowCount).toBe(1);
      expect(metrics2.used).toBe(8); // No data added

      // Another overflow
      expect(buffer.dataIn(new Uint8Array(20))).toBe(false);
      const metrics3 = buffer.getMetrics();
      expect(metrics3.overflowCount).toBe(2);
    });
  });

  describe('edge cases', () => {
    it('handles invalid constructor parameters', () => {
      expect(() => new RingBuffer(0)).toThrow();
      expect(() => new RingBuffer(-10)).toThrow();
      expect(() => new RingBuffer(100, 50)).toThrow(); // initial > max
    });

    it('handles writing with offset and length', () => {
      const buffer = new RingBuffer(100);
      const data = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);

      // Write middle portion
      expect(buffer.dataIn(data, 3, 4)).toBe(true);
      expect(buffer.length).toBe(4);
      expect(buffer.dataOut(4)).toEqual(new Uint8Array([4, 5, 6, 7]));

      // Write from offset to end
      expect(buffer.dataIn(data, 7, -1)).toBe(true);
      expect(buffer.length).toBe(3);
      expect(buffer.dataOut(3)).toEqual(new Uint8Array([8, 9, 10]));
    });

    it('reset() restores initial state', () => {
      const buffer = new RingBuffer(10, 100);

      // Grow buffer and add data
      buffer.dataIn(new Uint8Array(50));
      const metrics1 = buffer.getMetrics();
      expect(metrics1.size).toBeGreaterThan(10);
      expect(metrics1.growthCount).toBeGreaterThan(0);
      expect(metrics1.peakUsage).toBe(50);

      // Reset
      buffer.reset();

      const metrics2 = buffer.getMetrics();
      expect(metrics2.size).toBe(10);
      expect(metrics2.used).toBe(0);
      expect(metrics2.growthCount).toBe(0);
      expect(metrics2.peakUsage).toBe(0);
      expect(metrics2.overflowCount).toBe(0);
      expect(metrics2.lastGrowth).toBeUndefined();

      // Can use after reset
      buffer.dataIn(new Uint8Array([99]));
      expect(buffer.dataOut(1)).toEqual(new Uint8Array([99]));
    });

    it('handles exact capacity operations', () => {
      const buffer = new RingBuffer(5, 5); // No growth

      // Fill exactly
      const data = new Uint8Array([1, 2, 3, 4, 5]);
      expect(buffer.dataIn(data)).toBe(true);
      expect(buffer.length).toBe(5);

      // Can't add more
      expect(buffer.dataIn(new Uint8Array([6]))).toBe(false);

      // Read all
      expect(buffer.dataOut(5)).toEqual(data);

      // Can fill again
      expect(buffer.dataIn(data)).toBe(true);
    });

    it('handles rapid write-read cycles without data loss', () => {
      const buffer = new RingBuffer(50, 200);
      const iterations = 100;

      for (let i = 0; i < iterations; i++) {
        const writeData = new Uint8Array(10).map((_, idx) => (i * 10 + idx) % 256);
        expect(buffer.dataIn(writeData)).toBe(true);

        const readData = buffer.dataOut(10);
        expect(readData).toEqual(writeData);
      }

      expect(buffer.length).toBe(0);

      const metrics = buffer.getMetrics();
      expect(metrics.overflowCount).toBe(0);
    });
  });

  describe('performance characteristics', () => {
    it('handles large data efficiently', () => {
      const buffer = new RingBuffer(64 * 1024, 1024 * 1024); // 64KB to 1MB

      // Simulate continuous tag data at 1000 tags/second
      // Each tag ~30 bytes, 1000 tags = 30KB/second
      const tagBatch = new Uint8Array(30000); // 1 second of tags

      const startTime = Date.now();

      // Simulate 10 seconds
      for (let i = 0; i < 10; i++) {
        expect(buffer.dataIn(tagBatch)).toBe(true);
        expect(buffer.dataOut(tagBatch.length)).toBeTruthy();
      }

      const elapsed = Date.now() - startTime;
      expect(elapsed).toBeLessThan(100); // Should be very fast

      const metrics = buffer.getMetrics();
      expect(metrics.overflowCount).toBe(0);
    });
  });
});