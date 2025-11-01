import React from 'react';
import { ArrowUp, ArrowDown, ArrowUpDown } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { EmptyState, SkeletonTableRow } from '@/components/shared';
import { AssetCard } from './AssetCard';
import type { Asset, SortState } from '@/types/assets';
import { Package } from 'lucide-react';

interface AssetTableProps {
  loading?: boolean;
  onAssetClick?: (asset: Asset) => void;
  onEdit?: (asset: Asset) => void;
  onDelete?: (asset: Asset) => void;
  className?: string;
}

type SortableField = 'identifier' | 'name' | 'type' | 'is_active';

export function AssetTable({
  loading = false,
  onAssetClick,
  onEdit,
  onDelete,
  className = '',
}: AssetTableProps) {
  const assets = useAssetStore((state) => state.getFilteredAssets());
  const { field: sortField, direction: sortDirection } = useAssetStore((state) => state.sort);
  const setSort = useAssetStore((state) => state.setSort);

  const handleSort = (field: SortableField) => {
    if (sortField === field) {
      // Toggle direction if same field
      const newDirection = sortDirection === 'asc' ? 'desc' : 'asc';
      setSort(field, newDirection);
    } else {
      // Default to asc for new field
      setSort(field, 'asc');
    }
  };

  const getSortIcon = (field: SortableField) => {
    if (sortField !== field) {
      return <ArrowUpDown className="h-4 w-4 text-gray-400" />;
    }
    return sortDirection === 'asc' ? (
      <ArrowUp className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    ) : (
      <ArrowDown className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    );
  };

  const SortableHeader = ({
    field,
    label,
  }: {
    field: SortableField;
    label: string;
  }) => (
    <th className="px-4 py-3 text-left">
      <button
        onClick={() => handleSort(field)}
        className="flex items-center gap-2 text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
      >
        {label}
        {getSortIcon(field)}
      </button>
    </th>
  );

  if (loading) {
    return (
      <div className={`hidden md:block overflow-x-auto ${className}`}>
        <table className="w-full">
          <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Type
              </th>
              <SortableHeader field="identifier" label="Identifier" />
              <SortableHeader field="name" label="Name" />
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Location
              </th>
              <SortableHeader field="is_active" label="Status" />
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <SkeletonTableRow key={i} columns={6} />
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  if (assets.length === 0) {
    return (
      <div className={`hidden md:block ${className}`}>
        <EmptyState
          icon={Package}
          title="No Assets Found"
          description="Try adjusting your filters or create a new asset to get started."
        />
      </div>
    );
  }

  return (
    <div className={`hidden md:block overflow-x-auto ${className}`}>
      <table className="w-full">
        <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
          <tr>
            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
              Type
            </th>
            <SortableHeader field="identifier" label="Identifier" />
            <SortableHeader field="name" label="Name" />
            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
              Location
            </th>
            <SortableHeader field="is_active" label="Status" />
            <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
              Actions
            </th>
          </tr>
        </thead>
        <tbody>
          {assets.map((asset, index) => (
            <AssetCard
              key={asset.id}
              asset={asset}
              variant="row"
              onClick={() => onAssetClick?.(asset)}
              onEdit={onEdit}
              onDelete={onDelete}
              className={index % 2 === 0 ? 'bg-white dark:bg-gray-900' : 'bg-gray-50 dark:bg-gray-800/50'}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}
