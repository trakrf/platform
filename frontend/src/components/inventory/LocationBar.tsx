/**
 * LocationBar - Shows detected/selected location with dropdown for manual selection
 */

import { Menu } from '@headlessui/react';
import { MapPin, ChevronDown, Check } from 'lucide-react';
import type { Location } from '@/types/locations';

interface LocationBarProps {
  detectedLocation: { id: number; name: string } | null;
  detectionMethod: 'tag' | 'manual' | null;
  selectedLocationId: number | null;
  onLocationChange: (locationId: number) => void;
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
  // Sort locations by path for proper hierarchy ordering
  const sortedLocations = [...locations].sort((a, b) => a.path.localeCompare(b.path));

  const resolvedLocation = selectedLocationId
    ? locations.find((l) => l.id === selectedLocationId)
    : detectedLocation
      ? locations.find((l) => l.id === detectedLocation.id)
      : null;

  const hasLocation = !!resolvedLocation;
  const methodText =
    detectionMethod === 'tag'
      ? 'via location tag (strongest signal)'
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
          <Menu as="div" className="relative ml-4">
            <Menu.Button className="px-3 py-1.5 text-sm font-medium text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors flex items-center">
              {hasLocation ? 'Change' : 'Select'}
              <ChevronDown className="w-3 h-3 ml-1" />
            </Menu.Button>

            <Menu.Items className="absolute right-0 mt-2 w-72 max-h-64 overflow-y-auto origin-top-right rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
              <div className="p-1">
                {sortedLocations.length === 0 ? (
                  <div className="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">
                    No locations available
                  </div>
                ) : (
                  sortedLocations.map((location) => {
                    const isSelected = resolvedLocation?.id === location.id;
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
                            style={{ paddingLeft: `${location.depth * 1 + 0.75}rem` }}
                          >
                            <MapPin className="mr-2 h-4 w-4 text-gray-400 dark:text-gray-500 flex-shrink-0" />
                            <span className="flex-1 text-left text-gray-900 dark:text-gray-100 truncate">
                              {location.name}
                            </span>
                            {isSelected && (
                              <Check className="h-4 w-4 text-green-600 dark:text-green-400 flex-shrink-0" />
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
        )}
      </div>
    </div>
  );
}
