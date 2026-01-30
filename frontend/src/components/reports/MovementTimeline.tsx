import { useMemo } from 'react';
import {
  formatDuration,
  formatDate,
  formatTime,
  getEndTime,
  groupTimelineByDate,
  calculateDurationProgress,
} from '@/lib/reports/utils';
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
  const groupedData = useMemo(() => groupTimelineByDate(data), [data]);

  if (isLoading) {
    return (
      <div className="space-y-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="animate-pulse">
            <div className="h-5 bg-gray-200 dark:bg-gray-700 rounded w-32 mb-3" />
            <div className="ml-4 space-y-3">
              <div className="flex gap-3">
                <div className="w-2 h-2 rounded-full bg-gray-300 dark:bg-gray-600 mt-1.5" />
                <div className="flex-1">
                  <div className="h-4 bg-gray-200 dark:bg-gray-700 rounded w-20 mb-1" />
                  <div className="h-3 bg-gray-200 dark:bg-gray-700 rounded w-24" />
                </div>
              </div>
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
    <div className="space-y-5">
      {groupedData.map((group, groupIndex) => {
        const groupDate = new Date(group.date);

        return (
          <div key={group.date}>
            {/* Date Header */}
            <div className="flex items-center gap-2 mb-3">
              <span className="text-sm font-semibold text-gray-900 dark:text-white">
                {formatDate(groupDate)}
              </span>
              {group.dateLabel && (
                <span className="text-xs text-gray-500 dark:text-gray-400">
                  {group.dateLabel}
                </span>
              )}
            </div>

            {/* Items for this date */}
            <div className="relative ml-1">
              {group.items.map((item, itemIndex) => {
                const isFirstOverall = groupIndex === 0 && itemIndex === 0;
                const isLastInGroup = itemIndex === group.items.length - 1;
                const isLastOverall =
                  groupIndex === groupedData.length - 1 && isLastInGroup;
                const showLine = !isLastInGroup || hasMore || !isLastOverall;

                const startTime = new Date(item.timestamp);
                const endTime = getEndTime(startTime, item.duration_seconds);
                const isOngoing = isFirstOverall && !endTime;
                const progressPercent = calculateDurationProgress(
                  item.duration_seconds,
                  isOngoing
                );

                return (
                  <div key={`${item.timestamp}-${itemIndex}`} className="flex gap-3">
                    {/* Timeline connector */}
                    <div className="flex flex-col items-center">
                      <div
                        className={`w-2 h-2 rounded-full flex-shrink-0 mt-1.5 ${
                          isFirstOverall
                            ? 'bg-green-500 ring-2 ring-green-500/30'
                            : 'bg-gray-300 dark:bg-gray-600'
                        }`}
                      />
                      {showLine && (
                        <div className="w-0.5 flex-1 bg-gray-200 dark:bg-gray-700 min-h-[50px]" />
                      )}
                    </div>

                    {/* Content */}
                    <div className={`flex-1 ${isLastInGroup && !hasMore ? 'pb-0' : 'pb-4'}`}>
                      {/* Time */}
                      <div className="flex items-center gap-2 text-sm">
                        <span className="text-gray-600 dark:text-gray-400">
                          {formatTime(startTime)}
                          {endTime && !isOngoing && (
                            <span className="text-gray-400 dark:text-gray-500">
                              {' '}- {formatTime(endTime)}
                            </span>
                          )}
                        </span>
                        {isFirstOverall && (
                          <span className="px-1.5 py-0.5 text-[10px] font-semibold bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400 rounded">
                            NOW
                          </span>
                        )}
                      </div>

                      {/* Location name */}
                      <p
                        className={`font-medium mt-0.5 ${
                          isFirstOverall
                            ? 'text-gray-900 dark:text-white'
                            : 'text-gray-700 dark:text-gray-300'
                        }`}
                      >
                        {item.location_name || 'Unknown Location'}
                      </p>

                      {/* Duration bar */}
                      {(item.duration_seconds || isOngoing) && (
                        <div className="mt-2">
                          <div className="h-2 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden w-full max-w-[200px]">
                            <div
                              className={`h-full rounded-full ${
                                isFirstOverall ? 'bg-green-500' : 'bg-blue-500'
                              }`}
                              style={{ width: `${progressPercent}%` }}
                            />
                          </div>
                          <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                            {item.duration_seconds ? formatDuration(item.duration_seconds) : ''}
                            {isOngoing && ' (ongoing)'}
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}

      {/* Load More Button */}
      {hasMore && (
        <div className="ml-1 flex gap-3">
          <div className="flex flex-col items-center">
            <div className="w-2 h-2 rounded-full bg-gray-200 dark:bg-gray-700 flex-shrink-0 mt-1" />
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
