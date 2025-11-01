import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssets } from '../useAssets';
import { useAsset } from '../useAsset';
import { useAssetMutations } from '../useAssetMutations';
import { useBulkUpload } from '../useBulkUpload';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

vi.mock('@/lib/api/assets');

const mockAsset: Asset = {
  id: 1,
  org_id: 100,
  identifier: 'LAP-001',
  name: 'Test Laptop',
  type: 'device',
  description: 'Test device',
  valid_from: '2024-01-01T00:00:00Z',
  valid_to: null,
  metadata: {},
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  deleted_at: null,
};

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('Asset Hooks Integration', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  describe('Full CRUD Flow', () => {
    it('should create, read, update, and delete asset', async () => {
      const wrapper = createWrapper();

      vi.mocked(assetsApi.list).mockResolvedValue({
        data: { data: [], count: 0, offset: 0, total_count: 0 },
      } as any);

      const { result: listResult } = renderHook(() => useAssets(), { wrapper });

      await waitFor(() => {
        expect(listResult.current.isLoading).toBe(false);
      });

      expect(listResult.current.assets).toHaveLength(0);

      vi.mocked(assetsApi.create).mockResolvedValue({
        data: { data: mockAsset },
      } as any);

      const { result: mutationsResult } = renderHook(() => useAssetMutations(), { wrapper });

      await mutationsResult.current.create({
        identifier: 'LAP-001',
        name: 'Test Laptop',
        type: 'device',
        valid_from: '2024-01-01',
        is_active: true,
      });

      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toEqual(mockAsset);

      const updatedAsset = { ...mockAsset, name: 'Updated Laptop' };
      vi.mocked(assetsApi.update).mockResolvedValue({
        data: { data: updatedAsset },
      } as any);

      await mutationsResult.current.update({
        id: mockAsset.id,
        updates: { name: 'Updated Laptop' },
      });

      expect(useAssetStore.getState().getAssetById(mockAsset.id)?.name).toBe('Updated Laptop');

      vi.mocked(assetsApi.delete).mockResolvedValue({
        data: { deleted: true },
      } as any);

      await mutationsResult.current.delete(mockAsset.id);

      expect(useAssetStore.getState().getAssetById(mockAsset.id)).toBeUndefined();
    });
  });

  describe('Cache Synchronization', () => {
    it('should share cache between useAssets and useAsset', async () => {
      const wrapper = createWrapper();

      vi.mocked(assetsApi.list).mockResolvedValue({
        data: { data: [mockAsset], count: 1, offset: 0, total_count: 1 },
      } as any);

      const { result: listResult } = renderHook(() => useAssets(), { wrapper });

      await waitFor(() => {
        expect(listResult.current.isLoading).toBe(false);
      });

      expect(listResult.current.assets).toHaveLength(1);

      const { result: singleResult } = renderHook(() => useAsset(mockAsset.id), { wrapper });

      expect(singleResult.current.asset).toEqual(mockAsset);
      expect(assetsApi.get).not.toHaveBeenCalled();
    });

    it('should update cache when mutation succeeds', async () => {
      const wrapper = createWrapper();

      useAssetStore.getState().addAsset(mockAsset);

      const { result: mutationsResult } = renderHook(() => useAssetMutations(), { wrapper });

      const updatedAsset = { ...mockAsset, name: 'Updated Name' };
      vi.mocked(assetsApi.update).mockResolvedValue({
        data: { data: updatedAsset },
      } as any);

      await mutationsResult.current.update({
        id: mockAsset.id,
        updates: { name: 'Updated Name' },
      });

      expect(useAssetStore.getState().getAssetById(mockAsset.id)?.name).toBe('Updated Name');

      const { result: assetResult } = renderHook(() => useAsset(mockAsset.id), { wrapper });
      expect(assetResult.current.asset?.name).toBe('Updated Name');
    });
  });

  describe('Error Handling', () => {
    it('should handle API errors without corrupting cache', async () => {
      const wrapper = createWrapper();

      useAssetStore.getState().addAsset(mockAsset);

      const { result: mutationsResult } = renderHook(() => useAssetMutations(), { wrapper });

      vi.mocked(assetsApi.update).mockRejectedValue(new Error('Network error'));

      await expect(
        mutationsResult.current.update({
          id: mockAsset.id,
          updates: { name: 'Should Fail' },
        })
      ).rejects.toThrow('Network error');

      expect(useAssetStore.getState().getAssetById(mockAsset.id)?.name).toBe('Test Laptop');
    });

    it('should propagate errors from useAssets', async () => {
      const wrapper = createWrapper();

      vi.mocked(assetsApi.list).mockRejectedValue(new Error('Server error'));

      const { result } = renderHook(() => useAssets(), { wrapper });

      await waitFor(() => {
        expect(result.current.error).toBeTruthy();
      });

      expect(result.current.error?.message).toBe('Server error');
      expect(result.current.assets).toHaveLength(0);
    });
  });

  describe('Bulk Upload Flow', () => {
    it('should handle CSV upload and polling to completion', async () => {
      const wrapper = createWrapper();

      vi.mocked(assetsApi.uploadCSV).mockResolvedValue({
        data: {
          status: 'accepted',
          job_id: 'job123',
          status_url: '/api/v1/assets/bulk/job123',
          message: 'Upload accepted',
        },
      } as any);

      vi.mocked(assetsApi.getJobStatus)
        .mockResolvedValueOnce({
          data: {
            job_id: 'job123',
            status: 'processing',
            total_rows: 100,
            processed_rows: 50,
            failed_rows: 0,
            created_at: '2024-01-01',
          },
        } as any)
        .mockResolvedValueOnce({
          data: {
            job_id: 'job123',
            status: 'completed',
            total_rows: 100,
            processed_rows: 100,
            failed_rows: 0,
            created_at: '2024-01-01',
          },
        } as any);

      const { result: uploadResult } = renderHook(() => useBulkUpload(), { wrapper });

      const file = new File(['test'], 'test.csv', { type: 'text/csv' });
      await uploadResult.current.upload(file);

      await waitFor(
        () => {
          expect(uploadResult.current.jobStatus).not.toBeNull();
        },
        { timeout: 3000 }
      );

      await waitFor(
        () => {
          expect(uploadResult.current.jobStatus).toBe('completed');
        },
        { timeout: 5000 }
      );

      expect(useAssetStore.getState().cache.lastFetch).toBeFalsy();
    });
  });

  describe('Multiple Hooks Coordination', () => {
    it('should coordinate updates across multiple hook instances', async () => {
      const wrapper = createWrapper();

      useAssetStore.getState().addAsset(mockAsset);

      const { result: assetResult1 } = renderHook(() => useAsset(mockAsset.id), { wrapper });
      const { result: assetResult2 } = renderHook(() => useAsset(mockAsset.id), { wrapper });
      const { result: mutationsResult } = renderHook(() => useAssetMutations(), { wrapper });

      expect(assetResult1.current.asset?.name).toBe('Test Laptop');
      expect(assetResult2.current.asset?.name).toBe('Test Laptop');

      const updatedAsset = { ...mockAsset, name: 'Coordinated Update' };
      vi.mocked(assetsApi.update).mockResolvedValue({
        data: { data: updatedAsset },
      } as any);

      await mutationsResult.current.update({
        id: mockAsset.id,
        updates: { name: 'Coordinated Update' },
      });

      await waitFor(() => {
        expect(assetResult1.current.asset?.name).toBe('Coordinated Update');
        expect(assetResult2.current.asset?.name).toBe('Coordinated Update');
      });
    });
  });
});
