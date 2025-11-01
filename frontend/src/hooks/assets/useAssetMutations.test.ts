import React, { type ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetMutations } from './useAssetMutations';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import type { Asset, CreateAssetRequest } from '@/types/assets';

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

describe('useAssetMutations', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should create asset', async () => {
    vi.mocked(assetsApi.create).mockResolvedValue({
      data: { data: mockAsset },
    } as any);

    const { result } = renderHook(() => useAssetMutations(), {
      wrapper: createWrapper(),
    });

    const createData: CreateAssetRequest = {
      identifier: 'LAP-001',
      name: 'Test Laptop',
      type: 'device',
      valid_from: '2024-01-01',
      valid_to: '2025-01-01',
      is_active: true,
    };

    await result.current.create(createData);

    expect(assetsApi.create).toHaveBeenCalledWith(createData);
  });

  it('should update asset', async () => {
    const updated = { ...mockAsset, name: 'Updated' };
    vi.mocked(assetsApi.update).mockResolvedValue({
      data: { data: updated },
    } as any);

    const { result } = renderHook(() => useAssetMutations(), {
      wrapper: createWrapper(),
    });

    await result.current.update({ id: 1, updates: { name: 'Updated' } });

    expect(assetsApi.update).toHaveBeenCalledWith(1, { name: 'Updated' });
  });

  it('should delete asset', async () => {
    vi.mocked(assetsApi.delete).mockResolvedValue({
      data: { deleted: true },
    } as any);

    const { result } = renderHook(() => useAssetMutations(), {
      wrapper: createWrapper(),
    });

    await result.current.delete(1);

    expect(assetsApi.delete).toHaveBeenCalledWith(1);
  });
});
