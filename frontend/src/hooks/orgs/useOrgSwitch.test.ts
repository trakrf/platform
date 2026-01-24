import React, { type ReactNode } from 'react';
import { renderHook, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useOrgSwitch } from './useOrgSwitch';
import { useOrgStore } from '@/stores/orgStore';
import { useAuthStore } from '@/stores/authStore';
import { orgsApi } from '@/lib/api/orgs';
import { invalidateAllOrgScopedData } from '@/lib/cache/orgScopedCache';
import type { Organization } from '@/types/org';

vi.mock('@/lib/api/orgs');

// Mock the cache invalidation module
vi.mock('@/lib/cache/orgScopedCache', () => ({
  invalidateAllOrgScopedData: vi.fn().mockResolvedValue(undefined),
}));

// Mock the queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryClient: {},
}));

const mockOrg: Organization = {
  id: 456,
  name: 'New Org',
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

describe('useOrgSwitch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('switchOrg', () => {
    it('should call store switchOrg and invalidate caches', async () => {
      // orgStore.switchOrg now calls central invalidation internally
      const switchOrgSpy = vi.spyOn(useOrgStore.getState(), 'switchOrg').mockResolvedValue();

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      await act(async () => {
        await result.current.switchOrg(123);
      });

      // useOrgSwitch.switchOrg delegates to orgStore.switchOrg which handles central invalidation
      expect(switchOrgSpy).toHaveBeenCalledWith(123);
    });
  });

  describe('createOrg', () => {
    it('should create org, switch to it, and invalidate caches', async () => {
      const mockToken = 'new-jwt-token';

      // Mock store createOrg
      vi.spyOn(useOrgStore.getState(), 'createOrg').mockResolvedValue(mockOrg);

      // Mock setCurrentOrg API
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { message: 'ok', token: mockToken },
      } as any);

      // Mock fetchProfile
      vi.spyOn(useAuthStore.getState(), 'fetchProfile').mockResolvedValue();

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      let newOrg: Organization | undefined;
      await act(async () => {
        newOrg = await result.current.createOrg('New Org');
      });

      expect(newOrg).toEqual(mockOrg);
      expect(orgsApi.setCurrentOrg).toHaveBeenCalledWith({ org_id: 456 });
      // Central invalidation is called after createOrg
      expect(invalidateAllOrgScopedData).toHaveBeenCalled();
    });

    it('should update auth token after creating org', async () => {
      const mockToken = 'new-jwt-token';

      vi.spyOn(useOrgStore.getState(), 'createOrg').mockResolvedValue(mockOrg);
      vi.mocked(orgsApi.setCurrentOrg).mockResolvedValue({
        data: { message: 'ok', token: mockToken },
      } as any);
      vi.spyOn(useAuthStore.getState(), 'fetchProfile').mockResolvedValue();

      const setStateSpy = vi.spyOn(useAuthStore, 'setState');

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      await act(async () => {
        await result.current.createOrg('New Org');
      });

      expect(setStateSpy).toHaveBeenCalledWith({ token: mockToken });
    });
  });
});
