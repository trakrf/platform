import { useState, useEffect, useMemo } from 'react';
import { ArrowLeft, Package, AlertCircle } from 'lucide-react';
import toast from 'react-hot-toast';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { EmptyState } from '@/components/shared';
import { AssetHistoryTable } from '@/components/reports/AssetHistoryTable';
import { AssetHistoryCard } from '@/components/reports/AssetHistoryCard';
import { useAssetHistory } from '@/hooks/reports';

export default function ReportsHistoryScreen() {
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // Parse asset ID from URL hash
  const assetId = useMemo(() => {
    const hash = window.location.hash;
    const queryIndex = hash.indexOf('?');
    if (queryIndex === -1) return null;

    const params = new URLSearchParams(hash.slice(queryIndex + 1));
    const id = params.get('id');
    return id ? parseInt(id, 10) : null;
  }, []);

  const { asset, data, totalCount, isLoading, error } = useAssetHistory(assetId, {
    limit: pageSize,
    offset: (currentPage - 1) * pageSize,
  });

  // Show error toast
  useEffect(() => {
    if (error) {
      toast.error('Failed to load asset history');
    }
  }, [error]);

  const handleBack = () => {
    window.location.hash = 'reports';
  };

  // Invalid asset ID
  if (!assetId) {
    return (
      <ProtectedRoute>
        <div className="h-full flex flex-col p-2">
          <button
            onClick={handleBack}
            className="flex items-center gap-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 mb-4"
          >
            <ArrowLeft className="w-4 h-4" />
            Back to Current Locations
          </button>

          <EmptyState
            icon={AlertCircle}
            title="Invalid Asset"
            description="No asset ID was provided. Please select an asset from the current locations report."
            action={{
              label: 'View Current Locations',
              onClick: handleBack,
            }}
          />
        </div>
      </ProtectedRoute>
    );
  }

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        {/* Back button */}
        <button
          onClick={handleBack}
          className="flex items-center gap-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 mb-4 w-fit"
        >
          <ArrowLeft className="w-4 h-4" />
          Back to Current Locations
        </button>

        {/* Asset header */}
        {isLoading ? (
          <div className="mb-6 animate-pulse">
            <div className="h-6 bg-gray-200 dark:bg-gray-700 rounded w-48 mb-2"></div>
            <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-32"></div>
          </div>
        ) : asset ? (
          <div className="mb-6">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-lg bg-blue-100 dark:bg-blue-900 flex items-center justify-center">
                <Package className="w-5 h-5 text-blue-600 dark:text-blue-400" />
              </div>
              <div>
                <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">
                  {asset.name}
                </h1>
                <p className="text-sm text-gray-500 dark:text-gray-400">
                  {asset.identifier}
                </p>
              </div>
            </div>
          </div>
        ) : error ? (
          <EmptyState
            icon={AlertCircle}
            title="Asset Not Found"
            description="This asset could not be found or you don't have permission to view it."
            action={{
              label: 'View Current Locations',
              onClick: handleBack,
            }}
          />
        ) : null}

        {/* History content */}
        {asset && (
          <>
            <h2 className="text-lg font-medium text-gray-900 dark:text-gray-100 mb-4">
              Movement History
            </h2>

            {/* Desktop: Table */}
            <AssetHistoryTable
              data={data}
              loading={isLoading}
              totalItems={totalCount}
              currentPage={currentPage}
              pageSize={pageSize}
              onPageChange={setCurrentPage}
              onPageSizeChange={setPageSize}
            />

            {/* Mobile: Cards */}
            <div className="md:hidden space-y-3">
              {isLoading ? (
                // Loading skeleton for mobile
                Array.from({ length: 3 }).map((_, i) => (
                  <div
                    key={i}
                    className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 animate-pulse"
                  >
                    <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-1/2 mb-2"></div>
                    <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-2/3"></div>
                  </div>
                ))
              ) : (
                data.map((item, index) => (
                  <AssetHistoryCard
                    key={`${item.timestamp}-${index}`}
                    item={item}
                    isFirst={index === 0}
                  />
                ))
              )}
            </div>
          </>
        )}
      </div>
    </ProtectedRoute>
  );
}
