import { logger } from '../../utils/logger.js';

/**
 * Optimized Ring Buffer implementation for CS108 RFID inventory data
 *
 * Key improvements over legacy 16MB implementation:
 * - Starts at 64KB (sufficient for worker thread without main thread blocking)
 * - Automatic growth up to 1MB when usage exceeds 80%
 * - Comprehensive metrics for monitoring and optimization
 * - Zero-copy operations where possible
 *
 * Based on CS108 specification requirements:
 * - Handles tags spanning multiple BLE packets (max 120 bytes per packet)
 * - Supports 1000+ tags/second throughput
 * - Accumulates data across packet boundaries seamlessly
 */

export interface BufferMetrics {
  size: number;               // Current buffer size in bytes
  used: number;               // Bytes currently in buffer
  available: number;          // Bytes available for writing
  peakUsage: number;          // Maximum bytes ever used
  utilizationPercent: number; // used/size * 100
  overflowCount: number;      // Times buffer was full
  growthCount: number;        // Times buffer was resized
  lastGrowth?: Date;          // When last grown
}

export class RingBuffer {
  private buffer: Uint8Array;
  private startPoint = 0;
  private size = 0;
  private readonly initialSize: number;
  private readonly maxSize: number;
  private peakUsage = 0;
  private overflowCount = 0;
  private growthCount = 0;
  private lastGrowth?: Date;

  constructor(
    initialSize: number = 64 * 1024,  // Default 64KB
    maxSize: number = 1024 * 1024      // Max 1MB
  ) {
    if (initialSize <= 0 || initialSize > maxSize) {
      throw new Error(`Invalid buffer size: initial=${initialSize}, max=${maxSize}`);
    }

    this.initialSize = initialSize;
    this.maxSize = maxSize;
    this.buffer = new Uint8Array(initialSize);

    logger.debug(`[RingBuffer] Initialized with ${initialSize / 1024}KB (max: ${maxSize / 1024}KB)`);
  }

  /**
   * Get current data length in buffer
   */
  get length(): number {
    return this.size;
  }

  /**
   * Get current buffer capacity
   */
  get capacity(): number {
    return this.buffer.length;
  }

  /**
   * Clear all data from buffer
   */
  clear(): void {
    this.startPoint = 0;
    this.size = 0;
  }

  /**
   * Add data to the ring buffer with automatic growth if needed
   * @param data Data to add
   * @param offset Starting offset in data array
   * @param length Number of bytes to add (-1 for all remaining)
   * @returns true if successful, false if buffer would overflow max size
   */
  dataIn(data: Uint8Array, offset = 0, length = -1): boolean {
    if (length < 0) {
      length = data.length - offset;
    }

    if (length === 0) {
      return true;
    }

    // Check if we need to grow
    const needed = this.size + length;
    if (needed > this.buffer.length) {
      // Check if growth would help
      if (needed > this.maxSize) {
        this.overflowCount++;
        logger.warn(`[RingBuffer] Overflow: need ${needed} bytes, max is ${this.maxSize}`);
        return false;
      }

      // Try to grow
      if (!this.tryGrow(needed)) {
        this.overflowCount++;
        return false;
      }
    }

    // Calculate write position with wrap-around
    const writePos = (this.startPoint + this.size) % this.buffer.length;

    if (writePos + length <= this.buffer.length) {
      // No wrap - straight copy
      this.buffer.set(data.subarray(offset, offset + length), writePos);
    } else {
      // Wrap around - split write
      const firstPart = this.buffer.length - writePos;
      this.buffer.set(data.subarray(offset, offset + firstPart), writePos);
      this.buffer.set(data.subarray(offset + firstPart, offset + length), 0);
    }

    this.size += length;

    // Update peak usage
    if (this.size > this.peakUsage) {
      this.peakUsage = this.size;
    }

    return true;
  }

  /**
   * Peek at data without removing it from buffer
   * @param length Number of bytes to peek
   * @returns Data array or null if not enough data
   */
  dataPreOut(length: number): Uint8Array | null {
    if (length > this.size) {
      return null;
    }

    const result = new Uint8Array(length);

    // Check if we need to wrap around
    if (this.startPoint + length <= this.buffer.length) {
      // No wrap - straight copy
      result.set(this.buffer.subarray(this.startPoint, this.startPoint + length));
    } else {
      // Need to wrap - split the read
      const firstPart = this.buffer.length - this.startPoint;
      const secondPart = length - firstPart;

      result.set(this.buffer.subarray(this.startPoint, this.startPoint + firstPart), 0);
      result.set(this.buffer.subarray(0, secondPart), firstPart);
    }

    return result;
  }

  /**
   * Read and remove data from buffer
   * @param length Number of bytes to read
   * @returns Data array or null if not enough data
   */
  dataOut(length: number): Uint8Array | null {
    const data = this.dataPreOut(length);
    if (data) {
      this.dataDel(length);
    }
    return data;
  }

  /**
   * Delete data from buffer without returning it
   * @param length Number of bytes to delete
   * @returns true if successful
   */
  dataDel(length: number): boolean {
    if (length > this.size) {
      return false;
    }

    this.size -= length;
    if (this.size === 0) {
      this.startPoint = 0;
    } else {
      this.startPoint = (this.startPoint + length) % this.buffer.length;
    }

    return true;
  }

  /**
   * Try to grow the buffer to accommodate more data
   * @param needed Minimum bytes needed
   * @returns true if growth successful
   */
  private tryGrow(needed: number): boolean {
    // Check if already at max
    if (this.buffer.length >= this.maxSize) {
      return false;
    }

    // Calculate new size (double current or minimum needed)
    let newSize = Math.max(this.buffer.length * 2, needed);
    newSize = Math.min(newSize, this.maxSize);

    if (newSize < needed) {
      return false;
    }

    logger.debug(`[RingBuffer] Growing buffer: ${this.buffer.length} → ${newSize} bytes`);

    // Extract current data
    const currentData = this.dataPreOut(this.size);
    if (!currentData) {
      return false;
    }

    // Allocate new buffer and copy data
    const newBuffer = new Uint8Array(newSize);
    newBuffer.set(currentData, 0);

    // Update buffer and reset pointers
    this.buffer = newBuffer;
    this.startPoint = 0;

    // Update metrics
    this.growthCount++;
    this.lastGrowth = new Date();

    return true;
  }

  /**
   * Get comprehensive buffer metrics for monitoring
   */
  getMetrics(): BufferMetrics {
    return {
      size: this.buffer.length,
      used: this.size,
      available: this.buffer.length - this.size,
      peakUsage: this.peakUsage,
      utilizationPercent: Math.round((this.size / this.buffer.length) * 100),
      overflowCount: this.overflowCount,
      growthCount: this.growthCount,
      lastGrowth: this.lastGrowth
    };
  }

  /**
   * Check if buffer should grow based on utilization
   * Called periodically to preemptively grow buffer
   */
  checkGrowth(threshold: number = 0.8): void {
    const utilization = this.size / this.buffer.length;
    if (utilization > threshold && this.buffer.length < this.maxSize) {
      const targetSize = Math.min(this.buffer.length * 2, this.maxSize);
      this.tryGrow(targetSize);
    }
  }

  /**
   * Reset buffer to initial size (useful for cleanup)
   */
  reset(): void {
    this.buffer = new Uint8Array(this.initialSize);
    this.startPoint = 0;
    this.size = 0;
    this.peakUsage = 0;
    this.overflowCount = 0;
    this.growthCount = 0;
    this.lastGrowth = undefined;

    logger.debug(`[RingBuffer] Reset to initial size: ${this.initialSize / 1024}KB`);
  }
}