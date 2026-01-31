import { useState, useEffect, useMemo, useCallback } from 'react';
import { Search, FileText, Package, CheckCircle, AlertTriangle } from 'lucide-react';
import toast from 'react-hot-toast';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import { EmptyState } from '@/components/shared';
import { CurrentLocationsTable } from '@/components/reports/CurrentLocationsTable';
import { CurrentLocationCard } from '@/components/reports/CurrentLocationCard';
import { ReportStatCard } from '@/components/reports/ReportStatCard';
import { AssetDetailPanel } from '@/components/reports/AssetDetailPanel';
import { AssetHistoryTab } from '@/components/reports/AssetHistoryTab';
import { LocationFilter } from '@/components/reports/LocationFilter';
import { TimeRangeFilter } from '@/components/reports/TimeRangeFilter';
import { ShareButton } from '@/components/ShareButton';
import { ExportModal } from '@/components/export';
import { useCurrentLocations, useReportsFilters } from '@/hooks/reports';
import { useExport } from '@/hooks/useExport';
import { getFreshnessStatus } from '@/lib/reports/utils';
import {
  generateCurrentLocationsCSV,
  generateCurrentLocationsExcel,
  generateCurrentLocationsPDF,
} from '@/utils/export';
import type { CurrentLocationItem } from '@/types/reports';
import type { ExportFormat, ExportResult } from '@/types/export';

type TabId = 'current' | 'movement';

