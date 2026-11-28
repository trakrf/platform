/**
 * Reports → Map (TRA-1402): a floor plan on a location, with the locations
 * below it pinned on the image and a live count of the people and assets in
 * each pinned zone. Admins upload the image and place the pins here.
 *
 * Presence is the same current-location data the Current tab shows, rolled
 * up to the nearest pinned zone. A marker shows the zone, not a position
 * inside it.
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import toast from 'react-hot-toast';
import { Map as MapIcon, Upload } from 'lucide-react';
import { useOrgStore } from '@/stores';
import { useLocations } from '@/hooks/locations/useLocations';
import { locationMapsApi, MAP_IMAGE_TYPES, MAX_MAP_IMAGE_BYTES } from '@/lib/api/locationMaps';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import type { MapPin } from '@/types/locationMaps';
import { LocationMapCanvas, type MapMarker } from './LocationMapCanvas';
import { MapOccupantsPanel } from './MapOccupantsPanel';
import { descendantsOf, filterPresence, type OccupantKind } from './mapPresence';
import { useLocationMapData } from './useLocationMapData';

const ADMIN_ROLES = ['owner', 'admin'];
const SEEN_WITHIN_OPTIONS = [
  { label: 'Seen in last 5 min', value: 300 },
  { label: 'Seen in last 15 min', value: 900 },
  { label: 'Seen in last hour', value: 3600 },
  { label: 'Seen in last 24 h', value: 86400 },
  { label: 'Any time', value: 0 },
];
const selectClass =
  'rounded-md border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-900 px-2 py-1.5 text-sm text-gray-900 dark:text-gray-100';
const buttonClass =
  'px-3 py-1.5 text-sm rounded bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200 hover:bg-gray-200 dark:hover:bg-gray-600 disabled:opacity-50';

export function LocationMapTab() {
  const currentRole = useOrgStore((s) => s.currentRole);
  const canEdit = !!currentRole && ADMIN_ROLES.includes(currentRole);
  const { locations } = useLocations();

  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [seenWithin, setSeenWithin] = useState(3600);
  const [kind, setKind] = useState<OccupantKind>('all');
  const [q, setQ] = useState('');
  const [selectedZone, setSelectedZone] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const [draftPins, setDraftPins] = useState<MapPin[]>([]);
  const [placingId, setPlacingId] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const { maps, mapsLoaded, reloadMaps, map, setMap, imageUrl, presence, presenceError } =
    useLocationMapData(selectedId, seenWithin, editing);

  // Open the first map once the list arrives.
  useEffect(() => {
    if (selectedId == null && maps.length > 0) setSelectedId(maps[0].location_id);
  }, [maps, selectedId]);

  const nameOf = useMemo(() => new Map(locations.map((l) => [l.id, l.name])), [locations]);
  const pinnable = useMemo(
    () => (selectedId == null ? [] : descendantsOf(selectedId, locations)),
    [selectedId, locations],
  );
  const filtered = useMemo(() => (presence ? filterPresence(presence, { kind, q }) : null), [presence, kind, q]);

  const markers: MapMarker[] = useMemo(() => {
    if (editing) {
      return draftPins.map((p) => ({
        ...p,
        name: nameOf.get(p.location_id) ?? `Location #${p.location_id}`,
        people: 0,
        assets: 0,
      }));
    }
    if (filtered) {
      return filtered.zones.map((z) => ({
        location_id: z.location_id,
        name: z.name,
        x_pct: z.x_pct,
        y_pct: z.y_pct,
        people: z.occupants.filter((o) => o.is_person).length,
        assets: z.occupants.filter((o) => !o.is_person).length,
      }));
    }
    return (map?.pins ?? []).map((p) => ({ ...p, name: nameOf.get(p.location_id) ?? '', people: 0, assets: 0 }));
  }, [editing, draftPins, filtered, map, nameOf]);

  const choose = (id: number | null) => {
    setSelectedId(id);
    setSelectedZone(null);
    setEditing(false);
  };

  const startEdit = () => {
    setDraftPins(map?.pins ? [...map.pins] : []);
    setPlacingId(pinnable[0]?.id ?? null);
    setEditing(true);
  };

  const place = (pos: { x_pct: number; y_pct: number }) => {
    if (placingId == null) return;
    setDraftPins((prev) => [...prev.filter((p) => p.location_id !== placingId), { location_id: placingId, ...pos }]);
  };

  const run = async (what: string, fn: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      toast.error(getApiErrorMessage(err, `${what} failed`));
    } finally {
      setBusy(false);
    }
  };

  const savePins = () =>
    run('Saving pins', async () => {
      if (selectedId == null) return;
      const { data } = await locationMapsApi.putPins(selectedId, draftPins);
      setMap(data.data);
      setEditing(false);
      await reloadMaps();
      toast.success('Pins saved');
    });

  const upload = (file: File | undefined) => {
    if (!file || selectedId == null) return;
    if (!MAP_IMAGE_TYPES.includes(file.type)) {
      toast.error('Use a PNG, JPEG or WebP image');
      return;
    }
    if (file.size > MAX_MAP_IMAGE_BYTES) {
      toast.error('Image must be 5 MB or smaller');
      return;
    }
    void run('Upload', async () => {
      const { data } = await locationMapsApi.putImage(selectedId, file);
      setMap(data.data);
      await reloadMaps();
      toast.success('Map image saved');
    });
  };

  const removeMap = () => {
    if (selectedId == null || !window.confirm('Remove this map and all its pins? The locations stay.')) return;
    void run('Removing map', async () => {
      await locationMapsApi.remove(selectedId);
      setMap(null);
      setEditing(false);
      await reloadMaps();
      setSelectedId(null);
    });
  };

  const mapIds = new Set(maps.map((m) => m.location_id));
  const unmapped = locations.filter((l) => !mapIds.has(l.id));
  const pinnedIds = new Set(draftPins.map((p) => p.location_id));

  return (
    <div className="space-y-4" data-testid="location-map-tab">
      {/* Map picker + filters */}
      <div className="flex flex-wrap items-end gap-2">
        <select
          value={selectedId ?? ''}
          onChange={(e) => choose(e.target.value ? Number(e.target.value) : null)}
          className={selectClass}
          aria-label="Map"
          data-testid="location-map-select"
        >
          {selectedId == null && <option value="">Choose a map…</option>}
          {maps.map((m) => (
            <option key={m.location_id} value={m.location_id}>
              {m.location_name}
            </option>
          ))}
          {selectedId != null && !mapIds.has(selectedId) && (
            <option value={selectedId}>{nameOf.get(selectedId) ?? 'New map'} (no image yet)</option>
          )}
        </select>
        {canEdit && unmapped.length > 0 && (
          <select
            value=""
            onChange={(e) => e.target.value && choose(Number(e.target.value))}
            className={selectClass}
            aria-label="Add a map to a location"
            data-testid="location-map-new"
          >
            <option value="">Add a map to…</option>
            {unmapped.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        )}
        {map && !editing && (
          <>
            <select value={kind} onChange={(e) => setKind(e.target.value as OccupantKind)} className={selectClass} aria-label="Show">
              <option value="all">People and assets</option>
              <option value="people">People</option>
              <option value="assets">Assets</option>
            </select>
            <select value={seenWithin} onChange={(e) => setSeenWithin(Number(e.target.value))} className={selectClass} aria-label="Seen within">
              {SEEN_WITHIN_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <input
              type="search"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Filter by name…"
              className={`${selectClass} w-44`}
              data-testid="location-map-search"
            />
          </>
        )}
        {canEdit && selectedId != null && (
          <div className="flex gap-2 ml-auto">
            <input
              ref={fileInput}
              type="file"
              accept={MAP_IMAGE_TYPES.join(',')}
              className="hidden"
              onChange={(e) => {
                upload(e.target.files?.[0]);
                e.target.value = '';
              }}
              data-testid="location-map-file"
            />
            {!editing && (
              <button type="button" onClick={() => fileInput.current?.click()} disabled={busy} className={buttonClass}>
                <Upload className="inline w-4 h-4 mr-1" />
                {map ? 'Replace image' : 'Upload image'}
              </button>
            )}
            {map && !editing && (
              <>
                <button type="button" onClick={startEdit} className={buttonClass} data-testid="location-map-edit">
                  Edit pins
                </button>
                <button type="button" onClick={removeMap} disabled={busy} className={`${buttonClass} text-red-600 dark:text-red-400`}>
                  Remove map
                </button>
              </>
            )}
            {editing && (
              <>
                <button type="button" onClick={() => setEditing(false)} className={buttonClass}>
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={savePins}
                  disabled={busy}
                  className="px-3 py-1.5 text-sm rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
                  data-testid="location-map-save"
                >
                  Save pins
                </button>
              </>
            )}
          </div>
        )}
      </div>

      {editing && (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-gray-200 dark:border-gray-700 p-3 text-sm">
          {pinnable.length > 0 ? (
            <>
              <span className="text-gray-600 dark:text-gray-300">Place pin for:</span>
              <select
                value={placingId ?? ''}
                onChange={(e) => setPlacingId(Number(e.target.value))}
                className={selectClass}
                data-testid="location-map-place-select"
              >
                {pinnable.map((l) => (
                  <option key={l.id} value={l.id}>
                    {l.name}
                    {pinnedIds.has(l.id) ? ' ✓' : ''}
                  </option>
                ))}
              </select>
              <span className="text-xs text-gray-500 dark:text-gray-400">then click the image to place it.</span>
            </>
          ) : (
            <span className="text-amber-600 dark:text-amber-400">
              This location has no locations below it to pin. Add child locations first.
            </span>
          )}
        </div>
      )}

      {/* Body */}
      {!mapsLoaded ? (
        <div className="text-sm text-gray-500 dark:text-gray-400">Loading maps…</div>
      ) : selectedId == null ? (
        <EmptyMaps canEdit={canEdit} />
      ) : !map ? (
        <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
          {canEdit
            ? 'Upload a floor plan or site image (PNG, JPEG or WebP, up to 5 MB) for this location.'
            : 'This location has no map yet.'}
        </div>
      ) : (
        <div className="flex flex-col lg:flex-row gap-4 items-start">
          <div className="min-w-0 flex-1">
            {imageUrl ? (
              <LocationMapCanvas
                imageUrl={imageUrl}
                markers={markers}
                selectedId={selectedZone}
                onSelect={(id) => setSelectedZone((cur) => (cur === id ? null : id))}
                editing={editing}
                placingLocationId={placingId}
                onPlace={place}
                onRemove={(id) => setDraftPins((prev) => prev.filter((p) => p.location_id !== id))}
              />
            ) : (
              <div className="text-sm text-gray-500 dark:text-gray-400">Loading image…</div>
            )}
            {!editing && map.pins.length === 0 && (
              <p className="mt-2 text-sm text-amber-600 dark:text-amber-400">
                No locations are pinned yet{canEdit ? ' — use Edit pins to place them.' : '.'}
              </p>
            )}
          </div>
          {!editing && (
            <MapOccupantsPanel presence={filtered} selectedZone={selectedZone} onClearZone={() => setSelectedZone(null)} stale={presenceError} />
          )}
        </div>
      )}
    </div>
  );
}

function EmptyMaps({ canEdit }: { canEdit: boolean }) {
  return (
    <div className="rounded-lg border border-dashed border-gray-300 dark:border-gray-700 p-8 text-center text-gray-500 dark:text-gray-400">
      <MapIcon className="mx-auto mb-2 w-8 h-8" />
      {canEdit
        ? 'No maps yet. Use "Add a map to…" to put a floor plan on a site, building or floor, then pin the rooms below it.'
        : 'No maps have been set up for this organization.'}
    </div>
  );
}
