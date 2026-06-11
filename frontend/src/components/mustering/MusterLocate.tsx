// MusterLocate — per-person last-known zone + live RSSI signal meter (TRA-978 Phase 4).
//
// Privacy gate: shows location data ONLY when a muster event is active. Outside
// of an active event a privacy notice is shown and nothing else.
//
// With an active event:
//   - Person picker (dropdown of person-assets, defaults to the deep-linked assetId).
//   - Last-known zone + last-seen age from the store's event entries.
//   - Live signal meter: subscribes /api/v1/reads/stream via the existing
//     openReadStream helper, filters client-side to the selected person's badge
//     EPC, renders per-reader RSSI bars with an age-fade gradient (same idiom
//     as LiveReadsFeed.tsx).

import { useEffect, useMemo, useState } from 'react';
import { ShieldOff } from 'lucide-react';
import { openReadStream } from '@/lib/readerfeed/stream';
import { ageSeconds, gradientBackground } from '@/lib/readerfeed/store';
import { API_BASE_URL } from '@/lib/api/client';
import { useAuthStore } from '@/stores/authStore';
import { useOrgStore } from '@/stores/orgStore';
import { useMusterStore } from '@/stores';
import { assetsApi } from '@/lib/api/assets';
import type { Asset } from '@/types/assets';
import type { TagState } from '@/types/readerfeed';

interface MusterLocateProps {
  /** Deep-link target from a Dashboard "Locate" action. */
  assetId?: number | null;
}

function relativeAge(isoString: string): string {
  const delta = Math.max(0, Math.floor((Date.now() - new Date(isoString).getTime()) / 1000));
  if (delta < 60) return `${delta}s ago`;
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  return `${Math.floor(delta / 3600)}h ago`;
}

