// MusterDashboard (TRA-978 POC).
//
// Two modes, driven by the active event in musterStore:
//   - Presence mode (event == null): zone headcount cards + total on-site +
//     "Activate muster" (operator+). NO per-person data.
//   - Muster mode (active event): status counters, elapsed clock, entries
//     grouped by status with Verify / Mark safe (operator+), break-glass
//     "Reveal locations" (unlock, logged), per-row Locate link, All clear
//     (→ inline report) and Cancel drill.
//
// A "Demo" disclosure exposes simulator controls (Seed, send everyone to a
// muster point, per-zone scatter) computing sightings client-side from the
// store's zones + the active event's entries.

import { useEffect, useMemo, useState } from 'react';
import toast from 'react-hot-toast';
import { Users, MapPin, ShieldCheck, AlertTriangle, ChevronDown, ChevronRight } from 'lucide-react';
import { ConfirmModal } from '@/components/shared';
import { useMusterStore, useOrgStore } from '@/stores';
import type {
  MusterEntry,
  MusterEntryStatus,
  MusterReport,
  MusterSighting,
  ZonePresence,
} from '@/types/mustering';
import { OPERATOR_PLUS, ADMIN_PLUS, STATUS_LABEL } from './helpers';

interface MusterDashboardProps {
  /** Deep-link a person into the Locate sub-tab. */
  onLocate: (assetId: number) => void;
}

const STATUS_ORDER: MusterEntryStatus[] = ['missing', 'at_muster', 'verified', 'safe_manual'];

