/**
 * Unified CS108 Packet Payload Types
 *
 * These types define all possible parsed payloads that can result from CS108 packets.
 * The parsers convert rawPayload (Uint8Array) to these typed payloads.
 *
 * This ensures type safety from hardware packets through to UI events.
 */

// Import RFID-specific packet types
import type {
  CompactInventoryPacket,
  NormalInventoryPacket,
  CommandBeginPacket,
  CommandEndPacket,
  InventoryCyclePacket,
  RegisterAccessPacket
} from './rfid/packet-types.js';

// ============================================================================
// Simple Scalar Payloads
// ============================================================================

/**
 * Many CS108 responses are just a single numeric value
 * Examples:
 * - Battery percentage (0-100)
 * - Trigger state (0 = released, 1 = pressed)
 * - Error codes
 * - Status values
 * - Register values
 */
export type ScalarPayload = number;

// ============================================================================
// System Payloads
// ============================================================================

/**
 * Error notification with optional message
 * Used by: ERROR_NOTIFICATION (0xA0FF)
 */
export interface ErrorPayload {
  code: number;
  message?: string;
}

// ============================================================================
// Barcode Payloads
// ============================================================================

/**
 * Barcode scan data
 * Used by: BARCODE_DATA (0x9100)
 */
export interface BarcodeDataPayload {
  symbology: string;     // "Code 128", "Data Matrix", etc.
  data: string;          // The decoded barcode text
  rawData?: Uint8Array;  // Original bytes (optional, for debugging)
}

// ============================================================================
// RFID Tag Payloads (simplified for event emission)
// ============================================================================

/**
 * Simplified tag data for events
 * This is what handlers emit after processing the complex RFID packets
 * Used by: INVENTORY_TAG (0x8100) after processing
 */
export interface TagDataPayload {
  epc: string;        // Hex string EPC (e.g., "E28011700000020818D07547")
  rssi: number;       // Signal strength in dBm (e.g., -65)
  pc?: number;        // Protocol Control word
  phase?: number;     // Phase angle (for locate mode)
  antenna?: number;   // Antenna port number (1-4)
  timestamp?: number; // When the tag was read
}

// ============================================================================
// Command Response Payloads
// ============================================================================

/**
 * Generic command response
 * Used by various command acknowledgments
 */
export interface CommandResponsePayload {
  command: string;
  success: boolean;
  response?: unknown;
}

// ============================================================================
// Complex RFID Payloads (intermediate parsing)
// ============================================================================

/**
 * The full RFID inventory packet payload
 * This is the parsed representation of the inventory data
 * that starts at byte 10 of the CS108 packet
 */
export type RFIDInventoryPayload =
  | CompactInventoryPacket
  | NormalInventoryPacket
  | CommandBeginPacket
  | CommandEndPacket
  | InventoryCyclePacket
  | RegisterAccessPacket;

// ============================================================================
// Discriminated Union for All Payloads
// ============================================================================

/**
 * All possible parsed payload types in CS108 packets
 * This is used as the payload type in CS108Packet
 */
export type CS108PayloadType =
  // Simple types
  | ScalarPayload                // Single number (battery %, trigger state, etc.)
  | ErrorPayload                 // Error code and message
  // Barcode
  | BarcodeDataPayload           // Barcode scan result
  // RFID simple
  | TagDataPayload               // Simplified tag for events
  // RFID complex
  | RFIDInventoryPayload         // Full inventory packet structure
  // Command responses
  | CommandResponsePayload       // Generic command response
  // Special cases
  | null                         // No payload
  | undefined;                   // Not yet parsed

// ============================================================================
// Type Guards
// ============================================================================

export function isScalarPayload(payload: unknown): payload is ScalarPayload {
  return typeof payload === 'number';
}

export function isErrorPayload(payload: unknown): payload is ErrorPayload {
  return typeof payload === 'object' &&
         payload !== null &&
         'code' in payload &&
         typeof (payload as ErrorPayload).code === 'number';
}

export function isBarcodeDataPayload(payload: unknown): payload is BarcodeDataPayload {
  return typeof payload === 'object' &&
         payload !== null &&
         'symbology' in payload &&
         'data' in payload &&
         typeof (payload as BarcodeDataPayload).symbology === 'string' &&
         typeof (payload as BarcodeDataPayload).data === 'string';
}

export function isTagDataPayload(payload: unknown): payload is TagDataPayload {
  return typeof payload === 'object' &&
         payload !== null &&
         'epc' in payload &&
         'rssi' in payload &&
         typeof (payload as TagDataPayload).epc === 'string' &&
         typeof (payload as TagDataPayload).rssi === 'number';
}

export function isRFIDInventoryPayload(payload: unknown): payload is RFIDInventoryPayload {
  return typeof payload === 'object' &&
         payload !== null &&
         'version' in payload &&
         'packetType' in payload;
}

export function isCommandResponsePayload(payload: unknown): payload is CommandResponsePayload {
  return typeof payload === 'object' &&
         payload !== null &&
         'command' in payload &&
         'success' in payload &&
         typeof (payload as CommandResponsePayload).command === 'string' &&
         typeof (payload as CommandResponsePayload).success === 'boolean';
}

// ============================================================================
// Parser Type Signatures
// ============================================================================

/**
 * Type-safe parser function signature
 * All parsers should follow this pattern
 */
export type PayloadParser<T extends CS108PayloadType> = (data: Uint8Array) => T;

// Specific parser types for type safety
export type ScalarParser = PayloadParser<ScalarPayload>;
export type ErrorParser = PayloadParser<ErrorPayload>;
export type BarcodeParser = PayloadParser<BarcodeDataPayload>;
export type TagParser = PayloadParser<TagDataPayload>;
export type InventoryParser = PayloadParser<RFIDInventoryPayload>;