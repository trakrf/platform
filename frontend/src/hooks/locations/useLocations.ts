import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

export interface UseLocationsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

const PAGE_SIZE = 100;

/**
 * Normalize a raw Location from the public API to the internal shape.
 * Maps surrogate_id → id. parent_location_id is resolved in a second pass
 * after all locations are fetched (we need the full list to resolve parent
 * natural key → surrogate ID).
 */
function normalizeLocation(raw: Location): Location {
  return { ...raw, id: raw.surrogate_id };
}

/**
 * Resolve parent_location_id for each location after the full list is known.
 * Builds a natural-key → surrogate-id index then patches each item.
 */
function resolveParentIds(locations: Location[]): Location[] {
  const byIdentifier = new Map(locations.map((l) => [l.identifier, l.id]));
  return locations.map((l) => ({
    ...l,
    parent_location_id: l.parent ? (byIdentifier.get(l.parent) ?? null) : null,
  }));
}

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
  allLocations.push(...firstPage.data.data.map(normalizeLocation));
  totalCount = firstPage.data.total_count;

  // Fetch remaining pages if needed
  while (allLocations.length < totalCount) {
    offset += PAGE_SIZE;
    const page = await locationsApi.list({ limit: PAGE_SIZE, offset });
    if (page.data.data.length === 0) break; // Safety: no more data
    allLocations.push(...page.data.data.map(normalizeLocation));
  }

  // Sanity check: warn if we somehow have fewer than expected
  if (allLocations.length < totalCount) {
    console.warn(
      `[useLocations] Fetched ${allLocations.length} locations but total_count is ${totalCount}. ` +
      `Some locations may be missing from cache.`
    );
  }

  // Resolve parent_location_id using natural key → surrogate ID lookup
  const resolved = resolveParentIds(allLocations);

  return { data: resolved, total_count: totalCount };
}

export function useLocations(options: UseLocationsOptions = {}) {
  const { enabled = true, refetchOnMount = false } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['locations', currentOrg?.id],
    queryFn: async () => {
      const result = await fetchAllLocations();
      useLocationStore.getState().setLocations(result.data);
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
