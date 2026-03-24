import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';
import { ensureOrgContext, refreshOrgToken } from '@/lib/auth/orgContext';

/**
 * Hook for saving scanned inventory to the database.
 *
 * Includes a JWT org-context guard that:
 * 1. Checks the token has a valid current_org_id before sending
 * 2. On 403, attempts one token refresh + retry
 */
export function useInventorySave() {
  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      // Guard: verify JWT has org context before sending
      await ensureOrgContext();

      try {
        const response = await inventoryApi.save(data);
        return response.data.data;
      } catch (error: unknown) {
        // On 403, attempt one token refresh + retry
        const axiosError = error as { response?: { status?: number } };
        if (axiosError.response?.status === 403) {
          console.warn('[InventorySave] Got 403, attempting token refresh and retry');
          const refreshed = await refreshOrgToken();
          if (refreshed) {
            const retryResponse = await inventoryApi.save(data);
            return retryResponse.data.data;
          }
        }
        throw error;
      }
    },
    onSuccess: (result) => {
      toast.success(`${result.count} assets saved to ${result.location_name}`);
    },
    onError: (error: Error) => {
      if (error.message.includes('No organization context')) {
        toast.error(error.message);
      } else {
        toast.error('Failed to save inventory');
      }
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
