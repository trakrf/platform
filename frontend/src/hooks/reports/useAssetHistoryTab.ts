import { useState, useEffect, useMemo, useCallback } from 'react';
import { useCurrentLocations } from './useCurrentLocations';
import { useAssetHistory } from './useAssetHistory';
import { formatDuration } from '@/lib/reports/utils';
import type { AssetHistoryItem } from '@/types/reports';

const PAGE_SIZE = 20;

export interface AssetOption {
  id: number;
  name: string;
  identifier: string;
}

interface AssetHistoryStats {
  locationsVisited: number;
  timeTracked: string;
  currentLocation: string | null;
}

interface UseAssetHistoryTabReturn {
  // Asset selection
  selectedAssetId: number | null;
  setSelectedAssetId: (id: number | null) => void;
  assetOptions: AssetOption[];
  isLoadingAssets: boolean;

  // Date range
  fromDate: string;
  toDate: string;
  setFromDate: (date: string) => void;
  setToDate: (date: string) => void;

  // Timeline data
  timelineData: AssetHistoryItem[];
  isLoadingTimeline: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
  handleLoadMore: () => void;

  // Calculated stats
  stats: AssetHistoryStats | null;

  // Selected asset info
  selectedAsset: AssetOption | null;
}

function getDefaultFromDate(): string {
  const d = new Date();
  d.setDate(d.getDate() - 30);
  return d.toISOString().split('T')[0];
}

function getDefaultToDate(): string {
  return new Date().toISOString().split('T')[0];
}

export function useAssetHistoryTab(): UseAssetHistoryTabReturn {
  // Asset selection
  const [selectedAssetId, setSelectedAssetId] = useState<number | null>(null);

  // Date range (YYYY-MM-DD format for native inputs)
  const [fromDate, setFromDate] = useState(getDefaultFromDate);
  const [toDate, setToDate] = useState(getDefaultToDate);

  // Pagination state
  const [offset, setOffset] = useState(0);
  const [accumulatedData, setAccumulatedData] = useState<AssetHistoryItem[]>(
    []
  );
  const [isLoadingMore, setIsLoadingMore] = useState(false);

  // Fetch asset list for dropdown
  const { data: assetsData, isLoading: isLoadingAssets } = useCurrentLocations({
    limit: 1000,
  });

  // Transform to AssetOption[]
  const assetOptions = useMemo<AssetOption[]>(
    () =>
      assetsData.map((a) => ({
        id: a.asset_id,
        name: a.asset_name,
        identifier: a.asset_identifier,
      })),
    [assetsData]
  );

  // Memoize history params to prevent infinite refetching
  const historyParams = useMemo(
    () => ({
      limit: PAGE_SIZE,
      offset,
      start_date: new Date(fromDate).toISOString(),
      end_date: new Date(toDate + 'T23:59:59').toISOString(),
    }),
    [fromDate, toDate, offset]
  );

  // Fetch timeline data
  const {
    data: historyData,
    totalCount,
    isLoading,
  } = useAssetHistory(selectedAssetId, historyParams);

  // Accumulate data when new data arrives
  useEffect(() => {
    if (historyData && historyData.length > 0) {
      if (offset === 0) {
        setAccumulatedData(historyData);
      } else {
        setAccumulatedData((prev) => [...prev, ...historyData]);
      }
      setIsLoadingMore(false);
    } else if (historyData && historyData.length === 0 && offset === 0) {
      setAccumulatedData([]);
    }
  }, [historyData, offset]);

  // Reset when filters change
  useEffect(() => {
    setOffset(0);
    setAccumulatedData([]);
  }, [selectedAssetId, fromDate, toDate]);

  // Calculate stats from accumulated data
  const stats = useMemo<AssetHistoryStats | null>(() => {
    if (accumulatedData.length === 0) return null;

    const uniqueLocations = new Set(
      accumulatedData.filter((d) => d.location_id).map((d) => d.location_id)
    );

    const totalSeconds = accumulatedData.reduce(
      (sum, d) => sum + (d.duration_seconds || 0),
      0
    );

    return {
      locationsVisited: uniqueLocations.size,
      timeTracked: formatDuration(totalSeconds),
      currentLocation: accumulatedData[0]?.location_name || null,
    };
  }, [accumulatedData]);

  // Find selected asset info
  const selectedAsset = useMemo<AssetOption | null>(
    () => assetOptions.find((a) => a.id === selectedAssetId) || null,
    [assetOptions, selectedAssetId]
  );

  const handleLoadMore = useCallback(() => {
    setIsLoadingMore(true);
    setOffset((prev) => prev + PAGE_SIZE);
  }, []);

  const hasMore = accumulatedData.length < totalCount;

  return {
    selectedAssetId,
    setSelectedAssetId,
    assetOptions,
    isLoadingAssets,
    fromDate,
    toDate,
    setFromDate,
    setToDate,
    timelineData: accumulatedData,
    isLoadingTimeline: isLoading && offset === 0,
    hasMore,
    isLoadingMore,
    handleLoadMore,
    stats,
    selectedAsset,
  };
}
