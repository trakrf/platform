import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { alarmDevicesApi } from '@/lib/api/alarmdevices';

export interface UseAlarmDevicesOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

const PER_PAGE = 100;

/**
 * Fetch all alarm devices for the current org. The list endpoint paginates;
 * walk pages until we've collected `total`.
 */
async function fetchAllAlarmDevices() {
  const first = await alarmDevicesApi.list({ page: 1, per_page: PER_PAGE });
  const all = [...first.data.data];
  const total = first.data.pagination?.total ?? all.length;

  let page = 1;
  while (all.length < total) {
    page += 1;
    const next = await alarmDevicesApi.list({ page, per_page: PER_PAGE });
    if (next.data.data.length === 0) break; // Safety: no more data
    all.push(...next.data.data);
  }

  return { data: all, total };
}

export function useAlarmDevices(options: UseAlarmDevicesOptions = {}) {
  const { enabled = true, refetchOnMount = true } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['alarmDevices', currentOrg?.id],
    queryFn: fetchAllAlarmDevices,
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    alarmDevices: query.data?.data ?? [],
    totalCount: query.data?.total ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
