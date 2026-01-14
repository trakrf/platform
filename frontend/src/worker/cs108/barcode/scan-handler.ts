/**
 * Barcode notification handlers
 *
 * Handles barcode scan notifications (0x9100, 0x9101) from the CS108 device.
 * Processes barcode data and good read confirmations.
 *
 * IMPORTANT: Barcode data emission uses a two-phase approach:
 * 1. BARCODE_DATA (0x9100) - Buffer the data, don't emit yet
 * 2. BARCODE_GOOD_READ (0x9101) - Confirm read, emit buffered data
 *
 * This ensures we have complete barcode data before emitting, avoiding
 * truncation issues caused by BLE fragmentation or timing.
 */

import type {
  NotificationHandler,
  NotificationContext,
} from '../notification/types';
import { logger } from '../../utils/logger.js';
import type { BarcodeData, ParsedBarcodePayload } from './types';
import type { CS108Packet } from '../type';
import { ReaderMode, ReaderState } from '../../types/reader';
import { postWorkerEvent, WorkerEventType } from '../../types/events';

/**
 * Shared barcode buffer between handlers
 * Allows BARCODE_DATA to buffer and BARCODE_GOOD_READ to emit
 */
interface PendingBarcode {
  data: BarcodeData;
  timestamp: number;
}

// Module-level shared buffer for cross-handler communication
let pendingBarcode: PendingBarcode | null = null;
const PENDING_BARCODE_TIMEOUT_MS = 2000; // Clear stale data after 2s

/**
 * Clear any pending barcode data
 * Call this when switching modes or disconnecting
 */
export function clearPendingBarcode(): void {
  if (pendingBarcode) {
    logger.debug('[BarcodeHandler] Clearing pending barcode buffer');
    pendingBarcode = null;
  }
}

/**
 * Barcode symbology mapping
 */
const SYMBOLOGY_NAMES: Record<number, string> = {
  0x01: 'Code 128',
  0x02: 'Code 39',
  0x03: 'Code 93',
  0x04: 'Codabar',
  0x05: 'Interleaved 2 of 5',
  0x06: 'EAN-8',
  0x07: 'EAN-13',
  0x08: 'UPC-A',
  0x09: 'UPC-E',
  0x0A: 'QR Code',
  0x0B: 'Data Matrix',
  0x0C: 'PDF417',
  0x0D: 'Aztec',
  0x0E: 'MaxiCode',
};

/**
 * Handler for barcode data notifications (0x9100)
 *
 * IMPORTANT: This handler BUFFERS data instead of emitting immediately.
 * The BarcodeGoodReadHandler (0x9101) will emit the buffered data.
 * This two-phase approach ensures complete barcode data even with BLE fragmentation.
 */
export class BarcodeDataHandler implements NotificationHandler {
  private lastScanTime = 0;
  private scanCount = 0;
  private readonly DUPLICATE_WINDOW_MS = 500; // Ignore duplicates within 500ms
  private lastBarcode: string | null = null;

  /**
   * Check if we should handle this packet
   * Only handle in BARCODE mode with valid data
   */
  canHandle(packet: CS108Packet, context: NotificationContext): boolean {
    // Must be in barcode mode
    if (context.currentMode !== ReaderMode.BARCODE) {
      return false;
    }

    // Must have barcode data
    if (!packet.payload || typeof packet.payload !== 'object') {
      return false;
    }

    // Check for required fields
    const payload = packet.payload as ParsedBarcodePayload;
    return payload && 'data' in payload && 'symbology' in payload;
  }

  /**
   * Handle barcode data notification
   * Buffers data and waits for BARCODE_GOOD_READ (0x9101) to emit
   */
  async handle(packet: CS108Packet, context: NotificationContext): Promise<void> {
    const payload = packet.payload as ParsedBarcodePayload;
    const now = Date.now();

    // Extract barcode data
    const barcodeData: BarcodeData = {
      data: payload.data,
      symbology: this.getSymbologyName(payload.symbology),
      rawData: payload.rawData,
      timestamp: now,
    };

    // Check for duplicate scan
    if (this.isDuplicate(barcodeData, now)) {
      if (context.metadata?.debug) {
        logger.debug('[BarcodeHandler] Ignoring duplicate scan');
      }
      return;
    }

    // Update tracking
    this.lastBarcode = barcodeData.data;
    this.lastScanTime = now;
    this.scanCount++;

    // Buffer data for BARCODE_GOOD_READ to emit
    // This ensures we have complete data before emitting
    pendingBarcode = {
      data: barcodeData,
      timestamp: now,
    };

    logger.debug(
      `[BarcodeHandler] Buffered barcode: ${barcodeData.data.substring(0, 20)}... ` +
      `(${barcodeData.data.length} chars), waiting for GOOD_READ confirmation`
    );

    // Log in debug mode
    if (context.metadata?.debug) {
      logger.debug(
        `[BarcodeHandler] Scanned: ${barcodeData.data} ` +
        `(${barcodeData.symbology}), Total: ${this.scanCount}`
      );
    }
  }

