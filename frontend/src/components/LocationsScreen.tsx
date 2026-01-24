import { useState, useEffect, useCallback } from 'react';
import { Plus } from 'lucide-react';
import toast from 'react-hot-toast';
import { useLocations, useLocationMutations } from '@/hooks/locations';
import { useLocationStore } from '@/stores/locations/locationStore';
import { useUIStore } from '@/stores';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { FloatingActionButton, ConfirmModal } from '@/components/shared';
import {
  LocationStats,
  LocationSearchSort,
  LocationFormModal,
  LocationDetailsModal,
  LocationMoveModal,
  LocationSplitPane,
  LocationMobileView,
} from '@/components/locations';
import { ProtectedRoute } from '@/components/ProtectedRoute';
import type { Location } from '@/types/locations';

export default function LocationsScreen() {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [createParentId, setCreateParentId] = useState<number | null>(null);
  const [viewingLocation, setViewingLocation] = useState<Location | null>(null);
  const [editingLocation, setEditingLocation] = useState<Location | null>(null);
  const [deletingLocation, setDeletingLocation] = useState<Location | null>(null);
  const [movingLocation, setMovingLocation] = useState<Location | null>(null);

  // Desktop breakpoint for split pane layout
  const isDesktop = useMediaQuery('(min-width: 1024px)');

  // Fetch locations data
  useLocations();
  const { delete: deleteLocation } = useLocationMutations();
  const { setActiveTab } = useUIStore();
  const getLocationById = useLocationStore((state) => state.getLocationById);
  const cache = useLocationStore((state) => state.cache);
  const filters = useLocationStore((state) => state.filters);

  useEffect(() => {
    setActiveTab('locations');
  }, [setActiveTab]);

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

  const handleCreateClick = () => {
    setIsCreateModalOpen(true);
  };

  const handleLocationClick = (location: Location) => {
    setViewingLocation(location);
  };

  // Handlers for split pane and mobile (by ID)
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

  const handleAddChild = useCallback((parentId: number) => {
    setCreateParentId(parentId);
    setIsCreateModalOpen(true);
  }, []);

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
                onAddChild={handleAddChild}
                className="h-full"
              />
            </div>
            <LocationStats className="mt-4" />
          </>
        ) : (
          /* Mobile/Tablet: Expandable cards layout */
          <>
            <div className="flex-1 flex flex-col gap-4 overflow-hidden">
              <LocationSearchSort className="flex-shrink-0" />

              <div className="flex-1 overflow-y-auto">
                <LocationMobileView
                  searchTerm={filters.search || ''}
                  onEdit={handleEditById}
                  onMove={handleMoveById}
                  onDelete={handleDeleteById}
                  onAddChild={handleAddChild}
                />
              </div>
            </div>
            <LocationStats className="mt-4" />
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
          parentLocationId={createParentId}
          onClose={() => {
            setIsCreateModalOpen(false);
            setCreateParentId(null);
          }}
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
