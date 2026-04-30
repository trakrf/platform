import { useMemo } from 'react';
import { X, Filter } from 'lucide-react';
import { useAssetStore } from '@/stores';
import { Container } from '@/components/shared';

interface AssetFiltersProps {
  isOpen?: boolean;
  onToggle?: () => void;
  className?: string;
}

export function AssetFilters({ isOpen = true, onToggle, className = '' }: AssetFiltersProps) {
  const filters = useAssetStore((state) => state.filters);
  const setFilters = useAssetStore((state) => state.setFilters);

  const activeFilterCount = useMemo(() => {
    let count = 0;
    if (filters.is_active !== 'all' && filters.is_active !== undefined) count++;
    if (filters.search && filters.search.trim() !== '') count++;
    return count;
  }, [filters]);

  const handleClearFilters = () => {
    setFilters({ is_active: 'all', search: '' });
  };

  const handleStatusChange = (status: boolean | 'all') => {
    setFilters({ is_active: status });
  };

  if (!isOpen) {
    return null;
  }

  return (
    <div className={className}>
      <Container variant="gray" padding="small" border={true} rounded={true}>
        {/* Header */}
        <div className="flex items-center justify-between mb-4 pb-3 border-b border-gray-300 dark:border-gray-600">
          <div className="flex items-center gap-2">
            <Filter className="h-5 w-5 text-gray-600 dark:text-gray-400" />
            <h3 className="text-base font-semibold text-gray-900 dark:text-white">Filters</h3>
            {activeFilterCount > 0 && (
              <span className="inline-flex items-center justify-center px-2 py-0.5 text-xs font-bold text-blue-700 bg-blue-100 dark:text-blue-300 dark:bg-blue-900/40 rounded-full">
                {activeFilterCount}
              </span>
            )}
          </div>
          {activeFilterCount > 0 && (
            <button
              onClick={handleClearFilters}
              className="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300 font-medium"
            >
              Clear All
            </button>
          )}
          {onToggle && (
            <button
              onClick={onToggle}
              className="md:hidden p-1 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            >
              <X className="h-5 w-5" />
            </button>
          )}
        </div>

        {/* Status Filter */}
        <div className="mb-6">
          <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">Status</h4>
          <div className="space-y-2">
            <label className="flex items-center gap-3 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700/50 p-2 rounded-lg transition-colors">
              <input
                type="radio"
                name="status"
                checked={filters.is_active === 'all' || filters.is_active === undefined}
                onChange={() => handleStatusChange('all')}
                className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 focus:ring-blue-500 dark:focus:ring-blue-400"
              />
              <span className="flex-1 text-sm text-gray-700 dark:text-gray-300">All</span>
            </label>
            <label className="flex items-center gap-3 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700/50 p-2 rounded-lg transition-colors">
              <input
                type="radio"
                name="status"
                checked={filters.is_active === true}
                onChange={() => handleStatusChange(true)}
                className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 focus:ring-blue-500 dark:focus:ring-blue-400"
              />
              <span className="flex-1 text-sm text-green-700 dark:text-green-400">Active</span>
            </label>
            <label className="flex items-center gap-3 cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-700/50 p-2 rounded-lg transition-colors">
              <input
                type="radio"
                name="status"
                checked={filters.is_active === false}
                onChange={() => handleStatusChange(false)}
                className="h-4 w-4 text-blue-600 border-gray-300 dark:border-gray-600 focus:ring-blue-500 dark:focus:ring-blue-400"
              />
              <span className="flex-1 text-sm text-gray-600 dark:text-gray-400">Inactive</span>
            </label>
          </div>
        </div>
      </Container>
    </div>
  );
}
