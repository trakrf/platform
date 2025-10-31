import { describe, it, expect } from 'vitest';
import {
  createAssetCSVFormData,
  validateCSVFile,
  extractErrorMessage,
} from './helpers';
import { CSV_VALIDATION } from '@/types/assets';

describe('Asset Helpers', () => {
  describe('createAssetCSVFormData()', () => {
    it('should create FormData with file', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const formData = createAssetCSVFormData(file);

      expect(formData).toBeInstanceOf(FormData);
      expect(formData.get('file')).toBe(file);
    });

    it('should use correct field name "file"', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const formData = createAssetCSVFormData(file);

      expect(formData.has('file')).toBe(true);
    });
  });

  describe('validateCSVFile()', () => {
    it('should return null for valid file', () => {
      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      const error = validateCSVFile(file);

      expect(error).toBeNull();
    });

    it('should reject file larger than 5MB', () => {
      const largeContent = new Array(6 * 1024 * 1024).fill('a').join('');
      const file = new File([largeContent], 'large.csv', {
        type: 'text/csv',
      });
      const error = validateCSVFile(file);

      expect(error).toContain('5MB');
      // Error shows formatted size in MB, not raw bytes
      expect(error).toMatch(/current: \d+\.\d+MB/);
    });

    it('should reject non-CSV extension', () => {
      const file = new File(['test'], 'test.txt', { type: 'text/plain' });
      const error = validateCSVFile(file);

      expect(error).toContain('.csv');
    });

    it('should accept .CSV extension (case insensitive)', () => {
      const file = new File(['test'], 'TEST.CSV', { type: 'text/csv' });
      const error = validateCSVFile(file);

      expect(error).toBeNull();
    });

    it('should reject invalid MIME type', () => {
      const file = new File(['test'], 'test.csv', {
        type: 'application/pdf',
      });
      const error = validateCSVFile(file);

      expect(error).toContain('Invalid file type');
      expect(error).toContain('application/pdf');
    });

    it('should accept all allowed MIME types', () => {
      CSV_VALIDATION.ALLOWED_MIME_TYPES.forEach((mimeType) => {
        const file = new File(['test'], 'test.csv', { type: mimeType });
        const error = validateCSVFile(file);
        expect(error).toBeNull();
      });
    });

    it('should handle missing MIME type (browser quirk)', () => {
      const file = new File(['test'], 'test.csv', { type: '' });
      const error = validateCSVFile(file);

      // Should not fail on missing MIME type - backend will validate
      expect(error).toBeNull();
    });
  });

  describe('extractErrorMessage()', () => {
    it('should extract RFC 7807 detail field', () => {
      const error = {
        response: {
          data: {
            error: {
              detail: 'Validation failed: identifier required',
            },
          },
        },
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Validation failed: identifier required');
    });

    it('should extract RFC 7807 title field when detail missing', () => {
      const error = {
        response: {
          data: {
            error: {
              title: 'Bad Request',
            },
          },
        },
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Bad Request');
    });

    it('should extract flat error string', () => {
      const error = {
        response: {
          data: {
            error: 'Network timeout',
          },
        },
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Network timeout');
    });

    it('should fallback to error.message', () => {
      const error = {
        message: 'Connection refused',
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Connection refused');
    });

    it('should use default message when no extraction possible', () => {
      const error = {};

      const message = extractErrorMessage(error);

      expect(message).toBe('An error occurred');
    });

    it('should use custom default message', () => {
      const error = {};

      const message = extractErrorMessage(error, 'Custom failure message');

      expect(message).toBe('Custom failure message');
    });

    it('should handle flat RFC 7807 detail field (not nested in error)', () => {
      const error = {
        response: {
          data: {
            detail: 'Resource not found',
            title: 'Not Found',
          },
        },
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Resource not found');
    });

    it('should ignore empty string values', () => {
      const error = {
        response: {
          data: {
            error: {
              detail: '   ',
              title: 'Valid Title',
            },
          },
        },
      };

      const message = extractErrorMessage(error);

      expect(message).toBe('Valid Title');
    });
  });
});
