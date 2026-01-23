import { useState, useEffect } from 'react';
import { MapPin, Building2, Calendar, CheckCircle, XCircle, FolderTree, Plus } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { LocationBreadcrumb } from './LocationBreadcrumb';
import { TagIdentifierList } from '@/components/assets';
import type { TagIdentifier } from '@/types/shared';

export interface LocationDetailsPanelProps {
  locationId: number | null;
  onEdit: (id: number) => void;
  onMove: (id: number) => void;
  onDelete: (id: number) => void;
  onAddChild?: (parentId: number) => void;
  onChildClick?: (id: number) => void;
  className?: string;
}

export function LocationDetailsPanel({
  locationId,
  onEdit,
  onMove,
  onDelete,
  onAddChild,
  onChildClick,
  className = '',
}: LocationDetailsPanelProps) {
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const getChildren = useLocationStore((state) => state.getChildren);
  const getDescendants = useLocationStore((state) => state.getDescendants);
  const getRootLocations = useLocationStore((state) => state.getRootLocations);
  const updateCachedLocation = useLocationStore((state) => state.updateCachedLocation);

  const [localIdentifiers, setLocalIdentifiers] = useState<TagIdentifier[]>([]);

  const location = locationId ? getLocationById(locationId) : undefined;
  const children = location ? getChildren(location.id) : [];
  const rootLocations = getRootLocations();
  const isRoot = location?.parent_location_id === null;
  const Icon = isRoot ? Building2 : MapPin;

  // Sync local identifiers with location
  useEffect(() => {
    if (location?.identifiers) {
      setLocalIdentifiers(location.identifiers);
    } else {
      setLocalIdentifiers([]);
    }
  }, [location?.identifiers]);

  const handleIdentifierRemoved = (identifierId: number) => {
    if (!location) return;

    const updatedIdentifiers = localIdentifiers.filter((i) => i.id !== identifierId);
    setLocalIdentifiers(updatedIdentifiers);

    updateCachedLocation(location.id, {
      ...location,
      identifiers: updatedIdentifiers,
    });
  };

  const formatDate = (dateString: string | null) => {
    if (!dateString) return 'Not set';
    return new Date(dateString).toLocaleDateString();
  };

  // Empty state when no location selected
  if (!location) {
    const totalLocations = rootLocations.reduce((acc, root) => {
      const descs = getDescendants(root.id);
      return acc + 1 + descs.length;
    }, 0);

    return (
      <div className={`h-full flex flex-col items-center justify-center p-8 text-gray-500 dark:text-gray-400 ${className}`}>
        <FolderTree className="h-16 w-16 mb-4 opacity-50" />
        <p className="text-lg font-medium mb-2">Select a location</p>
        <p className="text-sm text-center mb-6">
          Choose a location from the tree to view its details
        </p>
        <div className="text-center">
          <p className="text-xs text-gray-400 dark:text-gray-500">
            {rootLocations.length} root location{rootLocations.length !== 1 ? 's' : ''} • {totalLocations} total
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className={`h-full flex flex-col overflow-hidden ${className}`} data-testid="location-details-panel">
      {/* Header */}
      <div className="flex-shrink-0 border-b border-gray-200 dark:border-gray-700 px-6 py-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Icon className="h-6 w-6 text-gray-500 dark:text-gray-400" />
            <div>
              <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
                {location.identifier}
              </h2>
              <p className="text-sm text-gray-500 dark:text-gray-400">{location.name}</p>
            </div>
          </div>
          <span
            className={`
              inline-flex items-center gap-2 px-3 py-1 rounded-full text-sm font-medium
              ${
                location.is_active
                  ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                  : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
              }
            `}
          >
            {location.is_active ? (
              <>
                <CheckCircle className="h-4 w-4" />
                Active
              </>
            ) : (
              <>
                <XCircle className="h-4 w-4" />
                Inactive
              </>
            )}
          </span>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-6 py-6 space-y-6">
        {/* Breadcrumb */}
        <div>
          <LocationBreadcrumb
            locationId={location.id}
            onLocationClick={onChildClick ? (loc) => onChildClick(loc.id) : undefined}
          />
        </div>

        {/* Basic info */}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
              Identifier
            </label>
            <p className="text-lg font-semibold text-gray-900 dark:text-white">
              {location.identifier}
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
              Name
            </label>
            <p className="text-gray-900 dark:text-white">{location.name}</p>
          </div>

          {location.description && (
            <div>
              <label className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                Description
              </label>
              <p className="text-gray-700 dark:text-gray-300">{location.description}</p>
            </div>
          )}
        </div>

        {/* Tag Identifiers */}
        <TagIdentifierList
          identifiers={localIdentifiers}
          size="md"
          showHeader
          entityId={location.id}
          entityType="location"
          onIdentifierRemoved={handleIdentifierRemoved}
        />

        {/* Sub-locations */}
        {children.length > 0 && (
          <div className="border-t border-gray-200 dark:border-gray-700 pt-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">
              Sub-locations ({children.length})
            </h3>
            <div className="space-y-2 max-h-48 overflow-y-auto">
              {children.map((child) => (
                <div
                  key={child.id}
                  className="flex items-center gap-2 p-2 bg-gray-50 dark:bg-gray-800 rounded hover:bg-gray-100 dark:hover:bg-gray-700 cursor-pointer transition-colors"
                  onClick={() => onChildClick?.(child.id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      onChildClick?.(child.id);
                    }
                  }}
                >
                  <MapPin className="h-4 w-4 text-gray-500 dark:text-gray-400" />
                  <span className="text-sm font-medium text-gray-900 dark:text-white">
                    {child.identifier}
                  </span>
                  <span className="text-xs text-gray-500 dark:text-gray-400">
                    ({child.name})
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Validity Period */}
        <div className="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-4">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
            Validity Period
          </h3>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="flex items-center gap-2 text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                <Calendar className="h-4 w-4" />
                Valid From
              </label>
              <p className="text-gray-900 dark:text-white">{formatDate(location.valid_from)}</p>
            </div>

            <div>
              <label className="flex items-center gap-2 text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                <Calendar className="h-4 w-4" />
                Valid To
              </label>
              <p className="text-gray-900 dark:text-white">{formatDate(location.valid_to)}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Actions footer */}
      <div className="flex-shrink-0 border-t border-gray-200 dark:border-gray-700 px-6 py-4 bg-gray-50 dark:bg-gray-800">
        <div className="flex justify-between">
          <button
            onClick={() => onDelete(location.id)}
            className="px-4 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 rounded-lg transition-colors"
          >
            Delete
          </button>
          <div className="flex gap-3">
            {onAddChild && (
              <button
                onClick={() => onAddChild(location.id)}
                className="flex items-center gap-2 px-4 py-2 text-sm font-medium text-green-700 bg-green-50 hover:bg-green-100 dark:text-green-400 dark:bg-green-900/20 dark:hover:bg-green-900/40 border border-green-200 dark:border-green-800 rounded-lg transition-colors"
              >
                <Plus className="h-4 w-4" />
                Add Child
              </button>
            )}
            <button
              onClick={() => onMove(location.id)}
              className="px-4 py-2 text-sm font-medium text-purple-700 bg-purple-50 hover:bg-purple-100 dark:text-purple-400 dark:bg-purple-900/20 dark:hover:bg-purple-900/40 border border-purple-200 dark:border-purple-800 rounded-lg transition-colors"
            >
              Move
            </button>
            <button
              onClick={() => onEdit(location.id)}
              className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border border-blue-200 dark:border-blue-800 rounded-lg transition-colors"
            >
              Edit
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
