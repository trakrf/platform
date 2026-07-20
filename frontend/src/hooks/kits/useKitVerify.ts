import { useMutation } from '@tanstack/react-query';
import { kitsApi, type VerifyRequest, type VerifyResponse } from '@/lib/api/kits';
import { ensureOrgContext } from '@/lib/auth/orgContext';

/**
 * Dock-check verify of a scan session's EPCs (TRA-1033). The response is the
 * frozen top-level shape (no {data} envelope). Persistence of the audit row
 * happens server-side on this call — there is no separate save.
 */
export function useKitVerify() {
  const mutation = useMutation({
    mutationFn: async (request: VerifyRequest): Promise<VerifyResponse> => {
      await ensureOrgContext();
      const response = await kitsApi.verify(request);
      return response.data;
    },
  });

  return { verify: mutation.mutateAsync, isVerifying: mutation.isPending };
}
