import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { scanDevicesApi } from '@/lib/api/scandevices';

export interface UseScanDevicesOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

const PER_PAGE = 100;

/**
 * Fetch all scan devices for the current org. The list endpoint paginates;
 * walk pages until we've collected `total`.
 */
async function fetchAllScanDevices() {
  const first = await scanDevicesApi.list({ page: 1, per_page: PER_PAGE });
  const all = [...first.data.data];
  const total = first.data.pagination?.total ?? all.length;

  let page = 1;
  while (all.length < total) {
    page += 1;
    const next = await scanDevicesApi.list({ page, per_page: PER_PAGE });
    if (next.data.data.length === 0) break; // Safety: no more data
    all.push(...next.data.data);
  }

  return { data: all, total };
}

export function useScanDevices(options: UseScanDevicesOptions = {}) {
  const { enabled = true, refetchOnMount = true } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['scanDevices', currentOrg?.id],
    queryFn: fetchAllScanDevices,
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    scanDevices: query.data?.data ?? [],
    totalCount: query.data?.total ?? 0,
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}

export interface UseScanPointsOptions {
  enabled?: boolean;
}

/**
 * Fetch the scan points belonging to a single scan device.
 */
export function useScanPoints(deviceId: number | null, options: UseScanPointsOptions = {}) {
  const { enabled = true } = options;

  const query = useQuery({
    queryKey: ['scanPoints', deviceId],
    queryFn: async () => {
      const response = await scanDevicesApi.listPoints(deviceId as number);
      return response.data.data;
    },
    enabled: enabled && deviceId != null,
    staleTime: 60 * 60 * 1000,
  });

  return {
    scanPoints: query.data ?? [],
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}
