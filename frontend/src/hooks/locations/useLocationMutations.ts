import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useLocationStore } from '@/stores/locations/locationStore';
import { locationsApi } from '@/lib/api/locations';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  Location,
  CreateLocationRequest,
  UpdateLocationRequest,
} from '@/types/locations';

/** Normalize raw public API location to internal shape. Resolves parent_location_id from store cache. */
function normalizeLocation(raw: Location): Location {
  const byIdentifier = useLocationStore.getState().cache?.byIdentifier;
  const parentId = raw.parent
    ? (byIdentifier?.get(raw.parent)?.id ?? null)
    : null;
  return { ...raw, id: raw.surrogate_id, parent_location_id: parentId };
}

export function useLocationMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateLocationRequest) => {
      await ensureOrgContext();
      const response = await locationsApi.create(data);
      return normalizeLocation(response.data.data);
    },
    onSuccess: (location) => {
      useLocationStore.getState().addLocation(location);
      queryClient.invalidateQueries({ queryKey: ['locations'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateLocationRequest;
    }) => {
      await ensureOrgContext();
      const response = await locationsApi.update(id, updates);
      return normalizeLocation(response.data.data);
    },
    onSuccess: (location) => {
      useLocationStore.getState().updateLocation(location.id, location);
      queryClient.invalidateQueries({ queryKey: ['locations'] });
      queryClient.invalidateQueries({ queryKey: ['location', location.id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await locationsApi.delete(id);
      return id;
    },
    onSuccess: (id) => {
      useLocationStore.getState().deleteLocation(id);
      queryClient.invalidateQueries({ queryKey: ['locations'] });
      queryClient.invalidateQueries({ queryKey: ['location', id] });
    },
  });

  const moveMutation = useMutation({
    mutationFn: async ({
      id,
      newParentId,
    }: {
      id: number;
      newParentId: number | null;
    }) => {
      await ensureOrgContext();
      const response = await locationsApi.move(id, { new_parent_id: newParentId });
      return normalizeLocation(response.data.data);
    },
    onSuccess: (location) => {
      useLocationStore.getState().updateLocation(location.id, location);
      queryClient.invalidateQueries({ queryKey: ['locations'] });
      queryClient.invalidateQueries({ queryKey: ['location', location.id] });
    },
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    move: moveMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    isMoving: moveMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
    moveError: moveMutation.error,
  };
}
