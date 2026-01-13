import { describe, it, expect, vi, beforeEach } from 'vitest';
import { lookupApi } from '.';
import { apiClient } from '../client';
import type { Asset } from '@/types/assets';
import type { Location } from '@/types/locations';

// Mock apiClient
vi.mock('../client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

describe('lookupApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('byTag()', () => {
    it('should call GET /lookup/tag with type and value params', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        current_location_id: null,
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
        identifiers: [],
      };

      const mockResponse = {
        data: {
          entity_type: 'asset',
          entity_id: 1,
          asset: mockAsset,
        },
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await lookupApi.byTag('rfid', 'E2801160600002084D9F34E9');

      expect(apiClient.get).toHaveBeenCalledWith('/lookup/tag', {
        params: { type: 'rfid', value: 'E2801160600002084D9F34E9' },
      });
    });

    it('should return lookup result with asset', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        current_location_id: null,
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
        identifiers: [],
      };

      const mockResponse = {
        data: {
          entity_type: 'asset' as const,
          entity_id: 1,
          asset: mockAsset,
        },
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await lookupApi.byTag('rfid', 'EPC123');

      expect(result.data.data.entity_type).toBe('asset');
      expect(result.data.data.asset?.id).toBe(1);
      expect(result.data.data.asset?.identifier).toBe('LAPTOP-001');
    });

    it('should return lookup result with location', async () => {
      const mockLocation: Location = {
        id: 10,
        org_id: 1,
        identifier: 'ZONE-A',
        name: 'Zone A',
        description: 'Warehouse Zone A',
        parent_location_id: null,
        path: '10',
        depth: 0,
        valid_from: '2024-01-01',
        valid_to: null,
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
        deleted_at: null,
      };

      const mockResponse = {
        data: {
          entity_type: 'location' as const,
          entity_id: 10,
          location: mockLocation,
        },
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await lookupApi.byTag('rfid', 'ZONE-TAG');

      expect(result.data.data.entity_type).toBe('location');
      expect(result.data.data.location?.id).toBe(10);
    });

    it('should propagate 404 errors for not found tags', async () => {
      const mockError = {
        response: {
          status: 404,
          data: { error: { detail: 'Tag not found' } },
        },
      };

      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(
        lookupApi.byTag('rfid', 'UNKNOWN-EPC')
      ).rejects.toMatchObject(mockError);
    });

    it('should propagate network errors', async () => {
      const mockError = new Error('Network error');
      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(lookupApi.byTag('rfid', 'EPC123')).rejects.toThrow(
        'Network error'
      );
    });
  });

  describe('byTags()', () => {
    it('should call POST /lookup/tags with type and values', async () => {
      const mockResponse = {
        data: {},
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      await lookupApi.byTags({
        type: 'rfid',
        values: ['EPC123', 'EPC456', 'EPC789'],
      });

      expect(apiClient.post).toHaveBeenCalledWith('/lookup/tags', {
        type: 'rfid',
        values: ['EPC123', 'EPC456', 'EPC789'],
      });
    });

    it('should return batch lookup results', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        current_location_id: null,
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
        identifiers: [],
      };

      const mockResponse = {
        data: {
          EPC123: {
            entity_type: 'asset' as const,
            entity_id: 1,
            asset: mockAsset,
          },
          EPC456: null, // Not found
        },
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      const result = await lookupApi.byTags({
        type: 'rfid',
        values: ['EPC123', 'EPC456'],
      });

      expect(result.data.data['EPC123']).toBeDefined();
      expect(result.data.data['EPC123']?.entity_type).toBe('asset');
      expect(result.data.data['EPC456']).toBeNull();
    });

    it('should handle empty results for unknown EPCs', async () => {
      const mockResponse = {
        data: {},
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      const result = await lookupApi.byTags({
        type: 'rfid',
        values: ['UNKNOWN1', 'UNKNOWN2'],
      });

      expect(Object.keys(result.data.data)).toHaveLength(0);
    });

    it('should handle mixed results (some found, some not)', async () => {
      const mockAsset1: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'ASSET-1',
        name: 'Asset 1',
        type: 'device',
        description: '',
        current_location_id: null,
        valid_from: '2024-01-01',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
        deleted_at: null,
        identifiers: [],
      };

      const mockAsset2: Asset = {
        id: 2,
        org_id: 1,
        identifier: 'ASSET-2',
        name: 'Asset 2',
        type: 'device',
        description: '',
        current_location_id: null,
        valid_from: '2024-01-01',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
        deleted_at: null,
        identifiers: [],
      };

      const mockResponse = {
        data: {
          EPC1: { entity_type: 'asset' as const, entity_id: 1, asset: mockAsset1 },
          EPC2: null,
          EPC3: { entity_type: 'asset' as const, entity_id: 2, asset: mockAsset2 },
        },
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      const result = await lookupApi.byTags({
        type: 'rfid',
        values: ['EPC1', 'EPC2', 'EPC3'],
      });

      expect(result.data.data['EPC1']?.asset?.identifier).toBe('ASSET-1');
      expect(result.data.data['EPC2']).toBeNull();
      expect(result.data.data['EPC3']?.asset?.identifier).toBe('ASSET-2');
    });

    it('should propagate validation errors for batch size exceeded', async () => {
      const mockError = {
        response: {
          status: 400,
          data: { error: { detail: 'Batch size exceeds maximum of 500' } },
        },
      };

      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      // Create array of 501 EPCs
      const tooManyEpcs = Array.from({ length: 501 }, (_, i) => `EPC${i}`);

      await expect(
        lookupApi.byTags({ type: 'rfid', values: tooManyEpcs })
      ).rejects.toMatchObject(mockError);
    });

    it('should propagate network errors', async () => {
      const mockError = new Error('Network error');
      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      await expect(
        lookupApi.byTags({ type: 'rfid', values: ['EPC1'] })
      ).rejects.toThrow('Network error');
    });
  });
});
