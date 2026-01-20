import { useState, useEffect } from 'react';
import { X, MapPin, Building2, Calendar, CheckCircle, XCircle } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { LocationBreadcrumb } from './LocationBreadcrumb';
import { TagIdentifierList } from '@/components/assets';
import type { Location } from '@/types/locations';
import type { TagIdentifier } from '@/types/shared';

interface LocationDetailsModalProps {
  isOpen: boolean;
  location: Location;
  onClose: () => void;
  onEdit?: (location: Location) => void;
  onDelete?: (location: Location) => void;
  onMove?: (location: Location) => void;
  onLocationClick?: (location: Location) => void;
}

export function LocationDetailsModal({
  isOpen,
  location,
  onClose,
  onEdit,
  onDelete,
  onMove,
  onLocationClick,
}: LocationDetailsModalProps) {
  const getChildren = useLocationStore((state) => state.getChildren);
  const getDescendants = useLocationStore((state) => state.getDescendants);
  const updateCachedLocation = useLocationStore((state) => state.updateCachedLocation);

  const [localIdentifiers, setLocalIdentifiers] = useState<TagIdentifier[]>([]);

  useEffect(() => {
    setLocalIdentifiers(location.identifiers || []);
  }, [location.identifiers]);

  const handleIdentifierRemoved = (identifierId: number) => {
    const updatedIdentifiers = localIdentifiers.filter((i) => i.id !== identifierId);
    setLocalIdentifiers(updatedIdentifiers);

    updateCachedLocation(location.id, {
      ...location,
      identifiers: updatedIdentifiers,
    });
  };

  const children = getChildren(location.id);
  const descendants = getDescendants(location.id);
  const isRoot = location.parent_location_id === null;
  const Icon = isRoot ? Building2 : MapPin;

  if (!isOpen) return null;

  const formatDate = (dateString: string | null) => {
    if (!dateString) return 'Not set';
    return new Date(dateString).toLocaleDateString();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50">
      <div className="w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex justify-between items-center bg-white dark:bg-gray-900 z-10">
          <div className="flex items-center gap-3">
            <Icon className="h-6 w-6 text-gray-500 dark:text-gray-400" />
            <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
              Location Details
            </h2>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6 space-y-6">
          <div>
            <LocationBreadcrumb locationId={location.id} onLocationClick={onLocationClick} />
          </div>

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

            <div>
              <label className="block text-sm font-medium text-gray-500 dark:text-gray-400 mb-1">
                Status
              </label>
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

            {/* Tag Identifiers */}
            <TagIdentifierList
              identifiers={localIdentifiers}
              size="md"
              showHeader
              entityId={location.id}
              entityType="location"
              onIdentifierRemoved={handleIdentifierRemoved}
            />
          </div>

          <div className="border-t border-gray-200 dark:border-gray-700 pt-4 space-y-4">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
              Hierarchy Information
            </h3>

            <div className="grid grid-cols-2 gap-4">
              <div className="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">Type</p>
                <p className="font-medium text-gray-900 dark:text-white">
                  {isRoot ? 'Top Level' : 'Sub-location'}
                </p>
              </div>

              <div className="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">Sub-locations</p>
                <p className="font-medium text-gray-900 dark:text-white">{children.length}</p>
              </div>

              <div className="p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
                <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">Total Descendants</p>
                <p className="font-medium text-gray-900 dark:text-white">{descendants.length}</p>
              </div>
            </div>

            {children.length > 0 && (
              <div>
                <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
                  Sub-locations:
                </p>
                <div className="space-y-2">
                  {children.map((child) => (
                    <div
                      key={child.id}
                      className="flex items-center gap-2 p-2 bg-gray-50 dark:bg-gray-800 rounded hover:bg-gray-100 dark:hover:bg-gray-700 cursor-pointer transition-colors"
                      onClick={() => onLocationClick?.(child)}
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
          </div>

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

        <div className="sticky bottom-0 border-t border-gray-200 dark:border-gray-700 px-6 py-4 flex justify-between bg-gray-50 dark:bg-gray-800">
          <div className="flex gap-3">
            {onDelete && (
              <button
                onClick={() => {
                  onDelete(location);
                  onClose();
                }}
                className="px-4 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 rounded-lg transition-colors"
              >
                Delete
              </button>
            )}
          </div>
          <div className="flex gap-3">
            {onMove && (
              <button
                onClick={() => {
                  onMove(location);
                  onClose();
                }}
                className="px-4 py-2 text-sm font-medium text-purple-700 bg-purple-50 hover:bg-purple-100 dark:text-purple-400 dark:bg-purple-900/20 dark:hover:bg-purple-900/40 border border-purple-200 dark:border-purple-800 rounded-lg transition-colors"
              >
                Move
              </button>
            )}
            {onEdit && (
              <button
                onClick={() => {
                  onEdit(location);
                  onClose();
                }}
                className="px-4 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border border-blue-200 dark:border-blue-800 rounded-lg transition-colors"
              >
                Edit
              </button>
            )}
            <button
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600 rounded-lg transition-colors"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
