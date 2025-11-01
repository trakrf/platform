import React, { useState } from 'react';
import { X } from 'lucide-react';
import type { Asset, CreateAssetRequest, UpdateAssetRequest } from '@/types/assets';
import { AssetForm } from './AssetForm';
import { assetsApi } from '@/lib/api/assets';
import { useAssetStore } from '@/stores';

interface AssetFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  asset?: Asset;
  onClose: () => void;
}

export function AssetFormModal({ isOpen, mode, asset, onClose }: AssetFormModalProps) {
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

        if (!response.data || typeof response.data !== 'object' || !response.data.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        addAsset(response.data);
      } else if (mode === 'edit' && asset) {
        const response = await assetsApi.update(asset.id, data as UpdateAssetRequest);

        if (!response.data || typeof response.data !== 'object' || !response.data.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        updateCachedAsset(asset.id, response.data);
      }

      onClose();
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
        {/* Header */}
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 px-6 py-4 flex items-center justify-between z-10">
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
            {mode === 'create' ? 'Create New Asset' : `Edit Asset: ${asset?.identifier}`}
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

        {/* Form Content */}
        <div className="px-6 py-6">
          <AssetForm
            mode={mode}
            asset={asset}
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
