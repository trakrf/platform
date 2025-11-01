import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAsset } from './useAsset';
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

describe('useAsset', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should return null for null ID', () => {
    const { result } = renderHook(() => useAsset(null), {
      wrapper: createWrapper(),
    });

    expect(result.current.asset).toBeNull();
    expect(assetsApi.get).not.toHaveBeenCalled();
  });

  it('should return cached asset without API call', () => {
    useAssetStore.getState().addAsset(mockAsset);

    const { result } = renderHook(() => useAsset(mockAsset.id), {
      wrapper: createWrapper(),
    });

    expect(result.current.asset).toEqual(mockAsset);
    expect(assetsApi.get).not.toHaveBeenCalled();
  });

  it('should fetch from API when not cached', async () => {
    vi.mocked(assetsApi.get).mockResolvedValue({
      data: { data: mockAsset },
    } as any);

    const { result } = renderHook(() => useAsset(mockAsset.id), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.asset).toEqual(mockAsset);
    });

    expect(assetsApi.get).toHaveBeenCalledWith(mockAsset.id);
  });

  it('should handle errors', async () => {
    vi.mocked(assetsApi.get).mockRejectedValue(new Error('Not found'));

    const { result } = renderHook(() => useAsset(999), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });
  });
});
