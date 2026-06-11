// MusterReport — list of past muster events + per-event detail + CSV export (TRA-978 Phase 4).
//
// List view: past+active events with started_at, duration, status, accounted/expected counts.
// Detail view: counts, per-zone table (expected/accounted/cleared_at), muster-point arrivals,
//   per-person final status list, unlock log (if metadata.unlocks present).
// Export: client-side CSV download — per-person rows + summary, filename muster-report-<id>.csv.

import { useCallback, useEffect, useState } from 'react';
import { Download, ChevronLeft, Loader2 } from 'lucide-react';
import { useMusterStore } from '@/stores';
import type {
  MusterEvent,
  MusterEntry,
  MusterUnlockEntry,
} from '@/types/mustering';
import { STATUS_LABEL, STATUS_CLASS } from './helpers';

// ---- CSV export (pure function — testable in isolation) ----

/** Escape a CSV cell value: wraps in double-quotes if it contains a comma, quote, or newline. */
export function csvCell(v: string | number | null | undefined): string {
  const s = v == null ? '' : String(v);
  if (/[",\n\r]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
  return s;
}

/** Row delimiter: CRLF per RFC 4180. */
const CRLF = '\r\n';

export interface CsvRow {
  person: string;
  status: string;
  expected_zone: string;
  muster_location: string;
  first_muster_seen_at: string;
  verified_at: string;
  marked_safe_at: string;
  note: string;
}

/** Build per-person CSV rows from a list of entries. */
export function buildCsvRows(entries: MusterEntry[], zoneNameMap: Map<number, string>): CsvRow[] {
  return entries.map((e) => ({
    person: e.label,
    status: e.status,
    expected_zone: e.expected_location_id != null
      ? (zoneNameMap.get(e.expected_location_id) ?? String(e.expected_location_id))
      : '',
    muster_location: e.muster_location_id != null
      ? (zoneNameMap.get(e.muster_location_id) ?? String(e.muster_location_id))
      : '',
    first_muster_seen_at: e.first_muster_seen_at ?? '',
    verified_at: e.verified_at ?? '',
    marked_safe_at: e.marked_safe_at ?? '',
    note: e.marked_safe_note ?? '',
  }));
}

/**
 * Generate the full CSV string for a muster event report.
 * Pure function — no side effects, easily unit-tested.
 */
export function buildReportCsv(
  event: MusterEvent,
  zoneNameMap: Map<number, string>,
): string {
  const lines: string[] = [];

  // Summary header block
  lines.push(`# Muster Report — Event #${event.id}`);
  lines.push(`# Started,${csvCell(event.started_at)}`);
  lines.push(`# Ended,${csvCell(event.ended_at ?? '')}`);
  lines.push(`# Status,${csvCell(event.status)}`);
  if (event.report) {
    const r = event.report;
    const mins = Math.floor(r.total_seconds / 60);
    const secs = r.total_seconds % 60;
    lines.push(`# Duration,${mins}m ${secs}s`);
    lines.push(
      `# Counts,Expected ${r.counts.expected} / Verified ${r.counts.verified} / At muster ${r.counts.at_muster} / Safe ${r.counts.safe_manual} / Missing ${r.counts.missing}`,
    );
  }
  lines.push('');

  // Per-person rows
  const personHeader = 'Person,Status,Expected Zone,Muster Location,First Seen At Muster,Verified At,Marked Safe At,Note';
  lines.push(personHeader);
  for (const row of buildCsvRows(event.entries ?? [], zoneNameMap)) {
    lines.push(
      [
        csvCell(row.person),
        csvCell(row.status),
        csvCell(row.expected_zone),
        csvCell(row.muster_location),
        csvCell(row.first_muster_seen_at),
        csvCell(row.verified_at),
        csvCell(row.marked_safe_at),
        csvCell(row.note),
      ].join(','),
    );
  }

  // Zone breakdown (if report present)
  if (event.report?.zones?.length) {
    lines.push('');
    lines.push('# Zone Breakdown');
    lines.push('Zone,Expected,Accounted,Cleared At');
    for (const z of event.report.zones) {
      lines.push(
        [
          csvCell(z.name),
          csvCell(z.expected),
          csvCell(z.accounted),
          csvCell(z.cleared_at ?? ''),
        ].join(','),
      );
    }
  }

  // Muster-point arrivals
  if (event.report?.muster_points?.length) {
    lines.push('');
    lines.push('# Muster Point Arrivals');
    lines.push('Muster Point,Arrivals');
    for (const mp of event.report.muster_points) {
      lines.push([csvCell(mp.name), csvCell(mp.arrivals)].join(','));
    }
  }

  return lines.join(CRLF);
}

/** Trigger a client-side download of `content` as a CSV file. */
function downloadCsv(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// ---- Formatting helpers ----

function formatDuration(event: MusterEvent): string {
  if (event.report?.total_seconds != null) {
    const s = event.report.total_seconds;
    const mins = Math.floor(s / 60);
    const secs = s % 60;
    return `${mins}m ${secs}s`;
  }
  if (event.ended_at) {
    const secs = Math.max(
      0,
      Math.floor((new Date(event.ended_at).getTime() - new Date(event.started_at).getTime()) / 1000),
    );
    return `${Math.floor(secs / 60)}m ${secs % 60}s`;
  }
  return '—';
}

function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

// ---- Component ----

export default function MusterReport() {
  const fetchEvents = useMusterStore((s) => s.fetchEvents);
  const fetchEvent = useMusterStore((s) => s.fetchEvent);

  const [events, setEvents] = useState<MusterEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<MusterEvent | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const loadList = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const list = await fetchEvents();
      setEvents(list);
    } catch {
      setError('Failed to load muster events.');
    } finally {
      setLoading(false);
    }
  }, [fetchEvents]);

  useEffect(() => {
    void loadList();
  }, [loadList]);

  const handleSelect = async (ev: MusterEvent) => {
    setLoadingDetail(true);
    setSelected(null);
    try {
      const detail = await fetchEvent(ev.id);
      setSelected(detail);
    } catch {
      setError('Failed to load event detail.');
    } finally {
      setLoadingDetail(false);
    }
  };

  const handleBack = () => {
    setSelected(null);
  };

  const handleExportCsv = (ev: MusterEvent) => {
    // Build zone name map from report zones + muster points.
    const zoneNameMap = new Map<number, string>();
    for (const z of ev.report?.zones ?? []) zoneNameMap.set(z.location_id, z.name);
    for (const mp of ev.report?.muster_points ?? []) zoneNameMap.set(mp.location_id, mp.name);
    const csv = buildReportCsv(ev, zoneNameMap);
    downloadCsv(`muster-report-${ev.id}.csv`, csv);
  };

  // ---- Render: detail view ----
  if (loadingDetail) {
    return (
      <div className="flex items-center justify-center p-8 text-gray-500 dark:text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" /> Loading event detail…
      </div>
    );
  }

  if (selected) {
    const unlocks = selected.metadata?.unlocks as MusterUnlockEntry[] | undefined;
    return (
      <div className="space-y-6" data-testid="muster-report-detail">
        {/* Back button + export */}
        <div className="flex items-center justify-between flex-wrap gap-2">
          <button
            onClick={handleBack}
            className="inline-flex items-center gap-1 text-sm text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-gray-100"
            data-testid="muster-report-back"
          >
            <ChevronLeft className="w-4 h-4" />
            Back to list
          </button>
          <button
            onClick={() => handleExportCsv(selected)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border border-gray-300 dark:border-gray-600 text-sm text-gray-700 dark:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-700"
            data-testid="muster-report-export-csv"
          >
            <Download className="w-4 h-4" />
            Export CSV
          </button>
        </div>

        {/* Summary header */}
        <div>
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-1">
            Event #{selected.id}
          </h2>
          <div className="text-sm text-gray-500 dark:text-gray-400 space-y-0.5">
            <div>Started: {fmtTime(selected.started_at)}</div>
            <div>Ended: {fmtTime(selected.ended_at)}</div>
            <div>Duration: {formatDuration(selected)}</div>
            <div>
              Status:{' '}
              <span className="capitalize font-medium text-gray-700 dark:text-gray-200">
                {selected.status}
              </span>
            </div>
          </div>
        </div>

        {/* Counts */}
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
          <CountCard label="Expected" value={selected.counts.expected} />
          <CountCard label="Missing" value={selected.counts.missing} tone="danger" />
          <CountCard label="At muster" value={selected.counts.at_muster} tone="warn" />
          <CountCard label="Verified" value={selected.counts.verified} tone="ok" />
          <CountCard label="Safe" value={selected.counts.safe_manual} tone="ok" />
        </div>

        {/* Per-zone breakdown */}
        {selected.report?.zones && selected.report.zones.length > 0 && (
          <section>
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2">
              Zone breakdown
            </h3>
            <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 dark:bg-gray-800/60">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Zone</th>
                    <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Expected</th>
                    <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Accounted</th>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Cleared at</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {selected.report.zones.map((z) => (
                    <tr key={z.location_id} className="bg-white dark:bg-gray-900">
                      <td className="px-4 py-2 text-gray-900 dark:text-gray-100">{z.name}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">{z.expected}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">{z.accounted}</td>
                      <td className="px-4 py-2 text-gray-600 dark:text-gray-400">
                        {z.cleared_at ? new Date(z.cleared_at).toLocaleTimeString() : '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {/* Muster-point arrivals */}
        {selected.report?.muster_points && selected.report.muster_points.length > 0 && (
          <section>
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2">
              Muster point arrivals
            </h3>
            <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 dark:bg-gray-800/60">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Muster point</th>
                    <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Arrivals</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {selected.report.muster_points.map((mp) => (
                    <tr key={mp.location_id} className="bg-white dark:bg-gray-900">
                      <td className="px-4 py-2 text-gray-900 dark:text-gray-100">{mp.name}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">{mp.arrivals}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {/* Per-person final status */}
        {selected.entries && selected.entries.length > 0 && (
          <section>
            <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2">
              Per-person final status
            </h3>
            <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 dark:bg-gray-800/60">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Person</th>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">First at muster</th>
                    <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Note</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                  {selected.entries.map((entry) => (
                    <tr key={entry.id} className="bg-white dark:bg-gray-900">
                      <td className="px-4 py-2 text-gray-900 dark:text-gray-100">{entry.label}</td>
                      <td className="px-4 py-2">
                        <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${STATUS_CLASS[entry.status]}`}>
                          {STATUS_LABEL[entry.status]}
                        </span>
                      </td>
                      <td className="px-4 py-2 text-xs text-gray-500 dark:text-gray-400">
                        {entry.first_muster_seen_at ? fmtTime(entry.first_muster_seen_at) : '—'}
                      </td>
                      <td className="px-4 py-2 text-xs text-gray-500 dark:text-gray-400">
                        {entry.marked_safe_note || '—'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {/* Unlock log */}
        {unlocks && unlocks.length > 0 && (
          <section>
            <h3 className="text-sm font-semibold text-amber-700 dark:text-amber-300 mb-2">
              Break-glass location reveals
            </h3>
            <div className="rounded-lg border border-amber-200 dark:border-amber-800 overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-amber-50 dark:bg-amber-900/20">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium text-amber-700 dark:text-amber-300">User</th>
                    <th className="px-4 py-2 text-left font-medium text-amber-700 dark:text-amber-300">At</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-amber-100 dark:divide-amber-900/40">
                  {unlocks.map((u, i) => (
                    <tr key={i} className="bg-white dark:bg-gray-900">
                      <td className="px-4 py-2 text-gray-900 dark:text-gray-100">{u.email}</td>
                      <td className="px-4 py-2 text-xs text-gray-500 dark:text-gray-400">
                        {fmtTime(u.at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}
      </div>
    );
  }

  // ---- Render: list view ----
  return (
    <div className="space-y-4" data-testid="muster-report-list">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Muster history</h2>

      {error && (
        <div className="rounded-md bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 px-4 py-2 text-sm text-red-700 dark:text-red-300">
          {error}
        </div>
      )}

      {loading && (
        <div className="flex items-center justify-center p-8 text-gray-500 dark:text-gray-400">
          <Loader2 className="w-5 h-5 animate-spin mr-2" /> Loading…
        </div>
      )}

      {!loading && events.length === 0 && !error && (
        <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
          No muster events yet. Activate a drill from the Dashboard.
        </div>
      )}

      {!loading && events.length > 0 && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800/60">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Started</th>
                <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Expected</th>
                <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Accounted</th>
                <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Duration</th>
                <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {events.map((ev) => {
                const accounted =
                  (ev.counts.verified ?? 0) +
                  (ev.counts.at_muster ?? 0) +
                  (ev.counts.safe_manual ?? 0);
                return (
                  <tr
                    key={ev.id}
                    className="bg-white dark:bg-gray-900 hover:bg-gray-50 dark:hover:bg-gray-800/50 cursor-pointer"
                    onClick={() => void handleSelect(ev)}
                    data-testid={`muster-report-row-${ev.id}`}
                  >
                    <td className="px-4 py-3 text-gray-900 dark:text-gray-100">
                      {fmtTime(ev.started_at)}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium capitalize ${
                          ev.status === 'active'
                            ? 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
                            : ev.status === 'completed'
                              ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300'
                              : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300'
                        }`}
                      >
                        {ev.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">
                      {ev.counts.expected}
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-gray-300">
                      {accounted}
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums text-gray-500 dark:text-gray-400">
                      {formatDuration(ev)}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          void handleSelect(ev);
                        }}
                        className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
                        data-testid={`muster-report-view-${ev.id}`}
                      >
                        View
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---- Internal sub-components ----

function CountCard({ label, value, tone }: { label: string; value: number; tone?: 'danger' | 'warn' | 'ok' }) {
  const color =
    tone === 'danger'
      ? 'text-red-600 dark:text-red-400'
      : tone === 'warn'
        ? 'text-amber-600 dark:text-amber-400'
        : tone === 'ok'
          ? 'text-green-600 dark:text-green-400'
          : 'text-gray-900 dark:text-gray-100';
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-2 text-center">
      <div className={`text-2xl font-semibold ${color}`}>{value}</div>
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
    </div>
  );
}
