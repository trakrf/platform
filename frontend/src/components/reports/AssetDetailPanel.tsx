import { useState, useEffect } from 'react';
import { X, Download } from 'lucide-react';
import { useAssetHistory } from '@/hooks/reports';
import { FreshnessBadge } from './FreshnessBadge';
import { formatDuration } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';

interface AssetDetailPanelProps {
  asset: CurrentLocationItem | null;
  onClose: () => void;
}

type DateRange = 'today' | '7days' | '30days' | '90days';

function getDateRangeStart(range: DateRange): Date {
  const now = new Date();
  switch (range) {
    case 'today':
      return new Date(now.getFullYear(), now.getMonth(), now.getDate());
    case '7days':
      return new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000);
    case '30days':
      return new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);
    case '90days':
      return new Date(now.getTime() - 90 * 24 * 60 * 60 * 1000);
  }
}

export function AssetDetailPanel({ asset, onClose }: AssetDetailPanelProps) {
  const [dateRange, setDateRange] = useState<DateRange>('7days');
  const [isVisible, setIsVisible] = useState(false);

  const startDate = getDateRangeStart(dateRange);

  const { data: historyData, isLoading } = useAssetHistory(asset?.asset_id ?? null, {
    limit: 50,
    offset: 0,
    start_date: startDate.toISOString(),
  });

  // Animate in when asset changes
  useEffect(() => {
    if (asset) {
      // Small delay to trigger CSS transition
      requestAnimationFrame(() => setIsVisible(true));
    } else {
      setIsVisible(false);
    }
  }, [asset]);

  const handleClose = () => {
    setIsVisible(false);
    // Wait for animation to complete
    setTimeout(onClose, 200);
  };

  if (!asset) return null;

  const dateRangeOptions: { value: DateRange; label: string }[] = [
    { value: 'today', label: 'Today' },
    { value: '7days', label: '7 Days' },
    { value: '30days', label: '30 Days' },
    { value: '90days', label: '90 Days' },
  ];

  return (
    <>
      {/* Backdrop */}
      <div
        className={`fixed inset-0 bg-black/30 z-40 transition-opacity duration-200 ${
          isVisible ? 'opacity-100' : 'opacity-0'
        }`}
        onClick={handleClose}
      />

      {/* Panel */}
      <div
        className={`fixed right-0 top-0 h-full w-full max-w-md bg-white dark:bg-gray-900 shadow-xl z-50
          transform transition-transform duration-200 ease-out overflow-y-auto
          ${isVisible ? 'translate-x-0' : 'translate-x-full'}`}
      >
        {/* Header */}
        <div className="sticky top-0 bg-white dark:bg-gray-900 border-b border-gray-200 dark:border-gray-700 p-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            {asset.asset_name}
          </h2>
          <button
            onClick={handleClose}
            className="p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
          >
            <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
          </button>
        </div>

        <div className="p-4 space-y-6">
          {/* Asset Info Grid */}
          <div className="grid grid-cols-2 gap-4">
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
          <div>
            <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              Date Range
            </p>
            <div className="flex gap-2">
              {dateRangeOptions.map((option) => (
                <button
                  key={option.value}
                  onClick={() => setDateRange(option.value)}
                  className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
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
          <div>
            <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
              Movement Timeline
            </p>

            {isLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 5 }).map((_, i) => (
                  <div key={i} className="animate-pulse flex items-center gap-4">
                    <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-16" />
                    <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded flex-1" />
                    <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-12" />
                  </div>
                ))}
              </div>
            ) : historyData.length === 0 ? (
              <p className="text-sm text-gray-500 dark:text-gray-400 text-center py-4">
                No movement history in this date range
              </p>
            ) : (
              <div className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
                <table className="w-full">
                  <thead className="bg-gray-50 dark:bg-gray-800">
                    <tr>
                      <th className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                        Time
                      </th>
                      <th className="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                        Location
                      </th>
                      <th className="px-3 py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">
                        Duration
                      </th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                    {historyData.map((item, index) => {
                      const isFirst = index === 0;
                      const time = new Date(item.timestamp);
                      const isToday = new Date().toDateString() === time.toDateString();

                      return (
                        <tr
                          key={`${item.timestamp}-${index}`}
                          className="hover:bg-gray-50 dark:hover:bg-gray-800/50"
                        >
                          <td className="px-3 py-2">
                            <div className="text-sm text-gray-900 dark:text-white">
                              {time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                            </div>
                            {!isToday && (
                              <div className="text-xs text-gray-500 dark:text-gray-400">
                                {time.toLocaleDateString([], { month: 'short', day: 'numeric' })}
                              </div>
                            )}
                          </td>
                          <td className="px-3 py-2">
                            <span
                              className={`text-sm ${
                                isFirst
                                  ? 'text-green-600 dark:text-green-400 font-medium'
                                  : 'text-gray-700 dark:text-gray-300'
                              }`}
                            >
                              {item.location_name || 'Unknown'}
                              {isFirst && ' *'}
                            </span>
                          </td>
                          <td className="px-3 py-2 text-right text-sm text-gray-600 dark:text-gray-400">
                            {item.duration_seconds ? formatDuration(item.duration_seconds) : '—'}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
                <div className="px-3 py-2 bg-gray-50 dark:bg-gray-800 text-xs text-gray-500 dark:text-gray-400">
                  * Current location
                </div>
              </div>
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
        </div>
      </div>
    </>
  );
}
