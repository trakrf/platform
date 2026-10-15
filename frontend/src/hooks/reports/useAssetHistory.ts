import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type { AssetHistoryItem, AssetHistoryParams } from '@/types/reports';
import { MAX_PAGE_SIZE } from './useCurrentLocations';

export interface UseAssetHistoryOptions extends AssetHistoryParams {
  enabled?: boolean;
}

// Every stay in the range, paged at the backend cap. For exports, which must
// not stop at the pages a timeline has loaded so far.
export async function fetchAllAssetHistory(
  assetId: number,
  params: Omit<AssetHistoryParams, 'limit' | 'offset'>
): Promise<AssetHistoryItem[]> {
  const all: AssetHistoryItem[] = [];
  for (let offset = 0; ; offset += MAX_PAGE_SIZE) {
    const response = await reportsApi.listAssetHistory(assetId, {
      ...params,
      limit: MAX_PAGE_SIZE,
      offset,
    });
    const page = response.data;
    all.push(...page.data);
    if (all.length >= page.total_count || page.data.length === 0) break;
  }
  return all;
}

export function useAssetHistory(assetId: number | null, options: UseAssetHistoryOptions = {}) {
  const { enabled = true, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'asset-history', currentOrg?.id, assetId, params],
    queryFn: async () => {
      if (!assetId) throw new Error('Asset ID required');
      const response = await reportsApi.listAssetHistory(assetId, params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id && !!assetId,
    staleTime: 30 * 1000,
  });

  return {
    data: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    count: query.data?.limit ?? 0,
    offset: query.data?.offset ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
