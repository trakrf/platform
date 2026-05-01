import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useCurrentLocations } from './useCurrentLocations';
import { reportsApi } from '@/lib/api/reports';
import type { CurrentLocationItem } from '@/types/reports';

vi.mock('@/lib/api/reports');

vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

const mockData: CurrentLocationItem[] = [
  {
    asset_id: 1,
    asset_external_key: 'AST-001',
    location_id: 1,
    location_external_key: 'ROOM-101',
    last_seen: '2025-01-27T10:30:00Z',
  },
];

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useCurrentLocations', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should fetch and return current locations', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockResolvedValue({
      data: { data: mockData, count: 1, offset: 0, total_count: 1 },
    } as ReturnType<typeof reportsApi.getCurrentLocations>);

    const { result } = renderHook(() => useCurrentLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data).toEqual(mockData);
    expect(result.current.totalCount).toBe(1);
  });

  it('should pass params to API', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockResolvedValue({
      data: { data: [], count: 0, offset: 0, total_count: 0 },
    } as ReturnType<typeof reportsApi.getCurrentLocations>);

    renderHook(() => useCurrentLocations({ q: 'test', limit: 10 }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(reportsApi.getCurrentLocations).toHaveBeenCalledWith({
        q: 'test',
        limit: 10,
      });
    });
  });

  it('should handle errors', async () => {
    vi.mocked(reportsApi.getCurrentLocations).mockRejectedValue(new Error('Failed'));

    const { result } = renderHook(() => useCurrentLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });

    expect(result.current.data).toEqual([]);
  });

  it('should respect enabled option', async () => {
    const { result } = renderHook(() => useCurrentLocations({ enabled: false }), {
      wrapper: createWrapper(),
    });

    await new Promise((r) => setTimeout(r, 100));
    expect(reportsApi.getCurrentLocations).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });

  describe('fetchAll', () => {
    function makePage(count: number, startId: number): CurrentLocationItem[] {
      return Array.from({ length: count }, (_, i) => ({
        asset_id: startId + i,
        asset_external_key: `AST-${startId + i}`,
        location_id: 1,
        location_external_key: 'ROOM-101',
        last_seen: '2025-01-27T10:30:00Z',
      }));
    }

    it('pages until total_count is reached and concatenates results', async () => {
      const page1 = makePage(200, 1);
      const page2 = makePage(150, 201);

      vi.mocked(reportsApi.getCurrentLocations)
        .mockResolvedValueOnce({
          data: { data: page1, limit: 200, offset: 0, total_count: 350 },
        } as ReturnType<typeof reportsApi.getCurrentLocations>)
        .mockResolvedValueOnce({
          data: { data: page2, limit: 200, offset: 200, total_count: 350 },
        } as ReturnType<typeof reportsApi.getCurrentLocations>);

      const { result } = renderHook(
        () => useCurrentLocations({ fetchAll: true, q: 'foo' }),
        { wrapper: createWrapper() }
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(reportsApi.getCurrentLocations).toHaveBeenCalledTimes(2);
      expect(reportsApi.getCurrentLocations).toHaveBeenNthCalledWith(1, {
        q: 'foo',
        limit: 200,
        offset: 0,
      });
      expect(reportsApi.getCurrentLocations).toHaveBeenNthCalledWith(2, {
        q: 'foo',
        limit: 200,
        offset: 200,
      });
      expect(result.current.data).toHaveLength(350);
      expect(result.current.totalCount).toBe(350);
    });

    it('stops after a single page when total_count fits in one page', async () => {
      const page1 = makePage(50, 1);

      vi.mocked(reportsApi.getCurrentLocations).mockResolvedValueOnce({
        data: { data: page1, limit: 200, offset: 0, total_count: 50 },
      } as ReturnType<typeof reportsApi.getCurrentLocations>);

      const { result } = renderHook(
        () => useCurrentLocations({ fetchAll: true }),
        { wrapper: createWrapper() }
      );

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(reportsApi.getCurrentLocations).toHaveBeenCalledTimes(1);
      expect(result.current.data).toHaveLength(50);
      expect(result.current.totalCount).toBe(50);
    });
  });
});
