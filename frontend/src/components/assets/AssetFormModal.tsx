import React, { useState, useEffect } from 'react';
import { X, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { useQueryClient } from '@tanstack/react-query';
import type { Asset, CreateAssetRequest, UpdateAssetRequest, TagInput } from '@/types/assets';
import { AssetForm } from './AssetForm';
import { assetsApi } from '@/lib/api/assets';
import { normalizeAsset } from '@/lib/asset/normalize';
import { useAssetStore } from '@/stores';

interface AssetFormModalProps {
  isOpen: boolean;
  mode: 'create' | 'edit';
  asset?: Asset;
  onClose: (assetCreatedOrUpdated?: boolean) => void;
  initialIdentifier?: string;
}

// TRA-817: The outer component is a thin gate that returns null when closed,
// guaranteeing the stateful body (AssetFormModalBody) unmounts on close. This
// keeps `freshAsset`/`loadingAsset`/tag-diff baselines from leaking across
// open cycles when a parent keeps the modal mounted via an `isOpen` prop
// (e.g. InventoryTableRow). A leaked baseline can drive phantom DELETEs via
// the TRA-813 tag-diff path and brief stale renders of the prior edit's tags.
export function AssetFormModal(props: AssetFormModalProps) {
  if (!props.isOpen) {
    return null;
  }
  return <AssetFormModalBody {...props} />;
}

function AssetFormModalBody({ isOpen, mode, asset, onClose, initialIdentifier }: AssetFormModalProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [freshAsset, setFreshAsset] = useState<Asset | undefined>(asset);
  const [loadingAsset, setLoadingAsset] = useState(mode === 'edit' && asset?.id != null);

  // Fetch fresh asset data when modal opens in edit mode
  useEffect(() => {
    if (isOpen && mode === 'edit' && asset?.id) {
      setLoadingAsset(true);
      assetsApi.get(asset.id)
        .then((response) => {
          if (response.data?.data) {
            setFreshAsset(normalizeAsset(response.data.data));
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
  const queryClient = useQueryClient();

  const handleSubmit = async (data: CreateAssetRequest | UpdateAssetRequest) => {
    setLoading(true);
    setError(null);

    try {
      if (mode === 'create') {
        // Extract tags from the request (they need to be added separately after creation)
        const newTags = (data as CreateAssetRequest & { tags?: TagInput[] }).tags || [];
        const { tags: _, ...createData } = data as CreateAssetRequest & { tags?: TagInput[] };

        const response = await assetsApi.create(createData as CreateAssetRequest);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }
        const normalized = normalizeAsset(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        const newAssetId = normalized.id;

        const validTags = newTags.filter(t => t.value.trim() !== '');
        for (const tag of validTags) {
          try {
            await assetsApi.addTag(newAssetId, {
              tag_type: tag.type,
              value: tag.value,
            });
          } catch (tagErr: any) {
            console.error('Failed to add tag:', tagErr);
            toast.error(`Failed to add tag "${tag.value}": ${tagErr.message || 'Unknown error'}`);
          }
        }

        if (validTags.length > 0) {
          const freshResponse = await assetsApi.get(newAssetId);
          if (freshResponse.data?.data) {
            addAsset(normalizeAsset(freshResponse.data.data));
          } else {
            addAsset(normalized);
          }
        } else {
          addAsset(normalized);
        }

        toast.success(`Asset "${normalized.external_key}" created successfully`);
      } else if (mode === 'edit' && asset) {
        const allTags = (data as UpdateAssetRequest & { tags?: TagInput[] }).tags || [];
        const newTags = allTags.filter(t => !t.id);

        // TRA-813: diff original tag IDs against surviving (id-bearing) tags
        // to detect removals. The submit path previously only fired POSTs for
        // new tags, so a removed read-only row silently dropped from the UI
        // without a DELETE.
        const originalTags = (freshAsset ?? asset).tags || [];
        const survivingIds = new Set(
          allTags.filter(t => t.id != null).map(t => t.id as number),
        );
        const removedTagIds = originalTags
          .map(t => t.id)
          .filter(id => !survivingIds.has(id));

        const { tags: _, ...updateData } = data as UpdateAssetRequest & { tags?: TagInput[] };

        const response = await assetsApi.update(asset.id, updateData);

        const raw = response.data?.data;
        if (!raw || typeof raw !== 'object') {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }
        const normalized = normalizeAsset(raw);
        if (!normalized.id) {
          throw new Error('Invalid response from server. Asset API may not be available.');
        }

        for (const tagId of removedTagIds) {
          try {
            await assetsApi.removeTag(asset.id, tagId);
          } catch (tagErr: any) {
            console.error('Failed to remove tag:', tagErr);
            toast.error(`Failed to remove tag: ${tagErr.message || 'Unknown error'}`);
          }
        }

        for (const tag of newTags) {
          try {
            await assetsApi.addTag(asset.id, {
              tag_type: tag.type,
              value: tag.value,
            });
          } catch (tagErr: any) {
            console.error('Failed to add tag:', tagErr);
            toast.error(`Failed to add tag "${tag.value}": ${tagErr.message || 'Unknown error'}`);
          }
        }

        const freshResponse = await assetsApi.get(asset.id);
        if (freshResponse.data?.data) {
          updateCachedAsset(asset.id, normalizeAsset(freshResponse.data.data));
        } else {
          updateCachedAsset(asset.id, normalized);
        }

        toast.success(`Asset "${normalized.external_key}" updated successfully`);
      }

      // TRA-824: this modal calls assetsApi directly instead of going through
      // useAssetMutations, so TanStack's ['assets'] cache is not invalidated
      // automatically. Invalidate here so other screens (Inventory) refetch
      // on next mount rather than serving stale list data until a hard refresh.
      queryClient.invalidateQueries({ queryKey: ['assets'] });

      onClose(true);
    } catch (err: any) {
      // Extract error detail from API response if available
      const apiError = err.response?.data?.error?.detail;

      if (err.code === 'ERR_NETWORK' || err.message?.includes('Network Error')) {
        setError('Cannot connect to server. Please check your connection and try again.');
      } else if (err.response?.status === 404) {
        setError('Asset API endpoint not found. The backend may not be running.');
      } else if (err.response?.status === 409) {
        // Conflict - duplicate identifier
        setError(apiError || 'An asset with this identifier already exists.');
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
            {mode === 'create' ? 'Create New Asset' : `Edit Asset: ${asset?.external_key}`}
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
