/**
 * Webhook subscription types (TRA-1043).
 *
 * One webhook per organization, one event type (`asset.moved`), fired only when
 * an asset's location actually changes. Delivery is at-most-once.
 */

export interface Webhook {
  id: number;
  org_id: number;
  url: string;
  /**
   * Cleartext ONLY in the create response — every later response returns a
   * masked form (`whsec_…abcd`). There is no way to read it back, and rotation
   * is not implemented yet, so a lost secret means delete and re-create.
   */
  secret: string;
  enabled: boolean;
  created_at: string;
  updated_at?: string | null;
}

export interface WebhookTestResult {
  /** 0 when the request never completed (blocked target, DNS failure, timeout). */
  status_code: number;
  error?: string;
}

export interface CreateWebhookRequest {
  url: string;
  enabled?: boolean;
}

export interface UpdateWebhookRequest {
  url?: string;
  enabled?: boolean;
}
