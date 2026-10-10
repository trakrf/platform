import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetHistory, fetchAllAssetHistory } from './useAssetHistory';
import { reportsApi } from '@/lib/api/reports';

vi.mock('@/lib/api/reports');

vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

const mockResponse = {
  data: [
    {
      timestamp: '2025-01-27T10:30:00Z',
      location_id: 1,
      location_external_key: 'ROOM-101',
      duration_seconds: null,
    },
  ],
  limit: 1,
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
    vi.mocked(reportsApi.listAssetHistory).mockResolvedValue({
      data: mockResponse,
    } as ReturnType<typeof reportsApi.listAssetHistory>);

    const { result } = renderHook(() => useAssetHistory(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data).toEqual(mockResponse.data);
  });

  it('should not fetch when assetId is null', async () => {
    const { result } = renderHook(() => useAssetHistory(null), {
      wrapper: createWrapper(),
    });

    await new Promise((r) => setTimeout(r, 100));
    expect(reportsApi.listAssetHistory).not.toHaveBeenCalled();
  });

  it('should handle 404 errors', async () => {
    vi.mocked(reportsApi.listAssetHistory).mockRejectedValue(new Error('Not found'));

    const { result } = renderHook(() => useAssetHistory(999), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });
  });

  it('should pass date params to API', async () => {
    vi.mocked(reportsApi.listAssetHistory).mockResolvedValue({
      data: mockResponse,
    } as ReturnType<typeof reportsApi.listAssetHistory>);

    renderHook(
      () =>
        useAssetHistory(1, {
          from: '2025-01-01T00:00:00Z',
          to: '2025-01-27T23:59:59Z',
        }),
      {
        wrapper: createWrapper(),
      }
    );

    await waitFor(() => {
      expect(reportsApi.listAssetHistory).toHaveBeenCalledWith(1, {
        from: '2025-01-01T00:00:00Z',
        to: '2025-01-27T23:59:59Z',
      });
    });
  });
});

const historyRows = (n: number, start = 0) =>
  Array.from({ length: n }, (_, i) => ({
    timestamp: '2025-01-27T10:30:00Z',
    location_id: start + i,
    location_external_key: `LOC-${start + i}`,
    duration_seconds: null,
  }));

const historyPage = (rows: ReturnType<typeof historyRows>, offset: number, total: number) =>
  ({
    data: { data: rows, limit: 200, offset, total_count: total },
  }) as Awaited<ReturnType<typeof reportsApi.listAssetHistory>>;

describe('fetchAllAssetHistory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('pages through every row in the range at the backend page cap', async () => {
    vi.mocked(reportsApi.listAssetHistory)
      .mockResolvedValueOnce(historyPage(historyRows(200), 0, 250))
      .mockResolvedValueOnce(historyPage(historyRows(50, 200), 200, 250));

    const rows = await fetchAllAssetHistory(7, { from: '2025-01-01T00:00:00Z' });

    expect(rows).toHaveLength(250);
    expect(reportsApi.listAssetHistory).toHaveBeenCalledTimes(2);
    expect(reportsApi.listAssetHistory).toHaveBeenNthCalledWith(1, 7, {
      from: '2025-01-01T00:00:00Z',
      limit: 200,
      offset: 0,
    });
    expect(reportsApi.listAssetHistory).toHaveBeenNthCalledWith(2, 7, {
      from: '2025-01-01T00:00:00Z',
      limit: 200,
      offset: 200,
    });
  });

  it('stops on an empty page even when total_count claims more', async () => {
    vi.mocked(reportsApi.listAssetHistory)
      .mockResolvedValueOnce(historyPage(historyRows(200), 0, 500))
      .mockResolvedValueOnce(historyPage([], 200, 500));

    const rows = await fetchAllAssetHistory(7, {});

    expect(rows).toHaveLength(200);
    expect(reportsApi.listAssetHistory).toHaveBeenCalledTimes(2);
  });
});
