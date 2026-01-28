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
    asset_name: 'Projector A1',
    asset_identifier: 'AST-001',
    location_id: 1,
    location_name: 'Room 101',
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

    renderHook(() => useCurrentLocations({ search: 'test', limit: 10 }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(reportsApi.getCurrentLocations).toHaveBeenCalledWith({
        search: 'test',
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
});
