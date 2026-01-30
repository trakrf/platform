import { FileText } from 'lucide-react';
import { EmptyState } from '@/components/shared';
import { useAssetHistoryTab } from '@/hooks/reports';
import { AssetSelector } from './AssetSelector';
import { DateRangeInputs } from './DateRangeInputs';
import { ExportCsvButton } from './ExportCsvButton';
import { AssetSummaryCard } from './AssetSummaryCard';
import { MovementTimeline } from './MovementTimeline';

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

  return (
    <div className="flex-1 flex flex-col min-h-0">
      {/* Controls Row */}
      <div className="flex flex-wrap items-end gap-4 mb-4">
        <AssetSelector
          value={selectedAssetId}
          onChange={setSelectedAssetId}
          assets={assetOptions}
          isLoading={isLoadingAssets}
          className="w-full md:w-72"
        />
        <DateRangeInputs
          fromDate={fromDate}
          toDate={toDate}
          onFromDateChange={setFromDate}
          onToDateChange={setToDate}
        />
        <div className="flex-1" />
        <ExportCsvButton
          data={timelineData}
          assetName={selectedAsset?.name || 'asset'}
          disabled={!selectedAssetId}
        />
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
    </div>
  );
}
