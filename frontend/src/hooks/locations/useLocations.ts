import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { locationsApi } from '@/lib/api/locations';

export interface UseLocationsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

export function useLocations(options: UseLocationsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['locations', currentOrg?.id],
    queryFn: async () => {
      const response = await locationsApi.list();
      useLocationStore.getState().setLocations(response.data.data);
      return response.data;
    },
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    locations: query.data?.data ?? [],
    totalCount: query.data?.total_count ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
