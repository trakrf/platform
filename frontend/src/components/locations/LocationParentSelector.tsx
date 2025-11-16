import { useMemo } from 'react';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

interface LocationParentSelectorProps {
  value: number | null;
  onChange: (value: number | null) => void;
  currentLocationId?: number;
  disabled?: boolean;
  className?: string;
}

export function LocationParentSelector({
  value,
  onChange,
  currentLocationId,
  disabled = false,
  className = '',
}: LocationParentSelectorProps) {
  const cache = useLocationStore((state) => state.cache);

  const availableLocations = useMemo(() => {
    const allLocations = Array.from(cache.byId.values());

    if (!currentLocationId) {
      return allLocations;
    }

    const excludedIds = new Set<number>([currentLocationId]);

    const getDescendants = (locationId: number) => {
      const children = cache.byParentId.get(locationId) || new Set();
      children.forEach((childId) => {
        excludedIds.add(childId);
        getDescendants(childId);
      });
    };

    getDescendants(currentLocationId);

    return allLocations.filter((loc) => !excludedIds.has(loc.id));
  }, [cache.byId, cache.byParentId, currentLocationId]);

  const sortedLocations = useMemo(() => {
    return [...availableLocations].sort((a, b) => {
      if (a.parent_location_id === null && b.parent_location_id !== null) return -1;
      if (a.parent_location_id !== null && b.parent_location_id === null) return 1;
      return a.identifier.localeCompare(b.identifier);
    });
  }, [availableLocations]);

  const getDisplayName = (location: Location): string => {
    const path: string[] = [];
    let current: Location | undefined = location;

    while (current) {
      path.unshift(current.identifier);
      if (current.parent_location_id === null) break;
      current = cache.byId.get(current.parent_location_id);
    }

    if (path.length > 1) {
      return path.join(' > ');
    }
    return location.identifier;
  };

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const selectedValue = e.target.value;
    onChange(selectedValue === '' ? null : parseInt(selectedValue, 10));
  };

  return (
    <select
      value={value ?? ''}
      onChange={handleChange}
      disabled={disabled}
      className={`block w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-50 ${className}`}
    >
      <option value="">None (Root Location)</option>
      {sortedLocations.map((location) => (
        <option key={location.id} value={location.id}>
          {getDisplayName(location)}
        </option>
      ))}
    </select>
  );
}
