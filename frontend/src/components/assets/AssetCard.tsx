import React, { useState, useEffect } from 'react';
import { Pencil, Trash2, User, Laptop, Package, Archive, HelpCircle, MapPin } from 'lucide-react';
import type { Asset } from '@/types/assets';
import type { TagIdentifier } from '@/types/shared';
import { useLocationStore, useAssetStore } from '@/stores';
import { TagCountBadge } from './TagCountBadge';
import { TagIdentifiersModal } from './TagIdentifiersModal';
import { LocateTagPopover } from './LocateTagPopover';

interface AssetCardProps {
  asset: Asset;
  onClick?: () => void;
  onEdit?: (asset: Asset) => void;
  onDelete?: (asset: Asset) => void;
  variant?: 'card' | 'row';
  showActions?: boolean;
  className?: string;
}

const TYPE_ICONS = {
  person: User,
  device: Laptop,
  asset: Package,
  inventory: Archive,
  other: HelpCircle,
} as const;

export function AssetCard({
  asset,
  onClick,
  onEdit,
  onDelete,
  variant = 'card',
  showActions = true,
  className = '',
}: AssetCardProps) {
  const TypeIcon = TYPE_ICONS[asset.type] || HelpCircle;
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const locationData = asset.current_location_id ? getLocationById(asset.current_location_id) : null;
  const locationName = locationData?.name;

  const [tagsModalOpen, setTagsModalOpen] = useState(false);
  const [localIdentifiers, setLocalIdentifiers] = useState<TagIdentifier[]>(asset.identifiers || []);
  const updateCachedAsset = useAssetStore((state) => state.updateCachedAsset);

  // Sync local identifiers when asset prop changes
  useEffect(() => {
    setLocalIdentifiers(asset.identifiers || []);
  }, [asset.identifiers]);

  const handleOpenTagsModal = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (localIdentifiers.length > 0) {
      setTagsModalOpen(true);
    }
  };

  const handleIdentifierRemoved = (identifierId: number) => {
    // Update local state
    const updatedIdentifiers = localIdentifiers.filter((i) => i.id !== identifierId);
    setLocalIdentifiers(updatedIdentifiers);

    // Update the asset store cache so the change persists when opening forms
    updateCachedAsset(asset.id, {
      ...asset,
      identifiers: updatedIdentifiers,
    });
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit?.(asset);
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.(asset);
  };

  const handleClick = () => {
    onClick?.();
  };

  if (variant === 'row') {
    return (
      <>
        <tr
          onClick={handleClick}
          className={`
            border-b border-gray-200 dark:border-gray-700
            hover:bg-blue-50 dark:hover:bg-blue-900/20
            cursor-pointer transition-colors
            ${className}
          `}
        >
          {/* Icon + Type */}
          <td className="px-2 sm:px-4 py-2 sm:py-3">
            <div className="flex items-center gap-1 sm:gap-2">
              <TypeIcon className="h-4 w-4 sm:h-5 sm:w-5 text-gray-500 dark:text-gray-400" />
              <span className="text-xs sm:text-sm text-gray-700 dark:text-gray-300 capitalize hidden sm:inline">
                {asset.type}
              </span>
            </div>
          </td>

          {/* Identifier */}
          <td className="px-2 sm:px-4 py-2 sm:py-3">
            <span className="text-xs sm:text-sm font-medium text-gray-900 dark:text-white truncate block max-w-[100px] sm:max-w-none">
              {asset.identifier}
            </span>
          </td>

          {/* Name */}
          <td className="px-2 sm:px-4 py-2 sm:py-3 hidden md:table-cell">
            <span className="text-sm text-gray-700 dark:text-gray-300">{asset.name}</span>
          </td>

          {/* Location */}
          <td className="px-2 sm:px-4 py-2 sm:py-3 hidden lg:table-cell">
            {locationName ? (
              <span className="inline-flex items-center gap-1 text-sm text-gray-600 dark:text-gray-400">
                <MapPin className="h-3.5 w-3.5" />
                {locationName}
              </span>
            ) : (
              <span className="text-sm text-gray-400 dark:text-gray-500">-</span>
            )}
          </td>

          {/* Tags */}
          <td className="px-2 sm:px-4 py-2 sm:py-3">
            <TagCountBadge
              identifiers={localIdentifiers}
              onClick={localIdentifiers.length ? handleOpenTagsModal : undefined}
            />
          </td>

          {/* Status */}
          <td className="px-2 sm:px-4 py-2 sm:py-3">
            <span
              className={`
                inline-flex items-center px-1.5 sm:px-2.5 py-0.5 rounded-full text-xs font-medium
                ${
                  asset.is_active
                    ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                    : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
                }
              `}
            >
              <span className="hidden sm:inline">{asset.is_active ? 'Active' : 'Inactive'}</span>
              <span className="sm:hidden">{asset.is_active ? '✓' : '✗'}</span>
            </span>
          </td>

          {/* Actions */}
          <td className="px-2 sm:px-4 py-2 sm:py-3">
            {showActions && (
              <div className="flex items-center gap-1 sm:gap-2">
                <LocateTagPopover
                  identifiers={localIdentifiers}
                  entityIdentifier={asset.identifier}
                  isActive={asset.is_active}
                  variant="icon"
                />
                <button
                  onClick={handleEdit}
                  className="p-1 sm:p-1.5 text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/20 rounded transition-colors"
                  aria-label={`Edit ${asset.identifier}`}
                >
                  <Pencil className="h-4 w-4" />
                </button>
                <button
                  onClick={handleDelete}
                  className="p-1 sm:p-1.5 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 rounded transition-colors"
                  aria-label={`Delete ${asset.identifier}`}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            )}
          </td>
        </tr>

        {/* Tag Identifiers Modal */}
        <TagIdentifiersModal
          identifiers={localIdentifiers}
          entityId={asset.id}
          entityName={asset.identifier}
          entityType="asset"
          isOpen={tagsModalOpen}
          onClose={() => setTagsModalOpen(false)}
          onIdentifierRemoved={handleIdentifierRemoved}
        />
      </>
    );
  }

  return (
    <>
      <div
        onClick={handleClick}
        className={`
          border border-gray-200 dark:border-gray-700
          rounded-lg p-3 sm:p-4
          hover:border-blue-500 hover:bg-blue-50 dark:hover:border-blue-500 dark:hover:bg-blue-900/20
          cursor-pointer transition-colors
          ${className}
        `}
      >
        {/* Header: Icon + Identifier */}
        <div className="flex items-start gap-2 sm:gap-3 mb-2 sm:mb-3">
          <TypeIcon className="h-5 w-5 sm:h-6 sm:w-6 text-gray-500 dark:text-gray-400 mt-0.5 sm:mt-1 flex-shrink-0" />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-0.5 flex-wrap">
              <h3 className="text-sm sm:text-base font-semibold text-gray-900 dark:text-white truncate">
                {asset.identifier}
              </h3>
              {localIdentifiers.length > 0 && (
                <TagCountBadge
                  identifiers={localIdentifiers}
                  onClick={handleOpenTagsModal}
                />
              )}
            </div>
            <p className="text-xs sm:text-sm text-gray-700 dark:text-gray-300 truncate">{asset.name}</p>
          </div>
        </div>

        {/* Location */}
        {locationName && (
          <div className="mb-2 sm:mb-3 pl-7 sm:pl-9">
            <p className="text-xs sm:text-sm text-gray-600 dark:text-gray-400 flex items-center gap-1">
              <MapPin className="h-3 w-3 sm:h-3.5 sm:w-3.5 flex-shrink-0" />
              <span className="truncate">{locationName}</span>
            </p>
          </div>
        )}

        {/* Status */}
        <div className="mb-3 sm:mb-4 pl-7 sm:pl-9">
          <span
            className={`
              inline-flex items-center px-2 sm:px-3 py-0.5 sm:py-1 rounded-full text-xs font-medium
              ${
                asset.is_active
                  ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                  : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
              }
            `}
          >
            {asset.is_active ? 'Active' : 'Inactive'}
          </span>
        </div>

        {/* Actions */}
        {showActions && (
          <div className="flex gap-1.5 sm:gap-2 pt-2 sm:pt-3 border-t border-gray-200 dark:border-gray-700">
            <LocateTagPopover
              identifiers={localIdentifiers}
              entityIdentifier={asset.identifier}
              isActive={asset.is_active}
              variant="button"
            />
            <button
              onClick={handleEdit}
              className="flex-1 flex items-center justify-center gap-1 sm:gap-2 px-2 sm:px-3 py-1.5 sm:py-2 text-xs sm:text-sm font-medium text-gray-700 bg-gray-50 hover:bg-gray-100 dark:text-gray-400 dark:bg-gray-900/20 dark:hover:bg-gray-900/40 border border-gray-200 dark:border-gray-800 rounded-lg transition-colors"
            >
              <Pencil className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              <span className="hidden xs:inline">Edit</span>
            </button>
            <button
              onClick={handleDelete}
              className="flex-1 flex items-center justify-center gap-1 sm:gap-2 px-2 sm:px-3 py-1.5 sm:py-2 text-xs sm:text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 rounded-lg transition-colors"
            >
              <Trash2 className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              <span className="hidden xs:inline">Delete</span>
            </button>
          </div>
        )}
      </div>

      {/* Tag Identifiers Modal */}
      <TagIdentifiersModal
        identifiers={localIdentifiers}
        entityId={asset.id}
        entityName={asset.identifier}
        entityType="asset"
        isOpen={tagsModalOpen}
        onClose={() => setTagsModalOpen(false)}
        onIdentifierRemoved={handleIdentifierRemoved}
      />
    </>
  );
}
