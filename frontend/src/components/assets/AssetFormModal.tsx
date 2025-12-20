import React, { useState, useEffect } from 'react';
import { X, Loader2 } from 'lucide-react';
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
  const [freshAsset, setFreshAsset] = useState<Asset | undefined>(asset);
  const [loadingAsset, setLoadingAsset] = useState(false);

  // Fetch fresh asset data when modal opens in edit mode
  useEffect(() => {
    if (isOpen && mode === 'edit' && asset?.id) {
      setLoadingAsset(true);
      assetsApi.get(asset.id)
        .then((response) => {
          if (response.data?.data) {
            setFreshAsset(response.data.data);
          } else {
            setFreshAsset(asset);
          }
        })
        .catch(() => {
          // Fall back to passed asset if fetch fails
          setFreshAsset(asset);
        })
        .finally(() => {
          setLoadingAsset(false);
        });
    } else if (isOpen && mode === 'create') {
      setFreshAsset(undefined);
    }
  }, [isOpen, mode, asset?.id]);

  const addAsset = useAssetStore((state) => state.addAsset);
  const updateCachedAsset = useAssetStore((state) => state.updateCachedAsset);

  const handleSubmit = async (data: CreateAssetRequest | UpdateAssetRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        // Extract identifiers from the request (they need to be added separately after creation)
        const identifiers = (data as CreateAssetRequest & { identifiers?: TagIdentifierInput[] }).identifiers || [];
        const { identifiers: _, ...createData } = data as CreateAssetRequest & { identifiers?: TagIdentifierInput[] };

        const response = await assetsApi.create(createData as CreateAssetRequest);

        if (!response.data?.data || typeof response.data.data !== 'object' || !response.data.data.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        const newAssetId = response.data.data.id;

        // Add identifiers to the newly created asset
        const validIdentifiers = identifiers.filter(id => id.value.trim() !== '');
        for (const identifier of validIdentifiers) {
          try {
            await assetsApi.addIdentifier(newAssetId, {
              type: identifier.type,
              value: identifier.value,
            });
          } catch (idErr: any) {
            console.error('Failed to add identifier:', idErr);
            toast.error(`Failed to add tag "${identifier.value}": ${idErr.message || 'Unknown error'}`);
          }
        }

        // Fetch fresh asset data with all identifiers if any were added
        if (validIdentifiers.length > 0) {
          const freshResponse = await assetsApi.get(newAssetId);
          if (freshResponse.data?.data) {
            addAsset(freshResponse.data.data);
          } else {
            addAsset(response.data.data);
          }
        } else {
          addAsset(response.data.data);
        }

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
          {loadingAsset ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
              <span className="ml-3 text-gray-600 dark:text-gray-400">Loading asset data...</span>
            </div>
          ) : (
            <AssetForm
              mode={mode}
              asset={freshAsset}
              onSubmit={handleSubmit}
              onCancel={() => onClose()}
              loading={loading}
              error={error}
              initialIdentifier={initialIdentifier}
            />
          )}
        </div>
      </div>
    </div>
  );
}
