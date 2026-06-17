import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { scanDevicesApi } from '@/lib/api/scandevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  ReaderCapabilities,
  ReaderConfig,
  ReaderBusy,
  SetReaderConfigRequest,
} from '@/types/scandevices';

const key = (deviceId: number, force: boolean) => ['readerConfig', deviceId, force];

/** Pull a typed busy state out of a 409 reader_busy axios error, else null. */
export function parseReaderBusy(err: unknown): ReaderBusy | null {
  const e = err as { response?: { status?: number; data?: { error?: string; held_by?: string } } };
  if (e?.response?.status === 409 && e.response.data?.error === 'reader_busy') {
    return { held_by: e.response.data.held_by ?? '' };
  }
  return null;
}

/**
 * Fetch a fixed reader's live capabilities + config via the MQTT-RPC contract.
 * The reader allows one root session at a time, so the GET can come back "busy"
 * (held by another client / the reader's own web UI): that surfaces as `busy`,
 * and `retryWithForce` re-requests with force=true (force-logout-and-retry).
 */
export function useReaderConfig(deviceId: number | null) {
  const [force, setForce] = useState(false);
  const query = useQuery({
    queryKey: key(deviceId as number, force),
    queryFn: async () => {
      const res = await scanDevicesApi.getReaderConfig(deviceId as number, force);
      return res.data.data;
    },
    enabled: deviceId != null,
    retry: false,
  });

  const busy = parseReaderBusy(query.error);

  return {
    capabilities: query.data?.capabilities as ReaderCapabilities | undefined,
    config: query.data?.config as ReaderConfig | undefined,
    isLoading: query.isLoading,
    // a busy response is not a hard error — it has its own UX
    error: busy ? null : query.error,
    busy,
    retryWithForce: () => setForce(true),
  };
}

/**
 * Push antenna enablement + power to a fixed reader. `setConfig` takes the body
 * and an optional force flag (used to retry after a busy response). On success
 * the config query is invalidated so the UI reflects the reader's confirmed state.
 */
export function useSetReaderConfig(deviceId: number) {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async ({ body, force = false }: { body: SetReaderConfigRequest; force?: boolean }) => {
      await ensureOrgContext();
      const res = await scanDevicesApi.setReaderConfig(deviceId, body, force);
      return res.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['readerConfig', deviceId] });
    },
  });

  return {
    setConfig: mutation.mutateAsync,
    isSetting: mutation.isPending,
    error: mutation.error,
  };
}
