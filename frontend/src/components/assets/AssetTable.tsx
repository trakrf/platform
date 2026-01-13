import { useMemo } from 'react';
import { Package } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { DataTable, Column } from '@/components/shared/DataTable';
import { AssetCard } from './AssetCard';
import type { Asset } from '@/types/assets';

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

const columns: Column<Asset>[] = [
  { key: 'identifier', label: 'Asset ID', sortable: true },
  { key: 'name', label: 'Name', sortable: true },
  { key: 'location', label: 'Location', sortable: false },
  { key: 'tags', label: 'Tags', sortable: false },
  { key: 'is_active', label: 'Status', sortable: true },
  { key: 'actions', label: 'Actions', sortable: false },
];

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

  const cachedAssets = useMemo(() => {
    return useAssetStore.getState().getFilteredAssets();
  }, [cache.byId.size, filters, sort]);

  const assets = propAssets ?? cachedAssets;

  const handleSort = (field: string, direction: 'asc' | 'desc') => {
    setSort(field as any, direction);
  };

  return (
    <DataTable
      data={assets}
      columns={columns}
      loading={loading}
      totalItems={totalAssets}
      currentPage={currentPage}
      pageSize={pageSize}
      sortField={sortField}
      sortDirection={sortDirection}
      onSort={handleSort}
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      renderRow={(asset, _index, rowProps) => (
        <AssetCard
          key={asset.id}
          asset={asset}
          variant="row"
          onClick={() => onAssetClick?.(asset)}
          onEdit={onEdit}
          onDelete={onDelete}
          className={rowProps.className}
        />
      )}
      emptyStateIcon={Package}
      emptyStateTitle="No Assets Found"
      emptyStateDescription="Try adjusting your filters or create a new asset to get started."
      className={className}
    />
  );
}
