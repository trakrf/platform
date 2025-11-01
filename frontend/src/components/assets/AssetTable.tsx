import { useMemo } from 'react';
import { ArrowUp, ArrowDown, ArrowUpDown } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { EmptyState, SkeletonTableRow } from '@/components/shared';
import { PaginationControls } from '@/components/shared/pagination';
import { AssetCard } from './AssetCard';
import type { Asset } from '@/types/assets';
import { Package } from 'lucide-react';

interface AssetTableProps {
  loading?: boolean;
  assets?: Asset[];
  totalAssets?: number;
  currentPage?: number;
  pageSize?: number;
  onPageChange?: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  onAssetClick?: (asset: Asset) => void;
  onEdit?: (asset: Asset) => void;
  onDelete?: (asset: Asset) => void;
  className?: string;
}

type SortableField = 'identifier' | 'name' | 'type' | 'is_active';

export function AssetTable({
  loading = false,
  assets: propAssets,
  totalAssets = 0,
  currentPage = 1,
  pageSize = 10,
  onPageChange,
  onPageSizeChange,
  onAssetClick,
  onEdit,
  onDelete,
  className = '',
}: AssetTableProps) {
  const cache = useAssetStore((state) => state.cache);
  const filters = useAssetStore((state) => state.filters);
  const sort = useAssetStore((state) => state.sort);
  const { field: sortField, direction: sortDirection } = sort;
  const setSort = useAssetStore((state) => state.setSort);

  const assets = propAssets ?? useMemo(() => {
    return useAssetStore.getState().getFilteredAssets();
  }, [cache.byId.size, filters, sort]);

  const handleSort = (field: SortableField) => {
    if (sortField === field) {
      const newDirection = sortDirection === 'asc' ? 'desc' : 'asc';
      setSort(field, newDirection);
    } else {
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
              <SkeletonTableRow key={i} />
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

  const totalPages = Math.max(1, Math.ceil(totalAssets / pageSize));
  const startIndex = (currentPage - 1) * pageSize + 1;
  const endIndex = Math.min(currentPage * pageSize, totalAssets);

  const handleNext = () => {
    if (currentPage < totalPages) {
      onPageChange?.(currentPage + 1);
    }
  };

  const handlePrevious = () => {
    if (currentPage > 1) {
      onPageChange?.(currentPage - 1);
    }
  };

  const handleFirstPage = () => {
    onPageChange?.(1);
  };

  const handleLastPage = () => {
    onPageChange?.(totalPages);
  };

  return (
    <div className={`hidden md:flex md:flex-col ${className}`}>
      <div className="overflow-x-auto flex-1">
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

      {/* Pagination Controls */}
      {totalAssets > 0 && onPageChange && onPageSizeChange && (
        <div className="sticky bottom-0 px-4 py-3 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
          <PaginationControls
            currentPage={currentPage}
            totalPages={totalPages}
            startIndex={startIndex}
            endIndex={endIndex}
            totalItems={totalAssets}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onNext={handleNext}
            onPrevious={handlePrevious}
            onFirstPage={handleFirstPage}
            onLastPage={handleLastPage}
            onPageSizeChange={onPageSizeChange}
          />
        </div>
      )}
    </div>
  );
}
