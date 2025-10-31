import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';

export interface UseAssetsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

export function useAssets(options: UseAssetsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;

  const allIdsLength = useAssetStore((state) => state.cache.allIds.length);
  const filters = useAssetStore((state) => state.filters);
  const sort = useAssetStore((state) => state.sort);
  const pagination = useAssetStore((state) => state.pagination);

  const query = useQuery({
    queryKey: ['assets'],
    queryFn: async () => {
      const response = await assetsApi.list();
      const assets = response.data.data;
      useAssetStore.getState().addAssets(assets);
      return assets;
    },
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  const assets = useMemo(() => {
    const filteredAssets = useAssetStore.getState().getFilteredAssets();
    const { currentPage, pageSize } = pagination;
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    return filteredAssets.slice(startIndex, endIndex);
  }, [allIdsLength, filters, sort, pagination]);

  const totalCount = useMemo(() => {
    return useAssetStore.getState().getFilteredAssets().length;
  }, [allIdsLength, filters]);

  return {
    assets,
    totalCount,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
