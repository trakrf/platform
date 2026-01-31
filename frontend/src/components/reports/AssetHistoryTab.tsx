import { useCallback } from 'react';
import { FileText } from 'lucide-react';
import { EmptyState } from '@/components/shared';
import { ShareButton } from '@/components/ShareButton';
import { ExportModal } from '@/components/export';
import { useAssetHistoryTab } from '@/hooks/reports';
import { useExport } from '@/hooks/useExport';
import { AssetSelector } from './AssetSelector';
import { DateRangeInputs } from './DateRangeInputs';
import { AssetSummaryCard } from './AssetSummaryCard';
import { MovementTimeline } from './MovementTimeline';
import {
  generateAssetHistoryCSV,
  generateAssetHistoryExcel,
  generateAssetHistoryPDF,
} from '@/utils/export';
import type { ExportFormat, ExportResult } from '@/types/export';

export function AssetHistoryTab() {
  const {
    selectedAssetId,
    setSelectedAssetId,
    assetOptions,
    isLoadingAssets,
    fromDate,
    toDate,
    setFromDate,
    setToDate,
    timelineData,
    isLoadingTimeline,
    hasMore,
    isLoadingMore,
    handleLoadMore,
    stats,
    selectedAsset,
  } = useAssetHistoryTab();

  const { isModalOpen: isExportModalOpen, selectedFormat, openExport, closeExport } = useExport();

  const generateExport = useCallback(
    (format: ExportFormat): ExportResult => {
      const assetName = selectedAsset?.name || 'asset';
      const assetIdentifier = selectedAsset?.identifier || '';
      switch (format) {
        case 'csv':
          return generateAssetHistoryCSV(timelineData, assetName);
        case 'xlsx':
          return generateAssetHistoryExcel(timelineData, assetName, assetIdentifier);
        case 'pdf':
          return generateAssetHistoryPDF(timelineData, assetName, assetIdentifier);
        default:
          throw new Error(`Unsupported format: ${format}`);
      }
    },
    [timelineData, selectedAsset]
  );

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Controls Row */}
      <div className="flex flex-col gap-3 mb-4">
        {/* Asset selector row - full width on mobile */}
        <div className="flex gap-2">
          <AssetSelector
            value={selectedAssetId}
            onChange={setSelectedAssetId}
            assets={assetOptions}
            isLoading={isLoadingAssets}
            className="flex-1 md:flex-none md:w-72"
          />
          {/* Share Button - icon only on mobile */}
          <div className="md:hidden">
            <ShareButton
              onFormatSelect={openExport}
              disabled={!selectedAssetId || timelineData.length === 0}
              iconOnly
            />
          </div>
        </div>

        {/* Date range row */}
        <div className="flex flex-col md:flex-row gap-2 md:items-end">
          <DateRangeInputs
            fromDate={fromDate}
            toDate={toDate}
            onFromDateChange={setFromDate}
            onToDateChange={setToDate}
            className="flex-1 md:flex-none"
          />
          <div className="hidden md:flex md:flex-1" />
          {/* Share Button - full button on desktop */}
          <div className="hidden md:block">
            <ShareButton
              onFormatSelect={openExport}
              disabled={!selectedAssetId || timelineData.length === 0}
            />
          </div>
        </div>

        {/* Results count */}
        {selectedAssetId && !isLoadingTimeline && timelineData.length > 0 && (
          <div className="text-sm text-gray-500 dark:text-gray-400">
            Showing {timelineData.length} movement{timelineData.length === 1 ? '' : 's'}
            {hasMore && ' (scroll to load more)'}
          </div>
        )}
      </div>

      {/* Summary Card - shown when asset selected and has stats */}
      {selectedAsset && stats && (
        <AssetSummaryCard
          assetName={selectedAsset.name}
          assetIdentifier={selectedAsset.identifier}
          locationsVisited={stats.locationsVisited}
          timeTracked={stats.timeTracked}
          currentLocation={stats.currentLocation}
        />
      )}

      {/* Empty state when no asset selected */}
      {!selectedAssetId && (
        <EmptyState
          icon={FileText}
          title="Select an Asset"
          description="Choose an asset from the dropdown above to view its movement history."
          className="flex-1"
        />
      )}

      {/* Timeline card - shown when asset is selected */}
      {selectedAssetId && (
        <div className="flex-1 min-h-0 overflow-auto bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
          <h3 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-4">
            Movement Timeline
          </h3>
          <MovementTimeline
            data={timelineData}
            isLoading={isLoadingTimeline}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onLoadMore={handleLoadMore}
          />
        </div>
      )}

      {/* Export Modal */}
      <ExportModal
        isOpen={isExportModalOpen}
        onClose={closeExport}
        selectedFormat={selectedFormat}
        itemCount={timelineData.length}
        itemLabel="movements"
        generateExport={generateExport}
        shareTitle={selectedAsset ? `${selectedAsset.name} History` : 'Asset History'}
      />
    </div>
  );
}
