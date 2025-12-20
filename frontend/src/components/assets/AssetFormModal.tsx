import React, { useState } from 'react';
import { X } from 'lucide-react';
import toast from 'react-hot-toast';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, TagIdentifierInput } from '@/types/assets';
import { AssetForm } from './AssetForm';
import { assetsApi } from '@/lib/api/assets';
import { useAssetStore } from '@/stores';

interface AssetFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  asset?: Asset;
  onClose: (assetCreatedOrUpdated?: boolean) => void;
  initialIdentifier?: string;
}

export function AssetFormModal({ isOpen, mode, asset, onClose, initialIdentifier }: AssetFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addAsset = useAssetStore((state) => state.addAsset);
  const updateCachedAsset = useAssetStore((state) => state.updateCachedAsset);

  const handleSubmit = async (data: CreateAssetRequest | UpdateAssetRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        const response = await assetsApi.create(data as CreateAssetRequest);

        if (!response.data?.data || typeof response.data.data !== 'object' || !response.data.data.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        addAsset(response.data.data);
        toast.success(`Asset "${response.data.data.identifier}" created successfully`);
      } else if (mode === 'edit' && asset) {
        // Extract new identifiers (those without an id) from the request
        const identifiers = (data as UpdateAssetRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const newIdentifiers = identifiers.filter(id => !id.id);

        // Remove identifiers from the update request (backend doesn't support it)
        const { identifiers: _, ...updateData } = data as UpdateAssetRequest & { identifiers?: TagIdentifierInput[] };

        // Update the asset first
        const response = await assetsApi.update(asset.id, updateData);

        if (!response.data?.data || typeof response.data.data !== 'object' || !response.data.data.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        // Add new identifiers one by one
        for (const identifier of newIdentifiers) {
          try {
            await assetsApi.addIdentifier(asset.id, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr: any) {
            // If adding identifier fails, show a warning but don't fail the whole operation
            console.error('Failed to add identifier:', idErr);
            toast.error(`Failed to add tag "${identifier.value}": ${idErr.message || 'Unknown error'}`);
          }
        }

        // Fetch fresh asset data with all identifiers
        const freshResponse = await assetsApi.get(asset.id);
        if (freshResponse.data?.data) {
          updateCachedAsset(asset.id, freshResponse.data.data);
        } else {
          // Fall back to the update response if get fails
          updateCachedAsset(asset.id, response.data.data);
        }

        toast.success(`Asset "${response.data.data.identifier}" updated successfully`);
      }

      onClose(true);
    } catch (err: any) {
      if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
        setError('Cannot connect to server. Please check your connection and try again.');
      } else if (err.response?.status === 404) {
        setError('Asset API endpoint not found. The backend may not be running.');
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
            {mode === 'create' ? 'Create New Asset' : `Edit Asset: ${asset?.identifier}`}
          </h2>
          <button
            onClick={() => onClose()}
            disabled={loading}
            className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg transition-colors disabled:opacity-50"
            aria-label="Close modal"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="px-6 py-6">
          <AssetForm
            mode={mode}
            asset={asset}
            onSubmit={handleSubmit}
            onCancel={() => onClose()}
            loading={loading}
            error={error}
            initialIdentifier={initialIdentifier}
          />
        </div>
      </div>
    </div>
  );
}
