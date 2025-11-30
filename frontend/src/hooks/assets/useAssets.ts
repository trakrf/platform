import { useQuery } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useTagStore } from '@/stores/tagStore';
import { assetsApi } from '@/lib/api/assets';

export interface UseAssetsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

export function useAssets(options: UseAssetsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;

  const pagination = useAssetStore((state) => state.pagination);

  const query = useQuery({
    queryKey: ['assets', pagination.currentPage, pagination.pageSize],
    queryFn: async () => {
      const offset = (pagination.currentPage - 1) * pagination.pageSize;
      const response = await assetsApi.list({
        limit: pagination.pageSize,
        offset,
      });
      useAssetStore.getState().addAssets(response.data.data);
      // Re-enrich tags with newly loaded assets
      useTagStore.getState().refreshAssetEnrichment();
      return response.data;
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
