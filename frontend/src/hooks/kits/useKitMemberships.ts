import { useQueries } from '@tanstack/react-query';
import { kitsApi, type KitSummary } from '@/lib/api/kits';

export const KIT_MEMBERSHIP_QUERY_KEY = 'kit-membership';

/**
 * Resolve which ACTIVE kit (if any) each scanned EPC already belongs to
 * (TRA-1033). One GET /kits?member_epc= per EPC — handheld sessions are a few
 * dozen tags at most, and results are cached per EPC for the session. The
 * commission flow uses this to flag/exclude already-kitted tags instead of
 * letting the whole save die on the 409.
 *
 * Invalidate the KIT_MEMBERSHIP_QUERY_KEY scope after a successful commission
 * so freshly kitted tags show correctly on the next scan.
 */
export function useKitMemberships(epcs: string[]): Map<string, KitSummary> {
  const results = useQueries({
    queries: epcs.map((epc) => ({
      queryKey: [KIT_MEMBERSHIP_QUERY_KEY, epc],
      queryFn: async () => {
        const response = await kitsApi.listByMemberEpc(epc);
        return response.data.data.find((k) => k.status === 'active') ?? null;
      },
      staleTime: 60_000,
      retry: 1,
    })),
  });

  const memberships = new Map<string, KitSummary>();
  results.forEach((result, i) => {
    if (result.data) {
      memberships.set(epcs[i], result.data);
    }
  });
  return memberships;
}
