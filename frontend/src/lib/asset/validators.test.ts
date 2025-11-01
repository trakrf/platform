import { describe, it, expect } from 'vitest';
import { validateDateRange, validateAssetType } from './validators';

describe('Validators', () => {
  describe('validateDateRange()', () => {
    it('should return null for valid date range', () => {
      expect(validateDateRange('2024-01-15', '2024-12-31')).toBeNull();
      expect(validateDateRange('2024-01-01', '2024-01-02')).toBeNull();
    });

    it('should return error when end date is before start date', () => {
      const error = validateDateRange('2024-12-31', '2024-01-15');
      expect(error).toBe('End date must be after start date');
    });

    it('should return error when end date equals start date', () => {
      const error = validateDateRange('2024-01-15', '2024-01-15');
      expect(error).toBe('End date must be after start date');
    });

    it('should return null when end date is null', () => {
      expect(validateDateRange('2024-01-15', null)).toBeNull();
    });

    it('should return error for invalid date format in start date', () => {
      const error = validateDateRange('invalid-date', '2024-12-31');
      expect(error).toBe('Invalid date format');
    });

    it('should return error for invalid date format in end date', () => {
      const error = validateDateRange('2024-01-15', 'invalid-date');
      expect(error).toBe('Invalid date format');
    });

    it('should handle ISO datetime strings', () => {
      expect(
        validateDateRange('2024-01-15T10:00:00Z', '2024-12-31T23:59:59Z')
      ).toBeNull();
    });
  });

  describe('validateAssetType()', () => {
    it('should return true for valid asset types', () => {
      expect(validateAssetType('person')).toBe(true);
      expect(validateAssetType('device')).toBe(true);
      expect(validateAssetType('asset')).toBe(true);
      expect(validateAssetType('inventory')).toBe(true);
      expect(validateAssetType('other')).toBe(true);
    });

    it('should return false for invalid asset type', () => {
      expect(validateAssetType('computer')).toBe(false);
      expect(validateAssetType('machine')).toBe(false);
      expect(validateAssetType('equipment')).toBe(false);
    });

    it('should be case-sensitive', () => {
      expect(validateAssetType('Device')).toBe(false);
      expect(validateAssetType('PERSON')).toBe(false);
    });

    it('should return false for empty string', () => {
      expect(validateAssetType('')).toBe(false);
    });
  });
});
