import { useState, useMemo, useCallback } from 'react';
import { useLocations } from '@/hooks/locations/useLocations';
import { useCurrentLocations } from './useCurrentLocations';
import { useDebounce } from '@/hooks/useDebounce';
import { matchesTimeRange, type TimeRangeFilter } from '@/lib/reports/utils';
import type { CurrentLocationItem } from '@/types/reports';
import type { Location } from '@/types/locations';

interface UseReportsFiltersProps {
  pageSize: number;
  currentPage: number;
}

interface UseReportsFiltersReturn {
  // Filter state
  selectedLocationId: number | null;
  setSelectedLocationId: (id: number | null) => void;
  selectedTimeRange: TimeRangeFilter;
  setSelectedTimeRange: (range: TimeRangeFilter) => void;
  search: string;
  setSearch: (search: string) => void;

  // Location data for dropdown
  locations: Location[];
  isLoadingLocations: boolean;

  // Filtered results
  filteredData: CurrentLocationItem[];
  totalCount: number;
  isLoading: boolean;
  error: Error | null;

  // Filter helpers
  hasActiveFilters: boolean;
  clearFilters: () => void;
  activeFilterDescription: string;
}

export function useReportsFilters({
  pageSize,
  currentPage,
}: UseReportsFiltersProps): UseReportsFiltersReturn {
  // 1. Filter state
  const [selectedLocationId, setSelectedLocationId] = useState<number | null>(null);
  const [selectedTimeRange, setSelectedTimeRange] = useState<TimeRangeFilter>('all');
  const [search, setSearch] = useState('');

  // 2. Fetch locations for dropdown
  const { locations, isLoading: isLoadingLocations } = useLocations();

  // 3. Fetch current locations with server-side location filter
  const debouncedSearch = useDebounce(search, 300);
  const { data, totalCount, isLoading, error } = useCurrentLocations({
    search: debouncedSearch || undefined,
    location_id: selectedLocationId || undefined,
    limit: pageSize,
    offset: (currentPage - 1) * pageSize,
  });

  // 4. Apply client-side time range filter
  const filteredData = useMemo(() => {
    if (selectedTimeRange === 'all') return data;
    return data.filter((item) => matchesTimeRange(item.last_seen, selectedTimeRange));
  }, [data, selectedTimeRange]);

  // 5. Helper: check if any filters are active
  const hasActiveFilters = useMemo(
    () => selectedLocationId !== null || selectedTimeRange !== 'all' || search !== '',
    [selectedLocationId, selectedTimeRange, search]
  );

  // 6. Helper: clear all filters
  const clearFilters = useCallback(() => {
    setSelectedLocationId(null);
    setSelectedTimeRange('all');
    setSearch('');
  }, []);

  // 7. Helper: generate description of active filters for empty state
  const activeFilterDescription = useMemo(() => {
    const parts: string[] = [];
    if (selectedLocationId) {
      const loc = locations.find((l) => l.id === selectedLocationId);
      if (loc) parts.push(`at "${loc.name}"`);
    }
    if (selectedTimeRange !== 'all') {
      const labels: Record<TimeRangeFilter, string> = {
        all: '',
        live: 'live',
        today: 'seen today',
        week: 'seen this week',
        stale: 'stale',
      };
      parts.push(labels[selectedTimeRange]);
    }
    if (search) parts.push(`matching "${search}"`);
    return parts.join(' and ');
  }, [selectedLocationId, selectedTimeRange, search, locations]);

  return {
    selectedLocationId,
    setSelectedLocationId,
    selectedTimeRange,
    setSelectedTimeRange,
    search,
    setSearch,
    locations,
    isLoadingLocations,
    filteredData,
    totalCount,
    isLoading,
    error,
    hasActiveFilters,
    clearFilters,
    activeFilterDescription,
  };
}
