import { useMutation } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import {
  inventoryApi,
  type SaveInventoryRequest,
  type SaveInventoryResponse,
} from '@/lib/api/inventory';
import { ensureOrgContext } from '@/lib/auth/orgContext';

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
 * - On 403 (storage-path access-denied from inventory.Save) logs the backend
 *   detail and surfaces it directly via the onError toast. The backend
 *   returns an accurate, generic message naming the real failure cause
 *   (TRA-812 — the prior "not found or access denied" wording was a single
 *   blanket message covering four distinct causes, and this hook used to
 *   re-translate it into a misleading "org mismatch" toast on top of that).
 *   No auto-retry: the in-flight payload that tripped the guard would just
 *   trip it again.
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
        const axiosError = error as AxiosLikeError;
        if (axiosError.response?.status === 403) {
          console.warn('[InventorySave] 403 from inventory/save', {
            detail: extractDetail(error),
            location_identifier: data.location_identifier,
            asset_identifiers_count: data.asset_identifiers.length,
          });
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
      // Surface the backend's RFC 7807 detail directly. The backend message
      // names a real cause (e.g. "N of M assets are unavailable"); this hook
      // used to re-translate every storage-path 403 into a hard-coded "org
      // mismatch" toast, which was wrong for the duplicate, soft-deleted, and
      // nonexistent failure paths. (TRA-812)
      toast.error(extractDetail(error) || 'Failed to save inventory');
    },
  });

  return {
    save: saveMutation.mutateAsync,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  };
}
