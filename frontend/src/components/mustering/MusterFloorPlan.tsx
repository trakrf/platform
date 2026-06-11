// MusterFloorPlan (TRA-978 phase 7).
//
// Optional static floor-plan sub-tab: a single image per org with manually
// positioned pins, each referencing an existing Location (a zone or muster
// point from musterStore.zones). No drag-builder — admins toggle "Edit pins",
// pick a location, and click the image to place/replace its pin.
//
// Live overlay (all data from musterStore, no new polling):
//   - Presence mode (no active event): zone pins show live headcount.
//   - Drill mode (active event): zone pins show MISSING count (red badge);
//     muster-point pins show arrivals — at_muster/verified entries whose
//     muster_location_id matches the pin (green badge).

import { useEffect, useMemo, useState } from 'react';
import toast from 'react-hot-toast';
import { MapPin, X } from 'lucide-react';
import { useMusterStore, useOrgStore } from '@/stores';
import { musteringApi } from '@/lib/api/mustering';
import type { FloorPlan, FloorPlanPin, MusterEvent, ZonePresence } from '@/types/mustering';
import { ADMIN_PLUS } from './helpers';

/** What a pin should display, computed purely from store state. */
export interface PinBadge {
  /** 'present' = live headcount (blue), 'missing' = drill MISSING (red),
   *  'arrivals' = drill muster-point arrivals (green), 'none' = no badge. */
  kind: 'present' | 'missing' | 'arrivals' | 'none';
  count: number;
}

/**
 * Compute the badge for one pinned location, given the live zones list and the
 * active event (null in presence mode). Pure — unit-tested.
 *
 *  - No active event → live headcount from the matching ZonePresence ('present').
 *  - Active event, muster-point location → arrivals: entries at_muster/verified
 *    whose muster_location_id === locationId ('arrivals', green).
 *  - Active event, zone location → MISSING: entries still 'missing' whose
 *    expected_location_id === locationId ('missing', red).
 */
export function pinBadge(
  locationId: number,
  zones: ZonePresence[],
  event: MusterEvent | null,
): PinBadge {
  const zone = zones.find((z) => z.location_id === locationId);
  const isMusterPoint = zone?.muster_point ?? false;

  if (!event || event.status !== 'active') {
    return { kind: 'present', count: zone?.count ?? 0 };
  }

  const entries = event.entries ?? [];
  if (isMusterPoint) {
    const arrivals = entries.filter(
      (e) =>
        (e.status === 'at_muster' || e.status === 'verified') &&
        e.muster_location_id === locationId,
    ).length;
    return { kind: 'arrivals', count: arrivals };
  }

  const missing = entries.filter(
    (e) => e.status === 'missing' && e.expected_location_id === locationId,
  ).length;
  return { kind: 'missing', count: missing };
}

const BADGE_CLASS: Record<PinBadge['kind'], string> = {
  present: 'bg-blue-600 text-white',
  missing: 'bg-red-600 text-white',
  arrivals: 'bg-green-600 text-white',
  none: 'bg-gray-500 text-white',
};

