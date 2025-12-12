import React, { type ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useLocationMutations } from './useLocationMutations';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location, CreateLocationRequest } from '@/types/locations';

vi.mock('@/lib/api/locations');

const mockLocation: Location = {
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

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useLocationMutations', () => {
  beforeEach(() => {
    useLocationStore.getState().invalidateCache();
    vi.clearAllMocks();
  });

  it('should create location', async () => {
    vi.mocked(locationsApi.create).mockResolvedValue({
      data: { data: mockLocation },
    } as any);

    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    const createData: CreateLocationRequest = {
      identifier: 'usa',
      name: 'United States',
      description: '',
    };

    await result.current.create(createData);

    expect(locationsApi.create).toHaveBeenCalledWith(createData);
    const cached = useLocationStore.getState().getLocationById(1);
    expect(cached).toEqual(mockLocation);
  });

  it('should update location', async () => {
    const updated = { ...mockLocation, name: 'Updated Name' };
    vi.mocked(locationsApi.update).mockResolvedValue({
      data: { data: updated },
    } as any);

    useLocationStore.getState().addLocation(mockLocation);

    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    await result.current.update({ id: 1, updates: { name: 'Updated Name' } });

    expect(locationsApi.update).toHaveBeenCalledWith(1, { name: 'Updated Name' });
    const cached = useLocationStore.getState().getLocationById(1);
    expect(cached?.name).toBe('Updated Name');
  });

  it('should delete location', async () => {
    vi.mocked(locationsApi.delete).mockResolvedValue({
      data: { message: 'Deleted' },
    } as any);

    useLocationStore.getState().addLocation(mockLocation);

    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    await result.current.delete(1);

    expect(locationsApi.delete).toHaveBeenCalledWith(1);
    const cached = useLocationStore.getState().getLocationById(1);
    expect(cached).toBeUndefined();
  });

  it('should move location', async () => {
    const moved = { ...mockLocation, parent_location_id: 2, path: 'parent.usa' };
    vi.mocked(locationsApi.move).mockResolvedValue({
      data: { data: moved },
    } as any);

    useLocationStore.getState().addLocation(mockLocation);

    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    await result.current.move({ id: 1, newParentId: 2 });

    expect(locationsApi.move).toHaveBeenCalledWith(1, { new_parent_id: 2 });
    const cached = useLocationStore.getState().getLocationById(1);
    expect(cached?.parent_location_id).toBe(2);
  });

  it('should expose loading states', () => {
    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isCreating).toBe(false);
    expect(result.current.isUpdating).toBe(false);
    expect(result.current.isDeleting).toBe(false);
    expect(result.current.isMoving).toBe(false);
  });

  it('should expose error states', () => {
    const { result } = renderHook(() => useLocationMutations(), {
      wrapper: createWrapper(),
    });

    expect(result.current.createError).toBe(null);
    expect(result.current.updateError).toBe(null);
    expect(result.current.deleteError).toBe(null);
    expect(result.current.moveError).toBe(null);
  });
});
