import { useMemo } from 'react';
import { MapPin } from 'lucide-react';
import { useLocationStore } from '@/stores/locations/locationStore';
import { DataTable, Column } from '@/components/shared/DataTable';
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

const columns: Column<Location>[] = [
  { key: 'type', label: 'Type', sortable: false },
  { key: 'identifier', label: 'Identifier', sortable: true },
  { key: 'name', label: 'Name', sortable: true },
  { key: 'description', label: 'Description', sortable: false },
  { key: 'status', label: 'Status', sortable: false },
  { key: 'actions', label: 'Actions', sortable: false },
];

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

  const handleSort = (field: string, direction: 'asc' | 'desc') => {
    setSort({ field: field as any, direction });
  };

  return (
    <DataTable
      data={locations}
      columns={columns}
      loading={loading}
      totalItems={totalLocations}
      currentPage={currentPage}
      pageSize={pageSize}
      sortField={sortField}
      sortDirection={sortDirection}
      onSort={handleSort}
      onPageChange={onPageChange}
      onPageSizeChange={onPageSizeChange}
      renderRow={(location, _index, rowProps) => (
        <LocationCard
          key={location.id}
          location={location}
          variant="row"
          onClick={() => onLocationClick?.(location)}
          onEdit={onEdit}
          onDelete={onDelete}
          className={rowProps.className}
        />
      )}
      emptyStateIcon={MapPin}
      emptyStateTitle="No Locations Found"
      emptyStateDescription="Try adjusting your filters or create a new location to get started."
      className={className}
    />
  );
}
