import React, { useState } from 'react';
import { Pencil, Trash2, User, Laptop, Package, Archive, HelpCircle, MapPin, Target } from 'lucide-react';
import type { Asset } from '@/types/assets';
import { useLocationStore } from '@/stores';
import { TagCountBadge } from './TagCountBadge';
import { TagIdentifierList } from './TagIdentifierList';

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

  const [tagsExpanded, setTagsExpanded] = useState(false);

  const handleToggleTags = (e: React.MouseEvent) => {
    e.stopPropagation();
    setTagsExpanded(!tagsExpanded);
  };

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit?.(asset);
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.(asset);
  };

  const handleLocate = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (asset.is_active && asset.identifier) {
      window.location.hash = `#locate?epc=${encodeURIComponent(asset.identifier)}`;
    }
  };

  const hasIdentifier = !!asset.identifier;
  const canLocate = asset.is_active && hasIdentifier;

  const handleClick = () => {
    onClick?.();
  };

  if (variant === 'row') {
    return (
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
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            <TypeIcon className="h-5 w-5 text-gray-500 dark:text-gray-400" />
            <span className="text-sm text-gray-700 dark:text-gray-300 capitalize">
              {asset.type}
            </span>
          </div>
        </td>

        {/* Identifier */}
        <td className="px-4 py-3">
          <span className="text-sm font-medium text-gray-900 dark:text-white">
            {asset.identifier}
          </span>
        </td>

        {/* Name */}
        <td className="px-4 py-3">
          <span className="text-sm text-gray-700 dark:text-gray-300">{asset.name}</span>
        </td>

        {/* Location */}
        <td className="px-4 py-3">
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
        <td className="px-4 py-3">
          <TagCountBadge identifiers={asset.identifiers} />
        </td>

        {/* Status */}
        <td className="px-4 py-3">
          <span
            className={`
              inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium
              ${
                asset.is_active
                  ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                  : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
              }
            `}
          >
            {asset.is_active ? 'Active' : 'Inactive'}
          </span>
        </td>

        {/* Actions */}
        <td className="px-4 py-3">
          {showActions && (
            <div className="flex items-center gap-2">
              {hasIdentifier && (
                <button
                  onClick={handleLocate}
                  disabled={!canLocate}
                  data-testid="locate-button"
                  className={`p-1.5 rounded transition-colors ${
                    canLocate
                      ? 'text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20'
                      : 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
                  }`}
                  aria-label={`Locate ${asset.identifier}`}
                >
                  <Target className="h-4 w-4" />
                </button>
              )}
              <button
                onClick={handleEdit}
                className="p-1.5 text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/20 rounded transition-colors"
                aria-label={`Edit ${asset.identifier}`}
              >
                <Pencil className="h-4 w-4" />
              </button>
              <button
                onClick={handleDelete}
                className="p-1.5 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 rounded transition-colors"
                aria-label={`Delete ${asset.identifier}`}
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          )}
        </td>
      </tr>
    );
  }

  return (
    <div
      onClick={handleClick}
      className={`
        border border-gray-200 dark:border-gray-700
        rounded-lg p-4
        hover:border-blue-500 hover:bg-blue-50 dark:hover:border-blue-500 dark:hover:bg-blue-900/20
        cursor-pointer transition-colors
        ${className}
      `}
    >
      {/* Header: Icon + Identifier */}
      <div className="flex items-start gap-3 mb-3">
        <TypeIcon className="h-6 w-6 text-gray-500 dark:text-gray-400 mt-1" />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <h3 className="text-base font-semibold text-gray-900 dark:text-white truncate">
              {asset.identifier}
            </h3>
            {asset.identifiers && asset.identifiers.length > 0 && (
              <TagCountBadge
                identifiers={asset.identifiers}
                onClick={handleToggleTags}
                expanded={tagsExpanded}
              />
            )}
          </div>
          <p className="text-sm text-gray-700 dark:text-gray-300 truncate">{asset.name}</p>
        </div>
      </div>

      {/* Expanded Tag List */}
      {asset.identifiers && (
        <TagIdentifierList
          identifiers={asset.identifiers}
          expanded={tagsExpanded}
          className="mb-3 pl-9"
        />
      )}

      {/* Location */}
      {locationName && (
        <div className="mb-3">
          <p className="text-sm text-gray-600 dark:text-gray-400 flex items-center gap-1">
            <MapPin className="h-3.5 w-3.5" />
            <span className="font-medium">Location:</span> {locationName}
          </p>
        </div>
      )}

      {/* Status */}
      <div className="mb-4">
        <span
          className={`
            inline-flex items-center px-3 py-1 rounded-full text-xs font-medium
            ${
              asset.is_active
                ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
            }
          `}
        >
          {asset.is_active ? 'Active ✓' : 'Inactive'}
        </span>
      </div>

      {/* Actions */}
      {showActions && (
        <div className="flex gap-2 pt-3 border-t border-gray-200 dark:border-gray-700">
          {hasIdentifier && (
            <button
              onClick={handleLocate}
              disabled={!canLocate}
              data-testid="locate-button"
              className={`flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium rounded-lg transition-colors border ${
                canLocate
                  ? 'text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border-blue-200 dark:border-blue-800'
                  : 'text-gray-400 bg-gray-100 dark:text-gray-500 dark:bg-gray-800 border-gray-200 dark:border-gray-700 cursor-not-allowed'
              }`}
            >
              <Target className="h-4 w-4" />
              Locate
            </button>
          )}
          <button
            onClick={handleEdit}
            className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-gray-700 bg-gray-50 hover:bg-gray-100 dark:text-gray-400 dark:bg-gray-900/20 dark:hover:bg-gray-900/40 border border-gray-200 dark:border-gray-800 rounded-lg transition-colors"
          >
            <Pencil className="h-4 w-4" />
            Edit
          </button>
          <button
            onClick={handleDelete}
            className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-red-700 bg-red-50 hover:bg-red-100 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 border border-red-200 dark:border-red-800 rounded-lg transition-colors"
          >
            <Trash2 className="h-4 w-4" />
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
