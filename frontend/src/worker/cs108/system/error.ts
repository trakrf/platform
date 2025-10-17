/**
 * Error notification handler
 *
 * Handles error notifications (0xA101) from the CS108 device.
 * Maps error codes to human-readable messages and implements
 * rate limiting for repeated errors.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../notification/types';
import type { ErrorData } from './types';
import type { CS108Packet } from '../type';
import { isScalarPayload, isErrorPayload } from '../payload-types';
import { postWorkerEvent, WorkerEventType } from '../../types/events';
import { logger } from '../../utils/logger.js';

/**
 * CS108 error codes
 */
export enum CS108ErrorCode {
  WRONG_HEADER_PREFIX = 0x0001,
  PAYLOAD_LENGTH_TOO_LARGE = 0x0002,
  UNKNOWN_TARGET = 0x0003,
  UNKNOWN_EVENT = 0x0004,
  INVALID_PARAMETER = 0x0005,
  COMMAND_TIMEOUT = 0x0006,
  FIRMWARE_ERROR = 0x0007,
  HARDWARE_ERROR = 0x0008,
}

/**
 * Map error codes to descriptions
 */
const ERROR_DESCRIPTIONS: Record<number, string> = {
  [CS108ErrorCode.WRONG_HEADER_PREFIX]: 'Wrong header prefix',
  [CS108ErrorCode.PAYLOAD_LENGTH_TOO_LARGE]: 'Payload length too large',
  [CS108ErrorCode.UNKNOWN_TARGET]: 'Unknown target module',
  [CS108ErrorCode.UNKNOWN_EVENT]: 'Unknown event code',
  [CS108ErrorCode.INVALID_PARAMETER]: 'Invalid parameter',
  [CS108ErrorCode.COMMAND_TIMEOUT]: 'Command timeout',
  [CS108ErrorCode.FIRMWARE_ERROR]: 'Firmware error',
  [CS108ErrorCode.HARDWARE_ERROR]: 'Hardware error',
};

/**
 * Rate limiting configuration
 */
interface RateLimitInfo {
  count: number;
  firstSeen: number;
  lastLogged: number;
}

/**
 * Handler for error notifications
 * Implements rate limiting to prevent log spam
 */
export class ErrorNotificationHandler implements NotificationHandler {
  private errorRateLimit = new Map<number, RateLimitInfo>();
  private readonly ERROR_LOG_THRESHOLD = 3;
  private readonly ERROR_LOG_INTERVAL_MS = 5000;
  private readonly CLEANUP_INTERVAL_MS = 60000; // Clean up old entries every minute

  constructor() {
    // Periodically clean up old rate limit entries
    setInterval(() => this.cleanupRateLimits(), this.CLEANUP_INTERVAL_MS);
  }

  /**
   * Check if packet has error data
   */
  canHandle(packet: CS108Packet, _context: NotificationContext): boolean {
    return isScalarPayload(packet.payload) || isErrorPayload(packet.payload);
  }

  /**
   * Handle error notification
   * Applies rate limiting and emits ERROR event
   */
  handle(packet: CS108Packet, _context: NotificationContext): void {
    // Extract error code
    let errorCode: number;
    let errorModule: number | undefined;

    if (isScalarPayload(packet.payload)) {
      errorCode = packet.payload;
    } else if (isErrorPayload(packet.payload)) {
      errorCode = packet.payload.code;
      errorModule = packet.payload.message ? undefined : undefined; // Module not in ErrorPayload
    } else {
      logger.warn('[ErrorHandler] Invalid error payload format');
      return;
    }

    // Get error description
    const description = ERROR_DESCRIPTIONS[errorCode] || 'Unknown error';

    // Check rate limiting
    if (this.shouldLog(errorCode)) {
      const rateInfo = this.errorRateLimit.get(errorCode);
      const suffix = rateInfo && rateInfo.count > this.ERROR_LOG_THRESHOLD
        ? ` (${rateInfo.count} occurrences in last ${this.ERROR_LOG_INTERVAL_MS / 1000}s)`
        : '';

      logger.error(
        `[CS108 Error] ${description} (0x${errorCode.toString(16).padStart(4, '0')})${suffix}`
      );
    }

    // Always emit domain event (UI can do its own rate limiting)
    const errorData: ErrorData = {
      code: errorCode,
      message: description,
      module: errorModule,
      timestamp: Date.now(),
    };

    postWorkerEvent({
      type: WorkerEventType.DEVICE_ERROR,
      payload: {
        severity: this.getSeverity(errorCode),
        message: errorData.message,
        code: errorCode.toString(16).padStart(4, '0'),
        details: { module: errorData.module },
      },
    });
  }

  /**
   * Get severity based on error code
   */
  private getSeverity(errorCode: number): 'warning' | 'error' | 'critical' {
    if (errorCode === CS108ErrorCode.HARDWARE_ERROR) return 'critical';
    if (errorCode === CS108ErrorCode.FIRMWARE_ERROR) return 'error';
    return 'warning';
  }

  /**
   * Check if error should be logged based on rate limiting
   */
  private shouldLog(errorCode: number): boolean {
    const now = Date.now();
    let rateInfo = this.errorRateLimit.get(errorCode);

    if (!rateInfo) {
      // First occurrence
      rateInfo = {
        count: 1,
        firstSeen: now,
        lastLogged: now,
      };
      this.errorRateLimit.set(errorCode, rateInfo);
      return true;
    }

    rateInfo.count++;

    // Always log first few occurrences
    if (rateInfo.count <= this.ERROR_LOG_THRESHOLD) {
      rateInfo.lastLogged = now;
      return true;
    }

    // After threshold, only log periodically
    const timeSinceLastLog = now - rateInfo.lastLogged;
    if (timeSinceLastLog >= this.ERROR_LOG_INTERVAL_MS) {
      rateInfo.lastLogged = now;
      return true;
    }

    return false;
  }

  /**
   * Clean up old rate limit entries to prevent memory leak
   */
  private cleanupRateLimits(): void {
    const now = Date.now();
    const staleThreshold = 5 * 60 * 1000; // 5 minutes

    for (const [errorCode, rateInfo] of this.errorRateLimit.entries()) {
      if (now - rateInfo.lastLogged > staleThreshold) {
        this.errorRateLimit.delete(errorCode);
      }
    }
  }

  /**
   * Cleanup method called when handler is unregistered
   */
  cleanup(): void {
    this.errorRateLimit.clear();
  }
}