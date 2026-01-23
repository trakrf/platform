import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { useTagStore } from '@/stores/tagStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

export interface UseLocationsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

const PAGE_SIZE = 100;

/**
 * Fetch all locations by paginating through results.
 * Required for building complete byTagEpc cache for location tag detection.
 */
async function fetchAllLocations(): Promise<{ data: Location[]; total_count: number }> {
  const allLocations: Location[] = [];
  let offset = 0;
  let totalCount = 0;

  // Fetch first page to get total count
  const firstPage = await locationsApi.list({ limit: PAGE_SIZE, offset: 0 });
  allLocations.push(...firstPage.data.data);
  totalCount = firstPage.data.total_count;

  // Fetch remaining pages if needed
  while (allLocations.length < totalCount) {
    offset += PAGE_SIZE;
    const page = await locationsApi.list({ limit: PAGE_SIZE, offset });
    if (page.data.data.length === 0) break; // Safety: no more data
    allLocations.push(...page.data.data);
  }

  // Sanity check: warn if we somehow have fewer than expected
  if (allLocations.length < totalCount) {
    console.warn(
      `[useLocations] Fetched ${allLocations.length} locations but total_count is ${totalCount}. ` +
      `Some locations may be missing from cache.`
    );
  }

  return { data: allLocations, total_count: totalCount };
}

export function useLocations(options: UseLocationsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['locations', currentOrg?.id],
    queryFn: async () => {
      const result = await fetchAllLocations();
      useLocationStore.getState().setLocations(result.data);
      // Re-enrich any already-scanned tags with location data now that cache is populated
      useTagStore.getState()._enrichTagsWithLocations();
      return result;
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
