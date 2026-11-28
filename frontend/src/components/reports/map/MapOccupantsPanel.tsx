/**
 * Side panel for the location map (TRA-1402): who is in the selected zone (or
 * every zone), with last-seen times, plus anyone in the map's area who is not
 * under any pin.
 */

import { User, Package } from 'lucide-react';
import type { MapOccupant, MapPresence } from '@/types/locationMaps';
import { relativeTime } from './mapPresence';

interface Props {
  presence: MapPresence | null;
  selectedZone: number | null;
  onClearZone: () => void;
  /** The last poll failed; what is shown may be out of date. */
  stale: boolean;
}

function OccupantRow({ o, zoneName }: { o: MapOccupant; zoneName: string }) {
  const Icon = o.is_person ? User : Package;
  return (
    <li className="flex items-start gap-2 py-1.5" data-testid={`location-map-occupant-${o.asset_id}`}>
      <Icon className="w-4 h-4 mt-0.5 shrink-0 text-gray-400" />
      <div className="min-w-0">
        <div className="truncate text-sm text-gray-900 dark:text-gray-100">{o.name}</div>
        <div className="text-xs text-gray-500 dark:text-gray-400">
          {o.location_name && o.location_name !== zoneName ? `${o.location_name} · ` : ''}
          {relativeTime(o.last_seen)}
        </div>
      </div>
    </li>
  );
}

export function MapOccupantsPanel({ presence, selectedZone, onClearZone, stale }: Props) {
  if (!presence) {
    return (
      <aside className="w-full lg:w-72 shrink-0 text-sm text-gray-500 dark:text-gray-400">Loading who is here…</aside>
    );
  }
  const zones = selectedZone == null ? presence.zones : presence.zones.filter((z) => z.location_id === selectedZone);
  const total = presence.zones.reduce((n, z) => n + z.occupants.length, 0) + presence.unplaced.length;

  return (
    <aside
      className="w-full lg:w-72 shrink-0 rounded-lg border border-gray-200 dark:border-gray-700 p-3 space-y-3"
      data-testid="location-map-occupants"
    >
      <div className="flex items-center justify-between text-sm">
        <span className="font-semibold text-gray-700 dark:text-gray-200">
          {selectedZone == null ? `${total} on this map` : zones[0]?.name}
        </span>
        {selectedZone != null && (
          <button type="button" onClick={onClearZone} className="text-xs text-blue-600 dark:text-blue-400 hover:underline">
            All zones
          </button>
        )}
      </div>
      {stale && <p className="text-xs text-amber-600 dark:text-amber-400">Live update failed; retrying.</p>}
      {zones.map((z) => (
        <section key={z.location_id}>
          {selectedZone == null && (
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
              {z.name} ({z.occupants.length})
            </h3>
          )}
          {z.occupants.length === 0 ? (
            <p className="py-1 text-xs text-gray-400">Nobody here.</p>
          ) : (
            <ul className="divide-y divide-gray-100 dark:divide-gray-800">
              {z.occupants.map((o) => (
                <OccupantRow key={o.asset_id} o={o} zoneName={z.name} />
              ))}
            </ul>
          )}
        </section>
      ))}
      {selectedZone == null && presence.unplaced.length > 0 && (
        <section>
          <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">
            Not in a pinned zone ({presence.unplaced.length})
          </h3>
          <ul className="divide-y divide-gray-100 dark:divide-gray-800">
            {presence.unplaced.map((o) => (
              <OccupantRow key={o.asset_id} o={o} zoneName="" />
            ))}
          </ul>
        </section>
      )}
    </aside>
  );
}
