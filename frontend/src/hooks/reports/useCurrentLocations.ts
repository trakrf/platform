import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type { CurrentLocationsParams } from '@/types/reports';

export interface UseCurrentLocationsOptions extends CurrentLocationsParams {
  enabled?: boolean;
}

export function useCurrentLocations(options: UseCurrentLocationsOptions = {}) {
  const { enabled = true, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'current-locations', currentOrg?.id, params],
    queryFn: async () => {
      const response = await reportsApi.getCurrentLocations(params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id,
    staleTime: 30 * 1000, // 30 seconds - reports should refresh frequently
  });

  return {
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
