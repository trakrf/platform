import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useLocations } from './useLocations';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

vi.mock('@/lib/api/locations');

// Mock useOrgStore to provide currentOrg for query keys
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: vi.fn((selector) => {
    const state = { currentOrg: { id: 1, name: 'Test Org' } };
    return selector ? selector(state) : state;
  }),
}));

// Raw API shapes returned by the backend (public API response shape)
const apiLocations = [
  {
    surrogate_id: 1,
    identifier: 'usa',
    name: 'United States',
    description: '',
    parent: null,
    path: 'usa',
    depth: 1,
    valid_from: '2024-01-01',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    surrogate_id: 2,
    identifier: 'california',
    name: 'California',
    description: '',
    parent: 'usa',
    path: 'usa.california',
    depth: 2,
    valid_from: '2024-01-01',
    valid_to: null,
    is_active: true,
    metadata: {},
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

// Normalized shape (what the hook stores/returns after normalization)
const mockLocations: Location[] = [
  { ...apiLocations[0], id: 1, parent_location_id: null } as Location,
  { ...apiLocations[1], id: 2, parent_location_id: 1 } as Location,
];

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useLocations', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should fetch and return locations', async () => {
    vi.mocked(locationsApi.list).mockResolvedValue({
      data: { data: apiLocations, total_count: 2, limit: 100, offset: 0 },
    } as any);

    const { result } = renderHook(() => useLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.locations).toEqual(mockLocations);
    expect(result.current.totalCount).toBe(2);
    expect(locationsApi.list).toHaveBeenCalled();
  });

  it('should update store cache on fetch', async () => {
    vi.mocked(locationsApi.list).mockResolvedValue({
      data: { data: apiLocations, total_count: 2, limit: 100, offset: 0 },
    } as any);

    const { result } = renderHook(() => useLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const cachedLocation = useLocationStore.getState().getLocationById(1);
    expect(cachedLocation).toEqual(mockLocations[0]);
  });

  it('should respect enabled option', async () => {
    vi.mocked(locationsApi.list).mockResolvedValue({
      data: { data: apiLocations, total_count: 2, limit: 100, offset: 0 },
    } as any);

    const { result } = renderHook(() => useLocations({ enabled: false }), {
      wrapper: createWrapper(),
    });

    await new Promise((resolve) => setTimeout(resolve, 100));

    expect(result.current.isLoading).toBe(false);
    expect(locationsApi.list).not.toHaveBeenCalled();
  });

  it('should handle errors', async () => {
    const error = new Error('Failed to fetch');
    vi.mocked(locationsApi.list).mockRejectedValue(error);

    const { result } = renderHook(() => useLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });

    expect(result.current.locations).toEqual([]);
  });

  it('should support refetch', async () => {
    vi.mocked(locationsApi.list).mockResolvedValue({
      data: { data: apiLocations, total_count: 2, limit: 100, offset: 0 },
    } as any);

    const { result } = renderHook(() => useLocations(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    vi.clearAllMocks();
    await result.current.refetch();

    expect(locationsApi.list).toHaveBeenCalled();
  });
});
