// LiveReadsScreen — global reader live-view / coverage diagnostic (TRA-902).
//
// Shows the raw firehose of tag reads off every fixed reader, whether or not a
// read maps to a registered asset, so an operator can tune antenna placement
// and the RSSI threshold. Reads arrive over the backend SSE proxy, org-filtered
// server-side — the browser never connects to the broker (TRA-924).
//
// As of TRA-931 the feed UI lives in the reusable <LiveReadsFeed>; this screen
// is just the page chrome. The same component, scoped to one reader, also backs
// the live-read panel inside reader edit.

import { useEffect } from 'react';
import { useUIStore } from '@/stores';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { LiveReadsFeed } from '@/components/readerfeed/LiveReadsFeed';
import { READ_TTL_SECONDS } from '@/lib/readerfeed/store';

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
            Raw reader feed — every tag read (registered or not) for antenna &amp; RSSI coverage
            tuning. Reads expire after {READ_TTL_SECONDS}s of silence.
          </p>
        </div>

        <div className="flex-1 min-h-0">
          <LiveReadsFeed />
        </div>
      </div>
    </ProtectedRoute>
  );
}
