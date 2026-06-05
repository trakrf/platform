// LiveReadsFeed — the reusable reader live-feed (TRA-931, extracted from the
// TRA-902 LiveReadsScreen). Renders the connection status, coverage stat strip
// and the firehose table for the org's tag reads. Owns the once-per-second tick
// so the Age column and row coloring advance even between reads.
//
// One component, two mounts:
//   - global Live Reads page  — <LiveReadsFeed />               (whole org feed)
//   - reader-scoped panel      — <LiveReadsFeed filterReaderKey={key} compact />
//
// Filtering is delegated to useReaderFeed; this component never talks to the
// stream directly, so the scoped panel and the global page share one feed.

import { useEffect, useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useReaderFeed } from '@/hooks/readerfeed/useReaderFeed';
import { ageSeconds, ageBandClass, READ_TTL_SECONDS } from '@/lib/readerfeed/store';
import type { ReaderFeedStatus } from '@/types/readerfeed';

const STATUS_LABEL: Record<ReaderFeedStatus, string> = {
  connecting: 'Connecting…',
  connected: 'Connected',
  error: 'Error',
  closed: 'Disconnected',
};

const STATUS_DOT: Record<ReaderFeedStatus, string> = {
  connecting: 'bg-amber-400 animate-pulse',
  connected: 'bg-green-500',
  error: 'bg-red-500',
  closed: 'bg-gray-400',
};

export interface LiveReadsFeedProps {
  /** Scope the feed to a single reader's key. Omit for the whole org feed. */
  filterReaderKey?: string;
  /**
   * Tighter layout for an embedded panel: drops the (always-one) Readers stat
   * and caps the table height. The global page leaves this off.
   */
  compact?: boolean;
}

export function LiveReadsFeed({ filterReaderKey, compact = false }: LiveReadsFeedProps) {
  const { reads, status, error, readerCount } = useReaderFeed(filterReaderKey);

  // Re-render once per second so the live Age column and row coloring advance
  // even between reads.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const sorted = [...reads].sort((a, b) => b.receivedAt - a.receivedAt);
  const rssiValues = reads.map((r) => r.rssi).filter((v) => v !== 0);
  const rssiRange =
    rssiValues.length > 0 ? `${Math.min(...rssiValues)} … ${Math.max(...rssiValues)} dBm` : '—';

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-end text-sm text-gray-600 dark:text-gray-300">
        <span className={`inline-block w-2.5 h-2.5 rounded-full mr-2 ${STATUS_DOT[status]}`} />
        {STATUS_LABEL[status]}
      </div>

      {/* Coverage stat strip */}
      <div className={`grid gap-3 ${compact ? 'grid-cols-3' : 'grid-cols-2 sm:grid-cols-4'}`}>
        <Stat label="Tags in view" value={String(reads.length)} />
        {!compact && <Stat label="Readers" value={String(readerCount)} />}
        <Stat label="RSSI range" value={rssiRange} />
        <Stat label="Window" value={`${READ_TTL_SECONDS}s`} />
      </div>

      <div
        className={`overflow-auto border border-gray-200 dark:border-gray-700 rounded-lg ${
          compact ? 'max-h-72' : 'flex-1'
        }`}
      >
        {status === 'error' ? (
          <div className="flex items-start gap-3 p-4 text-sm text-red-700 dark:text-red-400">
            <AlertTriangle className="w-5 h-5 shrink-0" />
            <div>
              <p className="font-medium">Could not connect to the reader feed.</p>
              <p className="text-gray-500 dark:text-gray-400 break-all">{error}</p>
            </div>
          </div>
        ) : reads.length === 0 ? (
          <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
            {status === 'connected'
              ? 'Connected — waiting for reads…'
              : 'Connecting to the reader feed…'}
          </p>
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-gray-50 dark:bg-gray-700">
              <tr className="text-left text-xs font-semibold uppercase tracking-wider text-gray-700 dark:text-gray-300 border-b border-gray-200 dark:border-gray-600">
                <th className="py-2.5 px-3">EPC</th>
                <th className="px-3">Reader</th>
                <th className="px-3">Capture Point</th>
                <th className="px-3 text-right">Antenna</th>
                <th className="px-3 text-right">RSSI</th>
                <th className="px-3 text-right">Age</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((r) => {
                const age = ageSeconds(r, now);
                return (
                  <tr
                    key={r.id}
                    className={`border-b border-gray-100 dark:border-gray-800 transition-colors ${ageBandClass(age)}`}
                  >
                    <td className="py-2 px-3 font-mono text-xs text-gray-900 dark:text-gray-100">{r.epc}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{r.readerKey}</td>
                    <td className="px-3 text-gray-700 dark:text-gray-300">{r.capturePointName || '—'}</td>
                    <td className="px-3 text-right text-gray-700 dark:text-gray-300">{r.antennaPort}</td>
                    <td className="px-3 text-right font-mono text-gray-700 dark:text-gray-300">
                      {r.rssi === 0 ? '—' : `${r.rssi}`}
                    </td>
                    <td className="px-3 text-right tabular-nums text-gray-600 dark:text-gray-400">{age}s</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2">
      <div className="text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">{label}</div>
      <div className="text-lg font-semibold text-gray-900 dark:text-white">{value}</div>
    </div>
  );
}
