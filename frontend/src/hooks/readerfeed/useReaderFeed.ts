// useReaderFeed — owns the live read feed over the backend SSE proxy (TRA-924).
// Opens an authenticated SSE stream on mount, feeds each org-filtered read
// through the pure merge/expire reducer, and tears the stream down on unmount.
// The browser no longer talks to the MQTT broker; the backend enforces org
// scoping and holds the broker connection.

import { useEffect, useMemo, useState } from 'react';
import { mergeReads, expireReads, READ_TTL_SECONDS } from '@/lib/readerfeed/store';
import { openReadStream } from '@/lib/readerfeed/stream';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';
import type { LiveRead, ReaderFeedStatus } from '@/types/readerfeed';

const EXPIRY_TICK_MS = 1000;

export interface ReaderFeedState {
  reads: LiveRead[];
  status: ReaderFeedStatus;
  error: string | null;
  /** Distinct reader keys currently in view. */
  readerCount: number;
}

export function useReaderFeed(): ReaderFeedState {
  const [tags, setTags] = useState<Map<string, LiveRead>>(new Map());
  const [status, setStatus] = useState<ReaderFeedStatus>('connecting');
  const [error, setError] = useState<string | null>(null);

  // SSE stream lifecycle.
  useEffect(() => {
    const handle = openReadStream({
      baseURL: API_BASE_URL,
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
        onReads: (reads) => setTags((prev) => mergeReads(prev, reads, Date.now())),
        onError: (err) => {
          setStatus('error');
          setError(err instanceof Error ? err.message : 'stream error');
        },
      },
    });
    return () => handle.close();
  }, []);

  // Age-based expiry tick (drops reads past the TTL window).
  useEffect(() => {
    const id = setInterval(() => {
      setTags((prev) => expireReads(prev, Date.now(), READ_TTL_SECONDS));
    }, EXPIRY_TICK_MS);
    return () => clearInterval(id);
  }, []);

  const reads = useMemo(() => [...tags.values()], [tags]);
  const readerCount = useMemo(() => new Set(reads.map((r) => r.readerKey)).size, [reads]);

  return { reads, status, error, readerCount };
}