function formatElapsed(fromIso: string, now: number): string {
  const start = new Date(fromIso).getTime();
  let secs = Math.max(0, Math.floor((now - start) / 1000));
  const h = Math.floor(secs / 3600);
  secs -= h * 3600;
  const m = Math.floor(secs / 60);
  const s = secs - m * 60;
  const mm = String(m).padStart(2, '0');
  const ss = String(s).padStart(2, '0');
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

export default function MusterDashboard({ onLocate }: MusterDashboardProps) {
  const zones = useMusterStore((s) => s.zones);
  const personsOnSite = useMusterStore((s) => s.personsOnSite);
  const event = useMusterStore((s) => s.event);
  const revealUnlocked = useMusterStore((s) => s.revealUnlocked);
  const error = useMusterStore((s) => s.error);
  const activate = useMusterStore((s) => s.activate);
  const allClear = useMusterStore((s) => s.allClear);
  const cancel = useMusterStore((s) => s.cancel);
  const verify = useMusterStore((s) => s.verify);
  const markSafe = useMusterStore((s) => s.markSafe);
  const unlock = useMusterStore((s) => s.unlock);
  const simulate = useMusterStore((s) => s.simulate);
  const seed = useMusterStore((s) => s.seed);

  const currentRole = useOrgStore((s) => s.currentRole);
  const canOperate = !!currentRole && OPERATOR_PLUS.includes(currentRole);
  const canSeed = !!currentRole && ADMIN_PLUS.includes(currentRole);

  const [windowMinutes, setWindowMinutes] = useState(15);
  const [busy, setBusy] = useState(false);
  const [showAllClearConfirm, setShowAllClearConfirm] = useState(false);
  const [showCancelConfirm, setShowCancelConfirm] = useState(false);
  const [showRevealConfirm, setShowRevealConfirm] = useState(false);
  const [showDemo, setShowDemo] = useState(false);
  const [completedReport, setCompletedReport] = useState<{ id: number; report: MusterReport } | null>(null);

  // Tick for the elapsed clock while an event is active.
  const [now, setNow] = useState(() => Date.now());
  const isActive = event?.status === 'active';
  useEffect(() => {
    if (!isActive) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [isActive]);

  const zoneName = useMemo(() => {
    const m = new Map<number, string>();
    for (const z of zones) m.set(z.location_id, z.name);
    return m;
  }, [zones]);

  const musterPointIds = useMemo(
    () => zones.filter((z) => z.muster_point).map((z) => z.location_id),
    [zones],
  );
  const nonMusterZones = useMemo(() => zones.filter((z) => !z.muster_point), [zones]);

  const guard = (fn: () => Promise<unknown>) => async () => {
    if (busy) return;
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    } finally {
      setBusy(false);
    }
  };

  const handleActivate = guard(async () => {
    // Drop any prior drill's report so it can't reappear if this new drill is
    // later cancelled (cancel produces no report and never clears this).
    setCompletedReport(null);
    await activate(windowMinutes);
  });

  const handleAllClear = guard(async () => {
    const ev = await allClear();
    if (ev?.report) setCompletedReport({ id: ev.id, report: ev.report });
  });

  // --- Simulator (demo) sighting builders ---

  const rosterAssetIds = useMemo(() => {
    // Prefer the active event's entries; the simulator targets those persons.
    if (event?.entries?.length) return event.entries.map((e) => e.asset_id);
    return [];
  }, [event]);

  const sendEveryoneToMuster = guard(async () => {
    const target = musterPointIds[0];
    if (target == null) {
      toast.error('No muster point seeded.');
      return;
    }
    if (rosterAssetIds.length === 0) {
      toast.error('No roster — activate a drill (or seed) first.');
      return;
    }
    const sightings: MusterSighting[] = rosterAssetIds.map((asset_id) => ({
      asset_id,
      location_id: target,
    }));
    await simulate(sightings);
    // Name the actual target zone rather than hardcoding "A" — the demo seeds
    // more than one muster point and we always pick the first.
    const targetName = zoneName.get(target) ?? `Muster point #${target}`;
    toast.success(`Sent everyone to ${targetName}`);
  });

  const scatterAcrossZones = guard(async () => {
    if (nonMusterZones.length === 0 || rosterAssetIds.length === 0) {
      toast.error('Need seeded zones and a roster to scatter.');
      return;
    }
    const sightings: MusterSighting[] = rosterAssetIds.map((asset_id, i) => ({
      asset_id,
      location_id: nonMusterZones[i % nonMusterZones.length].location_id,
    }));
    await simulate(sightings);
    toast.success('Scattered people across zones');
  });

  const handleSeed = guard(async () => {
    await seed();
    toast.success('Demo data seeded');
  });

  return (
    <div className="space-y-6">
      {error && (
        <div className="rounded-md bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-800 px-4 py-2 text-sm text-red-700 dark:text-red-300">
          {error}
        </div>
      )}

      {event && event.status === 'active' ? (
        <MusterMode
          event={event}
          now={now}
          zoneName={zoneName}
          revealUnlocked={revealUnlocked}
          canOperate={canOperate}
          busy={busy}
          onVerify={(id) => guard(() => verify(id))()}
          onMarkSafe={(id) => guard(() => markSafe(id))()}
          onLocate={onLocate}
          onReveal={() => setShowRevealConfirm(true)}
          onAllClear={() => setShowAllClearConfirm(true)}
          onCancel={() => setShowCancelConfirm(true)}
        />
      ) : (
        <PresenceMode
          zones={zones}
          personsOnSite={personsOnSite}
          canOperate={canOperate}
          busy={busy}
          windowMinutes={windowMinutes}
          setWindowMinutes={setWindowMinutes}
          onActivate={handleActivate}
          completedReport={completedReport && (!event || event.status !== 'active') ? completedReport : null}
          onDismissReport={() => setCompletedReport(null)}
        />
      )}

      {/* Demo / simulator controls */}
      {canOperate && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700">
          <button
            onClick={() => setShowDemo((v) => !v)}
            className="w-full flex items-center justify-between px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-300"
          >
            <span>Demo controls (simulator)</span>
            {showDemo ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
          </button>
          {showDemo && (
            <div className="px-4 pb-4 flex flex-wrap gap-2">
              {canSeed && (
                <button
                  onClick={handleSeed}
                  disabled={busy}
                  className="px-3 py-1.5 text-sm rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
                >
                  Seed demo data
                </button>
              )}
              <button
                onClick={sendEveryoneToMuster}
                disabled={busy}
                className="px-3 py-1.5 text-sm rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
              >
                Send everyone to Muster Point A
              </button>
              <button
                onClick={scatterAcrossZones}
                disabled={busy}
                className="px-3 py-1.5 text-sm rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50"
              >
                Scatter across zones
              </button>
            </div>
          )}
        </div>
      )}

      <ConfirmModal
        isOpen={showAllClearConfirm}
        title="All clear?"
        message="End this muster drill and generate the final report. This cannot be undone."
        confirmText="All clear"
        onCancel={() => setShowAllClearConfirm(false)}
        onConfirm={() => {
          setShowAllClearConfirm(false);
          void handleAllClear();
        }}
      />
      <ConfirmModal
        isOpen={showCancelConfirm}
        title="Cancel drill?"
        message="Cancel this muster drill without generating a report."
        confirmText="Cancel drill"
        onCancel={() => setShowCancelConfirm(false)}
        onConfirm={() => {
          setShowCancelConfirm(false);
          void guard(() => cancel())();
        }}
      />
      <ConfirmModal
        isOpen={showRevealConfirm}
        title="Reveal last-known locations?"
        message="This exposes the last-known zone for missing people. This action is logged."
        confirmText="Reveal locations"
        onCancel={() => setShowRevealConfirm(false)}
        onConfirm={() => {
          setShowRevealConfirm(false);
          void guard(() => unlock())();
        }}
      />
    </div>
  );
}

// --- Presence mode ---

interface PresenceModeProps {
  zones: ZonePresence[];
  personsOnSite: number;
  canOperate: boolean;
  busy: boolean;
  windowMinutes: number;
  setWindowMinutes: (n: number) => void;
  onActivate: () => void;
  completedReport: { id: number; report: MusterReport } | null;
  onDismissReport: () => void;
}

function PresenceMode({
  zones,
  personsOnSite,
  canOperate,
  busy,
  windowMinutes,
  setWindowMinutes,
  onActivate,
  completedReport,
  onDismissReport,
}: PresenceModeProps) {
  return (
    <div className="space-y-6">
      {completedReport && (
        <ReportPanel report={completedReport.report} eventId={completedReport.id} onDismiss={onDismissReport} />
      )}

      <div className="flex items-center justify-between flex-wrap gap-4">
        <div className="flex items-center gap-2 text-gray-700 dark:text-gray-200">
          <Users className="w-5 h-5" />
          <span className="text-2xl font-semibold">{personsOnSite}</span>
          <span className="text-sm text-gray-500 dark:text-gray-400">on site</span>
        </div>

        {canOperate && (
          <div className="flex items-end gap-2">
            <label className="text-sm text-gray-600 dark:text-gray-300">
              <span className="block mb-1">Window (min)</span>
              <input
                type="number"
                min={1}
                value={windowMinutes}
                onChange={(e) => setWindowMinutes(Math.max(1, Number(e.target.value) || 1))}
                className="w-20 px-2 py-1.5 rounded border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <button
              onClick={onActivate}
              disabled={busy}
              className="px-4 py-2 rounded bg-red-600 text-white font-medium hover:bg-red-700 disabled:opacity-50"
              data-testid="muster-activate"
            >
              Activate muster
            </button>
          </div>
        )}
      </div>

      {zones.length === 0 ? (
        <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
          No zones yet. Seed demo data or register readers to see headcounts.
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {zones.map((z) => (
            <div
              key={z.location_id}
              className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 p-4"
            >
              <div className="flex items-center justify-between mb-2">
                <span className="font-medium text-gray-900 dark:text-gray-100">{z.name}</span>
                {z.muster_point && (
                  <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-100 dark:bg-green-900/40 text-green-700 dark:text-green-300">
                    <MapPin className="w-3 h-3" />
                    Muster point
                  </span>
                )}
              </div>
              <div className="text-3xl font-semibold text-gray-900 dark:text-gray-100">{z.count}</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">in this zone</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// --- Muster mode ---

interface MusterModeProps {
  event: NonNullable<ReturnType<typeof useMusterStore.getState>['event']>;
  now: number;
  zoneName: Map<number, string>;
  revealUnlocked: boolean;
  canOperate: boolean;
  busy: boolean;
  onVerify: (entryId: number) => void;
  onMarkSafe: (entryId: number) => void;
  onLocate: (assetId: number) => void;
  onReveal: () => void;
  onAllClear: () => void;
  onCancel: () => void;
}

function MusterMode({
  event,
  now,
  zoneName,
  revealUnlocked,
  canOperate,
  busy,
  onVerify,
  onMarkSafe,
  onLocate,
  onReveal,
  onAllClear,
  onCancel,
}: MusterModeProps) {
  const counts = event.counts;
  const grouped = useMemo(() => {
    const g: Record<MusterEntryStatus, MusterEntry[]> = {
      missing: [],
      at_muster: [],
      verified: [],
      safe_manual: [],
    };
    for (const e of event.entries ?? []) g[e.status]?.push(e);
    return g;
  }, [event.entries]);

  return (
    <div className="space-y-6">
      {/* Counters + clock */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-3">
          <Counter label="Expected" value={counts.expected} />
          <Counter label="Missing" value={counts.missing} tone="danger" />
          <Counter label="At muster" value={counts.at_muster} tone="warn" />
          <Counter label="Verified" value={counts.verified} tone="ok" />
          <Counter label="Safe" value={counts.safe_manual} tone="ok" />
        </div>
        <div className="text-right">
          <div className="text-xs text-gray-500 dark:text-gray-400">Elapsed</div>
          <div className="text-2xl font-mono font-semibold text-gray-900 dark:text-gray-100">
            {formatElapsed(event.started_at, now)}
          </div>
        </div>
      </div>

      {/* Drill actions */}
      {canOperate && (
        <div className="flex flex-wrap gap-2">
          <button
            onClick={onAllClear}
            disabled={busy}
            className="px-4 py-2 rounded bg-green-600 text-white font-medium hover:bg-green-700 disabled:opacity-50"
            data-testid="muster-all-clear"
          >
            All clear
          </button>
          {!revealUnlocked && (
            <button
              onClick={onReveal}
              disabled={busy}
              className="px-4 py-2 rounded border border-amber-400 text-amber-700 dark:text-amber-300 font-medium hover:bg-amber-50 dark:hover:bg-amber-900/20 disabled:opacity-50"
              data-testid="muster-reveal"
            >
              Reveal locations
            </button>
          )}
          <button
            onClick={onCancel}
            disabled={busy}
            className="px-4 py-2 rounded text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50"
            data-testid="muster-cancel"
          >
            Cancel drill
          </button>
        </div>
      )}

      {/* Entries grouped by status */}
      <div className="space-y-6">
        {STATUS_ORDER.map((status) => {
          const list = grouped[status];
          if (list.length === 0) return null;
          return (
            <div key={status}>
              <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-200 mb-2 flex items-center gap-2">
                {status === 'missing' && <AlertTriangle className="w-4 h-4 text-red-500" />}
                {status === 'verified' && <ShieldCheck className="w-4 h-4 text-green-500" />}
                {STATUS_LABEL[status]} ({list.length})
              </h3>
              <div className="rounded-lg border border-gray-200 dark:border-gray-700 divide-y divide-gray-200 dark:divide-gray-700">
                {list.map((entry) => (
                  <EntryRow
                    key={entry.id}
                    entry={entry}
                    zoneName={zoneName}
                    revealUnlocked={revealUnlocked}
                    canOperate={canOperate}
                    busy={busy}
                    onVerify={onVerify}
                    onMarkSafe={onMarkSafe}
                    onLocate={onLocate}
                  />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

interface EntryRowProps {
  entry: MusterEntry;
  zoneName: Map<number, string>;
  revealUnlocked: boolean;
  canOperate: boolean;
  busy: boolean;
  onVerify: (entryId: number) => void;
  onMarkSafe: (entryId: number) => void;
  onLocate: (assetId: number) => void;
}

function EntryRow({ entry, zoneName, revealUnlocked, canOperate, busy, onVerify, onMarkSafe, onLocate }: EntryRowProps) {
  const lastKnown =
    entry.last_seen_location_id != null
      ? zoneName.get(entry.last_seen_location_id) ?? `Location #${entry.last_seen_location_id}`
      : null;

  return (
    <div className="flex items-center justify-between gap-3 px-4 py-2.5">
      <div className="min-w-0">
        <div className="font-medium text-gray-900 dark:text-gray-100 truncate">{entry.label}</div>
        {entry.status === 'missing' && revealUnlocked && (
          <div className="text-xs text-amber-600 dark:text-amber-400">
            {lastKnown ? `Last known: ${lastKnown}` : 'Last known: unknown'}
          </div>
        )}
        {entry.status === 'safe_manual' && entry.marked_safe_note && (
          <div className="text-xs text-gray-500 dark:text-gray-400 truncate">Note: {entry.marked_safe_note}</div>
        )}
      </div>

      <div className="flex items-center gap-2 shrink-0">
        {entry.status === 'missing' && (
          <button
            onClick={() => onLocate(entry.asset_id)}
            className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
          >
            Locate
          </button>
        )}
        {canOperate && entry.status === 'at_muster' && (
          <button
            onClick={() => onVerify(entry.id)}
            disabled={busy}
            className="px-2.5 py-1 text-xs rounded bg-green-600 text-white hover:bg-green-700 disabled:opacity-50"
          >
            Verify
          </button>
        )}
        {canOperate && (entry.status === 'missing' || entry.status === 'at_muster') && (
          <button
            onClick={() => onMarkSafe(entry.id)}
            disabled={busy}
            className="px-2.5 py-1 text-xs rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50"
          >
            Mark safe
          </button>
        )}
      </div>
    </div>
  );
}

function Counter({ label, value, tone }: { label: string; value: number; tone?: 'danger' | 'warn' | 'ok' }) {
  const color =
    tone === 'danger'
      ? 'text-red-600 dark:text-red-400'
      : tone === 'warn'
        ? 'text-amber-600 dark:text-amber-400'
        : tone === 'ok'
          ? 'text-green-600 dark:text-green-400'
          : 'text-gray-900 dark:text-gray-100';
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-4 py-2 text-center min-w-[5rem]">
      <div className={`text-2xl font-semibold ${color}`}>{value}</div>
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
    </div>
  );
}

// --- Inline report (shown after All clear) ---

function ReportPanel({ report, eventId, onDismiss }: { report: MusterReport; eventId: number; onDismiss: () => void }) {
  const mins = Math.floor(report.total_seconds / 60);
  const secs = report.total_seconds % 60;
  return (
    <div className="rounded-lg border border-green-200 dark:border-green-800 bg-green-50 dark:bg-green-900/20 p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-semibold text-green-800 dark:text-green-200">
          Drill #{eventId} complete — report
        </h3>
        <button onClick={onDismiss} className="text-sm text-green-700 dark:text-green-300 hover:underline">
          Dismiss
        </button>
      </div>
      <div className="text-sm text-green-900 dark:text-green-100 mb-3">
        Total time: {mins}m {secs}s · Expected {report.counts.expected} · Verified {report.counts.verified} · Safe{' '}
        {report.counts.safe_manual} · Missing {report.counts.missing}
      </div>
      {report.zones.length > 0 && (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-green-700 dark:text-green-300">
              <th className="py-1 pr-4 font-medium">Zone</th>
              <th className="py-1 pr-4 font-medium">Expected</th>
              <th className="py-1 pr-4 font-medium">Accounted</th>
              <th className="py-1 font-medium">Cleared at</th>
            </tr>
          </thead>
          <tbody>
            {report.zones.map((z) => (
              <tr key={z.location_id} className="border-t border-green-200 dark:border-green-800">
                <td className="py-1 pr-4">{z.name}</td>
                <td className="py-1 pr-4">{z.expected}</td>
                <td className="py-1 pr-4">{z.accounted}</td>
                <td className="py-1">{z.cleared_at ? new Date(z.cleared_at).toLocaleTimeString() : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
