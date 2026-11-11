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

export type UpdateOutcome =
  | 'watching'
  | 'done'
  | 'rolled_back'
  | 'failed'
  | 'unguarded'
  | 'timeout';

/**
 * Where an update to `target`, started at `startedAt`, stands given the latest
 * reader software report (undefined while the reader is restarting and does not
 * answer). Only an outcome recorded for `target` counts, so an earlier attempt's
 * rollback is not mistaken for this one's — and only a report fetched after the
 * update started (`fetchedAt`), so a retry of the same version is not judged by
 * the previous attempt's cached result.
 *
 * `checkedAt` is the last time the reader was asked, answered or not. Timeout
 * needs a check after the deadline: in a background tab the clock runs on while
 * refetches pause, and a deadline that passed unobserved proves nothing.
 */
export function updateOutcome(
  target: string,
  software: ReaderSoftware | undefined,
  startedAt: number,
  now: number,
  fetchedAt: number = Number.POSITIVE_INFINITY,
  checkedAt: number = now
): UpdateOutcome {
  const fresh = fetchedAt > startedAt;
  const daemon = fresh ? software?.daemon : undefined;
  const update = daemon?.update;
  if (daemon?.version === target && update?.state === 'committed') return 'done';
  if (update?.target === target && update.state === 'rolled_back') return 'rolled_back';
  if (update?.target === target && update.state === 'failed') return 'failed';
  if (update?.target === target && update.state === 'unguarded') return 'unguarded';
  if (now - startedAt > UPDATE_WATCH_MS && checkedAt - startedAt > UPDATE_WATCH_MS)
    return 'timeout';
  return 'watching';
}

/**
 * Whether the reader itself reports an update that has not settled yet. True
 * whoever started it, so a panel opened mid-update (after a reload, or for
 * another admin's update) keeps polling instead of freezing on "pending".
 */
export function readerUpdateInFlight(software: ReaderSoftware | undefined): boolean {
  const state = software?.daemon?.update?.state;
  return state === 'downloading' || state === 'installing' || state === 'pending';
}

/** The start request was sent but its reply was lost (backend 504). */
function replyLost(err: unknown): boolean {
  return (err as { response?: { status?: number } } | null)?.response?.status === 504;
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
  const watching = watch != null && watch.settled == null;

  const query = useQuery({
    queryKey: ['readerSoftware', deviceId],
    queryFn: async () => (await scanDevicesApi.getReaderSoftware(deviceId as number)).data.data,
    enabled: deviceId != null,
    retry: false,
    refetchInterval: (q) =>
      watching || readerUpdateInFlight(q.state.data) ? UPDATE_POLL_MS : false,
  });
  const inFlight = readerUpdateInFlight(query.data);

  const outcome: UpdateOutcome | null = watch
    ? (watch.settled ??
      updateOutcome(
        watch.target,
        query.data,
        watch.startedAt,
        now,
        query.dataUpdatedAt,
        Math.max(query.dataUpdatedAt, query.errorUpdatedAt)
      ))
    : null;

  useEffect(() => {
    if (!watching) return;
    const t = setInterval(() => setNow(Date.now()), UPDATE_POLL_MS);
    return () => clearInterval(t);
  }, [watching]);

  useEffect(() => {
    if (watching && outcome && outcome !== 'watching') {
      setWatch((w) => (w ? { ...w, settled: outcome } : w));
    }
  }, [watching, outcome]);

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
    onError: (err, version) => {
      // 504: the request reached the reader but its reply was lost, so the
      // update may be running. Watch for its outcome rather than report it as
      // not started; a reader that never got it settles as a timeout.
      if (replyLost(err)) {
        const startedAt = Date.now();
        setNow(startedAt);
        setWatch({ target: version, startedAt });
      }
    },
  });

  return {
    software: query.data,
    isLoading: query.isLoading,
    error: watching ? null : query.error,
    startUpdate: async (version: string) => {
      await mutation.mutateAsync(version);
    },
    isStarting: mutation.isPending,
    startError: replyLost(mutation.error) ? null : (mutation.error as unknown),
    target: watch?.target ?? null,
    outcome,
    /** The reader reports an update in flight, whoever started it. */
    inFlight,
  };
}
