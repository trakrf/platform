import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { notificationsApi } from '@/lib/api/notifications';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type {
  CreateNotificationRecipientRequest,
  UpdateNotificationRecipientRequest,
} from '@/types/notifications';

export interface UseNotificationRecipientsOptions {
  enabled?: boolean;
  refetchOnMount?: boolean;
}

/**
 * Fetch all notification recipients (contacts) for the current org. The list
 * endpoint is not paginated and excludes deleted contacts.
 */
export function useNotificationRecipients(options: UseNotificationRecipientsOptions = {}) {
  const { enabled = true, refetchOnMount = true } = options;
  const currentOrg = useOrgStore((state) => state.currentOrg);

  const query = useQuery({
    queryKey: ['notificationRecipients', currentOrg?.id],
    queryFn: async () => {
      const response = await notificationsApi.listRecipients();
      return response.data.data;
    },
    enabled,
    refetchOnMount,
    staleTime: 60 * 60 * 1000,
  });

  return {
    recipients: query.data ?? [],
    isLoading: query.isLoading,
    isRefetching: query.isRefetching,
    error: query.error,
    refetch: query.refetch,
  };
}

export function useNotificationRecipientMutations() {
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: CreateNotificationRecipientRequest) => {
      await ensureOrgContext();
      const response = await notificationsApi.createRecipient(data);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notificationRecipients'] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({
      id,
      updates,
    }: {
      id: number;
      updates: UpdateNotificationRecipientRequest;
    }) => {
      await ensureOrgContext();
      const response = await notificationsApi.updateRecipient(id, updates);
      return response.data.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notificationRecipients'] });
      queryClient.invalidateQueries({ queryKey: ['assetSubscriptions'] });
    },
  });

  // Deleting a contact switches all its subscriptions off server-side, so
  // every asset's subscription list is stale afterwards.
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await ensureOrgContext();
      await notificationsApi.deleteRecipient(id);
      return id;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notificationRecipients'] });
      queryClient.invalidateQueries({ queryKey: ['assetSubscriptions'] });
    },
  });

  return {
    create: createMutation.mutateAsync,
    update: updateMutation.mutateAsync,
    delete: deleteMutation.mutateAsync,
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,
  };
}
