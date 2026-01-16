import { useQuery } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { useOrgStore } from '@/stores/orgStore';
import { assetsApi } from '@/lib/api/assets';

export interface UseAssetOptions {
  enabled?: boolean;
}

export function useAsset(id: number | null, options: UseAssetOptions = {}) {
  const { enabled = true } = options;

  const asset = useAssetStore((state) =>
    id ? state.getAssetById(id) ?? null : null
  );
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['asset', currentOrg?.id, id],
    queryFn: async ({ signal }) => {
      if (!id) return null;

      // Capture org ID at request time
      const orgIdAtFetch = currentOrg?.id;

      const response = await assetsApi.get(id, { signal });
      const fetchedAsset = response.data.data;

      // Validate org hasn't changed before updating store
      const currentOrgId = useOrgStore.getState().currentOrg?.id;
      if (currentOrgId !== orgIdAtFetch) {
        // Return data but skip store update - org changed during fetch
        return fetchedAsset;
      }

      useAssetStore.getState().addAsset(fetchedAsset);
      return fetchedAsset;
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
