import { describe, it, expect } from 'vitest';
import type { Asset, AssetType, CreateAssetRequest } from './asset';
import { CSV_VALIDATION } from './asset';

describe('Asset Types', () => {
  describe('CSV_VALIDATION constants', () => {
    it('should match backend MaxFileSize constant', () => {
      expect(CSV_VALIDATION.MAX_FILE_SIZE).toBe(5 * 1024 * 1024);
    });

    it('should match backend MaxRows constant', () => {
      expect(CSV_VALIDATION.MAX_ROWS).toBe(1000);
    });

    it('should include all allowed MIME types', () => {
      expect(CSV_VALIDATION.ALLOWED_MIME_TYPES).toEqual([
        'text/csv',
        'application/vnd.ms-excel',
        'application/csv',
        'text/plain',
      ]);
    });

    it('should specify .csv extension', () => {
      expect(CSV_VALIDATION.ALLOWED_EXTENSION).toBe('.csv');
    });
  });

  describe('AssetType union', () => {
    it('should accept valid asset types', () => {
      const validTypes: AssetType[] = [
        'person',
        'device',
        'asset',
        'inventory',
        'other',
      ];

      validTypes.forEach((type) => {
        const asset: Partial<Asset> = { type };
        expect(asset.type).toBe(type);
      });
    });
  });

  describe('Asset interface', () => {
    it('should allow valid asset object', () => {
      const asset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Development laptop',
        valid_from: '2024-01-15',
        valid_to: '2026-12-31',
        metadata: { serial: 'ABC123' },
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      expect(asset.identifier).toBe('LAPTOP-001');
    });

    it('should allow null valid_to', () => {
      const asset: Partial<Asset> = {
        valid_to: null,
      };

      expect(asset.valid_to).toBeNull();
    });
  });

  describe('CreateAssetRequest interface', () => {
    it('should require all mandatory fields', () => {
      const request: CreateAssetRequest = {
        identifier: 'TEST-001',
        name: 'Test Asset',
        type: 'device',
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      expect(request.identifier).toBe('TEST-001');
    });

    it('should allow optional fields', () => {
      const request: CreateAssetRequest = {
        identifier: 'TEST-001',
        name: 'Test Asset',
        type: 'device',
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
        description: 'Optional description',
        metadata: { key: 'value' },
      };

      expect(request.description).toBe('Optional description');
      expect(request.metadata).toEqual({ key: 'value' });
    });
  });
});
