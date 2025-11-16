import React, { useState, useMemo, useEffect } from 'react';
import { Plus, MapPin } from 'lucide-react';
import toast from 'react-hot-toast';
import { useLocations, useLocationMutations } from '@/hooks/locations';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useUIStore } from '@/stores';
import { FloatingActionButton, EmptyState, NoResults, ConfirmModal } from '@/components/shared';
import { LocationStats } from '@/components/locations/LocationStats';
import { LocationSearchSort } from '@/components/locations/LocationSearchSort';
import { LocationTable } from '@/components/locations/LocationTable';
import { LocationCard } from '@/components/locations/LocationCard';
import { LocationFormModal } from '@/components/locations/LocationFormModal';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import type { Location } from '@/types/locations';

export default function LocationsScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [editingLocation, setEditingLocation] = useState<Location | null>(null);
  const [deletingLocation, setDeletingLocation] = useState<Location | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const { isLoading } = useLocations();
  const { delete: deleteLocation } = useLocationMutations();
  const { setActiveTab } = useUIStore();

  useEffect(() => {
    setActiveTab('locations');
  }, [setActiveTab]);

  const cache = useLocationStore((state) => state.cache);
  const filters = useLocationStore((state) => state.filters);
  const sort = useLocationStore((state) => state.sort);
  const setFilters = useLocationStore((state) => state.setFilters);

  const filteredLocations = useMemo(() => {
    return useLocationStore.getState().getFilteredLocations();
  }, [cache.byId.size, filters, sort]);

  const paginatedLocations = useMemo(() => {
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    return filteredLocations.slice(startIndex, endIndex);
  }, [filteredLocations, currentPage, pageSize]);

  React.useEffect(() => {
    setCurrentPage(1);
  }, [filters, sort]);

  const hasActiveFilters =
    (filters.is_active !== 'all' && filters.is_active !== undefined) ||
    (filters.search && filters.search.trim() !== '');

  const handleEditLocation = (location: Location) => {
    setEditingLocation(location);
  };

  const handleDeleteLocation = (location: Location) => {
    setDeletingLocation(location);
  };

  const confirmDelete = async () => {
    if (deletingLocation) {
      try {
        await deleteLocation(deletingLocation.id);
        toast.success(`Location "${deletingLocation.identifier}" deleted successfully`);
        setDeletingLocation(null);
      } catch (error: any) {
        console.error('Delete error:', error);
        toast.error(error.message || 'Failed to delete location');
      }
    }
  };

  const handleClearFilters = () => {
    setFilters({ is_active: 'all', search: '' });
  };

  const handleCreateClick = () => {
    setIsCreateModalOpen(true);
  };

  const handleLocationClick = (location: Location) => {
    setEditingLocation(location);
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        <div className="flex gap-4 flex-1 overflow-hidden">
          <div className="flex-1 flex flex-col gap-4 min-w-0">
            <LocationSearchSort />

            {!isLoading && filteredLocations.length === 0 && !hasActiveFilters && (
              <EmptyState
                icon={MapPin}
                title="No locations yet"
                description="Get started by adding your first location"
                action={{
                  label: 'Create Location',
                  onClick: handleCreateClick,
                }}
              />
            )}

            {!isLoading && filteredLocations.length === 0 && hasActiveFilters && (
              <NoResults searchTerm={filters.search || ''} onClearFilters={handleClearFilters} />
            )}

            {!isLoading && filteredLocations.length > 0 && (
              <>
                <LocationTable
                  loading={isLoading}
                  locations={paginatedLocations}
                  totalLocations={filteredLocations.length}
                  currentPage={currentPage}
                  pageSize={pageSize}
                  onPageChange={setCurrentPage}
                  onPageSizeChange={setPageSize}
                  onLocationClick={handleLocationClick}
                  onEdit={handleEditLocation}
                  onDelete={handleDeleteLocation}
                />

                <div className="md:hidden space-y-3">
                  {paginatedLocations.map((location) => (
                    <LocationCard
                      key={location.id}
                      location={location}
                      variant="card"
                      onClick={() => handleLocationClick(location)}
                      onEdit={handleEditLocation}
                      onDelete={handleDeleteLocation}
                      showActions={true}
                    />
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
        <LocationStats className="mt-6" />

        <FloatingActionButton
          icon={Plus}
          onClick={handleCreateClick}
          ariaLabel="Create new location"
        />

        <LocationFormModal
          isOpen={isCreateModalOpen}
          mode="create"
          onClose={() => setIsCreateModalOpen(false)}
        />

        {editingLocation && (
          <LocationFormModal
            isOpen={true}
            mode="edit"
            location={editingLocation}
            onClose={() => setEditingLocation(null)}
          />
        )}

        <ConfirmModal
          isOpen={!!deletingLocation}
          title="Delete Location"
          message={`Are you sure you want to delete "${deletingLocation?.identifier}"? This action cannot be undone.`}
          onConfirm={confirmDelete}
          onCancel={() => setDeletingLocation(null)}
        />
      </div>
    </ProtectedRoute>
  );
}