export default function ReportsScreen() {
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [activeTab, setActiveTab] = useState<TabId>('current');
  const [selectedAsset, setSelectedAsset] = useState<CurrentLocationItem | null>(null);

  const {
    selectedLocationId,
    setSelectedLocationId,
    selectedTimeRange,
    setSelectedTimeRange,
    search,
    setSearch,
    locations,
    isLoadingLocations,
    filteredData,
    totalCount,
    isLoading,
    error,
    hasActiveFilters,
    clearFilters,
    activeFilterDescription,
  } = useReportsFilters({ pageSize, currentPage });

  const { isModalOpen: isExportModalOpen, selectedFormat, openExport, closeExport } = useExport();

  // Reset to page 1 when filters change
  useEffect(() => {
    setCurrentPage(1);
  }, [selectedLocationId, selectedTimeRange, search]);

  // Fetch all data for stats (no pagination)
  const { data: allData } = useCurrentLocations({
    limit: 1000,
    offset: 0,
  });

  // Calculate stats from all data
  const stats = useMemo(() => {
    if (!allData || allData.length === 0) {
      return { total: 0, seenToday: 0, stale: 0 };
    }

    let seenToday = 0;
    let stale = 0;

    allData.forEach((item) => {
      const status = getFreshnessStatus(item.last_seen);
      if (status === 'live' || status === 'today') {
        seenToday++;
      }
      if (status === 'stale') {
        stale++;
      }
    });

    return {
      total: allData.length,
      seenToday,
      stale,
    };
  }, [allData]);

  // Generate export based on filtered data
  const generateExport = useCallback(
    (format: ExportFormat): ExportResult => {
      switch (format) {
        case 'csv':
          return generateCurrentLocationsCSV(filteredData);
        case 'xlsx':
          return generateCurrentLocationsExcel(filteredData);
        case 'pdf':
          return generateCurrentLocationsPDF(filteredData);
        default:
          throw new Error(`Unsupported format: ${format}`);
      }
    },
    [filteredData]
  );

  // Show error toast
  useEffect(() => {
    if (error) {
      toast.error('Failed to load location data');
    }
  }, [error]);

  const handleRowClick = (item: CurrentLocationItem) => {
    setSelectedAsset(item);
  };

  const handleClosePanel = () => {
    setSelectedAsset(null);
  };

  const tabs: { id: TabId; label: string }[] = [
    { id: 'current', label: 'Locations History' },
    { id: 'movement', label: 'Asset History' },
  ];

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2 md:p-4">
        {/* Header */}
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Reports
        </h1>

        {/* Stat Cards */}
        <div className="grid grid-cols-3 gap-2 md:gap-4 mb-4 md:mb-6">
          <ReportStatCard
            title="Total Assets Tracked"
            value={stats.total}
            subtitle={stats.total > 0 ? `${stats.total} assets with location data` : undefined}
            icon={Package}
            iconColor="text-blue-500"
            iconBgColor="bg-blue-500/10"
          />
          <ReportStatCard
            title="Assets Seen Today"
            value={stats.seenToday}
            subtitle={
              stats.total > 0
                ? `${Math.round((stats.seenToday / stats.total) * 100)}% of total`
                : undefined
            }
            icon={CheckCircle}
            iconColor="text-green-500"
            iconBgColor="bg-green-500/10"
          />
          <ReportStatCard
            title="Stale Assets (>7 days)"
            value={stats.stale}
            subtitle={
              stats.total > 0
                ? `${Math.round((stats.stale / stats.total) * 100)}% of total`
                : undefined
            }
            icon={AlertTriangle}
            iconColor="text-amber-500"
            iconBgColor="bg-amber-500/10"
          />
        </div>

        {/* Tabs */}
        <div className="flex gap-1 border-b border-gray-200 dark:border-gray-700 mb-4">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab === tab.id
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Content based on active tab */}
        {activeTab === 'current' && (
          <>
            {/* Search and Filters - all on one line, wraps on mobile */}
            <div className="flex flex-wrap items-end gap-2 md:gap-3 mb-4">
              {/* Search input */}
              <div className="relative min-w-[200px] flex-1 md:flex-none md:w-64">
                <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
                <input
                  type="text"
                  placeholder="Search by asset name..."
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  className="w-full h-[42px] pl-10 pr-4 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <LocationFilter
                value={selectedLocationId}
                onChange={setSelectedLocationId}
                locations={locations}
                isLoading={isLoadingLocations}
              />
              <TimeRangeFilter
                value={selectedTimeRange}
                onChange={setSelectedTimeRange}
              />

              {/* Share Button - icon only on mobile, full on desktop */}
              <div className="md:hidden">
                <ShareButton
                  onFormatSelect={openExport}
                  disabled={filteredData.length === 0}
                  iconOnly
                />
              </div>
              <div className="hidden md:block">
                <ShareButton
                  onFormatSelect={openExport}
                  disabled={filteredData.length === 0}
                />
              </div>
            </div>

            {/* Results count */}
            {!isLoading && filteredData.length > 0 && (
              <div className="text-sm text-gray-500 dark:text-gray-400 mb-4">
                Showing {filteredData.length} {filteredData.length === 1 ? 'result' : 'results'}
                {hasActiveFilters && ' (filtered)'}
              </div>
            )}

            {/* Empty state: no data at all */}
            {!isLoading && filteredData.length === 0 && !hasActiveFilters && (
              <EmptyState
                icon={FileText}
                title="No Location Data"
                description="No assets have been scanned yet. Assets will appear here once they are detected by RFID readers."
                className="flex-1"
              />
            )}

            {/* Empty state: filters applied but no results */}
            {!isLoading && filteredData.length === 0 && hasActiveFilters && (
              <EmptyState
                icon={Search}
                title="No Results"
                description={`No assets found ${activeFilterDescription}.`}
                action={{
                  label: 'Clear filters',
                  onClick: clearFilters,
                }}
                className="flex-1"
              />
            )}

            {/* Locations History content - only show when we have data or loading */}
            {(isLoading || filteredData.length > 0) && (
              <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700">
                {/* Desktop: Table */}
                <CurrentLocationsTable
                  data={filteredData}
                  loading={isLoading}
                  totalItems={totalCount}
                  currentPage={currentPage}
                  pageSize={pageSize}
                  onPageChange={setCurrentPage}
                  onPageSizeChange={setPageSize}
                  onRowClick={handleRowClick}
                />

                {/* Mobile: Cards */}
                <div className="md:hidden flex-1 overflow-auto p-3 space-y-3">
                  {isLoading ? (
                    Array.from({ length: 3 }).map((_, i) => (
                      <div
                        key={i}
                        className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 animate-pulse"
                      >
                        <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-1/2 mb-2" />
                        <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-1/3 mb-3" />
                        <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-2/3" />
                      </div>
                    ))
                  ) : (
                    filteredData.map((item) => (
                      <CurrentLocationCard
                        key={item.asset_id}
                        item={item}
                        onClick={() => handleRowClick(item)}
                      />
                    ))
                  )}
                </div>
              </div>
            )}
          </>
        )}

        {activeTab === 'movement' && <AssetHistoryTab />}
      </div>

      {/* Asset Detail Side Panel */}
      <AssetDetailPanel asset={selectedAsset} onClose={handleClosePanel} />

      {/* Export Modal */}
      <ExportModal
        isOpen={isExportModalOpen}
        onClose={closeExport}
        selectedFormat={selectedFormat}
        itemCount={filteredData.length}
        itemLabel="assets"
        generateExport={generateExport}
        shareTitle="Locations History"
      />
    </ProtectedRoute>
  );
}
