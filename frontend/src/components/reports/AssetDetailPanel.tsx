import { useCallback, useMemo, useState } from 'react';
import { X, Download, ChevronDown, MapPin, Loader2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { ExportModal } from '@/components/export';
import { useAssetDetailPanel, fetchAllAssetHistory } from '@/hooks/reports';
import { useReportHydration } from '@/hooks/reports/useReportHydration';
import { useExport } from '@/hooks/useExport';
import { DATE_RANGE_OPTIONS, getDateRangeStart } from '@/lib/reports/utils';
import {
  generateAssetHistoryCSV,
  generateAssetHistoryExcel,
  generateAssetHistoryPDF,
} from '@/utils/export';
import { FreshnessBadge } from './FreshnessBadge';
import { MovementTimeline } from './MovementTimeline';
import type { ExportFormat, ExportResult } from '@/types/export';
import type { AssetHistoryItem, CurrentLocationItem } from '@/types/reports';

interface AssetDetailPanelProps {
  asset: CurrentLocationItem | null;
  onClose: () => void;
}

export function AssetDetailPanel({ asset, onClose }: AssetDetailPanelProps) {
  const {
    isVisible,
    handleClose,
    dateRange,
    setDateRange,
    timelineData,
    isLoading,
    error,
    hasMore,
    isLoadingMore,
    handleLoadMore,
    isNotFoundError,
  } = useAssetDetailPanel({ asset, onClose });

  const { isModalOpen, selectedFormat, openExport, closeExport } = useExport();
  // The export covers every stay in the range, so it is fetched on click
  // rather than taken from the timeline's loaded pages.
  const [exportRows, setExportRows] = useState<AssetHistoryItem[]>([]);
  const [isPreparingExport, setIsPreparingExport] = useState(false);

  const hydrationIds = useMemo(
    () => ({
      assetIds: asset?.asset_id != null ? [asset.asset_id] : [],
      locationIds: [
        asset?.location_id ?? null,
        ...timelineData.map((t) => t.location_id),
        ...exportRows.map((r) => r.location_id),
      ],
    }),
    [asset?.asset_id, asset?.location_id, timelineData, exportRows]
  );
  const { getAssetName, getLocationName } = useReportHydration(hydrationIds);
  const locationNameOf = useCallback(
    (item: AssetHistoryItem) =>
      getLocationName(item.location_id, item.location_external_key),
    [getLocationName]
  );

  const assetName = asset
    ? getAssetName(asset.asset_id, asset.asset_external_key, asset.asset_deleted_at)
    : '';
  const assetKey = asset?.asset_external_key ?? '';

  const generateExport = useCallback(
    (format: ExportFormat): ExportResult => {
      const opts = { assetName, assetKey, getLocationName: locationNameOf };
      switch (format) {
        case 'csv':
          return generateAssetHistoryCSV(exportRows, opts);
        case 'xlsx':
          return generateAssetHistoryExcel(exportRows, opts);
        case 'pdf':
          return generateAssetHistoryPDF(exportRows, opts);
        default:
          throw new Error(`Unsupported format: ${format}`);
      }
    },
    [exportRows, assetName, assetKey, locationNameOf]
  );

  const handleDownloadHistory = useCallback(async () => {
    if (asset?.asset_id == null) return;
    setIsPreparingExport(true);
    try {
      const rows = await fetchAllAssetHistory(asset.asset_id, {
        from: getDateRangeStart(dateRange).toISOString(),
      });
      setExportRows(rows);
      openExport('csv');
    } catch {
      toast.error('Failed to load movement history for download');
    } finally {
      setIsPreparingExport(false);
    }
  }, [asset?.asset_id, dateRange, openExport]);

  if (!asset) return null;

  const showAssetKeySubtext = assetKey && assetKey !== assetName;
  const currentLocationName = getLocationName(
    asset.location_id,
    asset.location_external_key
  );
  const currentLocationKey = asset.location_external_key ?? '';
  const showLocationKeySubtext =
    currentLocationKey &&
    currentLocationKey !== currentLocationName &&
    currentLocationName !== 'Unknown';

  const panelContent = (
    <>
      {/* Asset Info Grid */}
      <div className="grid grid-cols-2 gap-4 mb-6">
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Asset</p>
          <p className="font-medium text-gray-900 dark:text-white">{assetName || '—'}</p>
          {showAssetKeySubtext && (
            <p className="text-xs text-gray-500 dark:text-gray-400">{assetKey}</p>
          )}
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Type</p>
          <p className="font-medium text-gray-900 dark:text-white">Asset</p>
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Current Location</p>
          <p className="font-medium text-blue-600 dark:text-blue-400">
            {currentLocationName === 'Unknown' ? 'Unknown' : currentLocationName}
          </p>
          {showLocationKeySubtext && (
            <p className="text-xs text-gray-500 dark:text-gray-400">
              {currentLocationKey}
            </p>
          )}
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Status</p>
          <FreshnessBadge lastSeen={asset.asset_last_seen} />
        </div>
      </div>

      {/* Date Range */}
      <div className="mb-6">
        <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
          Date Range
        </p>
        <div className="flex flex-wrap gap-2">
          {DATE_RANGE_OPTIONS.map((option) => (
            <button
              key={option.value}
              onClick={() => setDateRange(option.value)}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
                dateRange === option.value
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700'
              }`}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      {/* Movement Timeline */}
      <div className="mb-6">
        <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
          Movement Timeline
        </p>
        {error ? (
          isNotFoundError ? (
            <div className="text-center py-8">
              <MapPin className="w-10 h-10 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
              <p className="text-sm text-gray-500 dark:text-gray-400">
                No movement history available for this asset
              </p>
            </div>
          ) : (
            <div className="text-center py-4">
              <p className="text-sm text-red-500 dark:text-red-400">
                Failed to load movement history
              </p>
              <p className="text-xs text-gray-500 mt-1">
                {error instanceof Error ? error.message : 'Unknown error'}
              </p>
            </div>
          )
        ) : (
          <MovementTimeline
            data={timelineData}
            isLoading={isLoading}
            hasMore={hasMore}
            isLoadingMore={isLoadingMore}
            onLoadMore={handleLoadMore}
            getLocationName={locationNameOf}
          />
        )}
      </div>

      {/* Download Button */}
      <button
        className="w-full flex items-center justify-center gap-2 bg-blue-600 hover:bg-blue-700
          text-white font-medium py-3 px-4 rounded-lg transition-colors
          disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-blue-600"
        onClick={handleDownloadHistory}
        disabled={
          isPreparingExport ||
          isLoading ||
          !!error ||
          timelineData.length === 0 ||
          asset.asset_id == null
        }
      >
        {isPreparingExport ? (
          <Loader2 className="w-4 h-4 animate-spin" />
        ) : (
          <Download className="w-4 h-4" />
        )}
        Download History CSV
      </button>
    </>
  );

  return (
    <>
      {/* Backdrop */}
      <div
        className={`fixed inset-0 bg-black/30 z-40 transition-opacity duration-200 ${
          isVisible ? 'opacity-100' : 'opacity-0 pointer-events-none'
        }`}
        onClick={handleClose}
      />

      {/* Desktop: Side Panel (hidden on mobile) */}
      <div
        className={`hidden md:block fixed right-0 top-0 h-full w-full max-w-md bg-white dark:bg-gray-900 shadow-xl z-50
          transform transition-transform duration-200 ease-out overflow-y-auto
          ${isVisible ? 'translate-x-0' : 'translate-x-full'}`}
      >
        {/* Header */}
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white truncate pr-2">
            {assetName}
          </h2>
          <button
            onClick={handleClose}
            className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors flex-shrink-0"
          >
            <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
          </button>
        </div>

        <div className="p-4">{panelContent}</div>
      </div>

      {/* Mobile: Bottom Sheet (hidden on desktop) */}
      <div
        className={`md:hidden fixed inset-x-0 bottom-0 z-50 transform transition-transform duration-300 ease-out
          ${isVisible ? 'translate-y-0' : 'translate-y-full'}`}
      >
        <div className="bg-white dark:bg-gray-900 rounded-t-2xl shadow-xl max-h-[85vh] flex flex-col">
          {/* Drag handle */}
          <div className="flex justify-center py-2">
            <div className="w-10 h-1 bg-gray-300 dark:bg-gray-600 rounded-full" />
          </div>

          {/* Header */}
          <div className="flex items-center justify-between px-4 pb-3 border-b border-gray-200 dark:border-gray-700">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white truncate pr-2">
              {assetName}
            </h2>
            <button
              onClick={handleClose}
              className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors flex-shrink-0"
            >
              <ChevronDown className="w-5 h-5 text-gray-500 dark:text-gray-400" />
            </button>
          </div>

          {/* Content */}
          <div className="p-4 overflow-y-auto flex-1 min-h-0">{panelContent}</div>
        </div>
      </div>

      {/* Outside panelContent: that renders twice, and both containers are
          transformed, which would pin a fixed modal to the panel. */}
      <ExportModal
        isOpen={isModalOpen}
        onClose={closeExport}
        selectedFormat={selectedFormat}
        itemCount={exportRows.length}
        itemLabel="movements"
        generateExport={generateExport}
        shareTitle={`${assetName} History`}
      />
    </>
  );
}
