// LiveReadsFeed — the reusable tag-presence inventory (TRA-936), modeled on
// Impinj ItemTest's Inventory view: one row per present (reader,epc) tag with
// read count, RSSI aggregates (last/avg/min/max), first/last seen and a live
// "age" that drives a smooth staleness gradient. A header session timer and a
// footer (Tags in view + Read Rate) mirror ItemTest's chrome.
//
// One component, two mounts:
//   - global Live Reads page  — <LiveReadsFeed />               (whole org feed)
//   - reader-scoped panel      — <LiveReadsFeed filterReaderKey={key} compact />
//
// Presence + aggregation are server-authoritative (useReaderFeed reduces SSE
// deltas); this component only renders. The gradient and age recompute locally
// on a 1s tick from server `lastSeen`, so the fade costs zero network.

import { useEffect, useRef, useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useReaderFeed } from '@/hooks/readerfeed/useReaderFeed';
import { ageSeconds, gradientBackground } from '@/lib/readerfeed/store';
import type { ReaderFeedStatus, TagState } from '@/types/readerfeed';

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
   * and the secondary RSSI columns, and caps the table height.
   */
  compact?: boolean;
}

export function LiveReadsFeed({ filterReaderKey, compact = false }: LiveReadsFeedProps) {
  const { tags, status, error, readerCount, readRate } = useReaderFeed(filterReaderKey);

  // Re-render once per second so the live Age column, gradient and session timer
  // advance even between reads.
  const [now, setNow] = useState(() => Date.now());
  const startedAt = useRef(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const sorted = [...tags].sort((a, b) => b.lastSeen - a.lastSeen);
  const rssiValues = tags.map((t) => t.lastRssi).filter((v) => v !== 0);
  const rssiRange =
    rssiValues.length > 0 ? `${Math.min(...rssiValues)} … ${Math.max(...rssiValues)} dBm` : '—';

  return (
    <div className={`flex flex-col gap-4 ${compact ? '' : 'h-full min-h-0'}`}>
      <div className="flex items-center justify-between text-sm text-gray-600 dark:text-gray-300">
        <span className="font-mono tabular-nums text-gray-500 dark:text-gray-400">
          {formatElapsed(now - startedAt.current)}
        </span>
        <span className="flex items-center">
          <span className={`inline-block w-2.5 h-2.5 rounded-full mr-2 ${STATUS_DOT[status]}`} />
          {STATUS_LABEL[status]}
        </span>
      </div>

      {/* Coverage stat strip */}
      <div className={`grid gap-3 ${compact ? 'grid-cols-3' : 'grid-cols-2 sm:grid-cols-4'}`}>
        <Stat label="Tags in view" value={String(tags.length)} />
        {!compact && <Stat label="Readers" value={String(readerCount)} />}
        <Stat label="RSSI range" value={rssiRange} />
        <Stat label="Read rate" value={readRate > 0 ? `${readRate.toFixed(1)}/s` : '—'} />
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
        ) : tags.length === 0 ? (
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
                <th className="px-3 text-right">Ant</th>
                <th className="px-3 text-right">Reads</th>
                <th className="px-3 text-right">Last RSSI</th>
                <th className="px-3 text-right">Avg</th>
                {!compact && <th className="px-3 text-right">Min</th>}
                {!compact && <th className="px-3 text-right">Max</th>}
                <th className="px-3 text-right">Age</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((t) => (
                <Row key={`${t.readerKey} ${t.epc}`} tag={t} now={now} compact={compact} />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function Row({ tag, now, compact }: { tag: TagState; now: number; compact: boolean }) {
  const age = ageSeconds(tag.lastSeen, now);
  const cell = 'px-3 text-right tabular-nums text-gray-700 dark:text-gray-300';
  return (
    <tr
      className="border-b border-gray-100 dark:border-gray-800 transition-colors"
      style={{ backgroundColor: gradientBackground(age) }}
      title={`First seen ${formatClock(tag.firstSeen)} · Last seen ${formatClock(tag.lastSeen)}`}
    >
      <td className="py-2 px-3 font-mono text-xs text-gray-900 dark:text-gray-100">
        {tag.alias || tag.epc}
      </td>
      <td className="px-3 text-gray-700 dark:text-gray-300">{tag.readerKey}</td>
      <td className={cell}>{tag.antennaPort}</td>
      <td className={`${cell} font-medium`}>{tag.readCount}</td>
      <td className={`${cell} font-mono`}>{tag.lastRssi === 0 ? '—' : tag.lastRssi}</td>
      <td className={`${cell} font-mono`}>{tag.rssiAvg === 0 ? '—' : tag.rssiAvg}</td>
      {!compact && <td className={`${cell} font-mono`}>{tag.rssiMin}</td>}
      {!compact && <td className={`${cell} font-mono`}>{tag.rssiMax}</td>}
      <td className={`${cell} text-gray-600 dark:text-gray-400`}>{age}s</td>
    </tr>
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

/** mm:ss elapsed for the session timer (ItemTest's run stopwatch). */
function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

/** Local wall-clock HH:MM:SS for first/last-seen tooltips. */
function formatClock(epochMs: number): string {
  return new Date(epochMs).toLocaleTimeString();
}
