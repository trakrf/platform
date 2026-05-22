import { useMemo } from 'react';
import { useCurrentLocations } from './useCurrentLocations';
import type { CurrentLocationItem } from '@/types/reports';

/**
 * Current location for every asset in the org, keyed by asset id.
 *
 * TRA-799: asset location is fact data and is no longer carried on the asset
 * resource. The assets screen sources current location from the reporting
 * endpoint (GET /api/v1/reports/asset-locations). React Query dedupes the
 * underlying fetch across every consumer, so components may call this hook
 * directly without prop-drilling.
 */
export function useAssetLocations() {
  const { data, isLoading, error } = useCurrentLocations({ fetchAll: true });

  const byAssetId = useMemo(() => {
    const map = new Map<number, CurrentLocationItem>();
    for (const item of data) {
      if (item.asset_id != null) map.set(item.asset_id, item);
    }
    return map;
  }, [data]);

  return { byAssetId, isLoading, error };
}
