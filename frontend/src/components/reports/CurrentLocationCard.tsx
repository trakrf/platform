import { FreshnessBadge } from './FreshnessBadge';
import { formatRelativeTime } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import { MapPin, Clock } from 'lucide-react';

interface CurrentLocationCardProps {
  item: CurrentLocationItem;
  onClick: () => void;
}

export function CurrentLocationCard({ item, onClick }: CurrentLocationCardProps) {
  return (
    <div
      className="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-4 cursor-pointer hover:border-blue-300 dark:hover:border-blue-600 transition-colors"
      onClick={onClick}
    >
      <div className="flex justify-between items-start mb-2">
        <div>
          <h3 className="font-medium text-gray-900 dark:text-gray-100">
            {item.asset_name}
          </h3>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            {item.asset_identifier}
          </p>
        </div>
        <FreshnessBadge lastSeen={item.last_seen} />
      </div>

      <div className="flex items-center gap-4 text-sm text-gray-600 dark:text-gray-400">
        <div className="flex items-center gap-1">
          <MapPin className="w-4 h-4" />
          <span>{item.location_name || 'Unknown'}</span>
        </div>
        <div className="flex items-center gap-1">
          <Clock className="w-4 h-4" />
          <span>{formatRelativeTime(item.last_seen)}</span>
        </div>
      </div>
    </div>
  );
}
