// useReaderFeed — owns the tag-presence feed over the backend SSE proxy
// (TRA-936). Opens an authenticated SSE stream on mount and reduces the
// org-filtered snapshot/enter/update/leave deltas into a (reader,epc)-keyed map.
// The backend is authoritative for presence and aggregates (read count, RSSI);
// the browser only renders. A client-side backstop drops a tag we somehow stop
// hearing about (a LEAVE missed across a reconnect), but the server owns normal
// eviction.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  applyEvent,
  expireStale,
  filterByReader,
  newestServerTimestamp,
  BACKSTOP_TTL_SECONDS,
} from '@/lib/readerfeed/store';
import { openReadStream } from '@/lib/readerfeed/stream';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
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
  /**
   * Drop the local map and reopen the stream. A fresh server session starts its
   * presence (and per-session read counts) from zero, so this is how "Clear"
   * resets the counts — there is no per-row baseline to subtract (TRA-937).
   */
  reconnect: () => void;
  /**
   * Estimated server−client clock offset (ms): add it to the browser `Date.now()`
   * to get a server-aligned "now" for age/staleness. Lets a client with a skewed
   * clock still render correct ages (and stops the backstop wiping the list).
   */
  clockOffsetMs: number;
}

/**
 * Owns the presence feed. Pass `filterReaderKey` to scope the view to a single
 * reader; omit it for the whole org. The SSE stream is always the full org feed
 * — filtering is a presentation concern, so the scoped panel and the global page
 * share one stream and one reducer.
 */
export function useReaderFeed(filterReaderKey?: string, showAllAdverts = false): ReaderFeedState {
  const [tagMap, setTagMap] = useState<Map<string, TagState>>(new Map());
  const [status, setStatus] = useState<ReaderFeedStatus>('connecting');
  const [error, setError] = useState<string | null>(null);
  const [readRate, setReadRate] = useState(0);

  // Server−client clock offset, derived from the freshest server `lastSeen` we
  // receive. The ref feeds the backstop interval (which reads the latest value
  // without re-subscribing); the state drives consumers' age rendering. The
  // client clock's *rate* is sound — only its absolute offset drifts — so this
  // is captured from real deltas and then held (never recomputed on a tick).
  const [clockOffsetMs, setClockOffsetMs] = useState(0);
  const offsetRef = useRef(0);

  // The active org scopes the stream server-side (the JWT bearer carries the
  // org_id claim). Switching orgs mints a new token, so the stream must tear
  // down and reopen on the new context — otherwise the old connection keeps
  // delivering the previous org's reads until a page refresh (TRA-964).
  const activeOrgId = useOrgStore((s) => s.currentOrg?.id);

  // Bumping this forces the stream effect to tear down and reopen — the
  // mechanism behind reconnect()/Clear.
  const [reconnectNonce, setReconnectNonce] = useState(0);
  const reconnect = useCallback(() => setReconnectNonce((n) => n + 1), []);

  // SSE stream lifecycle. The reader scope is applied server-side (the backend
  // only streams/tracks that reader for this session), so changing it — or the
  // active org — reconnects. Resetting here clears the previous context's reads
  // so the list never shows stale rows across the switch.
  useEffect(() => {
    setTagMap(new Map());
    setReadRate(0);
    setStatus('connecting');

    const handle = openReadStream({
      baseURL: API_BASE_URL,
      readerKey: filterReaderKey,
      showAllAdverts,
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
          // Re-sync the clock offset from the freshest server timestamp in this
          // batch (held between batches so age still advances with wall time).
          const serverTs = newestServerTimestamp(events);
          if (serverTs !== null) {
            const off = serverTs - Date.now();
            offsetRef.current = off;
            setClockOffsetMs(off);
          }
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
  }, [filterReaderKey, activeOrgId, reconnectNonce, showAllAdverts]);

  // Backstop expiry tick — only catches a LEAVE dropped during a reconnect.
  // Uses the server-aligned clock (Date.now() + offset) so a skewed browser
  // clock can't make every tag look stale and wipe the list each tick.
  useEffect(() => {
    const id = setInterval(() => {
      setTagMap((prev) => expireStale(prev, Date.now() + offsetRef.current, BACKSTOP_TTL_SECONDS));
    }, BACKSTOP_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const tags = useMemo(
    () => filterByReader([...tagMap.values()], filterReaderKey),
    [tagMap, filterReaderKey]
  );
  const readerCount = useMemo(() => new Set(tags.map((t) => t.readerKey)).size, [tags]);

  return { tags, status, error, readerCount, readRate, reconnect, clockOffsetMs };
}
