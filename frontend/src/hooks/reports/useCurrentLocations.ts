import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { reportsApi } from '@/lib/api/reports';
import type {
  CurrentLocationsParams,
  CurrentLocationsResponse,
} from '@/types/reports';

// Keep in sync with backend httputil.maxListLimit (backend/internal/util/httputil/listparams.go).
const MAX_PAGE_SIZE = 200;

export interface UseCurrentLocationsOptions extends CurrentLocationsParams {
  enabled?: boolean;
  // When true, pages through every result with limit=MAX_PAGE_SIZE until
  // total_count is reached. Caller-supplied `limit`/`offset` are ignored.
  fetchAll?: boolean;
}

async function fetchAllPages(
  params: CurrentLocationsParams
): Promise<CurrentLocationsResponse> {
  const { limit: _ignoredLimit, offset: _ignoredOffset, ...rest } = params;
  const all: CurrentLocationsResponse['data'] = [];
  let totalCount = 0;
  for (let offset = 0; ; offset += MAX_PAGE_SIZE) {
    const response = await reportsApi.listAssetLocations({
      ...rest,
      limit: MAX_PAGE_SIZE,
      offset,
    });
    const page = response.data;
    all.push(...page.data);
    totalCount = page.total_count;
    if (all.length >= totalCount || page.data.length === 0) break;
  }
  return { data: all, limit: MAX_PAGE_SIZE, offset: 0, total_count: totalCount };
}

export function useCurrentLocations(options: UseCurrentLocationsOptions = {}) {
  const { enabled = true, fetchAll = false, ...params } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['reports', 'current-locations', currentOrg?.id, fetchAll, params],
    queryFn: async () => {
      if (fetchAll) return fetchAllPages(params);
      const response = await reportsApi.listAssetLocations(params);
      return response.data;
    },
    enabled: enabled && !!currentOrg?.id,
    staleTime: 30 * 1000, // 30 seconds - reports should refresh frequently
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