export default function MusterFloorPlan() {
  const zones = useMusterStore((s) => s.zones);
  const event = useMusterStore((s) => s.event);
  const currentRole = useOrgStore((s) => s.currentRole);
  const canEdit = !!currentRole && ADMIN_PLUS.includes(currentRole);

  const [plan, setPlan] = useState<FloorPlan | null>(null);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  // Edit-mode working copy (image_url + pins) so cancel discards.
  const [draftImageUrl, setDraftImageUrl] = useState('');
  const [draftPins, setDraftPins] = useState<FloorPlanPin[]>([]);
  const [selectedLocationId, setSelectedLocationId] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const { data } = await musteringApi.getFloorPlan();
        if (!cancelled) setPlan(data.data);
      } catch {
        if (!cancelled) setPlan({ image_url: '', pins: [] });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const zoneName = useMemo(() => {
    const m = new Map<number, string>();
    for (const z of zones) m.set(z.location_id, z.name);
    return m;
  }, [zones]);

  const hasImage = !!plan?.image_url;

  const startEdit = () => {
    setDraftImageUrl(plan?.image_url ?? '');
    setDraftPins(plan?.pins ? [...plan.pins] : []);
    setSelectedLocationId(zones[0]?.location_id ?? null);
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setSelectedLocationId(null);
  };

  const handleImageClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!editing || selectedLocationId == null) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const xPct = ((e.clientX - rect.left) / rect.width) * 100;
    const yPct = ((e.clientY - rect.top) / rect.height) * 100;
    const x = Math.min(100, Math.max(0, Number(xPct.toFixed(2))));
    const y = Math.min(100, Math.max(0, Number(yPct.toFixed(2))));
    setDraftPins((prev) => {
      const others = prev.filter((p) => p.location_id !== selectedLocationId);
      return [...others, { location_id: selectedLocationId, x_pct: x, y_pct: y }];
    });
  };

  const removePin = (locationId: number) => {
    setDraftPins((prev) => prev.filter((p) => p.location_id !== locationId));
  };

  const save = async () => {
    if (saving) return;
    setSaving(true);
    try {
      const body: FloorPlan = { image_url: draftImageUrl.trim(), pins: draftPins };
      const { data } = await musteringApi.putFloorPlan(body);
      setPlan(data.data);
      setEditing(false);
      setSelectedLocationId(null);
      toast.success('Floor plan saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <div className="text-sm text-gray-500 dark:text-gray-400">Loading floor plan…</div>;
  }

  // Pins + image to render: drafts while editing, else the saved plan.
  const renderImageUrl = editing ? draftImageUrl : plan?.image_url ?? '';
  const renderPins = editing ? draftPins : plan?.pins ?? [];
  const pinnedLocationIds = new Set(renderPins.map((p) => p.location_id));

  return (
    <div className="space-y-4" data-testid="muster-floor-plan">
      {/* Header / edit toggle */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h2 className="text-sm font-semibold text-gray-700 dark:text-gray-200">Floor plan</h2>
        {canEdit && !editing && (
          <button
            onClick={startEdit}
            className="px-3 py-1.5 text-sm rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600"
            data-testid="floor-plan-edit"
          >
            {hasImage ? 'Edit pins' : 'Set up floor plan'}
          </button>
        )}
        {editing && (
          <div className="flex gap-2">
            <button
              onClick={cancelEdit}
              className="px-3 py-1.5 text-sm rounded text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              data-testid="floor-plan-cancel"
            >
              Cancel
            </button>
            <button
              onClick={save}
              disabled={saving}
              className="px-3 py-1.5 text-sm rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              data-testid="floor-plan-save"
            >
              Save
            </button>
          </div>
        )}
      </div>

      {/* Empty state for non-admins / unset plan */}
      {!hasImage && !editing && (
        <div
          className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400"
          data-testid="floor-plan-empty"
        >
          {canEdit
            ? 'No floor plan yet. Click "Set up floor plan" to add a site image and place pins.'
            : 'No floor plan configured for this site.'}
        </div>
      )}

      {/* Edit controls: image URL + location picker */}
      {editing && (
        <div className="space-y-3 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
          <label className="block text-sm text-gray-600 dark:text-gray-300">
            <span className="block mb-1">Image URL (http(s) or data:)</span>
            <input
              type="text"
              value={draftImageUrl}
              onChange={(e) => setDraftImageUrl(e.target.value)}
              placeholder="https://example.com/site.png"
              className="w-full rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-3 py-1.5 text-sm text-gray-900 dark:text-gray-100"
              data-testid="floor-plan-image-url"
            />
          </label>
          {zones.length > 0 ? (
            <div className="flex items-center gap-2 flex-wrap text-sm">
              <span className="text-gray-600 dark:text-gray-300">Place pin for:</span>
              <select
                value={selectedLocationId ?? ''}
                onChange={(e) => setSelectedLocationId(Number(e.target.value))}
                className="rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1 text-sm text-gray-900 dark:text-gray-100"
                data-testid="floor-plan-location-select"
              >
                {zones.map((z) => (
                  <option key={z.location_id} value={z.location_id}>
                    {z.name}
                    {z.muster_point ? ' (muster point)' : ''}
                    {pinnedLocationIds.has(z.location_id) ? ' ✓' : ''}
                  </option>
                ))}
              </select>
              <span className="text-xs text-gray-500 dark:text-gray-400">
                then click the image to place it.
              </span>
            </div>
          ) : (
            <div className="text-xs text-amber-600 dark:text-amber-400">
              No locations available to pin. Seed demo data or register readers first.
            </div>
          )}
        </div>
      )}

      {/* Image + pin overlay */}
      {renderImageUrl && (
        <div className="relative inline-block max-w-full" data-testid="floor-plan-canvas">
          <img
            src={renderImageUrl}
            alt="Floor plan"
            onClick={handleImageClick}
            className={`block max-w-full h-auto rounded-lg border border-gray-200 dark:border-gray-700 ${
              editing && selectedLocationId != null ? 'cursor-crosshair' : ''
            }`}
            data-testid="floor-plan-image"
          />
          {renderPins.map((pin) => {
            const badge = pinBadge(pin.location_id, zones, event);
            const name = zoneName.get(pin.location_id) ?? `Location #${pin.location_id}`;
            return (
              <div
                key={pin.location_id}
                className="absolute -translate-x-1/2 -translate-y-1/2 flex flex-col items-center"
                style={{ left: `${pin.x_pct}%`, top: `${pin.y_pct}%` }}
                data-testid={`floor-plan-pin-${pin.location_id}`}
              >
                <div
                  className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold shadow ${BADGE_CLASS[badge.kind]}`}
                  title={name}
                >
                  <MapPin className="w-3 h-3" />
                  <span>{badge.count}</span>
                </div>
                <span className="mt-0.5 max-w-[8rem] truncate rounded bg-black/60 px-1 text-[10px] text-white">
                  {name}
                </span>
                {editing && (
                  <button
                    onClick={() => removePin(pin.location_id)}
                    className="mt-0.5 rounded-full bg-white/90 dark:bg-gray-800/90 p-0.5 text-red-600 hover:bg-white"
                    title="Remove pin"
                    data-testid={`floor-plan-remove-${pin.location_id}`}
                  >
                    <X className="w-3 h-3" />
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
