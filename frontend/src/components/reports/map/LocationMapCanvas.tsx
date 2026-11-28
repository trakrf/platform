/**
 * The map image with a marker per pinned location (TRA-1402).
 *
 * A marker sits at its zone's pin and carries a count, never a position
 * inside the room: the zone is all the location data knows, so the map does
 * not draw individuals at made-up coordinates.
 */

import { MapPin, User, Package, X } from 'lucide-react';
import { pinPercentFromClick } from './mapPresence';

export interface MapMarker {
  location_id: number;
  name: string;
  x_pct: number;
  y_pct: number;
  people: number;
  assets: number;
}

interface Props {
  imageUrl: string;
  markers: MapMarker[];
  selectedId: number | null;
  onSelect: (locationId: number) => void;
  /** Edit mode: when set, a click on the image places this location's pin. */
  placingLocationId?: number | null;
  onPlace?: (pos: { x_pct: number; y_pct: number }) => void;
  onRemove?: (locationId: number) => void;
  editing?: boolean;
}

export function LocationMapCanvas({
  imageUrl,
  markers,
  selectedId,
  onSelect,
  placingLocationId = null,
  onPlace,
  onRemove,
  editing = false,
}: Props) {
  const placing = editing && placingLocationId != null;

  const handleClick = (e: React.MouseEvent<HTMLImageElement>) => {
    if (!placing || !onPlace) return;
    onPlace(pinPercentFromClick(e.currentTarget.getBoundingClientRect(), e.clientX, e.clientY));
  };

  return (
    <div className="relative inline-block max-w-full" data-testid="location-map-canvas">
      <img
        src={imageUrl}
        alt="Location map"
        onClick={handleClick}
        draggable={false}
        className={`block max-w-full h-auto rounded-lg border border-gray-200 dark:border-gray-700 select-none ${
          placing ? 'cursor-crosshair' : ''
        }`}
        data-testid="location-map-image"
      />
      {markers.map((m) => {
        const total = m.people + m.assets;
        const selected = selectedId === m.location_id;
        return (
          <div
            key={m.location_id}
            className="absolute -translate-x-1/2 -translate-y-1/2 flex flex-col items-center"
            style={{ left: `${m.x_pct}%`, top: `${m.y_pct}%` }}
            data-testid={`location-map-pin-${m.location_id}`}
          >
            <button
              type="button"
              onClick={() => onSelect(m.location_id)}
              title={`${m.name}: ${m.people} people, ${m.assets} assets`}
              className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold shadow transition-colors ${
                selected ? 'ring-2 ring-offset-1 ring-blue-400' : ''
              } ${total > 0 ? 'bg-blue-600 text-white' : 'bg-gray-500/90 text-white'}`}
            >
              <MapPin className="w-3 h-3" />
              {editing ? (
                <span>{m.name}</span>
              ) : (
                <>
                  <User className="w-3 h-3" />
                  <span data-testid={`location-map-pin-${m.location_id}-people`}>{m.people}</span>
                  <Package className="w-3 h-3 ml-0.5" />
                  <span data-testid={`location-map-pin-${m.location_id}-assets`}>{m.assets}</span>
                </>
              )}
            </button>
            {!editing && (
              <span className="mt-0.5 max-w-[9rem] truncate rounded bg-black/60 px-1 text-[10px] text-white">
                {m.name}
              </span>
            )}
            {editing && onRemove && (
              <button
                type="button"
                onClick={() => onRemove(m.location_id)}
                className="mt-0.5 rounded-full bg-white/90 dark:bg-gray-800/90 p-0.5 text-red-600 hover:bg-white"
                title="Remove pin"
                data-testid={`location-map-remove-${m.location_id}`}
              >
                <X className="w-3 h-3" />
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}
