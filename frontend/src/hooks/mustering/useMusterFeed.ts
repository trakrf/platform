// useMusterFeed — owns the mustering SSE connection lifecycle (TRA-978 POC).
//
// Opens the org-scoped muster stream (openMusterStream) while mounted and pipes
// its parsed frames into musterStore (the reducer target). Keyed on the active
// org id: switching orgs mints a new JWT (and a new server-side org scope), so
// the old stream MUST tear down and reopen — otherwise it keeps delivering the
// previous org's presence/event frames until a page refresh (the same hazard
// useReaderFeed documents for the reads feed).
//
// Stream lifecycle state is fully closure-scoped inside openMusterStream, so a
// StrictMode double-mount or rapid remount can't alias one connection's state
// onto another's.

import { useEffect } from 'react';
import { openMusterStream } from '@/stores/musterStore';
import { useMusterStore } from '@/stores';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';

export function useMusterFeed(): void {
  const activeOrgId = useOrgStore((s) => s.currentOrg?.id);

  useEffect(() => {
    const store = useMusterStore.getState();
    store.setConnection('connecting');

    const handle = openMusterStream({
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
          const store = useMusterStore.getState();
          store.setConnection('live');
          // Re-arm the pre-snapshot presence gate for this fresh connection
          // (BUG C-1) and rely on the backend's first `snapshot` frame for the
          // authoritative state. We deliberately do NOT call refreshStatus() here:
          // racing a separate GET /status against the stream is exactly what let a
          // stale presence frame win and stick (the 4/Office-only freeze). The
          // server sends a snapshot as the first framed event on every connect.
          store.resetConnection();
        },
        onRetry: () => useMusterStore.getState().setConnection('connecting'),
        onFrames: (frames) => {
          const apply = useMusterStore.getState().applyFrame;
          for (const f of frames) apply(f);
        },
        onError: (err) => {
          useMusterStore
            .getState()
            .setStreamError(err instanceof Error ? err.message : 'stream error');
        },
      },
    });

    return () => {
      handle.close();
      useMusterStore.getState().setConnection('idle');
    };
  }, [activeOrgId]);
}
