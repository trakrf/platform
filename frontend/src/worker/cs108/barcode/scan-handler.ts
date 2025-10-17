/**
 * Barcode notification handlers
 *
 * Handles barcode scan notifications (0x9100, 0x9101) from the CS108 device.
 * Processes barcode data and good read confirmations.
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
   * Formats and emits barcode scan event
   * Auto-stops scanning and provides vibrator feedback
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

    // Emit barcode read event
    postWorkerEvent({
      type: WorkerEventType.BARCODE_READ,
      payload: {
        barcode: barcodeData.data,
        symbology: barcodeData.symbology,
        rawData: barcodeData.rawData ? Array.from(barcodeData.rawData).map(b => b.toString(16).padStart(2, '0')).join('') : undefined,
        timestamp: barcodeData.timestamp,
      },
    });

    // Auto-stop scanning if we're currently scanning
    if (context.readerState === ReaderState.SCANNING) {
      logger.debug('[BarcodeHandler] Auto-stopping barcode scan after successful read');

      // Emit auto-stop request through the callback so it can be intercepted
      context.emitNotificationEvent({
        type: WorkerEventType.BARCODE_AUTO_STOP_REQUEST,
        payload: {
          barcode: barcodeData.data,
          reason: 'Barcode successfully scanned'
        },
      });
    }

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
 * This is a simple notification that the scan was successful
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
   * Emits feedback event for UI/audio/haptic response
   */
  handle(_packet: CS108Packet, context: NotificationContext): void {
    this.confirmationCount++;

    // Emit good read event for UI feedback
    postWorkerEvent({
      type: WorkerEventType.BARCODE_GOOD_READ,
      payload: {
        confirmationNumber: this.confirmationCount,
      },
    });

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
  }
}