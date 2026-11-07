/**
 * Notifications API Client
 *
 * Type-safe wrapper around the session-authenticated notification recipient
 * and asset subscription endpoints. Uses the shared apiClient with automatic
 * JWT injection and org_id context. Errors propagate unchanged — callers
 * extract RFC 7807. Alert history has no endpoint yet, so no call here.
 */

import { apiClient } from '../client';
import type {
  NotificationRecipientResponse,
  ListNotificationRecipientsResponse,
  CreateNotificationRecipientRequest,
  UpdateNotificationRecipientRequest,
  NotificationSubscriptionResponse,
  ListNotificationSubscriptionsResponse,
  CreateNotificationSubscriptionRequest,
  UpdateNotificationSubscriptionRequest,
} from '@/types/notifications';

/**
 * Notifications API methods.
 */
export const notificationsApi = {
  /**
   * List all notification recipients for the current organization.
   * GET /api/v1/notification-recipients
   */
  listRecipients: () =>
    apiClient.get<ListNotificationRecipientsResponse>('/notification-recipients'),

  /**
   * Get a single recipient by ID.
   * GET /api/v1/notification-recipients/:id
   */
  getRecipient: (id: number) =>
    apiClient.get<NotificationRecipientResponse>(`/notification-recipients/${id}`),

  /**
   * Create a recipient. Needs an email or a phone.
   * POST /api/v1/notification-recipients
   */
  createRecipient: (data: CreateNotificationRecipientRequest) =>
    apiClient.post<NotificationRecipientResponse>('/notification-recipients', data),

  /**
   * Update a recipient by ID (application/json PATCH).
   * PATCH /api/v1/notification-recipients/:id
   */
  updateRecipient: (id: number, data: UpdateNotificationRecipientRequest) =>
    apiClient.patch<NotificationRecipientResponse>(`/notification-recipients/${id}`, data, {
      headers: { 'Content-Type': 'application/json' },
    }),

  /**
   * Delete a recipient by ID. The server switches off its subscriptions.
   * DELETE /api/v1/notification-recipients/:id
   */
  deleteRecipient: (id: number) => apiClient.delete<void>(`/notification-recipients/${id}`),

  /**
   * List every subscription on an asset, active or switched off.
   * GET /api/v1/assets/:assetId/notification-subscriptions
   */
  listAssetSubscriptions: (assetId: number) =>
    apiClient.get<ListNotificationSubscriptionsResponse>(
      `/assets/${assetId}/notification-subscriptions`
    ),

  /**
   * Subscribe a recipient to an asset. Idempotent: an existing subscription is
   * switched back on and returned.
   * POST /api/v1/assets/:assetId/notification-subscriptions
   */
  subscribe: (assetId: number, data: CreateNotificationSubscriptionRequest) =>
    apiClient.post<NotificationSubscriptionResponse>(
      `/assets/${assetId}/notification-subscriptions`,
      data
    ),

  /**
   * Change a subscription's channel or switch it off/on (application/json PATCH).
   * PATCH /api/v1/assets/:assetId/notification-subscriptions/:id
   */
  updateSubscription: (
    assetId: number,
    subscriptionId: number,
    data: UpdateNotificationSubscriptionRequest
  ) =>
    apiClient.patch<NotificationSubscriptionResponse>(
      `/assets/${assetId}/notification-subscriptions/${subscriptionId}`,
      data,
      { headers: { 'Content-Type': 'application/json' } }
    ),
};
