// LiveReadsScreen — global reader live-view / coverage diagnostic (TRA-902).
//
// Shows a live tag-presence inventory across every fixed reader (TRA-936),
// whether or not a tag maps to a registered asset, so an operator can tune
// antenna placement and the RSSI threshold. Presence + read-count/RSSI
// aggregation are server-authoritative, streamed over the backend SSE proxy and
// org-filtered server-side — the browser never connects to the broker (TRA-924).
//
// As of TRA-931 the feed UI lives in the reusable <LiveReadsFeed>; this screen
// is just the page chrome. The same component, scoped to one reader, also backs
// the live-read panel inside reader edit.

import { useEffect } from 'react';
import { useUIStore } from '@/stores';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { LiveReadsFeed } from '@/components/readerfeed/LiveReadsFeed';
import { PaidGate } from '@/components/entitlement';

export default function LiveReadsScreen() {
  const { setActiveTab } = useUIStore();

  useEffect(() => {
    setActiveTab('live-reads');
  }, [setActiveTab]);

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="mb-4">
          <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Live Reads</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            Live tag inventory — every tag in range (registered or not) with read count and RSSI,
            for antenna &amp; RSSI coverage tuning. A tag drops out after 30s of silence.
          </p>
        </div>

        <PaidGate surface="live-reads" panel className="flex-1 min-h-0">
          <div className="h-full">
            <LiveReadsFeed />
          </div>
        </PaidGate>
      </div>
    </ProtectedRoute>
  );
}
