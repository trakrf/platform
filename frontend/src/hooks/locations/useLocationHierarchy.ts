import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

/**
 * Normalize a raw public API location to the internal cache shape.
 * Maps surrogate_id → id and resolves parent_location_id from the store's
 * natural-key index. Mirrors the helper used in the other location hooks;
 * required because PublicLocationView only carries surrogate_id, and
 * locationStore.addLocation keys by .id.
 */
function normalizeLocation(raw: Location): Location {
  const byIdentifier = useLocationStore.getState().cache?.byIdentifier;
  const parentId = raw.parent
    ? (byIdentifier?.get(raw.parent)?.id ?? null)
    : null;
  return { ...raw, id: raw.surrogate_id ?? raw.id, parent_location_id: parentId };
}

export function useLocationHierarchy(locationId: number | null) {
  const location = useLocationStore((state) =>
    locationId ? state.getLocationById(locationId) : undefined
  );

  const parentLocation = useLocationStore((state) =>
    location?.parent_location_id
      ? state.getLocationById(location.parent_location_id)
      : undefined
  );

  const getChildren = useLocationStore((state) => state.getChildren);
  const getDescendants = useLocationStore((state) => state.getDescendants);
  const getAncestors = useLocationStore((state) => state.getAncestors);

  const subsidiaries = useMemo(
    () => (locationId ? getChildren(locationId) : []),
    [locationId, getChildren]
  );

  const allSubsidiaries = useMemo(
    () => (locationId ? getDescendants(locationId) : []),
    [locationId, getDescendants]
  );

  const locationPath = useMemo(
    () => (locationId ? getAncestors(locationId) : []),
    [locationId, getAncestors]
  );

  const parentsQuery = useQuery({
    queryKey: ['location-parents', locationId],
    queryFn: async () => {
      if (!locationId) return [];
      const response = await locationsApi.getAncestors(locationId);
      const normalized = response.data.data.map(normalizeLocation);
      normalized.forEach((loc) => {
        useLocationStore.getState().addLocation(loc);
      });
      return normalized;
    },
    enabled: !!locationId && locationPath.length === 0,
    staleTime: 60 * 60 * 1000,
  });

  const subsidiariesQuery = useQuery({
    queryKey: ['location-subsidiaries', locationId],
    queryFn: async () => {
      if (!locationId) return [];
      const response = await locationsApi.getDescendants(locationId);
      const normalized = response.data.data.map(normalizeLocation);
      normalized.forEach((loc) => {
        useLocationStore.getState().addLocation(loc);
      });
      return normalized;
    },
    enabled: !!locationId && subsidiaries.length === 0,
    staleTime: 60 * 60 * 1000,
  });

  return {
    parentLocation: parentLocation ?? parentsQuery.data?.[0],
    subsidiaries: subsidiaries.length > 0 ? subsidiaries : (subsidiariesQuery.data ?? []),
    allSubsidiaries,
    locationPath: locationPath.length > 0 ? locationPath : (parentsQuery.data ?? []),
    isRoot: location ? location.parent_location_id === null : false,
    hasSubsidiaries: subsidiaries.length > 0 || allSubsidiaries.length > 0,
    fetchParents: parentsQuery.refetch,
    fetchSubsidiaries: subsidiariesQuery.refetch,
    isLoadingParents: parentsQuery.isLoading,
    isLoadingSubsidiaries: subsidiariesQuery.isLoading,
  };
}
