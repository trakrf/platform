import React, { useState, useMemo, useEffect, useCallback } from 'react';
import { Plus, MapPin, List, Network } from 'lucide-react';
import toast from 'react-hot-toast';
import { useLocations, useLocationMutations } from '@/hooks/locations';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useUIStore } from '@/stores';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { FloatingActionButton, EmptyState, NoResults, ConfirmModal } from '@/components/shared';
import {
  LocationStats,
  LocationSearchSort,
  LocationTable,
  LocationCard,
  LocationFormModal,
  LocationTreeView,
  LocationDetailsModal,
  LocationMoveModal,
  LocationSplitPane,
} from '@/components/locations';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import type { Location } from '@/types/locations';

type ViewMode = 'list' | 'tree';

export default function LocationsScreen() {
  const [viewMode, setViewMode] = useState<ViewMode>('tree');
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [viewingLocation, setViewingLocation] = useState<Location | null>(null);
  const [editingLocation, setEditingLocation] = useState<Location | null>(null);
  const [deletingLocation, setDeletingLocation] = useState<Location | null>(null);
  const [movingLocation, setMovingLocation] = useState<Location | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // Desktop breakpoint for split pane layout
  const isDesktop = useMediaQuery('(min-width: 1024px)');

  const { isLoading } = useLocations();
  const { delete: deleteLocation } = useLocationMutations();
  const { setActiveTab } = useUIStore();
  const getLocationById = useLocationStore((state) => state.getLocationById);

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
    setViewingLocation(location);
  };

  // Handlers for split pane (by ID)
  const handleEditById = useCallback(
    (id: number) => {
      const location = getLocationById(id);
      if (location) setEditingLocation(location);
    },
    [getLocationById]
  );

  const handleDeleteById = useCallback(
    (id: number) => {
      const location = getLocationById(id);
      if (location) setDeletingLocation(location);
    },
    [getLocationById]
  );

  const handleMoveById = useCallback(
    (id: number) => {
      const location = getLocationById(id);
      if (location) setMovingLocation(location);
    },
    [getLocationById]
  );

  const handleMoveLocation = async (locationId: number, newParentId: number | null) => {
    const location = cache.byId.get(locationId);
    if (!location) return;

    try {
      await useLocationStore.getState().updateLocation(locationId, {
        parent_location_id: newParentId,
      });
      toast.success(`Location "${location.identifier}" moved successfully`);
    } catch (error: any) {
      console.error('Move error:', error);
      throw new Error(error.message || 'Failed to move location');
    }
  };

  return (
    <ProtectedRoute>
      <div className="h-full flex flex-col p-2">
        {isDesktop ? (
          /* Desktop: Split pane layout */
          <>
            <div className="flex items-center gap-4 mb-4">
              <LocationSearchSort className="flex-1" />
            </div>
            <div className="flex-1 overflow-hidden border border-gray-200 dark:border-gray-700 rounded-lg">
              <LocationSplitPane
                searchTerm={filters.search || ''}
                onEdit={handleEditById}
                onMove={handleMoveById}
                onDelete={handleDeleteById}
                className="h-full"
              />
            </div>
            <LocationStats className="mt-4" />
          </>
        ) : (
          /* Mobile/Tablet: Original layout with view mode toggle */
          <>
            <div className="flex gap-4 flex-1 overflow-hidden">
              <div className="flex-1 flex flex-col gap-4 min-w-0">
                <div className="flex items-center justify-between gap-4">
                  <LocationSearchSort className="flex-1" />

                  <div className="flex gap-2 bg-gray-100 dark:bg-gray-800 p-1 rounded-lg">
                    <button
                      onClick={() => setViewMode('list')}
                      className={`flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                        viewMode === 'list'
                          ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                          : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
                      }`}
                    >
                      <List className="h-4 w-4" />
                      List
                    </button>
                    <button
                      onClick={() => setViewMode('tree')}
                      className={`flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                        viewMode === 'tree'
                          ? 'bg-white dark:bg-gray-700 text-blue-600 dark:text-blue-400 shadow-sm'
                          : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200'
                      }`}
                    >
                      <Network className="h-4 w-4" />
                      Tree
                    </button>
                  </div>
                </div>

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
                    {viewMode === 'list' ? (
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
                    ) : (
                      <div className="flex-1 overflow-y-auto bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded-lg p-4">
                        <LocationTreeView
                          onLocationClick={handleLocationClick}
                          onEdit={handleEditLocation}
                          onDelete={handleDeleteLocation}
                          selectedLocationId={viewingLocation?.id}
                        />
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
            <LocationStats className="mt-6" />
          </>
        )}

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

        {viewingLocation && (
          <LocationDetailsModal
            isOpen={true}
            location={viewingLocation}
            onClose={() => setViewingLocation(null)}
            onEdit={(location) => {
              setViewingLocation(null);
              setEditingLocation(location);
            }}
            onDelete={(location) => {
              setViewingLocation(null);
              setDeletingLocation(location);
            }}
            onMove={(location) => {
              setViewingLocation(null);
              setMovingLocation(location);
            }}
            onLocationClick={handleLocationClick}
          />
        )}

        {editingLocation && (
          <LocationFormModal
            isOpen={true}
            mode="edit"
            location={editingLocation}
            onClose={() => setEditingLocation(null)}
          />
        )}

        {movingLocation && (
          <LocationMoveModal
            isOpen={true}
            location={movingLocation}
            onClose={() => setMovingLocation(null)}
            onConfirm={handleMoveLocation}
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
