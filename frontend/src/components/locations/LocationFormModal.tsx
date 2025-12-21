import React, { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import type { Location, CreateLocationRequest, UpdateLocationRequest, TagIdentifierInput } from '@/types/locations';
import { LocationForm } from './LocationForm';
import { locationsApi } from '@/lib/api/locations';
import { useLocationStore } from '@/stores/locations/locationStore';

interface LocationFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  location?: Location;
  onClose: () => void;
}

export function LocationFormModal({ isOpen, mode, location, onClose }: LocationFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addLocation = useLocationStore((state) => state.addLocation);
  const updateLocation = useLocationStore((state) => state.updateLocation);

  const handleSubmit = async (data: CreateLocationRequest | UpdateLocationRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        // Identifiers must be added separately after creation
        const identifiers = (data as CreateLocationRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const { identifiers: _, ...createData } = data as CreateLocationRequest & { identifiers?: TagIdentifierInput[] };

        const response = await locationsApi.create(createData as CreateLocationRequest);

        if (!response.data?.data || typeof response.data.data !== 'object' || !response.data.data.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        const newLocationId = response.data.data.id;
        const validIdentifiers = identifiers.filter(id => id.value.trim() !== '');
        for (const identifier of validIdentifiers) {
          try {
            await locationsApi.addIdentifier(newLocationId, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr) {
            console.error('Failed to add identifier:', idErr);
          }
        }

        // Refetch to get identifiers included in response
        if (validIdentifiers.length > 0) {
          const freshResponse = await locationsApi.get(newLocationId);
          if (freshResponse.data?.data) {
            addLocation(freshResponse.data.data);
          } else {
            addLocation(response.data.data);
          }
        } else {
          addLocation(response.data.data);
        }

        toast.success(`Location "${response.data.data.identifier}" created successfully`);
      } else if (mode === 'edit' && location) {
        // New identifiers (without id) need to be added after update
        const identifiers = (data as UpdateLocationRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const newIdentifiers = identifiers.filter(id => !id.id);

        // Backend doesn't support identifiers in update request
        const { identifiers: _, ...updateData } = data as UpdateLocationRequest & { identifiers?: TagIdentifierInput[] };

        const response = await locationsApi.update(location.id, updateData);

        if (!response.data?.data || typeof response.data.data !== 'object' || !response.data.data.id) {
          throw new Error('Invalid response from server. Location API may not be available.');
        }

        for (const identifier of newIdentifiers) {
          try {
            await locationsApi.addIdentifier(location.id, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr) {
            console.error('Failed to add identifier:', idErr);
          }
        }

        const freshResponse = await locationsApi.get(location.id);
        if (freshResponse.data?.data) {
          updateLocation(location.id, freshResponse.data.data);
        } else {
          updateLocation(location.id, response.data.data);
        }

        toast.success(`Location "${response.data.data.identifier}" updated successfully`);
      }

      onClose();
    } catch (err: any) {
      if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
        setError('Cannot connect to server. Please check your connection and try again.');
      } else if (err.response?.status === 404) {
        setError('Location API endpoint not found. The backend may not be running.');
      } else if (err.response?.status >= 500) {
        setError('Server error. Please try again later.');
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

  if (!isOpen) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="relative w-full max-w-2xl bg-white dark:bg-gray-900 rounded-lg shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Location' : `Edit Location: ${location?.identifier}`}
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
