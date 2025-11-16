import React from 'react';
import { Pencil, Trash2, MapPin, Building2 } from 'lucide-react';
import type { Location } from '@/types/locations';

interface LocationCardProps {
  location: Location;
  onClick?: () => void;
  onEdit?: (location: Location) => void;
  onDelete?: (location: Location) => void;
  variant?: 'card' | 'row';
  showActions?: boolean;
  className?: string;
}

export function LocationCard({
  location,
  onClick,
  onEdit,
  onDelete,
  variant = 'card',
  showActions = true,
  className = '',
}: LocationCardProps) {
  const Icon = location.parent_location_id === null ? Building2 : MapPin;
  const isRoot = location.parent_location_id === null;

  const handleEdit = (e: React.MouseEvent) => {
    e.stopPropagation();
    onEdit?.(location);
  };

  const handleDelete = (e: React.MouseEvent) => {
    e.stopPropagation();
    onDelete?.(location);
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
        <td className="px-4 py-3">
          <div className="flex items-center gap-2">
            <Icon className="h-5 w-5 text-gray-500 dark:text-gray-400" />
            <span className="text-sm text-gray-700 dark:text-gray-300">
              {isRoot ? 'Root' : 'Subsidiary'}
            </span>
          </div>
        </td>

        <td className="px-4 py-3">
          <span className="text-sm font-medium text-gray-900 dark:text-white">
            {location.identifier}
          </span>
        </td>

        <td className="px-4 py-3">
          <span className="text-sm text-gray-700 dark:text-gray-300">{location.name}</span>
        </td>

        <td className="px-4 py-3">
          {location.description && (
            <span className="text-sm text-gray-600 dark:text-gray-400 truncate max-w-xs block">
              {location.description}
            </span>
          )}
        </td>

        <td className="px-4 py-3">
          <span
            className={`
              inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium
              ${
                location.is_active
                  ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                  : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
              }
            `}
          >
            {location.is_active ? 'Active' : 'Inactive'}
          </span>
        </td>

        <td className="px-4 py-3">
          {showActions && (
            <div className="flex items-center gap-2">
              <button
                onClick={handleEdit}
                className="p-1.5 text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20 rounded transition-colors"
                aria-label={`Edit ${location.identifier}`}
              >
                <Pencil className="h-4 w-4" />
              </button>
              <button
                onClick={handleDelete}
                className="p-1.5 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20 rounded transition-colors"
                aria-label={`Delete ${location.identifier}`}
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
      <div className="flex items-start gap-3 mb-3">
        <Icon className="h-6 w-6 text-gray-500 dark:text-gray-400 mt-1" />
        <div className="flex-1 min-w-0">
          <h3 className="text-base font-semibold text-gray-900 dark:text-white truncate">
            {location.identifier}
          </h3>
          <p className="text-sm text-gray-700 dark:text-gray-300 truncate">{location.name}</p>
        </div>
      </div>

      {location.description && (
        <div className="mb-3">
          <p className="text-sm text-gray-600 dark:text-gray-400 line-clamp-2">
            {location.description}
          </p>
        </div>
      )}

      <div className="mb-4">
        <span
          className={`
            inline-flex items-center px-3 py-1 rounded-full text-xs font-medium
            ${
              location.is_active
                ? 'bg-green-50 text-green-700 border border-green-200 dark:bg-green-900/20 dark:text-green-400 dark:border-green-800'
                : 'bg-gray-50 text-gray-700 border border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800'
            }
          `}
        >
          {location.is_active ? 'Active ✓' : 'Inactive'}
        </span>
      </div>

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
