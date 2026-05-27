import { FreshnessBadge } from './FreshnessBadge';
import { formatRelativeTime, formatTimestampForExport } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import { MapPin, Clock } from 'lucide-react';

interface CurrentLocationCardProps {
  item: CurrentLocationItem;
  onClick: () => void;
  getAssetName: (item: CurrentLocationItem) => string;
  getLocationName: (item: CurrentLocationItem) => string;
}

export function CurrentLocationCard({
  item,
  onClick,
  getAssetName,
  getLocationName,
}: CurrentLocationCardProps) {
  const assetName = getAssetName(item);
  const assetKey = item.asset_external_key ?? '';
  const locationName = getLocationName(item);
  const locationKey = item.location_external_key ?? '';
  const showAssetSubtext = assetKey && assetKey !== assetName;
  const showLocationSubtext =
    locationKey && locationKey !== locationName && locationName !== 'Unknown';

  return (
    <div
      className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 cursor-pointer hover:border-blue-300 dark:hover:border-blue-600 transition-colors"
      onClick={onClick}
    >
      <div className="flex justify-between items-start mb-2">
        <div className="min-w-0">
          <h3 className="font-medium text-gray-900 dark:text-gray-100 truncate">
            {assetName}
          </h3>
          {showAssetSubtext && (
            <p className="text-sm text-gray-500 dark:text-gray-400 truncate">
              {assetKey}
            </p>
          )}
        </div>
        <FreshnessBadge lastSeen={item.asset_last_seen} />
      </div>

      <div className="flex items-center gap-4 text-sm text-gray-600 dark:text-gray-400 mb-2">
        <div className="flex items-start gap-1">
          <MapPin className="w-4 h-4 mt-0.5 flex-shrink-0" />
          <div className="min-w-0">
            <div>{locationName === 'Unknown' ? 'Unknown' : locationName}</div>
            {showLocationSubtext && (
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {locationKey}
              </div>
            )}
          </div>
        </div>
      </div>
      <div className="flex items-center gap-1 text-sm">
        <Clock className="w-4 h-4 text-gray-400" />
        <span className="text-gray-900 dark:text-gray-100">{formatTimestampForExport(item.asset_last_seen)}</span>
        <span className="text-gray-500 dark:text-gray-400">({formatRelativeTime(item.asset_last_seen)})</span>
      </div>
    </div>
  );
}
