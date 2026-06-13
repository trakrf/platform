// LiveReadsFeed — the reusable tag-presence inventory (TRA-936/937), modeled on
// Impinj ItemTest's Inventory view: one row per present tag with read count,
// RSSI aggregates (last/avg/min/max), first/last seen and a live "age" that
// drives a smooth staleness gradient. A header session timer and a footer
// (Tags in view + Read Rate) mirror ItemTest's chrome.
//
// One component, two mounts:
//   - global Live Reads page  — <LiveReadsFeed />               (whole org feed)
//   - reader-scoped panel      — <LiveReadsFeed filterReaderKey={key} compact />
//
// Presence + aggregation are server-authoritative (useReaderFeed reduces SSE
// deltas at the finest (reader,epc,antenna) granularity). This component owns the
// VIEW controls (TRA-937): filter, sortable headers, pause, clear, and the
// antenna aggregate/split toggle. The aggregate/split fold, filter and sort are
// pure transforms over the live map, so they are instant and stateless; pause
// freezes the source rows without dropping the stream, and clear reconnects to
// zero the server's per-session counts. The gradient and age recompute locally
// on a 1s tick from server `lastSeen`, so the fade costs zero network.

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Pause, Play, Trash2 } from 'lucide-react';
import { useReaderFeed } from '@/hooks/readerfeed/useReaderFeed';
import { ageSeconds, filterTags, gradientBackground, sortRows, toDisplayRows } from '@/lib/readerfeed/store';
import type { DisplayRow, ReaderFeedStatus, SortKey, SortState, TagState } from '@/types/readerfeed';

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

