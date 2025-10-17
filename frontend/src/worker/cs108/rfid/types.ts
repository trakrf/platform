/**
 * RFID-specific types
 */

/**
 * Tag data from inventory operations
 */
export interface TagData {
  epc: string;
  rssi: number;
  pc?: number;
  timestamp: number;
  phase?: number;
  antenna?: number;
  wbRssi?: number;  // Wideband RSSI for locate mode
}

/**
 * Parsed RFID tag payload from CS108
 */
export interface ParsedTagPayload {
  epc: string;
  rssi: number;
  pc?: number;
  phase?: number;
  antenna?: number;
}