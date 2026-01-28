import { formatDuration, formatRelativeTime } from '@/lib/reports/utils';
import type { AssetHistoryItem } from '@/types/reports';
import { MapPin, Clock } from 'lucide-react';

interface AssetHistoryCardProps {
  item: AssetHistoryItem;
  isFirst?: boolean;
}

export function AssetHistoryCard({ item, isFirst = false }: AssetHistoryCardProps) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4">
      <div className="flex justify-between items-start mb-2">
        <div className="flex items-center gap-2">
          <MapPin className="w-4 h-4 text-gray-500" />
          <span className="font-medium text-gray-900 dark:text-gray-100">
            {item.location_name || 'Unknown'}
          </span>
        </div>
        {isFirst && (
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300">
            Current
          </span>
        )}
      </div>

      <div className="flex items-center gap-4 text-sm text-gray-600 dark:text-gray-400">
        <div className="flex items-center gap-1">
          <Clock className="w-4 h-4" />
          <span>{formatRelativeTime(item.timestamp)}</span>
        </div>
        <div>
          <span className="text-gray-400">Duration: </span>
          <span>{formatDuration(item.duration_seconds)}</span>
        </div>
      </div>
    </div>
  );
}
