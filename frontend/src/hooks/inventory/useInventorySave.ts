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
 * - Verifies the JWT has a valid current_org_id before sending (TRA-332).
 * - On 403 with "not found or access denied" (storage-path 403 from
 *   inventory.Save), runs the central invalidateAllOrgScopedData (TRA-318)
 *   to drop stale caches, then refreshes the JWT and retries once. This
 *   recovers from the "JWT says org B but tagStore was enriched against
 *   org A" scenario — the usual culprit is a cross-tab org switch that
 *   updated the token in localStorage without firing central invalidation
 *   in this tab's in-memory stores.
 * - Surfaces the backend's RFC 7807 detail in toast + console on final
 *   failure so future 403s stop being opaque (TRA-426).
 */
export function useInventorySave() {
  const queryClient = useQueryClient();

  const saveMutation = useMutation({
    mutationFn: async (data: SaveInventoryRequest): Promise<SaveInventoryResponse> => {
      // Guard: verify JWT has org context before sending
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
            location_id: data.location_id,
            asset_ids_count: data.asset_ids.length,
          });

          // Storage-path 403 ("not found or access denied") means the
          // submitted IDs don't belong to the caller's current org. That
          // only happens when this tab's org-scoped caches drifted from
          // the token's current_org_id. Hand off to the single DRY sink
          // for org-context change (TRA-318) rather than re-scattering
          // clear logic into the save hook.
          if (detail && detail.includes('not found or access denied')) {
            await invalidateAllOrgScopedData(queryClient);
          }

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
