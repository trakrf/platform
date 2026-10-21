import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { notificationsApi } from '@/lib/api/notifications';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  NotificationChannel,
  NotificationSubscription,
  UpdateNotificationSubscriptionRequest,
} from '@/types/notifications';

export interface UseAssetSubscriptionsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

export interface SubscribeResult {
  succeeded: NotificationSubscription[];
  failed: { channel: NotificationChannel; error: unknown }[];
}

/**
 * Fetch every subscription on one asset, active or switched off. Disabled
 * until an asset id is known.
 */
export function useAssetSubscriptions(
  assetId: number | null | undefined,
  options: UseAssetSubscriptionsOptions = {}
) {
  const { enabled = true, refetchOnMount = true } = options;

  const query = useQuery({
    queryKey: ['assetSubscriptions', assetId],
    queryFn: async () => {
      const response = await notificationsApi.listAssetSubscriptions(assetId as number);
      return response.data.data;
    },
    enabled: enabled && assetId != null,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    subscriptions: query.data ?? [],
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}

export function useAssetSubscriptionMutations(assetId: number) {
  const queryClient = useQueryClient();

  // One POST per channel, in order. A failed channel is reported rather than
  // thrown so the caller can retry just that one. POST is idempotent: an
  // existing (asset, contact, channel) is switched back on and returned.
  const subscribeMutation = useMutation({
    mutationFn: async ({
      recipientId,
      channels,
    }: {
      recipientId: number;
      channels: NotificationChannel[];
    }): Promise<SubscribeResult> => {
      await ensureOrgContext();
      const result: SubscribeResult = { succeeded: [], failed: [] };
      for (const channel of channels) {
        try {
          const response = await notificationsApi.subscribe(assetId, {
            recipient_id: recipientId,
            channel,
          });
          result.succeeded.push(response.data.data);
        } catch (error) {
          result.failed.push({ channel, error });
        }
      }
      return result;
    },
    onSuccess: (result) => {
      if (result.succeeded.length === 0) return;
      queryClient.invalidateQueries({ queryKey: ['assetSubscriptions', assetId] });
      queryClient.invalidateQueries({ queryKey: ['notificationRecipients'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      ...updates
    }: { id: number } & UpdateNotificationSubscriptionRequest) => {
      await ensureOrgContext();
      const response = await notificationsApi.updateSubscription(assetId, id, updates);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['assetSubscriptions', assetId] });
    },
  });

  return {
    subscribe: subscribeMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    isSubscribing: subscribeMutation.isPending,
    isUpdating: updateMutation.isPending,
    subscribeError: subscribeMutation.error,
    updateError: updateMutation.error,
  };
}