// Default to the stable "natural" order (key: null): rows keep first-seen order
// and don't churn as reads stream in (TRA-992 — Keypr's reads view). A column
// sort is opt-in via a header click.
const DEFAULT_SORT: SortState = { key: null, dir: 'asc' };

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
  const { tags, status, error, readerCount, readRate, reconnect } = useReaderFeed(filterReaderKey);

  // View controls (TRA-937). All client-side over the live presence map.
  const [filterText, setFilterText] = useState('');
  const [antennaFilter, setAntennaFilter] = useState<number | null>(null);
  const [split, setSplit] = useState(false); // default = aggregate "overall" view
  const [sort, setSort] = useState<SortState>(DEFAULT_SORT);
  // Pause snapshots the source rows (and the clock) so the rendered table stops
  // applying deltas without dropping the stream; filter/sort/aggregate still work
  // on the frozen set. Resume re-syncs to the live feed.
  const [frozen, setFrozen] = useState<{ tags: TagState[]; now: number } | null>(null);
  const paused = frozen !== null;

  // Re-render once per second so the live Age column, gradient and session timer
  // advance even between reads.
  const [now, setNow] = useState(() => Date.now());
  const startedAt = useRef(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const sourceTags = frozen?.tags ?? tags;
  const displayNow = frozen?.now ?? now;

  const rows = useMemo(
    () =>
      sortRows(
        toDisplayRows(filterTags(sourceTags, { text: filterText, antenna: antennaFilter }), !split),
        sort,
      ),
    [sourceTags, filterText, antennaFilter, split, sort],
  );

  // Antenna ports actually present (live), for the filter dropdown.
  const antennaOptions = useMemo(
    () => [...new Set(tags.map((t) => t.antennaPort))].sort((a, b) => a - b),
    [tags],
  );

  const rssiValues = rows.map((r) => r.lastRssi).filter((v) => v !== 0);
  const rssiRange =
    rssiValues.length > 0 ? `${Math.min(...rssiValues)} … ${Math.max(...rssiValues)} dBm` : '—';

  const toggleSort = (key: SortKey) =>
    setSort((prev) =>
      prev.key === key
        ? { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'epc' || key === 'readerKey' ? 'asc' : 'desc' },
    );

  const togglePause = () => setFrozen(paused ? null : { tags, now });

  const clear = () => {
    setFrozen(null);
    reconnect(); // fresh server session ⇒ counts restart at zero
  };

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
        <Stat label="Tags in view" value={String(rows.length)} />
        {!compact && <Stat label="Readers" value={String(readerCount)} />}
        <Stat label="RSSI range" value={rssiRange} />
        <Stat label="Read rate" value={readRate > 0 ? `${readRate.toFixed(1)}/s` : '—'} />
      </div>

      {status !== 'error' && (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <input
            type="search"
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
            placeholder="Filter EPC / tag…"
            aria-label="Filter tags"
            className="flex-1 min-w-[8rem] rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-1.5 text-gray-900 dark:text-gray-100"
          />
          <select
            value={antennaFilter ?? ''}
            onChange={(e) => setAntennaFilter(e.target.value === '' ? null : Number(e.target.value))}
            aria-label="Antenna filter"
            className="rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-2 py-1.5 text-gray-900 dark:text-gray-100"
          >
            <option value="">All antennas</option>
            {antennaOptions.map((a) => (
              <option key={a} value={a}>
                Ant {a}
              </option>
            ))}
          </select>
          <label className="flex items-center gap-1.5 text-gray-700 dark:text-gray-300 select-none">
            <input
              type="checkbox"
              checked={split}
              onChange={(e) => setSplit(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600"
            />
            Split by antenna
          </label>
          <button
            type="button"
            onClick={togglePause}
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 dark:border-gray-600 px-3 py-1.5 text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700"
          >
            {paused ? <Play className="w-4 h-4" /> : <Pause className="w-4 h-4" />}
            {paused ? 'Resume' : 'Pause'}
          </button>
          <button
            type="button"
            onClick={clear}
            className="inline-flex items-center gap-1.5 rounded-md border border-gray-300 dark:border-gray-600 px-3 py-1.5 text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700"
          >
            <Trash2 className="w-4 h-4" />
            Clear
          </button>
        </div>
      )}

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
        ) : rows.length === 0 ? (
          <p className="p-4 text-sm text-gray-600 dark:text-gray-400">
            {status === 'connected'
              ? sourceTags.length === 0
                ? 'Connected — waiting for reads…'
                : 'No tags match the current filter.'
              : 'Connecting to the reader feed…'}
          </p>
        ) : (
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10 bg-gray-50 dark:bg-gray-700">
              <tr className="text-left text-xs font-semibold uppercase tracking-wider text-gray-700 dark:text-gray-300 border-b border-gray-200 dark:border-gray-600">
                <SortHeader label="EPC" sortKey="epc" sort={sort} onSort={toggleSort} className="py-2.5 px-3" />
                <SortHeader label="Reader" sortKey="readerKey" sort={sort} onSort={toggleSort} className="px-3" />
                <SortHeader label="Ant" sortKey="antennaPort" sort={sort} onSort={toggleSort} align="right" />
                <SortHeader label="Reads" sortKey="readCount" sort={sort} onSort={toggleSort} align="right" />
                <SortHeader label="Last RSSI" sortKey="lastRssi" sort={sort} onSort={toggleSort} align="right" />
                <SortHeader label="Avg" sortKey="rssiAvg" sort={sort} onSort={toggleSort} align="right" />
                {!compact && (
                  <SortHeader label="Min" sortKey="rssiMin" sort={sort} onSort={toggleSort} align="right" />
                )}
                {!compact && (
                  <SortHeader label="Max" sortKey="rssiMax" sort={sort} onSort={toggleSort} align="right" />
                )}
                <SortHeader label="Age" sortKey="lastSeen" sort={sort} onSort={toggleSort} align="right" />
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <Row key={r.rowKey} row={r} now={displayNow} compact={compact} />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

const SORT_GLYPH = { asc: '▲', desc: '▼' } as const;

function SortHeader({
  label,
  sortKey,
  sort,
  onSort,
  align = 'left',
  className,
}: {
  label: string;
  sortKey: SortKey;
  sort: SortState;
  onSort: (key: SortKey) => void;
  align?: 'left' | 'right';
  className?: string;
}) {
  const active = sort.key === sortKey;
  return (
    <th className={className ?? `px-3 ${align === 'right' ? 'text-right' : ''}`}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`inline-flex items-center gap-1 uppercase tracking-wider hover:text-gray-900 dark:hover:text-white ${
          align === 'right' ? 'flex-row-reverse' : ''
        } ${active ? 'text-gray-900 dark:text-white' : ''}`}
      >
        <span>{label}</span>
        {active && <span aria-hidden className="text-[0.6rem]">{SORT_GLYPH[sort.dir]}</span>}
      </button>
    </th>
  );
}

function Row({ row, now, compact }: { row: DisplayRow; now: number; compact: boolean }) {
  const age = ageSeconds(row.lastSeen, now);
  const cell = 'px-3 text-right tabular-nums text-gray-700 dark:text-gray-300';
  return (
    <tr
      className="border-b border-gray-100 dark:border-gray-800 transition-colors"
      style={{ backgroundColor: gradientBackground(age) }}
      title={`First seen ${formatClock(row.firstSeen)} · Last seen ${formatClock(row.lastSeen)}`}
    >
      <td className="py-2 px-3 font-mono text-xs text-gray-900 dark:text-gray-100">
        {row.alias || row.epc}
      </td>
      <td className="px-3 text-gray-700 dark:text-gray-300">{row.readerKey}</td>
      <td className={cell}>{row.antennaLabel}</td>
      <td className={`${cell} font-medium`}>{row.readCount}</td>
      <td className={`${cell} font-mono`}>{row.lastRssi === 0 ? '—' : row.lastRssi}</td>
      <td className={`${cell} font-mono`}>{row.rssiAvg === 0 ? '—' : row.rssiAvg}</td>
      {!compact && <td className={`${cell} font-mono`}>{row.rssiMin}</td>}
      {!compact && <td className={`${cell} font-mono`}>{row.rssiMax}</td>}
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
