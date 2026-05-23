import React, { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import type { Location, CreateLocationRequest, UpdateLocationRequest, TagInput } from '@/types/locations';
import { LocationForm } from './LocationForm';
import { locationsApi } from '@/lib/api/locations';
import { normalizeLocation } from '@/lib/location/normalize';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';

interface LocationFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  location?: Location;
  parentLocationId?: number | null;
  onClose: () => void;
}

// TRA-817: outer gate returns null when closed so the stateful body unmounts
// each cycle. Keeps the TRA-813 tag-diff baseline (`location.tags`) tied to
// the current open's prop instead of any state that could survive across
// open/close cycles when a parent keeps the modal mounted.
export function LocationFormModal(props: LocationFormModalProps) {
  if (!props.isOpen) {
    return null;
  }
  return <LocationFormModalBody {...props} />;
}

function LocationFormModalBody({ isOpen, mode, location, parentLocationId, onClose }: LocationFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addLocation = useLocationStore((state) => state.addLocation);
  const updateLocation = useLocationStore((state) => state.updateLocation);

  useEscapeToClose(isOpen, onClose, loading);

  const handleSubmit = async (data: CreateLocationRequest | UpdateLocationRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        // Tags must be added separately after creation
        const newTags = (data as CreateLocationRequest & { tags?: TagInput[] }).tags || [];
        const { tags: _, ...createData } = data as CreateLocationRequest & { tags?: TagInput[] };

        const response = await locationsApi.create(createData as CreateLocationRequest);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Location API may not be available.');
        }
        const normalized = normalizeLocation(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        const newLocationId = normalized.id;
        const validTags = newTags.filter(t => t.value.trim() !== '');
        for (const tag of validTags) {
          try {
            await locationsApi.addTag(newLocationId, {
              tag_type: tag.type,
              value: tag.value,
            });
          } catch (tagErr) {
            console.error('Failed to add tag:', tagErr);
          }
        }

        if (validTags.length > 0) {
          const freshResponse = await locationsApi.get(newLocationId);
          if (freshResponse.data?.data) {
            addLocation(normalizeLocation(freshResponse.data.data));
          } else {
            addLocation(normalized);
          }
        } else {
          addLocation(normalized);
        }

        toast.success(`Location "${normalized.external_key}" created successfully`);
      } else if (mode === 'edit' && location) {
        // New tags (without id) need to be added after update
        const allTags = (data as UpdateLocationRequest & { tags?: TagInput[] }).tags || [];
        const newTags = allTags.filter(t => !t.id);

        // Backend doesn't support tags in update request
        const { tags: _, ...updateData } = data as UpdateLocationRequest & { tags?: TagInput[] };

        const response = await locationsApi.update(location.id, updateData);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Location API may not be available.');
        }
        const normalized = normalizeLocation(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        for (const tag of newTags) {
          try {
            await locationsApi.addTag(location.id, {
              tag_type: tag.type,
              value: tag.value,
            });
          } catch (tagErr) {
            console.error('Failed to add tag:', tagErr);
          }
        }

        const freshResponse = await locationsApi.get(location.id);
        if (freshResponse.data?.data) {
          updateLocation(location.id, normalizeLocation(freshResponse.data.data));
        } else {
          updateLocation(location.id, normalized);
        }

        toast.success(`Location "${normalized.external_key}" updated successfully`);
      }

      onClose();
    } catch (err: any) {
      const apiError = err.response?.data?.error?.detail;

      if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
        setError('Cannot connect to server. Please check your connection and try again.');
      } else if (err.response?.status === 404) {
        setError('Location API endpoint not found. The backend may not be running.');
      } else if (err.response?.status === 409) {
        setError(apiError || 'A tag on this location is already attached elsewhere.');
      } else if (err.response?.status >= 500) {
        setError(apiError || 'Server error. Please try again later.');
      } else if (err.response?.status >= 400) {
        setError(apiError || err.message || 'Invalid request. Please check your input.');
      } else {
        setError(err.message || 'An error occurred. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !loading) {
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Location' : `Edit Location: ${location?.external_key}`}
          </h2>
          <button
            onClick={onClose}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6">
          <LocationForm
            mode={mode}
            location={location}
            parentLocationId={parentLocationId}
            onSubmit={handleSubmit}
            onCancel={onClose}
            loading={loading}
            error={error}
          />
        </div>
      </div>
    </div>
  );
}
