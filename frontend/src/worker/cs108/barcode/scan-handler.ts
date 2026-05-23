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
import type { CS108Packet } from '../type';
import { ReaderMode, ReaderState } from '../../types/reader';
import { postWorkerEvent, WorkerEventType } from '../../types/events';
import { BarcodeAccumulator } from './accumulator';
import { parseBarcodeData } from './parser';

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
  private accumulator = new BarcodeAccumulator();
  private lastBarcode: string | null = null;
  private lastScanTime = 0;
  private scanCount = 0;
  private readonly DUPLICATE_WINDOW_MS = 500;

  /**
   * Accept any 0x9100 packet while in BARCODE mode. We no longer require
   * `packet.payload` to be a pre-parsed object; the accumulator works on
   * `packet.rawPayload` directly.
   */
  canHandle(_packet: CS108Packet, context: NotificationContext): boolean {
    return context.currentMode === ReaderMode.BARCODE;
  }

  /**
   * Feed the 0x9100 raw payload into the accumulator and emit a
   * BARCODE_READ for each complete record returned. Auto-stop fires at
   * most once per call, after the first emit, so it does not interrupt
   * the firmware mid-stream.
   */
  async handle(packet: CS108Packet, context: NotificationContext): Promise<void> {
    const rawPayload = packet.rawPayload;
    if (!rawPayload || rawPayload.length === 0) {
      return;
    }

    const records = this.accumulator.appendAndExtract(rawPayload);
    if (records.length === 0) {
      return;
    }

    let emittedThisCall = false;
    const now = Date.now();

    for (const record of records) {
      const parsed = parseBarcodeData(record);
      if (!parsed.data || parsed.data.trim() === '') {
        continue;
      }

      if (this.isDuplicate(parsed.data, now)) {
        if (context.metadata?.debug) {
          logger.debug('[BarcodeHandler] Ignoring duplicate scan');
        }
        continue;
      }

      this.lastBarcode = parsed.data;
      this.lastScanTime = now;
      this.scanCount++;

      postWorkerEvent({
        type: WorkerEventType.BARCODE_READ,
        payload: {
          barcode: parsed.data,
          symbology: this.normalizeSymbology(parsed.symbology),
          rawData: parsed.rawData
            ? Array.from(parsed.rawData).map(b => b.toString(16).padStart(2, '0')).join('')
            : undefined,
          timestamp: now,
        },
      });

      emittedThisCall = true;
    }

    if (emittedThisCall && context.readerState === ReaderState.SCANNING) {
      logger.debug('[BarcodeHandler] Auto-stop requested after assembled read');
      context.emitNotificationEvent({
        type: WorkerEventType.BARCODE_AUTO_STOP_REQUEST,
        payload: {
          barcode: this.lastBarcode!,
          reason: 'Barcode successfully scanned',
        },
      });
    }
  }

  private isDuplicate(value: string, now: number): boolean {
    if (!this.lastBarcode) return false;
    return value === this.lastBarcode && (now - this.lastScanTime) < this.DUPLICATE_WINDOW_MS;
  }

  /**
   * `parseBarcodeData` already returns a human-readable symbology string
   * (e.g., "QR Code", "Code 128") for recognized AIM IDs. Numeric IDs
   * from older parsers are mapped through SYMBOLOGY_NAMES.
   */
  private normalizeSymbology(symbology: string): string {
    if (typeof symbology === 'string') return symbology;
    return SYMBOLOGY_NAMES[symbology as number] ?? `Unknown (0x${(symbology as number).toString(16)})`;
  }

  getStats(): { scansProcessed: number; lastScanTime: number; lastBarcode: string | null } {
    return {
      scansProcessed: this.scanCount,
      lastScanTime: this.lastScanTime,
      lastBarcode: this.lastBarcode,
    };
  }

  cleanup(): void {
    this.accumulator.reset();
    this.lastBarcode = null;
    this.lastScanTime = 0;
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