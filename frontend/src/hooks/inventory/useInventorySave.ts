import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';

/**
 * Hook for saving scanned inventory to the database.
 *
 * Follows the pattern from useLocationMutations.ts.
 * Provides mutation function, loading state, and error state.
 * Shows toast notifications on success/error.
 */
export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      const response = await inventoryApi.save(data);
      return response.data.data;
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
    },
    onError: () => {
      toast.error('Failed to save inventory');
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
