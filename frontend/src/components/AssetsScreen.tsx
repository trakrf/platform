import React, { useState, useMemo } from 'react';
import { Plus, Package } from 'lucide-react';
import toast from 'react-hot-toast';
import { useAssets, useAssetMutations } from '@/hooks/assets';
import { useAssetStore } from '@/stores';
import { FloatingActionButton, EmptyState, NoResults, ConfirmModal } from '@/components/shared';
import { AssetStats } from '@/components/assets/AssetStats';
import { AssetFilters } from '@/components/assets/AssetFilters';
import { AssetSearchSort } from '@/components/assets/AssetSearchSort';
import { AssetTable } from '@/components/assets/AssetTable';
import { AssetCard } from '@/components/assets/AssetCard';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
import { AssetCreateChoice } from '@/components/assets/AssetCreateChoice';
import { BulkUploadModal } from '@/components/assets/BulkUploadModal';
import { AssetDetailsModal } from '@/components/assets/AssetDetailsModal';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { GlobalUploadAlert } from '@/components/shared/GlobalUploadAlert';
import type { Asset } from '@/types/assets';

export default function AssetsScreen() {
  const [isFiltersOpen, setIsFiltersOpen] = useState(false);
  const [isChoiceModalOpen, setIsChoiceModalOpen] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isBulkUploadOpen, setIsBulkUploadOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<Asset | null>(null);
  const [deletingAsset, setDeletingAsset] = useState<Asset | null>(null);
  const [viewingAsset, setViewingAsset] = useState<Asset | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const { isLoading } = useAssets();
  const { delete: deleteAsset } = useAssetMutations();

  const cache = useAssetStore((state) => state.cache);
  const filters = useAssetStore((state) => state.filters);
  const sort = useAssetStore((state) => state.sort);
  const setFilters = useAssetStore((state) => state.setFilters);

  const filteredAssets = useMemo(() => {
    return useAssetStore.getState().getFilteredAssets();
  }, [cache.byId.size, cache.lastFetched, filters, sort]);

  const paginatedAssets = useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    return filteredAssets.slice(startIndex, endIndex);
  }, [filteredAssets, currentPage, pageSize]);

  React.useEffect(() => {
    setCurrentPage(1);
  }, [filters, sort]);

  const hasActiveFilters =
    (filters.type && filters.type !== 'all') ||
    (filters.is_active !== 'all' && filters.is_active !== undefined) ||
    (filters.search && filters.search.trim() !== '');

  const handleViewAsset = (asset: Asset) => {
    setViewingAsset(asset);
  };

  const handleEditAsset = (asset: Asset) => {
    setEditingAsset(asset);
  };

  const handleDeleteAsset = (asset: Asset) => {
    setDeletingAsset(asset);
  };

  const confirmDelete = async () => {
    if (deletingAsset) {
      try {
        await deleteAsset(deletingAsset.id);
        toast.success(`Asset "${deletingAsset.identifier}" deleted successfully`);
        setDeletingAsset(null);
      } catch (error: any) {
        console.error('Delete error:', error);
        toast.error(error.message || 'Failed to delete asset');
      }
    }
  };

  const handleClearFilters = () => {
    setFilters({ type: 'all', is_active: 'all', search: '' });
  };

  const handleCreateClick = () => {
    setIsChoiceModalOpen(true);
  };

  const handleSingleCreate = () => {
    setIsChoiceModalOpen(false);
    setIsCreateModalOpen(true);
  };

  const handleBulkUpload = () => {
    setIsChoiceModalOpen(false);
    setIsBulkUploadOpen(true);
  };

  const handleBulkUploadSuccess = () => {
    setIsBulkUploadOpen(false);
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <GlobalUploadAlert />

        <div className="flex gap-4 flex-1 overflow-hidden">
          <div className="hidden md:block w-72 flex-shrink-0">
            <AssetFilters isOpen={true} />
          </div>

          <div className="flex-1 flex flex-col gap-4 min-w-0">
            <AssetSearchSort />

            {!isLoading && filteredAssets.length === 0 && !hasActiveFilters && (
              <EmptyState
                icon={Package}
                title="No assets yet"
                description="Get started by adding your first asset"
                action={{
                  label: 'Create Asset',
                  onClick: handleCreateClick,
                }}
              />
            )}

            {!isLoading && filteredAssets.length === 0 && hasActiveFilters && (
              <NoResults searchTerm={filters.search || ''} onClearFilters={handleClearFilters} />
            )}

            {!isLoading && filteredAssets.length > 0 && (
              <>
                <AssetTable
                  loading={isLoading}
                  assets={paginatedAssets}
                  totalAssets={filteredAssets.length}
                  currentPage={currentPage}
                  pageSize={pageSize}
                  onPageChange={setCurrentPage}
                  onPageSizeChange={setPageSize}
                  onAssetClick={handleViewAsset}
                  onEdit={handleEditAsset}
                  onDelete={handleDeleteAsset}
                />

                <div className="md:hidden space-y-3">
                  {paginatedAssets.map((asset) => (
                    <AssetCard
                      key={asset.id}
                      asset={asset}
                      variant="card"
                      onClick={() => handleViewAsset(asset)}
                      onEdit={handleEditAsset}
                      onDelete={handleDeleteAsset}
                      showActions={true}
                    />
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
        <AssetStats className="mt-6" />

        <FloatingActionButton
          icon={Plus}
          onClick={handleCreateClick}
          ariaLabel="Create new asset"
        />

        <AssetCreateChoice
          isOpen={isChoiceModalOpen}
          onClose={() => setIsChoiceModalOpen(false)}
          onSingleCreate={handleSingleCreate}
          onBulkUpload={handleBulkUpload}
        />

        <BulkUploadModal
          isOpen={isBulkUploadOpen}
          onClose={() => setIsBulkUploadOpen(false)}
          onSuccess={handleBulkUploadSuccess}
        />

        <AssetFormModal
          isOpen={isCreateModalOpen}
          mode="create"
          onClose={() => setIsCreateModalOpen(false)}
        />

        {editingAsset && (
          <AssetFormModal
            isOpen={true}
            mode="edit"
            asset={editingAsset}
            onClose={() => setEditingAsset(null)}
          />
        )}

        <ConfirmModal
          isOpen={!!deletingAsset}
          title="Delete Asset"
          message={`Are you sure you want to delete "${deletingAsset?.identifier}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingAsset(null)}
        />

        <AssetDetailsModal
          asset={viewingAsset}
          isOpen={!!viewingAsset}
          onClose={() => setViewingAsset(null)}
        />

        {isFiltersOpen && (
          <div className="fixed inset-0 z-40 md:hidden">
            <div
              className="absolute inset-0 bg-black bg-opacity-50"
              onClick={() => setIsFiltersOpen(false)}
            />
            <div className="absolute right-0 top-0 bottom-0 w-80 bg-white dark:bg-gray-900 p-4 overflow-y-auto">
              <AssetFilters isOpen={true} onToggle={() => setIsFiltersOpen(false)} />
            </div>
          </div>
        )}
      </div>
    </ProtectedRoute>
  );
}
