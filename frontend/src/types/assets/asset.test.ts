import { describe, it, expect } from 'vitest';
import type { Asset, AssetType, CreateAssetRequest } from '.';
import { CSV_VALIDATION } from '.';

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
        'item',
        'person',
        'inventory',
      ];

      validTypes.forEach((asset_type) => {
        const asset: Partial<Asset> = { asset_type };
        expect(asset.asset_type).toBe(asset_type);
      });
    });
  });

  describe('Asset interface', () => {
    it('should allow valid asset object', () => {
      const asset: Asset = {
        id: 1,
        surrogate_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        asset_type: 'item',
        description: 'Development laptop',
        valid_from: '2024-01-15',
        valid_to: '2026-12-31',
        metadata: { serial: 'ABC123' },
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        tags: [],
      };

      expect(asset.identifier).toBe('LAPTOP-001');
      expect(asset.asset_type).toBe('item');
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
        asset_type: 'item',
      };

      expect(request.identifier).toBe('TEST-001');
    });

    it('should allow optional fields', () => {
      const request: CreateAssetRequest = {
        identifier: 'TEST-001',
        name: 'Test Asset',
        asset_type: 'item',
        description: 'Optional description',
        metadata: { key: 'value' },
      };

      expect(request.description).toBe('Optional description');
      expect(request.metadata).toEqual({ key: 'value' });
    });
  });
});
