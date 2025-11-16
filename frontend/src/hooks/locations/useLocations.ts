import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';

export interface UseLocationsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

export function useLocations(options: UseLocationsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;

  const query = useQuery({
    queryKey: ['locations'],
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
