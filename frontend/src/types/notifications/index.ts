/**
 * Notification types: recipients (contacts), their per-asset subscriptions,
 * and the proposed alert-history contract.
 *
 * Recipient and subscription shapes mirror the backend JSON exactly. Optional
 * fields are omitted by the server when unset, never sent as null.
 */

/** Delivery channel for a subscription. */
export type NotificationChannel = 'email' | 'sms';

/** A contact that can receive asset notifications. Has an email, a phone, or both. */
export interface NotificationRecipient {
  id: number;
  org_id: number;
  name: string;
  email?: string;
  phone?: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
}

export interface CreateNotificationRecipientRequest {
  name: string;
  email?: string;
  phone?: string;
  is_active?: boolean;
}

/**
 * Partial update: a set field replaces the stored value. Fields cannot be
 * cleared, so a recipient never loses its last contact method.
 */
export interface UpdateNotificationRecipientRequest {
  name?: string;
  email?: string;
  phone?: string;
  is_active?: boolean;
}

/** Links one recipient to one asset on one channel. Never deleted, only switched off. */
export interface NotificationSubscription {
  id: number;
  org_id: number;
  asset_id: number;
  recipient_id: number;
  channel: NotificationChannel;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

/**
 * Subscribing is idempotent: an existing subscription is switched back on and
 * returned. Channel defaults to email on the server when omitted.
 */
export interface CreateNotificationSubscriptionRequest {
  recipient_id: number;
  channel?: NotificationChannel;
}

/** Change the channel or switch a subscription off (is_active: false) or back on. */
export interface UpdateNotificationSubscriptionRequest {
  channel?: NotificationChannel;
  is_active?: boolean;
}

/** Single-item envelope. */
export interface NotificationRecipientResponse {
  data: NotificationRecipient;
}

/** List envelope. Not paginated: the server returns the whole collection. */
export interface ListNotificationRecipientsResponse {
  data: NotificationRecipient[];
}

export interface NotificationSubscriptionResponse {
  data: NotificationSubscription;
}

/** List envelope. Not paginated: every subscription on the asset, active or not. */
export interface ListNotificationSubscriptionsResponse {
  data: NotificationSubscription[];
}

/*
 * Alert history — a proposed contract. No endpoint serves it yet; the UI runs
 * against generated data until the backend implements this exact shape.
 */

export type NotificationAlertType = 'alarm' | 'location_change';

/** One notification sent to one recipient about one asset event. */
export interface NotificationAlert {
  id: number;
  asset_id: number;
  asset_name: string;
  asset_external_key: string;
  type: NotificationAlertType;
  /** When the event happened. */
  occurred_at: string;
  /** When the notification was sent. */
  sent_at: string;
  recipient_id: number;
  recipient_name: string;
  channel: NotificationChannel;
  /** Email only; null for SMS. */
  subject: string | null;
  /** The message as sent. */
  body: string;
  /** Event specifics, e.g. { location: 'Gate A', direction: 'unknown' }. */
  details: Record<string, string>;
}

export interface NotificationAlertFilters {
  asset_id?: number;
  type?: NotificationAlertType;
  recipient_id?: number;
  channel?: NotificationChannel;
  from?: string;
  to?: string;
  page?: number;
  per_page?: number;
}

export interface ListNotificationAlertsResponse {
  data: NotificationAlert[];
  pagination: { total: number; page: number; per_page: number };
}
