import { useMemo } from 'react';
import { ArrowUp, ArrowDown, ArrowUpDown, MapPin } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { EmptyState, SkeletonTableRow } from '@/components/shared';
import { PaginationControls } from '@/components/shared/pagination';
import { LocationCard } from './LocationCard';
import type { Location } from '@/types/locations';

interface LocationTableProps {
  loading?: boolean;
  locations?: Location[];
  totalLocations?: number;
  currentPage?: number;
  pageSize?: number;
  onPageChange?: (page: number) => void;
  onPageSizeChange?: (size: number) => void;
  onLocationClick?: (location: Location) => void;
  onEdit?: (location: Location) => void;
  onDelete?: (location: Location) => void;
  className?: string;
}

type SortableField = 'identifier' | 'name' | 'created_at';

export function LocationTable({
  loading = false,
  locations: propLocations,
  totalLocations = 0,
  currentPage = 1,
  pageSize = 10,
  onPageChange,
  onPageSizeChange,
  onLocationClick,
  onEdit,
  onDelete,
  className = '',
}: LocationTableProps) {
  const cache = useLocationStore((state) => state.cache);
  const filters = useLocationStore((state) => state.filters);
  const sort = useLocationStore((state) => state.sort);
  const { field: sortField, direction: sortDirection } = sort;
  const setSort = useLocationStore((state) => state.setSort);

  const locations = propLocations ?? useMemo(() => {
    return useLocationStore.getState().getFilteredLocations();
  }, [cache.byId.size, filters, sort]);

  const handleSort = (field: SortableField) => {
    if (sortField === field) {
      const newDirection = sortDirection === 'asc' ? 'desc' : 'asc';
      setSort({ field, direction: newDirection });
    } else {
      setSort({ field, direction: 'asc' });
    }
  };

  const getSortIcon = (field: SortableField) => {
    if (sortField !== field) {
      return <ArrowUpDown className="h-4 w-4 text-gray-400" />;
    }
    return sortDirection === 'asc' ? (
      <ArrowUp className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    ) : (
      <ArrowDown className="h-4 w-4 text-blue-600 dark:text-blue-400" />
    );
  };

  const SortableHeader = ({
    field,
    label,
  }: {
    field: SortableField;
    label: string;
  }) => (
    <th className="px-4 py-3 text-left">
      <button
        onClick={() => handleSort(field)}
        className="flex items-center gap-2 text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
      >
        {label}
        {getSortIcon(field)}
      </button>
    </th>
  );

  if (loading) {
    return (
      <div className={`hidden md:block overflow-x-auto ${className}`}>
        <table className="w-full">
          <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Type
              </th>
              <SortableHeader field="identifier" label="Identifier" />
              <SortableHeader field="name" label="Name" />
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Description
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Status
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <SkeletonTableRow key={i} />
            ))}
          </tbody>
        </table>
      </div>
    );
  }

  if (locations.length === 0) {
    return (
      <div className={`hidden md:block ${className}`}>
        <EmptyState
          icon={MapPin}
          title="No Locations Found"
          description="Try adjusting your filters or create a new location to get started."
        />
      </div>
    );
  }

  const totalPages = Math.max(1, Math.ceil(totalLocations / pageSize));
  const startIndex = (currentPage - 1) * pageSize + 1;
  const endIndex = Math.min(currentPage * pageSize, totalLocations);

  const handleNext = () => {
    if (currentPage < totalPages) {
      onPageChange?.(currentPage + 1);
    }
  };

  const handlePrevious = () => {
    if (currentPage > 1) {
      onPageChange?.(currentPage - 1);
    }
  };

  const handleFirstPage = () => {
    onPageChange?.(1);
  };

  const handleLastPage = () => {
    onPageChange?.(totalPages);
  };

  return (
    <div className={`hidden md:flex md:flex-col ${className}`}>
      <div className="overflow-x-auto flex-1">
        <table className="w-full">
          <thead className="sticky top-0 bg-gray-50 dark:bg-gray-700 z-20 border-b border-gray-200 dark:border-gray-600">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Type
              </th>
              <SortableHeader field="identifier" label="Identifier" />
              <SortableHeader field="name" label="Name" />
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Description
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Status
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {locations.map((location, index) => (
              <LocationCard
                key={location.id}
                location={location}
                variant="row"
                onClick={() => onLocationClick?.(location)}
                onEdit={onEdit}
                onDelete={onDelete}
                className={index % 2 === 0 ? 'bg-white dark:bg-gray-900' : 'bg-gray-50 dark:bg-gray-800/50'}
              />
            ))}
          </tbody>
        </table>
      </div>

      {totalLocations > 0 && onPageChange && onPageSizeChange && (
        <div className="sticky bottom-0 px-4 py-3 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700">
          <PaginationControls
            currentPage={currentPage}
            totalPages={totalPages}
            startIndex={startIndex}
            endIndex={endIndex}
            totalItems={totalLocations}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onNext={handleNext}
            onPrevious={handlePrevious}
            onFirstPage={handleFirstPage}
            onLastPage={handleLastPage}
            onPageSizeChange={onPageSizeChange}
          />
        </div>
      )}
    </div>
  );
}
