/**
 * useReportHydration (TRA-844)
 *
 * Reports endpoints return key-only rows (master-data / scan-data bifurcation,
 * TRA-734). The SPA hydrates names client-side by joining report rows to the
 * already-loaded asset/location stores; assets missing from the store are
 * fetched per-id via useQueries, and resolved-404s plus rows carrying
 * `asset_deleted_at` are marked `(deleted)`.
 */
import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { useLocations } from '@/hooks/locations/useLocations';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';

export interface UseReportHydrationInput {
  assetIds: Array<number | null | undefined>;
  locationIds: Array<number | null | undefined>;
}

export interface UseReportHydrationResult {
  getAssetName: (
    id: number | null | undefined,
    fallbackKey: string | null | undefined,
    deletedAt: string | null | undefined
  ) => string;
  getLocationName: (
    id: number | null | undefined,
    fallbackKey: string | null | undefined
  ) => string;
  isHydrating: boolean;
}

const UNKNOWN_LABEL = 'Unknown';

function withDeleted(label: string): string {
  return `${label} (deleted)`;
}

export function useReportHydration({
  assetIds,
  locationIds: _locationIds,
}: UseReportHydrationInput): UseReportHydrationResult {
  const currentOrgId = useOrgStore((s) => s.currentOrg?.id);

  // Eagerly mount the full location list (idempotent; React Query dedupes).
  const { isLoading: locationsLoading } = useLocations();

  const assetById = useAssetStore((s) => s.cache.byId);
  const locationById = useLocationStore((s) => s.cache.byId);

  const missingAssetIds = useMemo(() => {
    const set = new Set<number>();
    for (const id of assetIds) {
      if (id == null) continue;
      if (!assetById.has(id)) set.add(id);
    }
    return Array.from(set);
  }, [assetIds, assetById]);

  const assetQueries = useQueries({
    queries: missingAssetIds.map((id) => ({
      queryKey: ['asset', currentOrgId, id] as const,
      queryFn: async ({ signal }: { signal?: AbortSignal }) => {
        try {
          const response = await assetsApi.get(id, { signal });
          const asset = response.data.data as Asset;
          useAssetStore.getState().addAsset(asset);
          return asset;
        } catch (err: any) {
          if (err?.response?.status === 404) {
            // Sentinel: resolved 404 means asset truly gone.
            return null;
          }
          throw err;
        }
      },
      staleTime: 60 * 60 * 1000,
      retry: false,
    })),
  });

  const resolvedDeletedIds = useMemo(() => {
    const set = new Set<number>();
    assetQueries.forEach((q, i) => {
      if (q.isFetched && q.data === null) set.add(missingAssetIds[i]);
    });
    return set;
  }, [assetQueries, missingAssetIds]);

  const assetsLoading = assetQueries.some((q) => q.isLoading);

  return useMemo<UseReportHydrationResult>(
    () => ({
      getAssetName: (id, fallbackKey, deletedAt) => {
        const key = fallbackKey ?? '';
        if (id == null) return key || UNKNOWN_LABEL;
        const asset = assetById.get(id);
        if (asset?.name) return asset.name;
        if (deletedAt || resolvedDeletedIds.has(id)) {
          return key ? withDeleted(key) : UNKNOWN_LABEL;
        }
        return key || UNKNOWN_LABEL;
      },
      getLocationName: (id, fallbackKey) => {
        const key = fallbackKey ?? '';
        if (id == null) return key || UNKNOWN_LABEL;
        const loc = locationById.get(id);
        if (loc?.name) return loc.name;
        return key || UNKNOWN_LABEL;
      },
      isHydrating: locationsLoading || assetsLoading,
    }),
    [assetById, locationById, resolvedDeletedIds, locationsLoading, assetsLoading]
  );
}