  /**
   * Check if this is a duplicate scan
   */
  private isDuplicate(barcode: BarcodeData, now: number): boolean {
    if (!this.lastBarcode) {
      return false;
    }

    const timeSinceLastScan = now - this.lastScanTime;
    return barcode.data === this.lastBarcode &&
           timeSinceLastScan < this.DUPLICATE_WINDOW_MS;
  }

  /**
   * Get human-readable symbology name
   */
  private getSymbologyName(symbology: number | string): string {
    if (typeof symbology === 'string') {
      return symbology;
    }

    return SYMBOLOGY_NAMES[symbology] || `Unknown (0x${symbology.toString(16)})`;
  }

  /**
   * Get handler statistics
   */
  getStats(): {
    scansProcessed: number;
    lastScanTime: number;
    lastBarcode: string | null;
  } {
    return {
      scansProcessed: this.scanCount,
      lastScanTime: this.lastScanTime,
      lastBarcode: this.lastBarcode,
    };
  }

  /**
   * Cleanup when handler is unregistered
   */
  cleanup(): void {
    this.lastBarcode = null;
    this.scanCount = 0;
  }
}

/**
 * Handler for barcode good read confirmations (0x9101)
 *
 * This handler is the EMITTER - it takes buffered data from BarcodeDataHandler
 * and emits the complete barcode once we have confirmation.
 *
 * Sequence:
 * 1. BARCODE_DATA (0x9100) buffers data in pendingBarcode
 * 2. BARCODE_GOOD_READ (0x9101) emits the buffered data
 *
 * This ensures complete barcode data even with 20-byte MTU fragmentation.
 */
export class BarcodeGoodReadHandler implements NotificationHandler {
  private confirmationCount = 0;

  /**
   * Check if we should handle this packet
   * Good read notifications don't require specific payload
   */
  canHandle(_packet: CS108Packet, context: NotificationContext): boolean {
    // Only handle in barcode mode
    return context.currentMode === ReaderMode.BARCODE;
  }

  /**
   * Handle good read confirmation
   * Emits the buffered barcode data and triggers auto-stop
   */
  handle(_packet: CS108Packet, context: NotificationContext): void {
    this.confirmationCount++;
    const now = Date.now();

    // Emit good read event for UI feedback
    postWorkerEvent({
      type: WorkerEventType.BARCODE_GOOD_READ,
      payload: {
        confirmationNumber: this.confirmationCount,
      },
    });

    // Check for pending barcode data to emit
    if (pendingBarcode) {
      // Check if data is stale (shouldn't happen in normal flow)
      const age = now - pendingBarcode.timestamp;
      if (age > PENDING_BARCODE_TIMEOUT_MS) {
        logger.warn(
          `[BarcodeHandler] Discarding stale pending barcode (age: ${age}ms)`
        );
        pendingBarcode = null;
        return;
      }

      const barcodeData = pendingBarcode.data;

      logger.debug(
        `[BarcodeHandler] GOOD_READ confirmed - emitting barcode: ` +
        `${barcodeData.data.substring(0, 30)}${barcodeData.data.length > 30 ? '...' : ''} ` +
        `(${barcodeData.data.length} chars)`
      );

      // Emit the complete barcode read event
      postWorkerEvent({
        type: WorkerEventType.BARCODE_READ,
        payload: {
          barcode: barcodeData.data,
          symbology: barcodeData.symbology,
          rawData: barcodeData.rawData
            ? Array.from(barcodeData.rawData).map(b => b.toString(16).padStart(2, '0')).join('')
            : undefined,
          timestamp: barcodeData.timestamp,
        },
      });

      // Auto-stop scanning if we're currently scanning
      if (context.readerState === ReaderState.SCANNING) {
        logger.debug('[BarcodeHandler] Auto-stopping barcode scan after confirmed read');

        context.emitNotificationEvent({
          type: WorkerEventType.BARCODE_AUTO_STOP_REQUEST,
          payload: {
            barcode: barcodeData.data,
            reason: 'Barcode successfully scanned and confirmed'
          },
        });
      }

      // Clear the pending barcode
      pendingBarcode = null;
    } else {
      logger.warn('[BarcodeHandler] GOOD_READ received but no pending barcode data');
    }

    // Log in debug mode
    if (context.metadata?.debug) {
      logger.debug(`[BarcodeHandler] Good read confirmation #${this.confirmationCount}`);
    }
  }

  /**
   * Get handler statistics
   */
  getStats(): {
    confirmationsReceived: number;
  } {
    return {
      confirmationsReceived: this.confirmationCount,
    };
  }

  /**
   * Cleanup when handler is unregistered
   */
  cleanup(): void {
    this.confirmationCount = 0;
    pendingBarcode = null;
  }
}