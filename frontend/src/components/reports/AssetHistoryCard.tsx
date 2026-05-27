import { formatDuration, formatRelativeTime } from '@/lib/reports/utils';
import type { AssetHistoryItem } from '@/types/reports';
import { MapPin, Clock } from 'lucide-react';

interface AssetHistoryCardProps {
  item: AssetHistoryItem;
  isFirst?: boolean;
  getLocationName: (item: AssetHistoryItem) => string;
}

export function AssetHistoryCard({
  item,
  isFirst = false,
  getLocationName,
}: AssetHistoryCardProps) {
  const locationName = getLocationName(item);
  const locationKey = item.location_external_key ?? '';
  const showSubtext =
    locationKey && locationKey !== locationName && locationName !== 'Unknown';

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
      <div className="flex justify-between items-start mb-2">
        <div className="flex items-start gap-2 min-w-0">
          <MapPin className="w-4 h-4 text-gray-500 mt-0.5 flex-shrink-0" />
          <div className="min-w-0">
            <span className="font-medium text-gray-900 dark:text-gray-100 block truncate">
              {locationName === 'Unknown' ? 'Unknown' : locationName}
            </span>
            {showSubtext && (
              <span className="text-xs text-gray-500 dark:text-gray-400 block truncate">
                {locationKey}
              </span>
            )}
          </div>
        </div>
        {isFirst && (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300 flex-shrink-0">
            Current
          </span>
        )}
      </div>

      <div className="flex items-center gap-4 text-sm text-gray-600 dark:text-gray-400">
        <div className="flex items-center gap-1">
          <Clock className="w-4 h-4" />
          <span>{formatRelativeTime(item.event_observed_at)}</span>
        </div>
        <div>
          <span className="text-gray-400">Duration: </span>
          <span>{formatDuration(item.duration_seconds)}</span>
        </div>
      </div>
    </div>
  );
}
