import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useLocationHierarchy } from './useLocationHierarchy';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

vi.mock('@/lib/api/locations');

const mockRoot: Location = {
  id: 1,
  org_id: 100,
  identifier: 'usa',
  name: 'United States',
  description: '',
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

const mockChild: Location = {
  id: 2,
  org_id: 100,
  identifier: 'california',
  name: 'California',
  description: '',
  parent_location_id: 1,
  path: 'usa.california',
  depth: 2,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockGrandchild: Location = {
  id: 3,
  org_id: 100,
  identifier: 'warehouse1',
  name: 'Warehouse 1',
  description: '',
  parent_location_id: 2,
  path: 'usa.california.warehouse1',
  depth: 3,
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

describe('useLocationHierarchy', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should identify root location', () => {
    useLocationStore.getState().addLocation(mockRoot);

    const { result } = renderHook(() => useLocationHierarchy(1), {
      wrapper: createWrapper(),
    });

    expect(result.current.isRoot).toBe(true);
    expect(result.current.parentLocation).toBeUndefined();
  });

  it('should return parent location from cache', () => {
    useLocationStore.getState().addLocation(mockRoot);
    useLocationStore.getState().addLocation(mockChild);

    const { result } = renderHook(() => useLocationHierarchy(2), {
      wrapper: createWrapper(),
    });

    expect(result.current.isRoot).toBe(false);
    expect(result.current.parentLocation).toEqual(mockRoot);
  });

  it('should return subsidiaries from cache', () => {
    useLocationStore.getState().addLocation(mockRoot);
    useLocationStore.getState().addLocation(mockChild);

    const { result } = renderHook(() => useLocationHierarchy(1), {
      wrapper: createWrapper(),
    });

    expect(result.current.subsidiaries).toEqual([mockChild]);
    expect(result.current.hasSubsidiaries).toBe(true);
  });

  it('should return all subsidiaries (descendants)', () => {
    useLocationStore.getState().addLocation(mockRoot);
    useLocationStore.getState().addLocation(mockChild);
    useLocationStore.getState().addLocation(mockGrandchild);

    const { result } = renderHook(() => useLocationHierarchy(1), {
      wrapper: createWrapper(),
    });

    expect(result.current.allSubsidiaries).toHaveLength(2);
    expect(result.current.allSubsidiaries.map(l => l.id).sort()).toEqual([2, 3]);
  });

  it('should return location path (ancestors)', () => {
    useLocationStore.getState().addLocation(mockRoot);
    useLocationStore.getState().addLocation(mockChild);
    useLocationStore.getState().addLocation(mockGrandchild);

    const { result } = renderHook(() => useLocationHierarchy(3), {
      wrapper: createWrapper(),
    });

    expect(result.current.locationPath).toEqual([mockRoot, mockChild]);
  });

  it('should fetch parents from API when not in cache', async () => {
    vi.mocked(locationsApi.getAncestors).mockResolvedValue({
      data: { data: [mockRoot], total_count: 1, count: 1, offset: 0 },
    } as any);

    useLocationStore.getState().addLocation(mockChild);

    const { result } = renderHook(() => useLocationHierarchy(2), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoadingParents).toBe(false);
    });

    expect(locationsApi.getAncestors).toHaveBeenCalledWith(2);
  });

  it('should fetch subsidiaries from API when not in cache', async () => {
    vi.mocked(locationsApi.getDescendants).mockResolvedValue({
      data: { data: [mockChild], total_count: 1, count: 1, offset: 0 },
    } as any);

    useLocationStore.getState().addLocation(mockRoot);

    const { result } = renderHook(() => useLocationHierarchy(1), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoadingSubsidiaries).toBe(false);
    });

    expect(locationsApi.getDescendants).toHaveBeenCalledWith(1);
  });

  it('should return empty arrays for null locationId', () => {
    const { result } = renderHook(() => useLocationHierarchy(null), {
      wrapper: createWrapper(),
    });

    expect(result.current.subsidiaries).toEqual([]);
    expect(result.current.allSubsidiaries).toEqual([]);
    expect(result.current.locationPath).toEqual([]);
    expect(result.current.isRoot).toBe(false);
    expect(result.current.hasSubsidiaries).toBe(false);
  });

  it('should support refetch operations', async () => {
    vi.mocked(locationsApi.getAncestors).mockResolvedValue({
      data: { data: [mockRoot], total_count: 1, count: 1, offset: 0 },
    } as any);
    vi.mocked(locationsApi.getDescendants).mockResolvedValue({
      data: { data: [mockChild], total_count: 1, count: 1, offset: 0 },
    } as any);

    useLocationStore.getState().addLocation(mockChild);

    const { result } = renderHook(() => useLocationHierarchy(2), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoadingParents).toBe(false);
    });

    expect(typeof result.current.fetchParents).toBe('function');
    expect(typeof result.current.fetchSubsidiaries).toBe('function');
  });
});
