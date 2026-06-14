import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { scanDevicesApi } from '@/lib/api/scandevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  ReaderCapabilities,
  ReaderConfig,
  SetReaderConfigRequest,
} from '@/types/scandevices';

const key = (deviceId: number) => ['readerConfig', deviceId];

/**
 * Fetch a fixed reader's capabilities + current config via the MQTT-RPC
 * contract (TRA-993). The GET brokers a live RPC to the reader's agent, so it
 * can 502/503 when the reader is offline — callers surface that as an error
 * state rather than retrying aggressively.
 */
export function useReaderConfig(deviceId: number | null) {
  const query = useQuery({
    queryKey: key(deviceId as number),
    queryFn: async () => {
      const res = await scanDevicesApi.getReaderConfig(deviceId as number);
      return res.data.data;
    },
    enabled: deviceId != null,
    retry: false,
  });

  return {
    capabilities: query.data?.capabilities as ReaderCapabilities | undefined,
    config: query.data?.config as ReaderConfig | undefined,
    isLoading: query.isLoading,
    error: query.error,
  };
}

/**
 * Push a new transmit-power map to a fixed reader. Fire on slider commit
 * (debounced by the caller). On success the config query is invalidated so the
 * UI reflects the reader's confirmed values.
 */
export function useSetReaderConfig(deviceId: number) {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async (body: SetReaderConfigRequest) => {
      await ensureOrgContext();
      const res = await scanDevicesApi.setReaderConfig(deviceId, body);
      return res.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: key(deviceId) });
    },
  });

  return {
    setConfig: mutation.mutateAsync,
    isSetting: mutation.isPending,
    error: mutation.error,
  };
}
