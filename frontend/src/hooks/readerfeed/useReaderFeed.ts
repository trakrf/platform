// useReaderFeed — owns the tag-presence feed over the backend SSE proxy
// (TRA-936). Opens an authenticated SSE stream on mount and reduces the
// org-filtered snapshot/enter/update/leave deltas into a (reader,epc)-keyed map.
// The backend is authoritative for presence and aggregates (read count, RSSI);
// the browser only renders. A client-side backstop drops a tag we somehow stop
// hearing about (a LEAVE missed across a reconnect), but the server owns normal
// eviction.

import { useEffect, useMemo, useState } from 'react';
import {
  applyEvent,
  expireStale,
  filterByReader,
  BACKSTOP_TTL_SECONDS,
} from '@/lib/readerfeed/store';
import { openReadStream } from '@/lib/readerfeed/stream';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';
import type { ReaderFeedStatus, TagState } from '@/types/readerfeed';

const BACKSTOP_TICK_MS = 1000;

export interface ReaderFeedState {
  /** Present tags, scoped to filterReaderKey when given. */
  tags: TagState[];
  status: ReaderFeedStatus;
  error: string | null;
  /** Distinct reader keys currently in view. */
  readerCount: number;
  /** Server-reported reads/sec (whole-org; footer stat). */
  readRate: number;
}

/**
 * Owns the presence feed. Pass `filterReaderKey` to scope the view to a single
 * reader; omit it for the whole org. The SSE stream is always the full org feed
 * — filtering is a presentation concern, so the scoped panel and the global page
 * share one stream and one reducer.
 */
export function useReaderFeed(filterReaderKey?: string): ReaderFeedState {
  const [tagMap, setTagMap] = useState<Map<string, TagState>>(new Map());
  const [status, setStatus] = useState<ReaderFeedStatus>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [readRate, setReadRate] = useState(0);

  // SSE stream lifecycle. The reader scope is applied server-side (the backend
  // only streams/tracks that reader for this session), so changing it reconnects.
  useEffect(() => {
    const handle = openReadStream({
      baseURL: API_BASE_URL,
      readerKey: filterReaderKey,
      getToken: () => useAuthStore.getState().token,
      onUnauthorized: async () => {
        try {
          return await useAuthStore.getState().refresh();
        } catch {
          return false;
        }
      },
      callbacks: {
        onOpen: () => {
          setStatus('connected');
          setError(null);
        },
        onEvents: (events) => {
          setTagMap((prev) => {
            let next = prev;
            for (const ev of events) next = applyEvent(next, ev);
            return next;
          });
          // Footer read-rate rides the snapshot/keyframe.
          for (const ev of events) {
            if (ev.type === 'snapshot') setReadRate(ev.data.readRate);
          }
        },
        onError: (err) => {
          setStatus('error');
          setError(err instanceof Error ? err.message : 'stream error');
        },
      },
    });
    return () => handle.close();
  }, [filterReaderKey]);

  // Backstop expiry tick — only catches a LEAVE dropped during a reconnect.
  useEffect(() => {
    const id = setInterval(() => {
      setTagMap((prev) => expireStale(prev, Date.now(), BACKSTOP_TTL_SECONDS));
    }, BACKSTOP_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const tags = useMemo(
    () => filterByReader([...tagMap.values()], filterReaderKey),
    [tagMap, filterReaderKey]
  );
  const readerCount = useMemo(() => new Set(tags.map((t) => t.readerKey)).size, [tags]);

  return { tags, status, error, readerCount, readRate };
}
