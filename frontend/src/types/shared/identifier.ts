/**
 * Tag Identifier Types
 *
 * Types for RFID tag identifiers linked to assets and locations.
 * Matches backend: backend/internal/models/shared/identifier.go
 */

/**
 * Identifier type - currently only RFID supported
 */
export type IdentifierType = 'rfid';

/**
 * Tag identifier entity - returned from API
 * Reference: backend/internal/models/shared/identifier.go TagIdentifier
 */
export interface TagIdentifier {
  id: number;
  type: IdentifierType;
  value: string;
  is_active: boolean;
}
