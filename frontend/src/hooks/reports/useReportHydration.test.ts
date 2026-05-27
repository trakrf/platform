import React, { type ReactNode } from 'react';
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { assetsApi } from '@/lib/api/assets';
import { locationsApi } from '@/lib/api/locations';
import { useReportHydration } from './useReportHydration';
import type { Asset } from '@/types/assets';
import type { Location } from '@/types/locations';

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function seedAsset(
  partial: Partial<Asset> & { id: number; external_key: string; name: string }
) {
  useAssetStore.getState().addAsset({
    is_active: true,
    description: null,
    valid_from: '2026-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...partial,
  } as Asset);
}

function seedLocations(
  partials: Array<
    Partial<Location> & { id: number; external_key: string; name: string }
  >
) {
  useLocationStore.getState().setLocations(
    partials.map(
      (partial) =>
        ({
          description: '',
          parent_id: null,
          parent_external_key: null,
          valid_from: '2026-01-01T00:00:00Z',
          valid_to: null,
          is_active: true,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          ...partial,
        }) as Location
    )
  );
}

describe('useReportHydration', () => {
  beforeEach(() => {
    useAssetStore.getState().invalidateCache();
    useLocationStore.getState().invalidateCache();
    useOrgStore.setState({
      currentOrg: { id: 'org-1', slug: 'org-1', name: 'Org 1' } as any,
    });
    // Stub useLocations() bulk fetch so it doesn't hit the network.
    vi.spyOn(locationsApi, 'list').mockResolvedValue({
      data: { data: [], limit: 100, offset: 0, total_count: 0 },
    } as any);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns asset name from store when present', () => {
    seedAsset({ id: 42, external_key: 'ASSET-0042', name: 'Forklift 7' });

    const { result } = renderHook(
      () => useReportHydration({ assetIds: [42], locationIds: [] }),
      { wrapper: makeWrapper() }
    );

    expect(result.current.getAssetName(42, 'ASSET-0042', null)).toBe('Forklift 7');
  });

  it('returns location name from store when present', () => {
    seedLocations([{ id: 10, external_key: 'LOC-A', name: 'Warehouse A' }]);

    const { result } = renderHook(
      () => useReportHydration({ assetIds: [], locationIds: [10] }),
      { wrapper: makeWrapper() }
    );

    expect(result.current.getLocationName(10, 'LOC-A')).toBe('Warehouse A');
  });

  it('returns external_key + (deleted) when row carries deleted_at and no store hit', () => {
    const { result } = renderHook(
      () => useReportHydration({ assetIds: [], locationIds: [] }),
      { wrapper: makeWrapper() }
    );

    expect(
      result.current.getAssetName(99, 'ASSET-9999', '2026-05-01T00:00:00Z')
    ).toBe('ASSET-9999 (deleted)');
  });

  it('fetches asset by id when missing from store and uses the fetched name', async () => {
    const spy = vi.spyOn(assetsApi, 'get').mockResolvedValueOnce({
      data: {
        data: {
          id: 7,
          external_key: 'ASSET-0007',
          name: 'Pallet Jack 12',
          description: null,
          valid_from: '2026-01-01T00:00:00Z',
          valid_to: null,
          metadata: {},
          is_active: true,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          tags: [],
        },
      },
    } as any);

    const { result } = renderHook(
      () => useReportHydration({ assetIds: [7], locationIds: [] }),
      { wrapper: makeWrapper() }
    );

    await waitFor(() => {
      expect(result.current.getAssetName(7, 'ASSET-0007', null)).toBe(
        'Pallet Jack 12'
      );
    });
    expect(spy).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ signal: expect.anything() })
    );
  });

  it('returns external_key + (deleted) when fetch resolves 404 (resolved-deleted)', async () => {
    vi.spyOn(assetsApi, 'get').mockRejectedValueOnce({
      response: { status: 404 },
    });

    const { result } = renderHook(
      () => useReportHydration({ assetIds: [13], locationIds: [] }),
      { wrapper: makeWrapper() }
    );

    await waitFor(() => {
      expect(result.current.getAssetName(13, 'ASSET-0013', null)).toBe(
        'ASSET-0013 (deleted)'
      );
    });
  });
});
