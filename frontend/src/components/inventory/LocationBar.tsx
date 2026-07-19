/**
 * LocationBar - Shows detected/selected location with dropdown for manual selection
 */

import { Menu } from '@headlessui/react';
import { MapPin, ChevronDown, Check, Radio, RotateCcw, X } from 'lucide-react';
import type { Location } from '@/types/locations';

interface LocationBarProps {
  detectedLocation: { id: number; name: string } | null;
  detectionMethod: 'tag' | 'manual' | 'barcode' | null;
  selectedLocationId: number | null;
  onLocationChange: (locationId: number | null) => void;
  locations: Location[];
  isAuthenticated: boolean;
}

export function LocationBar({
  detectedLocation,
  detectionMethod,
  selectedLocationId,
  onLocationChange,
  locations,
  isAuthenticated,
}: LocationBarProps) {
  // TRA-684 dropped tree_path from the API. Derive each row's depth and
  // depth-first sort key from the parent_id chain. byId lookups are O(1)
  // and chain length is small (3–5 levels in TrakRF segments), so the
  // per-row walk is cheap.
  const byId = new Map(locations.map((l) => [l.id, l]));
  const treeKeyOf = (loc: Location): string[] => {
    const segments: string[] = [];
    let cur: Location | undefined = loc;
    const seen = new Set<number>();
    while (cur && !seen.has(cur.id)) {
      seen.add(cur.id);
      segments.unshift((cur.external_key ?? cur.name ?? String(cur.id)).toLowerCase());
      cur = cur.parent_id != null ? byId.get(cur.parent_id) : undefined;
    }
    return segments;
  };
  const decorated = locations.map((l) => {
    const key = treeKeyOf(l);
    return { loc: l, sortKey: key.join(''), depth: key.length - 1 };
  });
  decorated.sort((a, b) => a.sortKey.localeCompare(b.sortKey));
  const sortedLocations = decorated.map((d) => d.loc);
  const depthById = new Map(decorated.map((d) => [d.loc.id, d.depth]));

  const resolvedLocation = selectedLocationId
    ? locations.find((l) => l.id === selectedLocationId)
    : detectedLocation
      ? locations.find((l) => l.id === detectedLocation.id)
      : null;

  const hasLocation = !!resolvedLocation;
  const methodText =
    detectionMethod === 'tag'
      ? 'via location tag (strongest signal)'
      : detectionMethod === 'barcode'
        ? 'via barcode scan'
        : detectionMethod === 'manual'
          ? 'manually selected'
          : null;

  return (
    <div className="px-4 md:px-6 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
      <div className="flex items-center justify-between">
        <div className="flex items-center min-w-0">
          <MapPin
            className={`w-4 h-4 mr-2 flex-shrink-0 ${
              hasLocation ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-gray-500'
            }`}
          />
          <div className="min-w-0">
            <div
              className={`text-sm font-medium truncate ${
                hasLocation
                  ? 'text-gray-900 dark:text-gray-100'
                  : 'text-gray-500 dark:text-gray-400'
              }`}
            >
              {resolvedLocation?.name ?? 'No location tag detected'}
            </div>
            {methodText && (
              <div className="text-xs text-gray-500 dark:text-gray-400">{methodText}</div>
            )}
          </div>
        </div>

        {isAuthenticated && (
          <div className="flex items-center ml-4 flex-shrink-0">
            {selectedLocationId !== null && (
              // TRA-819: clear control for manual selection. Manual is
              // local page state, so clearing reverts to detected (if any)
              // or empty.
              <button
                type="button"
                aria-label="Clear location"
                onClick={() => onLocationChange(null)}
                className="mr-2 p-1 rounded text-gray-400 hover:text-gray-700 hover:bg-gray-200 dark:text-gray-500 dark:hover:text-gray-200 dark:hover:bg-gray-700 transition-colors"
              >
                <X className="w-4 h-4" />
              </button>
            )}
            <Menu as="div" className="relative">
            <Menu.Button className="px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors flex items-center">
              {hasLocation ? 'Change' : 'Select'}
              <ChevronDown className="w-3 h-3 ml-1" />
            </Menu.Button>

            <Menu.Items className="absolute right-0 mt-2 w-72 max-h-64 overflow-y-auto origin-top-right rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
              <div className="p-1">
                {/* "Use detected" option when there's a detected location and manual override */}
                {detectedLocation && selectedLocationId && selectedLocationId !== detectedLocation.id && (
                  <>
                    <Menu.Item>
                      {({ active }) => (
                        <button
                          onClick={() => onLocationChange(null)}
                          className={`${
                            active ? 'bg-blue-50 dark:bg-blue-900/20' : ''
                          } group flex w-full items-center rounded-md px-3 py-2 text-sm transition-colors text-blue-600 dark:text-blue-400`}
                        >
                          <RotateCcw className="mr-2 h-4 w-4 flex-shrink-0" />
                          <span className="flex-1 text-left">
                            Use detected: {detectedLocation.name}
                          </span>
                        </button>
                      )}
                    </Menu.Item>
                    <div className="my-1 border-t border-gray-200 dark:border-gray-700" />
                  </>
                )}

                {sortedLocations.length === 0 ? (
                  <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
                    No locations available
                  </div>
                ) : (
                  sortedLocations.map((location) => {
                    const isSelected = resolvedLocation?.id === location.id;
                    const isDetected = detectedLocation?.id === location.id;
                    return (
                      <Menu.Item key={location.id}>
                        {({ active }) => (
                          <button
                            onClick={() => onLocationChange(location.id)}
                            className={`${
                              active ? 'bg-gray-100 dark:bg-gray-700' : ''
                            } ${
                              isSelected ? 'bg-green-50 dark:bg-green-900/20' : ''
                            } group flex w-full items-center rounded-md px-3 py-2 text-sm transition-colors`}
                            style={{ paddingLeft: `${(depthById.get(location.id) ?? 0) * 1 + 0.75}rem` }}
                          >
                            <MapPin className="mr-2 h-4 w-4 text-gray-400 dark:text-gray-500 flex-shrink-0" />
                            <span className="flex-1 text-left text-gray-900 dark:text-gray-100 truncate">
                              {location.name}
                            </span>
                            {isDetected && (
                              <span className="ml-1 px-1.5 py-0.5 text-xs font-medium bg-blue-100 dark:bg-blue-900/40 text-blue-700 dark:text-blue-300 rounded flex items-center gap-1 flex-shrink-0">
                                <Radio className="h-3 w-3" />
                                detected
                              </span>
                            )}
                            {isSelected && (
                              <Check className="ml-1 h-4 w-4 text-green-600 dark:text-green-400 flex-shrink-0" />
                            )}
                          </button>
                        )}
                      </Menu.Item>
                    );
                  })
                )}
              </div>
            </Menu.Items>
          </Menu>
          </div>
        )}
      </div>
    </div>
  );
}
