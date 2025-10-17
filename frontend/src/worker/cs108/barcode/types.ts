/**
 * Barcode-specific types
 */

/**
 * Barcode scan data
 */
export interface BarcodeData {
  symbology: string;
  data: string;
  rawData?: Uint8Array;
  timestamp: number;
}

/**
 * Parsed barcode payload from CS108
 */
export interface ParsedBarcodePayload {
  symbology: string | number;  // Can be string name or numeric code
  data: string;
  rawData?: Uint8Array;
}