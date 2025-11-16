import { useState, useMemo } from 'react';
import { X, AlertCircle, MapPin } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { LocationParentSelector } from './LocationParentSelector';
import type { Location } from '@/types/locations';

interface LocationMoveModalProps {
  isOpen: boolean;
  location: Location;
  onClose: () => void;
  onConfirm: (locationId: number, newParentId: number | null) => Promise<void>;
}

export function LocationMoveModal({
  isOpen,
  location,
  onClose,
  onConfirm,
}: LocationMoveModalProps) {
  const [newParentId, setNewParentId] = useState<number | null>(location.parent_location_id);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const getLocationById = useLocationStore((state) => state.getLocationById);
  const getDescendants = useLocationStore((state) => state.getDescendants);
  const getAncestors = useLocationStore((state) => state.getAncestors);

  const descendants = useMemo(() => getDescendants(location.id), [location.id, getDescendants]);

  const hasChanges = newParentId !== location.parent_location_id;
  const willAffectDescendants = descendants.length > 0;

  const newPath = useMemo(() => {
    if (newParentId === null) {
      return [location.identifier];
    }
    const ancestors = getAncestors(newParentId);
    const parent = getLocationById(newParentId);
    if (!parent) return [location.identifier];
    return [...ancestors.map((a) => a.identifier), parent.identifier, location.identifier];
  }, [newParentId, location.identifier, getAncestors, getLocationById]);

  if (!isOpen) return null;

  const handleConfirm = async () => {
    setLoading(true);
    setError(null);
    try {
      await onConfirm(location.id, newParentId);
      onClose();
    } catch (err: any) {
      setError(err.message || 'Failed to move location');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50">
      <div className="w-full max-w-md bg-white dark:bg-gray-900 rounded-lg shadow-xl">
        <div className="border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex justify-between items-center">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">Move Location</h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            disabled={loading}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6 space-y-4">
          {error && (
            <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg">
              <p className="text-sm text-red-800 dark:text-red-200">{error}</p>
            </div>
          )}

          <div>
            <div className="flex items-center gap-2 mb-4 p-3 bg-gray-50 dark:bg-gray-800 rounded-lg">
              <MapPin className="h-5 w-5 text-gray-500 dark:text-gray-400" />
              <div>
                <span className="font-medium text-gray-900 dark:text-white">
                  {location.identifier}
                </span>
                <span className="text-sm text-gray-500 dark:text-gray-400 ml-2">
                  ({location.name})
                </span>
              </div>
            </div>

            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              New Parent Location
            </label>
            <LocationParentSelector
              value={newParentId}
              onChange={setNewParentId}
              currentLocationId={location.id}
              disabled={loading}
            />
          </div>

          {willAffectDescendants && (
            <div className="flex items-start gap-2 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg">
              <AlertCircle className="h-5 w-5 text-yellow-600 dark:text-yellow-500 flex-shrink-0 mt-0.5" />
              <div className="text-sm">
                <p className="font-medium text-yellow-800 dark:text-yellow-300">
                  This location has {descendants.length} descendant
                  {descendants.length === 1 ? '' : 's'}
                </p>
                <p className="text-yellow-700 dark:text-yellow-400 mt-1">
                  Moving this location will also move all its children and descendants.
                </p>
              </div>
            </div>
          )}

          {hasChanges && (
            <div className="p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
              <p className="text-sm font-medium text-blue-800 dark:text-blue-300 mb-1">
                New path:
              </p>
              <p className="text-sm text-blue-700 dark:text-blue-400">{newPath.join(' > ')}</p>
            </div>
          )}
        </div>

        <div className="border-t border-gray-200 dark:border-gray-700 px-6 py-4 flex justify-end gap-3 bg-gray-50 dark:bg-gray-800">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-gray-700 bg-white dark:text-gray-300 dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-600 transition-colors"
            disabled={loading}
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={!hasChanges || loading}
          >
            {loading ? 'Moving...' : 'Move Location'}
          </button>
        </div>
      </div>
    </div>
  );
}
