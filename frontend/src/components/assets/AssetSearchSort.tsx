import React, { useState, useEffect, useMemo } from 'react';
import { Search, X, ArrowUpDown, MapPin } from 'lucide-react';
import { useAssetStore, useLocationStore } from '@/stores';
import { useLocations } from '@/hooks/locations';
import type { SortState } from '@/types/assets';

interface AssetSearchSortProps {
  className?: string;
}

const SORT_OPTIONS: Array<{ value: SortState['field']; label: string }> = [
  { value: 'identifier', label: 'Identifier' },
  { value: 'name', label: 'Name' },
  { value: 'type', label: 'Type' },
  { value: 'is_active', label: 'Status' },
  { value: 'created_at', label: 'Created Date' },
];

export function AssetSearchSort({ className = '' }: AssetSearchSortProps) {
  const search = useAssetStore((state) => state.filters.search);
  const locationFilter = useAssetStore((state) => state.filters.location_id);
  const setSearchTerm = useAssetStore((state) => state.setSearchTerm);
  const setFilters = useAssetStore((state) => state.setFilters);
  const { field: sortField, direction: sortDirection } = useAssetStore((state) => state.sort);
  const setSort = useAssetStore((state) => state.setSort);
  const cache = useAssetStore((state) => state.cache);
  const filters = useAssetStore((state) => state.filters);
  const sort = useAssetStore((state) => state.sort);

  // Load locations for filter dropdown
  useLocations({ enabled: true });
  const locationCache = useLocationStore((state) => state.cache.byId);
  const locations = useMemo(() => Array.from(locationCache.values()), [locationCache]);

  const filteredAssetsCount = useMemo(() => {
    return useAssetStore.getState().getFilteredAssets().length;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cache.byId.size, cache.lastFetched, filters, sort]);

  const [localSearch, setLocalSearch] = useState(search ?? '');

  useEffect(() => {
    const timer = setTimeout(() => {
      setSearchTerm(localSearch);
    }, 300);

    return () => clearTimeout(timer);
  }, [localSearch, setSearchTerm]);

  useEffect(() => {
    setLocalSearch(search ?? '');
  }, [search]);

  const handleClearSearch = () => {
    setLocalSearch('');
    setSearchTerm('');
  };

  const handleSortChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newField = e.target.value as SortState['field'];
    setSort(newField, sortDirection);
  };

  const toggleSortDirection = () => {
    const newDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    setSort(sortField, newDirection);
  };

  const handleLocationChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const value = e.target.value;
    if (value === 'all') {
      setFilters({ location_id: 'all' });
    } else if (value === 'unassigned') {
      setFilters({ location_id: null });
    } else {
      setFilters({ location_id: Number(value) });
    }
  };

  return (
    <div className={`flex flex-col md:flex-row gap-3 md:items-center md:justify-between ${className}`}>
      {/* Search Input */}
      <div className="relative flex-1 max-w-md">
        <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
          <Search className="h-5 w-5 text-gray-400 dark:text-gray-500" />
        </div>
        <input
          type="text"
          value={localSearch}
          onChange={(e) => setLocalSearch(e.target.value)}
          placeholder="Search assets..."
          className="block w-full pl-10 pr-10 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 focus:border-transparent"
        />
        {localSearch && (
          <button
            onClick={handleClearSearch}
            className="absolute inset-y-0 right-0 pr-3 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          >
            <X className="h-5 w-5" />
          </button>
        )}
      </div>

      {/* Location Filter */}
      <div className="flex items-center gap-2">
        <MapPin className="h-4 w-4 text-gray-400 dark:text-gray-500" />
        <select
          id="location-filter"
          value={locationFilter === null ? 'unassigned' : locationFilter === 'all' ? 'all' : String(locationFilter)}
          onChange={handleLocationChange}
          className="block py-2 pl-3 pr-8 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 focus:border-transparent text-sm"
        >
          <option value="all">All Locations</option>
          <option value="unassigned">Unassigned</option>
          {locations
            .filter(loc => loc.is_active)
            .sort((a, b) => a.name.localeCompare(b.name))
            .map((location) => (
              <option key={location.id} value={location.id}>
                {location.name}
              </option>
            ))}
        </select>
      </div>

      {/* Sort Controls */}
      <div className="flex items-center gap-2">
        <label htmlFor="sort-select" className="text-sm font-medium text-gray-700 dark:text-gray-300 whitespace-nowrap">
          Sort by:
        </label>
        <select
          id="sort-select"
          value={sortField}
          onChange={handleSortChange}
          className="block py-2 pl-3 pr-10 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400 focus:border-transparent"
        >
          {SORT_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <button
          onClick={toggleSortDirection}
          className="p-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
          title={sortDirection === 'asc' ? 'Ascending' : 'Descending'}
        >
          <ArrowUpDown
            className={`h-5 w-5 transition-transform ${sortDirection === 'desc' ? 'rotate-180' : ''}`}
          />
        </button>
      </div>

      {/* Results Count */}
      <div className="text-sm text-gray-600 dark:text-gray-400 whitespace-nowrap">
        {filteredAssetsCount} {filteredAssetsCount === 1 ? 'result' : 'results'}
      </div>
    </div>
  );
}
