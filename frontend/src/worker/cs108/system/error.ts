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
  WRONG_HEADER_PREFIX = 0x0000,
  PAYLOAD_LENGTH_TOO_LARGE = 0x0001,
  UNKNOWN_TARGET = 0x0002,
  UNKNOWN_EVENT = 0x0003,
}

/**
 * Map error codes to descriptions.
 *
 * ⚠ These four are the whole of the spec's table. Until TRA-1229 this enum
 * numbered each of them one higher and carried four more —
 * `INVALID_PARAMETER`, `COMMAND_TIMEOUT`, `FIRMWARE_ERROR`, `HARDWARE_ERROR`
 * at 0x0005–0x0008 — which appear nowhere in the byte-stream spec. The shift
 * mattered: `0x0000` is the code the device sends in practice, and under the
 * old numbering it fell off the end of the map and reported as "Unknown error".
 *
 * `command.ts` has always read the same wire bytes correctly. Two tables that
 * disagree is the defect; this is now the only one, and `command.ts` imports it.
 */
export const ERROR_DESCRIPTIONS: Record<number, string> = {
  [CS108ErrorCode.WRONG_HEADER_PREFIX]: 'Wrong header prefix',
  [CS108ErrorCode.PAYLOAD_LENGTH_TOO_LARGE]: 'Payload length too large',
  [CS108ErrorCode.UNKNOWN_TARGET]: 'Unknown target',
  [CS108ErrorCode.UNKNOWN_EVENT]: 'Unknown event',
};

/** Describe a code, naming the number when the spec does not cover it. */
export function describeErrorCode(code: number): string {
  return ERROR_DESCRIPTIONS[code]
    ?? `Unknown error 0x${code.toString(16).padStart(4, '0')}`;
}

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
  /** Unconditional arrival counts, per code and in total. Never rate-limited. */
  private errorCounts = new Map<number, number>();
  private totalErrors = 0;
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
    const description = describeErrorCode(errorCode);

    // Count BEFORE the rate limiter, and unconditionally.
    //
    // The limiter caps log volume, which is right. What it must not do is make
    // the frames disappear: on the 2026-09-01 hardware run it turned 1716
    // arrivals into 8 log lines and nothing else recorded them, so an 86-minute
    // fault storm was invisible to the soak instrument. A count is cheap and it
    // is the thing an arm can read. Refs TRA-1229.
    this.errorCounts.set(errorCode, (this.errorCounts.get(errorCode) ?? 0) + 1);
    this.totalErrors += 1;

    // Check rate limiting
    if (this.shouldLog(errorCode)) {
      const rateInfo = this.errorRateLimit.get(errorCode);
      const suffix = rateInfo && rateInfo.count > this.ERROR_LOG_THRESHOLD
        ? ` (${rateInfo.count} occurrences in last ${this.ERROR_LOG_INTERVAL_MS / 1000}s)`
        : '';

      // The running total rides the line because the line is rate-limited and
      // the count must not be. A soak arm reads the highest total it sees, so
      // one surviving line still reports an accurate figure for a storm that
      // logged a hundredth of its arrivals. Parsed by
      // `scripts/suite-run-signals.mjs` — keep the wording in step with
      // `ERROR_NOTIFICATION_TOTAL_RE` there. Refs TRA-1229.
      logger.error(
        `[CS108 Error] ${description} (0x${errorCode.toString(16).padStart(4, '0')})${suffix}` +
        ` [${this.totalErrors} seen this session]`
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
  private getSeverity(_errorCode: number): 'warning' | 'error' | 'critical' {
    // The spec's four codes are all protocol-level rejections: the device
    // understood the frame well enough to refuse it. The previous branches
    // named HARDWARE_ERROR and FIRMWARE_ERROR, which are not in the spec and
    // which the device never sent in 7.5 hours of capture. Refs TRA-1229.
    return 'warning';
  }

  /** How many `0xA101` frames carried this code. Unaffected by log rate limiting. */
  getErrorCount(errorCode: number): number {
    return this.errorCounts.get(errorCode) ?? 0;
  }

  /** How many `0xA101` frames arrived in total. Unaffected by log rate limiting. */
  getTotalErrorCount(): number {
    return this.totalErrors;
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