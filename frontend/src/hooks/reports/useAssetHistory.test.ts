import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetHistory } from './useAssetHistory';
import { reportsApi } from '@/lib/api/reports';

vi.mock('@/lib/api/reports');

vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

const mockResponse = {
  asset: { id: 1, name: 'Projector A1', identifier: 'AST-001' },
  data: [
    {
      timestamp: '2025-01-27T10:30:00Z',
      location_id: 1,
      location_name: 'Room 101',
      duration_seconds: null,
    },
  ],
  count: 1,
  offset: 0,
  total_count: 1,
};

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useAssetHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch asset history', async () => {
    vi.mocked(reportsApi.getAssetHistory).mockResolvedValue({
      data: mockResponse,
    } as ReturnType<typeof reportsApi.getAssetHistory>);

    const { result } = renderHook(() => useAssetHistory(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.asset).toEqual(mockResponse.asset);
    expect(result.current.data).toEqual(mockResponse.data);
  });

  it('should not fetch when assetId is null', async () => {
    const { result } = renderHook(() => useAssetHistory(null), {
      wrapper: createWrapper(),
    });

    await new Promise((r) => setTimeout(r, 100));
    expect(reportsApi.getAssetHistory).not.toHaveBeenCalled();
    expect(result.current.asset).toBeNull();
  });

  it('should handle 404 errors', async () => {
    vi.mocked(reportsApi.getAssetHistory).mockRejectedValue(new Error('Not found'));

    const { result } = renderHook(() => useAssetHistory(999), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });
  });

  it('should pass date params to API', async () => {
    vi.mocked(reportsApi.getAssetHistory).mockResolvedValue({
      data: mockResponse,
    } as ReturnType<typeof reportsApi.getAssetHistory>);

    renderHook(
      () =>
        useAssetHistory(1, {
          start_date: '2025-01-01T00:00:00Z',
          end_date: '2025-01-27T23:59:59Z',
        }),
      {
        wrapper: createWrapper(),
      }
    );

    await waitFor(() => {
      expect(reportsApi.getAssetHistory).toHaveBeenCalledWith(1, {
        start_date: '2025-01-01T00:00:00Z',
        end_date: '2025-01-27T23:59:59Z',
      });
    });
  });
});
