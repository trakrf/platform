import { useQuery } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useOrgStore } from '@/stores/orgStore';
import { useTagStore } from '@/stores/tagStore';
import { assetsApi } from '@/lib/api/assets';
import { normalizeAsset } from '@/lib/asset/normalize';

export interface UseAssetsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

const PAGE_SIZE = 100;

/**
 * Fetch every asset by walking the pages (TRA-1098).
 *
 * The assets screen renders the whole cached set and paginates it in the
 * browser, so a single page left the table, the result count, the stat tiles
 * and every export silently short. Mirrors fetchAllLocations. The backend
 * clamps limit at 200.
 */
async function fetchAllAssets(signal?: AbortSignal) {
  const firstPage = await assetsApi.list({ limit: PAGE_SIZE, offset: 0, signal });
  const all = [...firstPage.data.data];
  const totalCount = firstPage.data.total_count;

  let offset = 0;
  while (all.length < totalCount) {
    offset += PAGE_SIZE;
    const page = await assetsApi.list({ limit: PAGE_SIZE, offset, signal });
    // Safety: a total_count the pages cannot satisfy (rows deleted mid-walk)
    // must not spin forever.
    if (page.data.data.length === 0) break;
    all.push(...page.data.data);
  }

  // count is "rows in this response", and the response is now every page.
  return { ...firstPage.data, data: all, count: all.length, total_count: totalCount };
}

export function useAssets(options: UseAssetsOptions = {}) {
  const { enabled = true, refetchOnMount = true } = options;

  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['assets', currentOrg?.id],
    queryFn: async ({ signal }) => {
      // Capture org ID at request time
      const orgIdAtFetch = currentOrg?.id;

      const response = await fetchAllAssets(signal);

      // Validate org hasn't changed before updating store
      const currentOrgId = useOrgStore.getState().currentOrg?.id;
      if (currentOrgId !== orgIdAtFetch) {
        // Return data but skip store update - org changed during fetch
        return response;
      }

      const normalized = response.data.map(normalizeAsset);
      // Replace, don't union: the screen renders the cache, so a stale entry
      // the server no longer returns would show as an extra row (TRA-1070).
      useAssetStore.getState().setAssets(normalized);
      // Re-enrich tags with newly loaded assets
      useTagStore.getState().refreshAssetEnrichment();
      return { ...response, data: normalized };
    },
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    assets: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
