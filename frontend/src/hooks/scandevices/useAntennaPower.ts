import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { scanDevicesApi } from '@/lib/api/scandevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type { AntennaPower, SetAntennaPowerRequest } from '@/types/scandevices';

const key = (deviceId: number) => ['antennaPower', deviceId];

/**
 * Last-known per-antenna power for a CS463 (TRA-993). Polls while any antenna is
 * still "pending" so the UI converges on the agent's confirmed values.
 */
export function useAntennaPower(deviceId: number | null, options: { enabled?: boolean } = {}) {
  const { enabled = true } = options;

  const query = useQuery({
    queryKey: key(deviceId as number),
    queryFn: async (): Promise<AntennaPower[]> => {
      const res = await scanDevicesApi.getAntennaPower(deviceId as number);
      return res.data.data;
    },
    enabled: enabled && deviceId != null,
    // Poll while a command is in flight (status "pending"); otherwise idle.
    refetchInterval: (q) =>
      (q.state.data as AntennaPower[] | undefined)?.some((a) => a.status === 'pending') ? 3000 : false,
  });

  return {
    antennas: query.data ?? [],
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}

/**
 * Set per-antenna power. Fire the mutation on slider commit (debounced by the
 * caller). On success the query is invalidated so polling picks up confirmation.
 */
export function useSetAntennaPower(deviceId: number) {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async (body: SetAntennaPowerRequest) => {
      await ensureOrgContext();
      const res = await scanDevicesApi.setAntennaPower(deviceId, body);
      return res.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: key(deviceId) });
    },
  });

  return {
    setPower: mutation.mutateAsync,
    isSetting: mutation.isPending,
    error: mutation.error,
  };
}
