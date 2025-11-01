import { useQuery } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';

export interface UseAssetOptions {
  enabled?: boolean;
}

export function useAsset(id: number | null, options: UseAssetOptions = {}) {
  const { enabled = true } = options;

  const asset = useAssetStore((state) =>
    id ? state.getAssetById(id) ?? null : null
  );

  const query = useQuery({
    queryKey: ['asset', id],
    queryFn: async () => {
      if (!id) return null;
      const response = await assetsApi.get(id);
      const asset = response.data.data;
      useAssetStore.getState().addAsset(asset);
      return asset;
    },
    enabled: enabled && !!id && !asset,
    staleTime: 60 * 60 * 1000,
  });

  return {
    asset: asset ?? query.data ?? null,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
