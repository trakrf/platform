import { useQueries, type UseQueryResult } from '@tanstack/react-query';
import { useAssets } from '@/hooks/assets';
import { notificationsApi } from '@/lib/api/notifications';
import type { NotificationSubscription } from '@/types/notifications';

// Module-level so useQueries only re-runs it when a query result changes.
function combineCounts(results: UseQueryResult<NotificationSubscription[]>[]) {
  const assetsByRecipient = new Map<number, Set<number>>();
  for (const result of results) {
    for (const sub of result.data ?? []) {
      if (!sub.is_active) continue;
      const assetIds = assetsByRecipient.get(sub.recipient_id) ?? new Set<number>();
      assetIds.add(sub.asset_id);
      assetsByRecipient.set(sub.recipient_id, assetIds);
    }
  }
  const counts = new Map<number, number>();
  assetsByRecipient.forEach((assetIds, recipientId) => counts.set(recipientId, assetIds.size));
  return { counts, isLoading: results.some((r) => r.isLoading) };
}

/**
 * Count, per contact, the assets it has at least one active subscription on.
 * A contact subscribed to one asset by both email and SMS counts once.
 *
 * One request per asset until the backend returns the count directly. The
 * query key and payload match useAssetSubscriptions, so the asset modal and
 * this hook share cache entries.
 */
export function useRecipientSubscriptionCounts() {
  const { assets, isLoading: assetsLoading } = useAssets();

  const { counts, isLoading } = useQueries({
    queries: assets.map((asset) => ({
      queryKey: ['assetSubscriptions', asset.id],
      queryFn: async () => {
        const response = await notificationsApi.listAssetSubscriptions(asset.id);
        return response.data.data;
      },
      staleTime: 5 * 60 * 1000,
    })),
    combine: combineCounts,
  });

  return { counts, isLoading: assetsLoading || isLoading };
}
