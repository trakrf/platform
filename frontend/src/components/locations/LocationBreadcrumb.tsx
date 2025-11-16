import { ChevronRight, Building2 } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import type { Location } from '@/types/locations';

interface LocationBreadcrumbProps {
  locationId: number;
  onLocationClick?: (location: Location) => void;
  className?: string;
}

export function LocationBreadcrumb({
  locationId,
  onLocationClick,
  className = '',
}: LocationBreadcrumbProps) {
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const getAncestors = useLocationStore((state) => state.getAncestors);

  const location = getLocationById(locationId);
  const ancestors = getAncestors(locationId);

  if (!location) {
    return null;
  }

  const path = [...ancestors, location];

  return (
    <nav className={`flex items-center gap-2 text-sm ${className}`} aria-label="Breadcrumb">
      <Building2 className="h-4 w-4 text-gray-400 dark:text-gray-500" />
      {path.map((loc, index) => {
        const isLast = index === path.length - 1;

        return (
          <div key={loc.id} className="flex items-center gap-2">
            {onLocationClick && !isLast ? (
              <button
                onClick={() => onLocationClick(loc)}
                className="px-2 py-1 rounded hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white"
              >
                {loc.identifier}
              </button>
            ) : (
              <span
                className={`px-2 py-1 ${
                  isLast
                    ? 'text-gray-900 dark:text-white font-medium'
                    : 'text-gray-600 dark:text-gray-400'
                }`}
              >
                {loc.identifier}
              </span>
            )}
            {!isLast && <ChevronRight className="h-4 w-4 text-gray-400 dark:text-gray-500" />}
          </div>
        );
      })}
    </nav>
  );
}
