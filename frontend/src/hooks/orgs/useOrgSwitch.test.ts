import React, { type ReactNode } from 'react';
import { renderHook, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useOrgSwitch } from './useOrgSwitch';
import { useOrgStore } from '@/stores/orgStore';
import { useAuthStore } from '@/stores/authStore';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useTagStore } from '@/stores/tagStore';
import { useBarcodeStore } from '@/stores/barcodeStore';
import { orgsApi } from '@/lib/api/orgs';
import type { Organization } from '@/types/org';

vi.mock('@/lib/api/orgs');

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
    // Reset stores
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useTagStore.getState().clearTags();
    useBarcodeStore.getState().clearBarcodes();
  });

  describe('switchOrg', () => {
    it('should call store switchOrg and invalidate caches', async () => {
      const switchOrgSpy = vi.spyOn(useOrgStore.getState(), 'switchOrg').mockResolvedValue();
      const invalidateAssetCache = vi.spyOn(useAssetStore.getState(), 'invalidateCache');
      const invalidateLocationCache = vi.spyOn(useLocationStore.getState(), 'invalidateCache');
      const clearTags = vi.spyOn(useTagStore.getState(), 'clearTags');
      const clearBarcodes = vi.spyOn(useBarcodeStore.getState(), 'clearBarcodes');

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      await act(async () => {
        await result.current.switchOrg(123);
      });

      expect(switchOrgSpy).toHaveBeenCalledWith(123);
      expect(invalidateAssetCache).toHaveBeenCalled();
      expect(invalidateLocationCache).toHaveBeenCalled();
      expect(clearTags).toHaveBeenCalled();
      expect(clearBarcodes).toHaveBeenCalled();
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

      // Spy on cache invalidation
      const invalidateAssetCache = vi.spyOn(useAssetStore.getState(), 'invalidateCache');
      const invalidateLocationCache = vi.spyOn(useLocationStore.getState(), 'invalidateCache');
      const clearTags = vi.spyOn(useTagStore.getState(), 'clearTags');
      const clearBarcodes = vi.spyOn(useBarcodeStore.getState(), 'clearBarcodes');

      const { result } = renderHook(() => useOrgSwitch(), {
        wrapper: createWrapper(),
      });

      let newOrg: Organization | undefined;
      await act(async () => {
        newOrg = await result.current.createOrg('New Org');
      });

      expect(newOrg).toEqual(mockOrg);
      expect(orgsApi.setCurrentOrg).toHaveBeenCalledWith({ org_id: 456 });
      expect(invalidateAssetCache).toHaveBeenCalled();
      expect(invalidateLocationCache).toHaveBeenCalled();
      expect(clearTags).toHaveBeenCalled();
      expect(clearBarcodes).toHaveBeenCalled();
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
