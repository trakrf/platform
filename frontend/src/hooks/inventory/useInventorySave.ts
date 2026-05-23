import { useMutation, useQueryClient } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';
import { ensureOrgContext, refreshOrgToken } from '@/lib/auth/orgContext';
import { invalidateAllOrgScopedData } from '@/lib/cache/orgScopedCache';

type RFC7807 = { detail?: string; title?: string };
type AxiosLikeError = {
  response?: { status?: number; data?: { error?: RFC7807 } | RFC7807 };
};

function extractDetail(error: unknown): string | null {
  const axiosError = error as AxiosLikeError;
  const data = axiosError.response?.data;
  if (!data) return null;
  const problem: RFC7807 | undefined =
    (data as { error?: RFC7807 }).error ?? (data as RFC7807);
  return problem?.detail?.trim() || problem?.title?.trim() || null;
}

/**
 * Hook for saving scanned inventory to the database.
 *
 * - Verifies the JWT has a valid current_org_id matching the profile's
 *   current_org before sending. ensureOrgContext realigns the token if
 *   they have drifted (TRA-332, TRA-812).
 * - On 403 with "not found or access denied" the submitted identifiers
 *   genuinely don't belong to the current org. Runs the central
 *   invalidateAllOrgScopedData (TRA-318) to drop the stale tag buffer
 *   and asset/location caches, refreshes the JWT, then surfaces the
 *   "clear and rescan" toast (TRA-426). No auto-retry: the in-flight
 *   payload was assembled from the same stale caches we just cleared,
 *   so resubmitting it would just trip the same guard. Once the caller
 *   rescans against the realigned token the next save will land.
 */
export function useInventorySave() {
  const queryClient = useQueryClient();

  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      // Guard: verify JWT has org context AND that it matches the profile's
      // current_org. Refreshes the token if they have drifted (TRA-812).
      await ensureOrgContext();

      try {
        const response = await inventoryApi.save(data);
        return response.data.data;
      } catch (error: unknown) {
        const axiosError = error as AxiosLikeError;
        if (axiosError.response?.status === 403) {
          const detail = extractDetail(error);
          console.warn('[InventorySave] 403 from inventory/save', {
            detail,
            location_identifier: data.location_identifier,
            asset_identifiers_count: data.asset_identifiers.length,
          });

          if (detail && detail.includes('not found or access denied')) {
            await invalidateAllOrgScopedData(queryClient);
            await refreshOrgToken();
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
        return;
      }
      const detail = extractDetail(error);
      if (detail && detail.includes('not found or access denied')) {
        toast.error('Some scans no longer match your current organization. Please clear and rescan.');
        return;
      }
      toast.error(detail || 'Failed to save inventory');
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
