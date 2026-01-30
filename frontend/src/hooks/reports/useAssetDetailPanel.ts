import { useState, useEffect, useMemo, useCallback } from 'react';
import { useAssetHistory } from './useAssetHistory';
import { getDateRangeStart, type DateRange } from '@/lib/reports/utils';
import type { CurrentLocationItem, AssetHistoryItem } from '@/types/reports';

const PAGE_SIZE = 20;

interface UseAssetDetailPanelOptions {
  asset: CurrentLocationItem | null;
  onClose: () => void;
}

interface UseAssetDetailPanelReturn {
  // Visibility state
  isVisible: boolean;
  handleClose: () => void;

  // Date range
  dateRange: DateRange;
  setDateRange: (range: DateRange) => void;

  // Timeline data
  timelineData: AssetHistoryItem[];
  isLoading: boolean;
  error: Error | null;
  hasMore: boolean;
  isLoadingMore: boolean;
  handleLoadMore: () => void;

  // Error helpers
  isNotFoundError: boolean;
}

export function useAssetDetailPanel({
  asset,
  onClose,
}: UseAssetDetailPanelOptions): UseAssetDetailPanelReturn {
  const [dateRange, setDateRange] = useState<DateRange>('7days');
  const [isVisible, setIsVisible] = useState(false);
  const [offset, setOffset] = useState(0);
  const [accumulatedData, setAccumulatedData] = useState<AssetHistoryItem[]>([]);
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Memoize params to prevent infinite refetching
  const historyParams = useMemo(
    () => ({
      limit: PAGE_SIZE,
      offset,
      start_date: getDateRangeStart(dateRange).toISOString(),
    }),
    [dateRange, offset]
  );

  const {
    data: historyData,
    totalCount,
    isLoading,
    error,
  } = useAssetHistory(asset?.asset_id ?? null, historyParams);

  // Accumulate data when new data arrives
  useEffect(() => {
    if (historyData && historyData.length > 0) {
      if (offset === 0) {
        setAccumulatedData(historyData);
      } else {
        setAccumulatedData((prev) => [...prev, ...historyData]);
      }
      setIsLoadingMore(false);
    }
  }, [historyData, offset]);

  // Reset when date range or asset changes
  useEffect(() => {
    setOffset(0);
    setAccumulatedData([]);
  }, [dateRange, asset?.asset_id]);

  // Animate in when asset changes
  useEffect(() => {
    if (asset) {
      requestAnimationFrame(() => setIsVisible(true));
    } else {
      setIsVisible(false);
    }
  }, [asset]);

  const handleLoadMore = useCallback(() => {
    setIsLoadingMore(true);
    setOffset((prev) => prev + PAGE_SIZE);
  }, []);

  const handleClose = useCallback(() => {
    setIsVisible(false);
    setTimeout(onClose, 200);
  }, [onClose]);

  const hasMore = accumulatedData.length < totalCount;

  // Check if error is a 404 (no history)
  const isNotFoundError =
    (error as { response?: { status?: number } })?.response?.status === 404;

  return {
    isVisible,
    handleClose,
    dateRange,
    setDateRange,
    timelineData: accumulatedData,
    isLoading: isLoading && offset === 0,
    error,
    hasMore,
    isLoadingMore,
    handleLoadMore,
    isNotFoundError,
  };
}
