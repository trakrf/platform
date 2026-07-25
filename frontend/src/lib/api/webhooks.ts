/**
 * Webhook API client (TRA-1043).
 *
 * Unwraps Axios `.data` internally so callers get the server body directly,
 * matching apiKeys.ts rather than orgs.ts.
 *
 * The list endpoint returns a collection that holds zero or one webhook — the
 * shape is a list so growing to N subscriptions later is not a breaking change,
 * but the UI treats it as "the org's webhook".
 */
import { apiClient } from './client';
import type {
  Webhook,
  WebhookTestResult,
  CreateWebhookRequest,
  UpdateWebhookRequest,
} from '@/types/webhook';

export const webhooksApi = {
  get: async (): Promise<Webhook | null> => {
    const resp = await apiClient.get<{ data: Webhook[] }>('/webhooks');
    return resp.data.data?.[0] ?? null;
  },

  /** The response carries the cleartext secret. It is not readable afterwards. */
  create: async (req: CreateWebhookRequest): Promise<Webhook> => {
    const resp = await apiClient.post<{ data: Webhook }>('/webhooks', req);
    return resp.data.data;
  },

  update: async (id: number, req: UpdateWebhookRequest): Promise<Webhook> => {
    const resp = await apiClient.patch<{ data: Webhook }>(`/webhooks/${id}`, req);
    return resp.data.data;
  },

  remove: async (id: number): Promise<void> => {
    await apiClient.delete<void>(`/webhooks/${id}`);
  },

  /**
   * Fires a synthetic asset.moved at the registered URL and reports what the
   * endpoint answered. A failed delivery still resolves — the failure is inside
   * the result, not thrown — because "your endpoint returned 502" is the answer
   * the operator is asking for.
   */
  test: async (id: number): Promise<WebhookTestResult> => {
    const resp = await apiClient.post<{ data: WebhookTestResult }>(`/webhooks/${id}/test`);
    return resp.data.data;
  },
};
