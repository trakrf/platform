import { useEffect, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { scanDevicesApi } from '@/lib/api/scandevices';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type { ReaderSoftware } from '@/types/scandevices';

/** How often the reader is polled while an update is being watched. */
export const UPDATE_POLL_MS = 5_000;

/**
 * How long to watch an update before giving up. The reader's rollback guard acts
 * 15-20 minutes after an update that never commits, so by 25 minutes the reader
 * has either committed, rolled back, or is genuinely unreachable.
 */
export const UPDATE_WATCH_MS = 25 * 60 * 1000;

export type UpdateOutcome = 'watching' | 'done' | 'rolled_back' | 'failed' | 'timeout';

/**
 * Where an update to `target`, started at `startedAt`, stands given the latest
 * reader software report (undefined while the reader is restarting and does not
 * answer). Only an outcome recorded for `target` counts, so an earlier attempt's
 * rollback is not mistaken for this one's — and only a report fetched after the
 * update started (`fetchedAt`), so a retry of the same version is not judged by
 * the previous attempt's cached result.
 */
export function updateOutcome(
  target: string,
  software: ReaderSoftware | undefined,
  startedAt: number,
  now: number,
  fetchedAt: number = Number.POSITIVE_INFINITY
): UpdateOutcome {
  const fresh = fetchedAt > startedAt;
  const daemon = fresh ? software?.daemon : undefined;
  const update = daemon?.update;
  if (daemon?.version === target && update?.state === 'committed') return 'done';
  if (update?.target === target && update.state === 'rolled_back') return 'rolled_back';
  if (update?.target === target && update.state === 'failed') return 'failed';
  if (now - startedAt > UPDATE_WATCH_MS) return 'timeout';
  return 'watching';
}

interface Watch {
  target: string;
  startedAt: number;
  /** Set once the outcome is known; polling stops but the outcome stays shown. */
  settled?: UpdateOutcome;
}

/**
 * A fixed reader's daemon version, plus starting and watching a remote update
 * (TRA-1317). While an update is watched the reader is polled; errors during
 * that window are expected (the daemon restarts) and are not surfaced.
 */
export function useReaderSoftware(deviceId: number | null) {
  const [watch, setWatch] = useState<Watch | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const polling = watch != null && watch.settled == null;

  const query = useQuery({
    queryKey: ['readerSoftware', deviceId],
    queryFn: async () => (await scanDevicesApi.getReaderSoftware(deviceId as number)).data.data,
    enabled: deviceId != null,
    retry: false,
    refetchInterval: polling ? UPDATE_POLL_MS : false,
  });

  const outcome: UpdateOutcome | null = watch
    ? (watch.settled ??
      updateOutcome(watch.target, query.data, watch.startedAt, now, query.dataUpdatedAt))
    : null;

  useEffect(() => {
    if (!polling) return;
    const t = setInterval(() => setNow(Date.now()), UPDATE_POLL_MS);
    return () => clearInterval(t);
  }, [polling]);

  useEffect(() => {
    if (polling && outcome && outcome !== 'watching') {
      setWatch((w) => (w ? { ...w, settled: outcome } : w));
    }
  }, [polling, outcome]);

  const mutation = useMutation({
    mutationFn: async (version: string) => {
      await ensureOrgContext();
      return (await scanDevicesApi.startReaderUpdate(deviceId as number, version)).data.data;
    },
    onSuccess: (res) => {
      const startedAt = Date.now();
      setNow(startedAt);
      setWatch({ target: res.to, startedAt });
    },
  });

  return {
    software: query.data,
    isLoading: query.isLoading,
    error: polling ? null : query.error,
    startUpdate: async (version: string) => {
      await mutation.mutateAsync(version);
    },
    isStarting: mutation.isPending,
    startError: mutation.error as unknown,
    target: watch?.target ?? null,
    outcome,
  };
}
