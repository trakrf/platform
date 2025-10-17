/**
 * Core types and interfaces for the notification handling system
 *
 * This module defines the contracts for handling CS108 notifications
 * in a clean, extensible way that avoids the spaghetti code of
 * massive switch statements.
 */

import type { ReaderModeType, ReaderStateType } from '../../types/reader';
import type { CS108Packet } from '../type';
import type { WorkerEvent } from '../../types/events';


/**
 * Context provided to notification handlers
 * Contains current state and dependencies needed for handling
 */
export interface NotificationContext {
  /** Current reader mode (IDLE, INVENTORY, LOCATE, BARCODE) */
  currentMode: ReaderModeType;

  /** Current reader state (READY, SCANNING, etc.) */
  readerState: ReaderStateType;

  /** Function to emit domain events to the application layer */
  emitNotificationEvent: (event: Omit<WorkerEvent, 'timestamp'>) => void;

  /** Optional function to get current store state */
  getState?: () => unknown;

  /** Optional metadata for handler-specific context */
  metadata?: Record<string, unknown>;
}

/**
 * Interface for notification handlers
 * Each handler is responsible for a specific notification type
 */
export interface NotificationHandler {
  /**
   * Check if this handler can process the given packet
   * Allows for conditional handling based on context
   */
  canHandle(packet: CS108Packet, context: NotificationContext): boolean;

  /**
   * Handle the notification packet
   * Should emit appropriate domain events via context
   */
  handle(packet: CS108Packet, context: NotificationContext): void;

  /**
   * Optional cleanup method called when handler is unregistered
   */
  cleanup?(): void;
}

/**
 * Configuration for batching strategies
 */
export interface BatchingConfig {
  /** Maximum number of items before forcing a flush */
  maxSize: number;

  /** Maximum time in ms before forcing a flush */
  timeWindowMs: number;

  /** Whether to flush immediately on mode changes */
  flushOnModeChange: boolean;

  /** Optional deduplication window in ms */
  deduplicationWindowMs?: number;
}

/**
 * Interface for batchers used by complex handlers
 */
export interface Batcher<T> {
  /** Add item to batch */
  add(item: T): void;

  /** Force flush of current batch */
  flush(): void;

  /** Clear batch without emitting */
  clear(): void;

  /** Get current batch size */
  size(): number;

  /** Set flush callback */
  onFlush(callback: (items: T[]) => void): void;
}

