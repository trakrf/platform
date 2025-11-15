/**
 * Location Validation Functions
 *
 * Pure validation functions for location data.
 * Return error string on failure, null on success.
 */

import type { Location } from '@/types/locations';

/**
 * Validates identifier is ltree-safe
 * Rule: lowercase alphanumeric + underscores only
 * Reference: PostgreSQL ltree extension requirements
 *
 * @param identifier - Location identifier to validate
 * @returns Error message or null if valid
 */
export function validateIdentifier(identifier: string): string | null {
  if (!identifier || identifier.trim().length === 0) {
    return 'Identifier is required';
  }

  if (identifier.length > 255) {
    return 'Identifier must be 255 characters or less';
  }

  const ltreePattern = /^[a-z0-9_]+$/;
  if (!ltreePattern.test(identifier)) {
    return 'Identifier must be lowercase letters, numbers, and underscores only';
  }

  return null;
}

/**
 * Validates name is not empty and within length limits
 *
 * @param name - Location name to validate
 * @returns Error message or null if valid
 */
export function validateName(name: string): string | null {
  if (!name || name.trim().length === 0) {
    return 'Name is required';
  }

  if (name.length > 255) {
    return 'Name must be 255 characters or less';
  }

  return null;
}

/**
 * Detects circular reference in parent relationship
 * Prevents location from becoming its own ancestor
 *
 * @param locationId - ID of location being modified
 * @param newParentId - ID of proposed new parent
 * @param locations - All locations (for traversal)
 * @returns true if circular reference detected, false otherwise
 */
export function detectCircularReference(
  locationId: number,
  newParentId: number,
  locations: Location[]
): boolean {
  let current = newParentId;
  const visited = new Set<number>([locationId]);

  while (current !== null) {
    if (visited.has(current)) {
      return true; // Circular reference detected
    }

    visited.add(current);
    const location = locations.find((l) => l.id === current);
    if (!location) break;

    current = location.parent_location_id || 0;
    if (current === 0) break;
  }

  return false;
}

/**
 * Extracts error message from API error response
 * Handles RFC 7807 Problem Details format from backend
 *
 * @param err - Error object from API call
 * @returns Human-readable error message
 */
export function extractErrorMessage(err: any): string {
  if (!err) {
    return 'An unknown error occurred';
  }
  if (err.response?.data?.detail) {
    return err.response.data.detail;
  }
  if (err.message) {
    return err.message;
  }
  return 'An unknown error occurred';
}
