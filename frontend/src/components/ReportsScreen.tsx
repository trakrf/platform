import { useState, useEffect } from 'react';
import { Search, FileText } from 'lucide-react';
import toast from 'react-hot-toast';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { EmptyState } from '@/components/shared';
import { CurrentLocationsTable } from '@/components/reports/CurrentLocationsTable';
import { CurrentLocationCard } from '@/components/reports/CurrentLocationCard';
import { useCurrentLocations } from '@/hooks/reports';
import { useDebounce } from '@/hooks/useDebounce';
import type { CurrentLocationItem } from '@/types/reports';

export default function ReportsScreen() {
  const [search, setSearch] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const debouncedSearch = useDebounce(search, 300);

  // Reset to page 1 when search changes
  useEffect(() => {
    setCurrentPage(1);
  }, [debouncedSearch]);

  const { data, totalCount, isLoading, error } = useCurrentLocations({
    search: debouncedSearch || undefined,
    limit: pageSize,
    offset: (currentPage - 1) * pageSize,
  });

  // Show error toast
  useEffect(() => {
    if (error) {
      toast.error('Failed to load location data');
    }
  }, [error]);

  const handleRowClick = (item: CurrentLocationItem) => {
    window.location.hash = `reports-history?id=${item.asset_id}`;
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        {/* Header */}
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">
            Current Asset Locations
          </h1>
        </div>

        {/* Search */}
        <div className="mb-4">
          <div className="relative max-w-md">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              placeholder="Search assets..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-10 pr-4 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            />
          </div>
        </div>

        {/* Content */}
        {!isLoading && data.length === 0 && !search && (
          <EmptyState
            icon={FileText}
            title="No Location Data"
            description="No assets have been scanned yet. Assets will appear here once they are detected by RFID readers."
          />
        )}

        {!isLoading && data.length === 0 && search && (
          <EmptyState
            icon={Search}
            title="No Results"
            description={`No assets matching "${search}" were found.`}
          />
        )}

        {(isLoading || data.length > 0) && (
          <>
            {/* Desktop: Table */}
            <CurrentLocationsTable
              data={data}
              loading={isLoading}
              totalItems={totalCount}
              currentPage={currentPage}
              pageSize={pageSize}
              onPageChange={setCurrentPage}
              onPageSizeChange={setPageSize}
              onRowClick={handleRowClick}
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
                    <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-1/3 mb-3"></div>
                    <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-2/3"></div>
                  </div>
                ))
              ) : (
                data.map((item) => (
                  <CurrentLocationCard
                    key={item.asset_id}
                    item={item}
                    onClick={() => handleRowClick(item)}
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
