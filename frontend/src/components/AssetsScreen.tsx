import React, { useState, useMemo, useCallback } from 'react';
import { Plus, Package, Upload } from 'lucide-react';
import toast from 'react-hot-toast';
import { useAssets, useAssetMutations } from '@/hooks/assets';
import { useAssetStore } from '@/stores';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { EmptyState, NoResults, ConfirmModal } from '@/components/shared';
import { GatedFab, PaidGate } from '@/components/entitlement';
import { AssetStats } from '@/components/assets/AssetStats';
import { AssetSearchSort } from '@/components/assets/AssetSearchSort';
import { AssetTable } from '@/components/assets/AssetTable';
import { AssetCard } from '@/components/assets/AssetCard';
import { AssetFormModal } from '@/components/assets/AssetFormModal';
import { BulkUploadModal } from '@/components/assets/BulkUploadModal';
import { AssetDetailsModal } from '@/components/assets/AssetDetailsModal';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { GlobalUploadAlert } from '@/components/shared/GlobalUploadAlert';
import { ShareButton } from '@/components/ShareButton';
import { ExportModal } from '@/components/export';
import { useExport } from '@/hooks/useExport';
import { useAssetLocations } from '@/hooks/reports';
import { generateAssetCSV, generateAssetExcel, generateAssetPDF } from '@/utils/export';
import type { Asset } from '@/types/assets';
import type { ExportFormat, ExportResult } from '@/types/export';

export default function AssetsScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isBulkUploadOpen, setIsBulkUploadOpen] = useState(false);
  const [editingAsset, setEditingAsset] = useState<Asset | null>(null);
  const [deletingAsset, setDeletingAsset] = useState<Asset | null>(null);
  const [viewingAsset, setViewingAsset] = useState<Asset | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const { isLoading } = useAssets();
  const { delete: deleteAsset } = useAssetMutations();
  const { isModalOpen: isExportModalOpen, selectedFormat, openExport, closeExport } = useExport();
  // TRA-799: current location is fact data sourced from the reports endpoint.
  const { byAssetId: assetLocations } = useAssetLocations();

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
        toast.success(`Asset "${deletingAsset.external_key}" deleted successfully`);
        setDeletingAsset(null);
      } catch (error: any) {
        console.error('Delete error:', error);
        toast.error(getApiErrorMessage(error, 'Failed to delete asset'));
      }
    }
  };

  const handleClearFilters = () => {
    setFilters({ is_active: 'all', search: '' });
  };

  const handleCreateClick = () => {
    setIsCreateModalOpen(true);
  };

  const handleBulkUploadSuccess = () => {
    setIsBulkUploadOpen(false);
  };

  const generateExport = useCallback(
    (format: ExportFormat): ExportResult => {
      switch (format) {
        case 'csv':
          return generateAssetCSV(filteredAssets, assetLocations);
        case 'xlsx':
          return generateAssetExcel(filteredAssets, assetLocations);
        case 'pdf':
          return generateAssetPDF(filteredAssets, assetLocations);
        default:
          throw new Error(`Unsupported format: ${format}`);
      }
    },
    [filteredAssets, assetLocations]
  );

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <GlobalUploadAlert />

        <div className="flex gap-4 flex-1 overflow-hidden">
          {/* <div className="hidden md:block w-72 flex-shrink-0">
            <AssetFilters isOpen={true} />
          </div> */}

          <div className="flex-1 flex flex-col gap-4 min-w-0">
            <div className="flex items-center gap-3">
              <div className="flex-1">
                <AssetSearchSort />
              </div>
              <PaidGate surface="assets-crud" silentImpression>
                <button
                  type="button"
                  onClick={() => setIsBulkUploadOpen(true)}
                  className="inline-flex items-center gap-1.5 px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 transition-colors"
                  aria-label="Import assets from CSV or XLSX"
                >
                  <Upload className="h-4 w-4" />
                  <span className="hidden sm:inline">Import</span>
                </button>
              </PaidGate>
              <ShareButton
                onFormatSelect={openExport}
                disabled={filteredAssets.length === 0}
              />
            </div>

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

        <GatedFab
          surface="assets-crud"
          icon={Plus}
          onClick={handleCreateClick}
          ariaLabel="Create new asset"
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
          message={`Are you sure you want to delete "${deletingAsset?.external_key}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingAsset(null)}
        />

        <AssetDetailsModal
          asset={viewingAsset}
          isOpen={!!viewingAsset}
          onClose={() => setViewingAsset(null)}
          onEdit={handleEditAsset}
        />

        <ExportModal
          isOpen={isExportModalOpen}
          onClose={closeExport}
          selectedFormat={selectedFormat}
          itemCount={filteredAssets.length}
          itemLabel="assets"
          generateExport={generateExport}
          shareTitle="Asset List"
        />

        {/* {isFiltersOpen && (
          <div className="fixed inset-0 z-40 md:hidden">
            <div
              className="absolute inset-0 bg-black bg-opacity-50"
              onClick={() => setIsFiltersOpen(false)}
            />
            <div className="absolute right-0 top-0 bottom-0 w-80 bg-white dark:bg-gray-900 p-4 overflow-y-auto">
              <AssetFilters isOpen={true} onToggle={() => setIsFiltersOpen(false)} />
            </div>
          </div>
        )} */}
      </div>
    </ProtectedRoute>
  );
}
