import React, { useState, useEffect } from 'react';
import { Plus, Package } from 'lucide-react';
import { useAssets, useAssetMutations } from '@/hooks/assets';
import { useAssetStore } from '@/stores';
import { FloatingActionButton, EmptyState, NoResults, ConfirmModal } from '@/components/shared';
import { AssetStats } from '@/components/assets/AssetStats';
import { AssetFilters } from '@/components/assets/AssetFilters';
import { AssetSearchSort } from '@/components/assets/AssetSearchSort';
import { AssetTable } from '@/components/assets/AssetTable';
import { AssetCard } from '@/components/assets/AssetCard';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
import type { Asset } from '@/types/assets';

export default function AssetsScreen() {
  const [isFiltersOpen, setIsFiltersOpen] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<Asset | null>(null);
  const [deletingAsset, setDeletingAsset] = useState<Asset | null>(null);

  const { data, isLoading } = useAssets();
  const { deleteAsset } = useAssetMutations();

  const filteredAssets = useAssetStore((state) => state.getFilteredAssets());
  const filters = useAssetStore((state) => state.filters);
  const setFilters = useAssetStore((state) => state.setFilters);

  // Load assets on mount
  useEffect(() => {
    // Assets are loaded by useAssets hook
  }, []);

  const hasActiveFilters =
    (filters.type && filters.type !== 'all') ||
    (filters.is_active !== 'all' && filters.is_active !== undefined) ||
    (filters.search && filters.search.trim() !== '');

  const handleViewAsset = (asset: Asset) => {
    // Future: Navigate to detail view
    console.log('View asset:', asset);
  };

  const handleEditAsset = (asset: Asset) => {
    setEditingAsset(asset);
  };

  const handleDeleteAsset = (asset: Asset) => {
    setDeletingAsset(asset);
  };

  const confirmDelete = async () => {
    if (deletingAsset) {
      await deleteAsset.mutateAsync(deletingAsset.id);
      setDeletingAsset(null);
    }
  };

  const handleClearFilters = () => {
    setFilters({ type: 'all', is_active: 'all', search: '' });
  };

  return (
    <div className="h-full flex flex-col p-4">
      {/* Stats Dashboard */}
      <AssetStats className="mb-6" />

      <div className="flex gap-4 flex-1 overflow-hidden">
        {/* Filters Sidebar (desktop only) */}
        <div className="hidden md:block w-72 flex-shrink-0">
          <AssetFilters isOpen={true} />
        </div>

        {/* Main Content */}
        <div className="flex-1 flex flex-col gap-4 min-w-0">
          {/* Search & Sort */}
          <AssetSearchSort />

          {/* Empty State (no assets at all) */}
          {!isLoading && filteredAssets.length === 0 && !hasActiveFilters && (
            <EmptyState
              icon={Package}
              title="No assets yet"
              description="Get started by adding your first asset"
              action={{
                label: 'Create Asset',
                onClick: () => setIsCreateModalOpen(true),
              }}
            />
          )}

          {/* No Results (with filters active) */}
          {!isLoading && filteredAssets.length === 0 && hasActiveFilters && (
            <NoResults searchTerm={filters.search || ''} onClearFilters={handleClearFilters} />
          )}

          {/* Data Display */}
          {!isLoading && filteredAssets.length > 0 && (
            <>
              {/* Desktop Table */}
              <AssetTable
                loading={isLoading}
                onAssetClick={handleViewAsset}
                onEdit={handleEditAsset}
                onDelete={handleDeleteAsset}
              />

              {/* Mobile Cards */}
              <div className="md:hidden space-y-3">
                {filteredAssets.map((asset) => (
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

      {/* Floating Action Button */}
      <FloatingActionButton
        icon={Plus}
        onClick={() => setIsCreateModalOpen(true)}
        ariaLabel="Create new asset"
      />

      {/* Create Modal */}
      <AssetFormModal
        isOpen={isCreateModalOpen}
        mode="create"
        onClose={() => setIsCreateModalOpen(false)}
      />

      {/* Edit Modal */}
      {editingAsset && (
        <AssetFormModal
          isOpen={true}
          mode="edit"
          asset={editingAsset}
          onClose={() => setEditingAsset(null)}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        isOpen={!!deletingAsset}
        title="Delete Asset"
        message={`Are you sure you want to delete "${deletingAsset?.identifier}"? This action cannot be undone.`}
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={confirmDelete}
        onCancel={() => setDeletingAsset(null)}
      />

      {/* Mobile Filters Drawer (future enhancement) */}
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
  );
}
