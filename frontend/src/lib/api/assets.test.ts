import { describe, it, expect, vi, beforeEach } from 'vitest';
import { assetsApi } from './assets';
import { apiClient } from './client';
import type {
  Asset,
  ListAssetsResponse,
  BulkUploadResponse,
  JobStatusResponse,
} from '@/types/asset';

// Mock apiClient
vi.mock('./client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('assetsApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('list()', () => {
    it('should call GET /assets with no params when options empty', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 0,
        total_count: 0,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list();

      expect(apiClient.get).toHaveBeenCalledWith('/assets');
    });

    it('should call GET /assets with limit param', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 0,
        total_count: 0,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list({ limit: 25 });

      expect(apiClient.get).toHaveBeenCalledWith('/assets?limit=25');
    });

    it('should call GET /assets with both limit and offset params', async () => {
      const mockResponse: ListAssetsResponse = {
        data: [],
        count: 0,
        offset: 50,
        total_count: 100,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.list({ limit: 25, offset: 50 });

      expect(apiClient.get).toHaveBeenCalledWith('/assets?limit=25&offset=50');
    });

    it('should return list response with assets', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: '2026-12-31',
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      const mockResponse: ListAssetsResponse = {
        data: [mockAsset],
        count: 1,
        offset: 0,
        total_count: 1,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await assetsApi.list();

      expect(result.data.data).toHaveLength(1);
      expect(result.data.data[0].identifier).toBe('LAPTOP-001');
    });

    it('should propagate errors unchanged', async () => {
      const mockError = new Error('Network error');
      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(assetsApi.list()).rejects.toThrow('Network error');
    });
  });

  describe('get()', () => {
    it('should call GET /assets/:id', async () => {
      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Dell XPS 15',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: true,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: { data: mockAsset } });

      await assetsApi.get(1);

      expect(apiClient.get).toHaveBeenCalledWith('/assets/1');
    });

    it('should handle 404 errors', async () => {
      const mockError = {
        response: { status: 404, data: { error: 'Not found' } },
      };
      vi.mocked(apiClient.get).mockRejectedValue(mockError);

      await expect(assetsApi.get(999)).rejects.toMatchObject(mockError);
    });
  });

  describe('create()', () => {
    it('should call POST /assets with request data', async () => {
      const requestData = {
        identifier: 'NEW-001',
        name: 'New Asset',
        type: 'device' as const,
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      const mockAsset: Asset = {
        id: 2,
        org_id: 1,
        ...requestData,
        description: '',
        metadata: {},
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T10:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.post).mockResolvedValue({
        data: { data: mockAsset },
      });

      await assetsApi.create(requestData);

      expect(apiClient.post).toHaveBeenCalledWith('/assets', requestData);
    });

    it('should handle validation errors', async () => {
      const requestData = {
        identifier: '', // Invalid - empty
        name: 'Test',
        type: 'device' as const,
        valid_from: '2024-01-01',
        valid_to: '2025-01-01',
        is_active: true,
      };

      const mockError = {
        response: {
          status: 400,
          data: { error: { detail: 'Validation failed: identifier required' } },
        },
      };

      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      await expect(assetsApi.create(requestData)).rejects.toMatchObject(
        mockError
      );
    });
  });

  describe('update()', () => {
    it('should call PUT /assets/:id with partial data', async () => {
      const updateData = {
        name: 'Updated Name',
        is_active: false,
      };

      const mockAsset: Asset = {
        id: 1,
        org_id: 1,
        identifier: 'LAPTOP-001',
        name: 'Updated Name',
        type: 'device',
        description: 'Dev laptop',
        valid_from: '2024-01-15',
        valid_to: null,
        metadata: {},
        is_active: false,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-01-15T11:00:00Z',
        deleted_at: null,
      };

      vi.mocked(apiClient.put).mockResolvedValue({ data: { data: mockAsset } });

      await assetsApi.update(1, updateData);

      expect(apiClient.put).toHaveBeenCalledWith('/assets/1', updateData);
    });

    it('should handle 409 duplicate identifier errors', async () => {
      const mockError = {
        response: {
          status: 409,
          data: { error: { detail: 'Duplicate identifier' } },
        },
      };

      vi.mocked(apiClient.put).mockRejectedValue(mockError);

      await expect(
        assetsApi.update(1, { identifier: 'DUP-001' })
      ).rejects.toMatchObject(mockError);
    });
  });

  describe('delete()', () => {
    it('should call DELETE /assets/:id', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue({ data: { deleted: true } });

      await assetsApi.delete(1);

      expect(apiClient.delete).toHaveBeenCalledWith('/assets/1');
    });

    it('should return deletion status', async () => {
      vi.mocked(apiClient.delete).mockResolvedValue({ data: { deleted: true } });

      const result = await assetsApi.delete(1);

      expect(result.data.deleted).toBe(true);
    });
  });

  describe('uploadCSV()', () => {
    it('should call POST /assets/bulk with FormData', async () => {
      const mockFile = new File(['test'], 'test.csv', { type: 'text/csv' });
      const mockResponse: BulkUploadResponse = {
        status: 'accepted',
        job_id: '123',
        status_url: '/api/v1/assets/bulk/123',
        message: 'Upload accepted',
      };

      vi.mocked(apiClient.post).mockResolvedValue({ data: mockResponse });

      await assetsApi.uploadCSV(mockFile);

      expect(apiClient.post).toHaveBeenCalledWith(
        '/assets/bulk',
        expect.any(FormData),
        { headers: { 'Content-Type': 'multipart/form-data' } }
      );
    });

    it('should handle file too large errors', async () => {
      const mockFile = new File(['test'], 'large.csv', { type: 'text/csv' });
      const mockError = {
        response: {
          status: 413,
          data: { error: { detail: 'File too large' } },
        },
      };

      vi.mocked(apiClient.post).mockRejectedValue(mockError);

      await expect(assetsApi.uploadCSV(mockFile)).rejects.toMatchObject(
        mockError
      );
    });
  });

  describe('getJobStatus()', () => {
    it('should call GET /assets/bulk/:jobId', async () => {
      const mockResponse: JobStatusResponse = {
        job_id: '123',
        status: 'completed',
        total_rows: 10,
        processed_rows: 10,
        failed_rows: 0,
        successful_rows: 10,
        created_at: '2024-01-15T10:00:00Z',
        completed_at: '2024-01-15T10:01:00Z',
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      await assetsApi.getJobStatus('123');

      expect(apiClient.get).toHaveBeenCalledWith('/assets/bulk/123');
    });

    it('should handle job status with errors', async () => {
      const mockResponse: JobStatusResponse = {
        job_id: '123',
        status: 'failed',
        total_rows: 10,
        processed_rows: 0,
        failed_rows: 10,
        created_at: '2024-01-15T10:00:00Z',
        completed_at: '2024-01-15T10:00:05Z',
        errors: [
          { row: 2, field: 'identifier', error: 'Duplicate identifier' },
          { row: 5, error: 'Invalid date format' },
        ],
      };

      vi.mocked(apiClient.get).mockResolvedValue({ data: mockResponse });

      const result = await assetsApi.getJobStatus('123');

      expect(result.data.status).toBe('failed');
      expect(result.data.errors).toHaveLength(2);
    });
  });
});
