import React from 'react';
import { Pencil, Trash2, User, Laptop, Package, Archive, HelpCircle } from 'lucide-react';
import type { Asset } from '@/types/assets';

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
  const TypeIcon = TYPE_ICONS[asset.type];
  const location = asset.metadata?.location as string | undefined;

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
          {location && (
            <span className="text-sm text-gray-600 dark:text-gray-400">{location}</span>
          )}
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
              <button
                onClick={handleEdit}
                className="p-1.5 text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20 rounded transition-colors"
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

  // Card variant (mobile)
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
          <h3 className="text-base font-semibold text-gray-900 dark:text-white truncate">
            {asset.identifier}
          </h3>
          <p className="text-sm text-gray-700 dark:text-gray-300 truncate">{asset.name}</p>
        </div>
      </div>

      {/* Location */}
      {location && (
        <div className="mb-3">
          <p className="text-sm text-gray-600 dark:text-gray-400">
            <span className="font-medium">Location:</span> {location}
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
          <button
            onClick={handleEdit}
            className="flex-1 flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-blue-700 bg-blue-50 hover:bg-blue-100 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 border border-blue-200 dark:border-blue-800 rounded-lg transition-colors"
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
