import { X, Download, ChevronDown, MapPin } from 'lucide-react';
import { useAssetDetailPanel } from '@/hooks/reports';
import { DATE_RANGE_OPTIONS } from '@/lib/reports/utils';
import { FreshnessBadge } from './FreshnessBadge';
import { MovementTimeline } from './MovementTimeline';
import type { CurrentLocationItem } from '@/types/reports';

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

  if (!asset) return null;

  const panelContent = (
    <>
      {/* Asset Info Grid */}
      <div className="grid grid-cols-2 gap-4 mb-6">
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Asset ID</p>
          <p className="font-medium text-gray-900 dark:text-white">
            {asset.asset_identifier || '—'}
          </p>
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Type</p>
          <p className="font-medium text-gray-900 dark:text-white">Asset</p>
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Current Location</p>
          <p className="font-medium text-blue-600 dark:text-blue-400">
            {asset.location_name || 'Unknown'}
          </p>
        </div>
        <div>
          <p className="text-sm text-gray-500 dark:text-gray-400">Status</p>
          <FreshnessBadge lastSeen={asset.last_seen} />
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
          />
        )}
      </div>

      {/* Download Button */}
      <button
        className="w-full flex items-center justify-center gap-2 bg-blue-600 hover:bg-blue-700
          text-white font-medium py-3 px-4 rounded-lg transition-colors"
        onClick={() => {
          // TODO: Implement CSV download
          console.log('Download history CSV for asset:', asset.asset_id);
        }}
      >
        <Download className="w-4 h-4" />
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
            {asset.asset_name}
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
              {asset.asset_name}
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
    </>
  );
}
