/**
 * Unit Tests for Location Validators
 *
 * Test pattern reference: frontend/src/lib/asset/filters.test.ts
 */

import { describe, it, expect } from 'vitest';
import {
  validateIdentifier,
  validateName,
  detectCircularReference,
  extractErrorMessage,
} from './validators';
import type { Location } from '@/types/locations';

describe('Validators', () => {
  describe('validateIdentifier()', () => {
    it('should accept valid ltree identifiers', () => {
      expect(validateIdentifier('usa')).toBeNull();
      expect(validateIdentifier('warehouse_1')).toBeNull();
      expect(validateIdentifier('section_a_001')).toBeNull();
      expect(validateIdentifier('a1b2c3')).toBeNull();
    });

    it('should reject empty identifiers', () => {
      expect(validateIdentifier('')).toBe('Identifier is required');
      expect(validateIdentifier('   ')).toBe('Identifier is required');
    });

    it('should reject uppercase letters', () => {
      const error = validateIdentifier('USA');
      expect(error).toContain('lowercase');
    });

    it('should reject hyphens', () => {
      const error = validateIdentifier('warehouse-1');
      expect(error).toContain('lowercase');
    });

    it('should reject spaces', () => {
      const error = validateIdentifier('warehouse 1');
      expect(error).toContain('lowercase');
    });

    it('should reject special characters', () => {
      const error = validateIdentifier('warehouse@1');
      expect(error).toContain('lowercase');
    });

    it('should reject dots (ltree separators)', () => {
      const error = validateIdentifier('usa.california');
      expect(error).toContain('lowercase');
    });

    it('should reject too long identifiers', () => {
      const longId = 'a'.repeat(256);
      expect(validateIdentifier(longId)).toContain('255 characters');
    });

    it('should accept maximum length identifier', () => {
      const maxId = 'a'.repeat(255);
      expect(validateIdentifier(maxId)).toBeNull();
    });
  });

  describe('validateName()', () => {
    it('should accept valid names', () => {
      expect(validateName('Warehouse 1')).toBeNull();
      expect(validateName('USA')).toBeNull();
      expect(validateName('Building A-1')).toBeNull();
    });

    it('should reject empty names', () => {
      expect(validateName('')).toBe('Name is required');
      expect(validateName('   ')).toBe('Name is required');
    });

    it('should reject too long names', () => {
      const longName = 'A'.repeat(256);
      expect(validateName(longName)).toContain('255 characters');
    });

    it('should accept maximum length name', () => {
      const maxName = 'A'.repeat(255);
      expect(validateName(maxName)).toBeNull();
    });
  });

  describe('detectCircularReference()', () => {
    const mockLocations: Location[] = [
      {
        id: 1,
        org_id: 1,
        identifier: 'root',
        name: 'Root',
        description: '',
        parent_location_id: null,
        path: 'root',
        depth: 1,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        metadata: {},
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 2,
        org_id: 1,
        identifier: 'child',
        name: 'Child',
        description: '',
        parent_location_id: 1,
        path: 'root.child',
        depth: 2,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        metadata: {},
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      {
        id: 3,
        org_id: 1,
        identifier: 'grandchild',
        name: 'Grandchild',
        description: '',
        parent_location_id: 2,
        path: 'root.child.grandchild',
        depth: 3,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        metadata: {},
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ];

    it('should detect direct circular reference (self-parent)', () => {
      const isCircular = detectCircularReference(1, 1, mockLocations);
      expect(isCircular).toBe(true);
    });

    it('should detect indirect circular reference (root <- grandchild)', () => {
      // Try to make root (id:1) child of grandchild (id:3)
      const isCircular = detectCircularReference(1, 3, mockLocations);
      expect(isCircular).toBe(true);
    });

    it('should detect circular reference (child <- grandchild)', () => {
      // Try to make child (id:2) child of grandchild (id:3)
      const isCircular = detectCircularReference(2, 3, mockLocations);
      expect(isCircular).toBe(true);
    });

    it('should allow valid parent assignment (new location)', () => {
      // Make new location (id:99) child of grandchild (id:3)
      const isCircular = detectCircularReference(99, 3, mockLocations);
      expect(isCircular).toBe(false);
    });

    it('should allow valid parent assignment (sibling move)', () => {
      // Move child (id:2) to be child of root (id:1) - already is, but valid
      const isCircular = detectCircularReference(2, 1, mockLocations);
      expect(isCircular).toBe(false);
    });

    it('should handle missing location in chain', () => {
      // Try to set parent to non-existent location
      const isCircular = detectCircularReference(2, 999, mockLocations);
      expect(isCircular).toBe(false);
    });
  });

  describe('extractErrorMessage()', () => {
    it('should extract RFC 7807 detail from API error', () => {
      const error = {
        response: {
          data: {
            detail: 'Location not found',
          },
        },
      };
      expect(extractErrorMessage(error)).toBe('Location not found');
    });

    it('should fallback to error.message', () => {
      const error = { message: 'Network error' };
      expect(extractErrorMessage(error)).toBe('Network error');
    });

    it('should handle unknown error format', () => {
      expect(extractErrorMessage({})).toBe('An unknown error occurred');
    });

    it('should handle null/undefined errors', () => {
      expect(extractErrorMessage(null)).toBe('An unknown error occurred');
      expect(extractErrorMessage(undefined)).toBe('An unknown error occurred');
    });

    it('should prefer detail over message', () => {
      const error = {
        response: {
          data: {
            detail: 'Detailed error',
          },
        },
        message: 'Generic error',
      };
      expect(extractErrorMessage(error)).toBe('Detailed error');
    });
  });
});
