import { Building2 } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { LocationExpandableCard } from './LocationExpandableCard';

export interface LocationMobileViewProps {
  searchTerm?: string;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
  className?: string;
}

export function LocationMobileView({
  searchTerm = '',
  onEdit,
  onMove,
  onDelete,
  className = '',
}: LocationMobileViewProps) {
  const getRootLocations = useLocationStore((state) => state.getRootLocations);
  const getDescendants = useLocationStore((state) => state.getDescendants);

  const rootLocations = getRootLocations();

  // When filtering, we need to also check if any location matches
  const getVisibleRoots = () => {
    if (!searchTerm.trim()) return rootLocations;

    const term = searchTerm.toLowerCase();
    return rootLocations.filter((root) => {
      const matches =
        root.identifier.toLowerCase().includes(term) ||
        root.name.toLowerCase().includes(term);

      if (matches) return true;

      // Check if any descendant matches
      const descendants = getDescendants(root.id);
      return descendants.some(
        (d) =>
          d.identifier.toLowerCase().includes(term) ||
          d.name.toLowerCase().includes(term)
      );
    });
  };

  const visibleRoots = getVisibleRoots();

  if (rootLocations.length === 0) {
    return (
      <div className={`flex flex-col items-center justify-center p-8 text-gray-500 dark:text-gray-400 ${className}`}>
        <Building2 className="h-16 w-16 mb-4 opacity-50" />
        <p className="text-lg font-medium mb-2">No locations yet</p>
        <p className="text-sm text-center">
          Tap the + button to create your first location
        </p>
      </div>
    );
  }

  if (visibleRoots.length === 0) {
    return (
      <div className={`flex flex-col items-center justify-center p-8 text-gray-500 dark:text-gray-400 ${className}`}>
        <Building2 className="h-12 w-12 mb-3 opacity-50" />
        <p className="text-base font-medium mb-1">No matching locations</p>
        <p className="text-sm text-center">
          Try a different search term
        </p>
      </div>
    );
  }

  return (
    <div className={`space-y-3 ${className}`} data-testid="location-mobile-view">
      {visibleRoots.map((location) => (
        <LocationExpandableCard
          key={location.id}
          location={location}
          depth={0}
          onEdit={onEdit}
          onMove={onMove}
          onDelete={onDelete}
          searchTerm={searchTerm}
        />
      ))}
    </div>
  );
}
