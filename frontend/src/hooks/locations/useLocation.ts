import { useQuery } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';

export interface UseLocationOptions {
  enabled?: boolean;
}

export function useLocation(id: number | null, options: UseLocationOptions = {}) {
  const { enabled = true } = options;

  const location = useLocationStore((state) =>
    id ? state.getLocationById(id) ?? null : null
  );

  const query = useQuery({
    queryKey: ['location', id],
    queryFn: async () => {
      if (!id) throw new Error('Location ID is required');
      const response = await locationsApi.get(id);
      const location = response.data.data;
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
