import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useAssetStore } from '@/stores/assets/assetStore';
import { assetsApi } from '@/lib/api/assets';
import type { CreateAssetRequest, UpdateAssetRequest } from '@/types/assets';

export function useAssetMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateAssetRequest) => {
      const response = await assetsApi.create(data);
      return response.data.data;
    },
    onSuccess: (asset) => {
      useAssetStore.getState().addAsset(asset);
      queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateAssetRequest;
    }) => {
      const response = await assetsApi.update(id, updates);
      return response.data.data;
    },
    onSuccess: (asset) => {
      useAssetStore.getState().updateCachedAsset(asset.id, asset);
      queryClient.invalidateQueries({ queryKey: ['assets'] });
      queryClient.invalidateQueries({ queryKey: ['asset', asset.id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await assetsApi.delete(id);
      return id;
    },
    onSuccess: (id) => {
      useAssetStore.getState().removeAsset(id);
      queryClient.invalidateQueries({ queryKey: ['assets'] });
      queryClient.invalidateQueries({ queryKey: ['asset', id] });
    },
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
  };
}