export default function MusterLocate({ assetId }: MusterLocateProps) {
  const event = useMusterStore((s) => s.event);
  const zones = useMusterStore((s) => s.zones);
  const activeOrgId = useOrgStore((s) => s.currentOrg?.id);

  const isActive = event?.status === 'active';

  // Person roster (person-assets) — only fetch when there's an active event.
  const [persons, setPersons] = useState<Asset[]>([]);
  const [loadingPersons, setLoadingPersons] = useState(false);

  useEffect(() => {
    if (!isActive) { setPersons([]); return; }
    setLoadingPersons(true);
    assetsApi
      .list({ limit: 500 })
      .then(({ data }) => {
        setPersons(data.data.filter((a) => a.is_active && a.metadata?.person === true));
      })
      .catch(() => { /* ignore — privacy gate not loaded */ })
      .finally(() => setLoadingPersons(false));
  }, [isActive]);

  // Selected person id — default to the deep-link prop, then fall back to first.
  const [selectedId, setSelectedId] = useState<number | null>(assetId ?? null);

  // Update selectedId when the deep-link prop changes.
  useEffect(() => {
    if (assetId != null) setSelectedId(assetId);
  }, [assetId]);

  // If no explicit selection yet and we just loaded persons, pick the first.
  useEffect(() => {
    if (selectedId == null && persons.length > 0) {
      setSelectedId(persons[0].id);
    }
  }, [persons, selectedId]);

  const selectedPerson = useMemo(
    () => persons.find((p) => p.id === selectedId) ?? null,
    [persons, selectedId],
  );

  // Badge EPC for the selected person (uppercased to match backend).
  const badgeEpc = useMemo(() => {
    const tag = selectedPerson?.tags.find((t) => t.tag_type === 'rfid');
    return tag?.value.toUpperCase() ?? null;
  }, [selectedPerson]);

  // Zone name lookup.
  const zoneName = useMemo(() => {
    const m = new Map<number, string>();
    for (const z of zones) m.set(z.location_id, z.name);
    return m;
  }, [zones]);

  // Last-seen info from the active event's entries.
  const entryForSelected = useMemo(() => {
    if (!event?.entries || selectedId == null) return null;
    return event.entries.find((e) => e.asset_id === selectedId) ?? null;
  }, [event, selectedId]);

  // Live RSSI feed — raw reads stream filtered client-side to the badge EPC.
  const [rssiMap, setRssiMap] = useState<Map<string, TagState>>(new Map());
  const [feedStatus, setFeedStatus] = useState<'connecting' | 'connected' | 'error'>('connecting');
  const [now, setNow] = useState(() => Date.now());

  // 1s tick for age fade.
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  // Stream lifecycle — only open when there's an active event and a selected EPC.
  // Keyed on org + badgeEpc so it reconnects when those change.
  useEffect(() => {
    if (!isActive || !badgeEpc) {
      setRssiMap(new Map());
      return;
    }

    setRssiMap(new Map());
    setFeedStatus('connecting');

    const handle = openReadStream({
      baseURL: API_BASE_URL,
      getToken: () => useAuthStore.getState().token,
      onUnauthorized: async () => {
        try { return await useAuthStore.getState().refresh(); }
        catch { return false; }
      },
      callbacks: {
        onOpen: () => setFeedStatus('connected'),
        onError: () => setFeedStatus('error'),
        onEvents: (events) => {
          setRssiMap((prev) => {
            let next = prev;
            for (const ev of events) {
              if (ev.type === 'snapshot') {
                const filtered = new Map<string, TagState>();
                for (const t of ev.data.tags) {
                  if (t.epc.toUpperCase() === badgeEpc) {
                    filtered.set(`${t.readerKey}/${t.antennaPort}`, t);
                  }
                }
                next = filtered;
              } else if (ev.type === 'upsert') {
                if (ev.data.epc.toUpperCase() === badgeEpc) {
                  const key = `${ev.data.readerKey}/${ev.data.antennaPort}`;
                  const m = new Map(next);
                  m.set(key, ev.data);
                  next = m;
                }
              } else if (ev.type === 'leave') {
                if (ev.data.epc.toUpperCase() === badgeEpc) {
                  const key = `${ev.data.readerKey}/${ev.data.antennaPort}`;
                  const m = new Map(next);
                  m.delete(key);
                  next = m;
                }
              }
            }
            return next;
          });
        },
      },
    });

    return () => handle.close();
  }, [isActive, badgeEpc, activeOrgId]);

  const rssiRows = useMemo(() => [...rssiMap.values()], [rssiMap]);

  // --- Render ---

  if (!isActive) {
    return (
      <div
        className="rounded-lg border border-amber-200 dark:border-amber-800 bg-amber-50 dark:bg-amber-900/20 p-8 flex flex-col items-center gap-3 text-center"
        data-testid="muster-locate-privacy"
      >
        <ShieldOff className="w-8 h-8 text-amber-500" />
        <p className="font-medium text-amber-800 dark:text-amber-200">
          Per-person location is available only during an active muster event.
        </p>
        <p className="text-sm text-amber-700 dark:text-amber-300">
          Activate a muster drill from the Dashboard to unlock real-time location tracking.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="muster-locate">
      {/* Person picker */}
      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
          Person
        </label>
        {loadingPersons ? (
          <div className="text-sm text-gray-500 dark:text-gray-400">Loading roster…</div>
        ) : (
          <select
            value={selectedId ?? ''}
            onChange={(e) => setSelectedId(e.target.value ? Number(e.target.value) : null)}
            className="rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 px-3 py-2 text-sm text-gray-900 dark:text-gray-100 w-full max-w-xs"
            data-testid="muster-locate-person-picker"
          >
            <option value="">— select a person —</option>
            {persons.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        )}
      </div>

      {selectedPerson && (
        <>
          {/* Last-known zone (from store entry) */}
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4 space-y-1">
            <div className="text-xs uppercase tracking-wider text-gray-500 dark:text-gray-400">
              Last-known location
            </div>
            <div className="text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {entryForSelected?.last_seen_location_id != null
                ? (zoneName.get(entryForSelected.last_seen_location_id) ??
                    `Location #${entryForSelected.last_seen_location_id}`)
                : '—'}
            </div>
            {entryForSelected?.last_seen_at && (
              <div className="text-sm text-gray-500 dark:text-gray-400">
                {relativeAge(entryForSelected.last_seen_at)}
              </div>
            )}
            {!badgeEpc && (
              <div className="text-xs text-amber-600 dark:text-amber-400 mt-1">
                No badge assigned — assign an RFID badge on the Badges tab to enable live tracking.
              </div>
            )}
          </div>

          {/* Live RSSI signal meter */}
          {badgeEpc && (
            <div className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium text-gray-700 dark:text-gray-300">
                  Live signal — <span className="font-mono">{badgeEpc}</span>
                </span>
                <span className="flex items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                  <span
                    className={`w-2 h-2 rounded-full ${
                      feedStatus === 'connected'
                        ? 'bg-green-500'
                        : feedStatus === 'connecting'
                          ? 'bg-yellow-400 animate-pulse'
                          : 'bg-red-500'
                    }`}
                  />
                  {feedStatus === 'connected' ? 'Live' : feedStatus === 'connecting' ? 'Connecting…' : 'Error'}
                </span>
              </div>

              {rssiRows.length === 0 ? (
                <div className="text-sm text-gray-500 dark:text-gray-400 italic">
                  {feedStatus === 'connected'
                    ? 'No live reads for this badge — badge may be out of range.'
                    : 'Waiting for live feed…'}
                </div>
              ) : (
                <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden">
                  <table className="w-full text-sm">
                    <thead className="bg-gray-50 dark:bg-gray-800/60">
                      <tr>
                        <th className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-300">Reader</th>
                        <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Ant</th>
                        <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">RSSI</th>
                        <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Reads</th>
                        <th className="px-4 py-2 text-right font-medium text-gray-600 dark:text-gray-300">Age</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
                      {rssiRows.map((row) => {
                        const age = ageSeconds(row.lastSeen, now);
                        return (
                          <tr
                            key={`${row.readerKey}/${row.antennaPort}`}
                            className="transition-colors"
                            style={{ backgroundColor: gradientBackground(age) }}
                            data-testid="muster-locate-rssi-row"
                          >
                            <td className="px-4 py-2 text-gray-900 dark:text-gray-100 font-mono text-xs">
                              {row.readerKey}
                            </td>
                            <td className="px-4 py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">
                              {row.antennaPort}
                            </td>
                            <td className="px-4 py-2 text-right tabular-nums font-mono font-medium text-gray-900 dark:text-gray-100">
                              {row.lastRssi !== 0 ? `${row.lastRssi} dBm` : '—'}
                            </td>
                            <td className="px-4 py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">
                              {row.readCount}
                            </td>
                            <td className="px-4 py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
                              {age}s
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </>
      )}

      {!selectedPerson && !loadingPersons && (
        <div className="text-sm text-gray-500 dark:text-gray-400 italic">
          Select a person above to see their location.
        </div>
      )}
    </div>
  );
}
