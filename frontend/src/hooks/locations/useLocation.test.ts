import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useLocation } from './useLocation';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

vi.mock('@/lib/api/locations');

const mockLocation: Location = {
  id: 1,
  org_id: 100,
  identifier: 'usa',
  name: 'United States',
  description: 'Main country location',
  parent_location_id: null,
  path: 'usa',
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useLocation', () => {
  beforeEach(() => {
    useLocationStore.getState().clearCache();
    vi.clearAllMocks();
  });

  it('should fetch and return location', async () => {
    vi.mocked(locationsApi.get).mockResolvedValue({
      data: { data: mockLocation },
    } as any);

    const { result } = renderHook(() => useLocation(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.location).toEqual(mockLocation);
    expect(locationsApi.get).toHaveBeenCalledWith(1);
  });

  it('should return cached location without fetching', async () => {
    useLocationStore.getState().addLocation(mockLocation);

    const { result } = renderHook(() => useLocation(1), {
      wrapper: createWrapper(),
    });

    expect(result.current.location).toEqual(mockLocation);
    expect(locationsApi.get).not.toHaveBeenCalled();
  });

  it('should return null for null id', async () => {
    vi.clearAllMocks();

    const { result } = renderHook(() => useLocation(null), {
      wrapper: createWrapper(),
    });

    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(result.current.location).toBeNull();
    expect(locationsApi.get).not.toHaveBeenCalled();
  });

  it('should respect enabled option', async () => {
    vi.mocked(locationsApi.get).mockResolvedValue({
      data: { data: mockLocation },
    } as any);

    const { result } = renderHook(() => useLocation(1, { enabled: false }), {
      wrapper: createWrapper(),
    });

    await new Promise((resolve) => setTimeout(resolve, 100));

    expect(locationsApi.get).not.toHaveBeenCalled();
  });

  it('should handle errors', async () => {
    const error = new Error('Location not found');
    vi.mocked(locationsApi.get).mockRejectedValue(error);

    const { result } = renderHook(() => useLocation(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.error).toBeTruthy();
    });

    expect(result.current.location).toBeNull();
  });

  it('should support refetch', async () => {
    vi.mocked(locationsApi.get).mockResolvedValue({
      data: { data: mockLocation },
    } as any);

    const { result } = renderHook(() => useLocation(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    vi.clearAllMocks();
    await result.current.refetch();

    expect(locationsApi.get).toHaveBeenCalledWith(1);
  });
});
