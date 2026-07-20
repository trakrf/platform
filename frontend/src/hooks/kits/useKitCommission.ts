import { useMutation } from '@tanstack/react-query';
import { kitsApi, type CommissionRequest, type Kit } from '@/lib/api/kits';
import { ensureOrgContext } from '@/lib/auth/orgContext';

/**
 * Commission a kit from scanned members (TRA-1033).
 *
 * Errors are NOT toasted here — the commission form surfaces them inline via
 * ErrorBanner, because the 409 detail names the kit that already owns a
 * member and the operator needs that to stay visible.
 */
export function useKitCommission() {
  const mutation = useMutation({
    mutationFn: async (request: CommissionRequest): Promise<Kit> => {
      await ensureOrgContext();
      const response = await kitsApi.commission(request);
      return response.data.data;
    },
  });

  return { commission: mutation.mutateAsync, isSaving: mutation.isPending };
}
