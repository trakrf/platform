import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useOrgStore } from '@/stores/orgStore';
import { locationsApi } from '@/lib/api/locations';
import type { Location } from '@/types/locations';

function normalizeLocation(raw: Location): Location {
  const byExternalKey = useLocationStore.getState().cache?.byExternalKey;
  const parentId = raw.parent_external_key
    ? (byExternalKey?.get(raw.parent_external_key)?.id ?? null)
    : null;
  return {
    ...raw,
    parent_id: parentId,
  };
}

export interface UseLocationOptions {
  enabled?: boolean;
}

export function useLocation(id: number | null, options: UseLocationOptions = {}) {
  const { enabled = true } = options;

  const location = useLocationStore((state) =>
    id ? state.getLocationById(id) ?? null : null
  );
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['location', currentOrg?.id, id],
    queryFn: async () => {
      if (!id) throw new Error('Location ID is required');
      const response = await locationsApi.get(id);
      const location = normalizeLocation(response.data.data);
      useLocationStore.getState().addLocation(location);
      return location;
    },
    enabled: enabled && id !== null && !location,
    staleTime: 60 * 60 * 1000,
  });

  return {
    location: location ?? query.data ?? null,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
