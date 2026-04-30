/**
 * Tag Types
 *
 * Types for physical-tag entities (RFID, BLE, NFC, barcode) linked to assets and locations.
 * Matches backend: backend/internal/models/shared/tag.go
 */

/**
 * Tag type — supported physical-tag technologies
 */
export type TagType = 'rfid' | 'ble' | 'nfc' | 'barcode';

/**
 * Tag entity — returned from API
 * Reference: backend/internal/models/shared/tag.go
 */
export interface Tag {
  id: number;
  tag_type: TagType;
  value: string;
  is_active: boolean;
}
