import { formatDuration } from '@/lib/reports/utils';
import type { AssetHistoryItem } from '@/types/reports';
import { MapPin, Loader2 } from 'lucide-react';

interface MovementTimelineProps {
  data: AssetHistoryItem[];
  isLoading: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  onLoadMore: () => void;
}

export function MovementTimeline({
  data,
  isLoading,
  hasMore,
  isLoadingMore,
  onLoadMore,
}: MovementTimelineProps) {
  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex gap-3 animate-pulse">
            <div className="flex flex-col items-center">
              <div className="w-3 h-3 rounded-full bg-gray-300 dark:bg-gray-600" />
              <div className="w-0.5 flex-1 bg-gray-200 dark:bg-gray-700 mt-1" />
            </div>
            <div className="flex-1 pb-6">
              <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-24 mb-2" />
              <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-32" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="text-center py-8">
        <MapPin className="w-10 h-10 text-gray-300 dark:text-gray-600 mx-auto mb-3" />
        <p className="text-sm text-gray-500 dark:text-gray-400">
          No movement history in this date range
        </p>
      </div>
    );
  }

  return (
    <div className="relative">
      {data.map((item, index) => {
        const isFirst = index === 0;
        const isLastItem = index === data.length - 1;
        const showLine = !isLastItem || hasMore; // Continue line if more data available
        const time = new Date(item.timestamp);
        const isToday = new Date().toDateString() === time.toDateString();

        return (
          <div key={`${item.timestamp}-${index}`} className="flex gap-3">
            {/* Timeline connector */}
            <div className="flex flex-col items-center">
              {/* Dot */}
              <div
                className={`w-3 h-3 rounded-full flex-shrink-0 ${
                  isFirst
                    ? 'bg-green-500 ring-4 ring-green-500/20'
                    : 'bg-gray-300 dark:bg-gray-600'
                }`}
              />
              {/* Line */}
              {showLine && (
                <div className="w-0.5 flex-1 bg-gray-200 dark:bg-gray-700 min-h-[40px]" />
              )}
            </div>

            {/* Content */}
            <div className={`flex-1 ${isLastItem && !hasMore ? 'pb-0' : 'pb-4'}`}>
              {/* Location name */}
              <p
                className={`font-medium ${
                  isFirst
                    ? 'text-green-600 dark:text-green-400'
                    : 'text-gray-900 dark:text-white'
                }`}
              >
                {item.location_name || 'Unknown Location'}
                {isFirst && (
                  <span className="ml-2 text-xs font-normal text-green-600 dark:text-green-400">
                    (Current)
                  </span>
                )}
              </p>

              {/* Time and duration */}
              <div className="flex items-center gap-2 mt-1 text-sm text-gray-500 dark:text-gray-400">
                <span>
                  {time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  {!isToday && (
                    <span className="ml-1">
                      · {time.toLocaleDateString([], { month: 'short', day: 'numeric' })}
                    </span>
                  )}
                </span>
                {item.duration_seconds && (
                  <>
                    <span className="text-gray-300 dark:text-gray-600">•</span>
                    <span>{formatDuration(item.duration_seconds)}</span>
                  </>
                )}
              </div>
            </div>
          </div>
        );
      })}

      {/* Load More Button */}
      {hasMore && (
        <div className="flex gap-3">
          <div className="flex flex-col items-center">
            <div className="w-3 h-3 rounded-full bg-gray-200 dark:bg-gray-700 flex-shrink-0" />
          </div>
          <div className="flex-1">
            <button
              onClick={onLoadMore}
              disabled={isLoadingMore}
              className="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300
                font-medium disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
            >
              {isLoadingMore ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  Loading...
                </>
              ) : (
                'Load more history'
              )}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
