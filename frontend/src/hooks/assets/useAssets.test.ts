import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssets } from './useAssets';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

vi.mock('@/lib/api/assets');

// Mock useOrgStore to provide currentOrg for query keys and getState()
const mockOrgState = { currentOrg: { id: 1, name: 'Test Org' } };
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: Object.assign(
    vi.fn((selector) => (selector ? selector(mockOrgState) : mockOrgState)),
    { getState: () => mockOrgState }
  ),
}));

const mockAsset: Asset = {
  id: 1,
  org_id: 100,
  external_key: 'LAP-001',
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

describe('useAssets', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should fetch and return assets with pagination params', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue({
      data: { data: [mockAsset], count: 1, offset: 0, total_count: 1 },
    } as any);

    const { result } = renderHook(() => useAssets(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.assets).toHaveLength(1);
    expect(result.current.totalCount).toBe(1);
    expect(assetsApi.list).toHaveBeenCalledWith(
      expect.objectContaining({
        limit: 100,
        offset: 0,
      })
    );
  });

  it('fetches every page until the total count is reached', async () => {
    // TRA-1098: the screen renders the whole cached set, so a single page
    // leaves the table, the result count, the stat tiles and every export
    // silently short.
    const page = (from: number, size: number) =>
      Array.from({ length: size }, (_, i) => ({
        ...mockAsset,
        id: from + i,
        external_key: `LAP-${from + i}`,
      }));

    vi.mocked(assetsApi.list).mockImplementation((params: any) =>
      Promise.resolve({
        data: {
          data: params.offset === 0 ? page(1, 100) : page(101, 20),
          count: params.offset === 0 ? 100 : 20,
          offset: params.offset,
          total_count: 120,
        },
      } as any)
    );

    const { result } = renderHook(() => useAssets(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.assets).toHaveLength(120);
    expect(result.current.totalCount).toBe(120);
    expect(useAssetStore.getState().getFilteredAssets()).toHaveLength(120);
    expect(assetsApi.list).toHaveBeenCalledTimes(2);
  });

  it('stops paging when a page comes back empty', async () => {
    // A total_count the pages cannot satisfy — rows deleted mid-walk — must
    // not spin forever.
    vi.mocked(assetsApi.list).mockImplementation((params: any) =>
      Promise.resolve({
        data: {
          data: params.offset === 0 ? [mockAsset] : [],
          count: params.offset === 0 ? 1 : 0,
          offset: params.offset,
          total_count: 500,
        },
      } as any)
    );

    const { result } = renderHook(() => useAssets(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.assets).toHaveLength(1);
    expect(assetsApi.list).toHaveBeenCalledTimes(2);
  });

  it('replaces cached assets the list response no longer contains', async () => {
    // TRA-1070: a stale entry — left by a previous org, a re-seeded local
    // database, or an asset deleted elsewhere — must not survive a list fetch.
    // It renders as a duplicate row because it carries the same external_key
    // and name as the asset the server did return, under a different id.
    useAssetStore
      .getState()
      .addAssets([{ ...mockAsset, id: 99 }]);

    vi.mocked(assetsApi.list).mockResolvedValue({
      data: { data: [mockAsset], count: 1, offset: 0, total_count: 1 },
    } as any);

    const { result } = renderHook(() => useAssets(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(
      useAssetStore
        .getState()
        .getFilteredAssets()
        .map((asset) => asset.id)
    ).toEqual([1]);
  });

  it('does not duplicate assets when the query function runs twice', async () => {
    // React.StrictMode double-invokes effects in development. Two identical
    // list responses must leave the cache holding one copy of each asset.
    vi.mocked(assetsApi.list).mockResolvedValue({
      data: { data: [mockAsset], count: 1, offset: 0, total_count: 1 },
    } as any);

    const { result, rerender } = renderHook(() => useAssets(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await result.current.refetch();
    rerender();

    expect(useAssetStore.getState().getFilteredAssets()).toHaveLength(1);
  });

  it('should not fetch when enabled is false', () => {
    const { result } = renderHook(() => useAssets({ enabled: false }), {
      wrapper: createWrapper(),
    });

    expect(assetsApi.list).not.toHaveBeenCalled();
    expect(result.current.assets).toEqual([]);
  });

  it('should handle errors', async () => {
    vi.mocked(assetsApi.list).mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useAssets(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });
  });
});
