import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type { AssetHistoryParams } from '@/types/reports';

export interface UseAssetHistoryOptions extends AssetHistoryParams {
  enabled?: boolean;
}

export function useAssetHistory(assetId: number | null, options: UseAssetHistoryOptions = {}) {
  const { enabled = true, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'asset-history', currentOrg?.id, assetId, params],
    queryFn: async () => {
      if (!assetId) throw new Error('Asset ID required');
      const response = await reportsApi.getAssetHistory(assetId, params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id && !!assetId,
    staleTime: 30 * 1000,
  });

  return {
    asset: query.data?.asset ?? null,
    data: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    count: query.data?.count ?? 0,
    offset: query.data?.offset ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
