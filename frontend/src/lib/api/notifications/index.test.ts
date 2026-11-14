import { describe, it, expect, vi, beforeEach } from 'vitest';
import { notificationsApi } from './index';
import { apiClient } from '../client';

vi.mock('../client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
    patch: vi.fn(),
    delete: vi.fn(),
  },
}));

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

describe('notificationsApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(apiClient.get).mockResolvedValue({ data: { data: [] } } as never);
    vi.mocked(apiClient.post).mockResolvedValue({
      data: { data: { id: 1 } },
    } as never);
    vi.mocked(apiClient.patch).mockResolvedValue({
      data: { data: { id: 1 } },
    } as never);
    vi.mocked(apiClient.delete).mockResolvedValue({ data: undefined } as never);
  });

  describe('recipients', () => {
    it('lists recipients without query params', async () => {
      await notificationsApi.listRecipients();
      expect(apiClient.get).toHaveBeenCalledWith('/notification-recipients');
    });

    it('gets one recipient by id', async () => {
      await notificationsApi.getRecipient(5);
      expect(apiClient.get).toHaveBeenCalledWith('/notification-recipients/5');
    });

    it('posts the create body through', async () => {
      const body = {
        name: 'Ana',
        email: 'ana@example.com',
        phone: '+256700000000',
      };
      await notificationsApi.createRecipient(body);
      expect(apiClient.post).toHaveBeenCalledWith('/notification-recipients', body);
    });

    it('patches with an explicit JSON content type', async () => {
      await notificationsApi.updateRecipient(5, { is_active: false });
      expect(apiClient.patch).toHaveBeenCalledWith(
        '/notification-recipients/5',
        { is_active: false },
        JSON_HEADERS
      );
    });

    it('deletes by id', async () => {
      await notificationsApi.deleteRecipient(5);
      expect(apiClient.delete).toHaveBeenCalledWith('/notification-recipients/5');
    });
  });

  describe('asset subscriptions', () => {
    it('lists subscriptions under the asset', async () => {
      await notificationsApi.listAssetSubscriptions(12);
      expect(apiClient.get).toHaveBeenCalledWith('/assets/12/notification-subscriptions');
    });

    it('posts the subscribe body under the asset', async () => {
      await notificationsApi.subscribe(12, { recipient_id: 5, channel: 'sms' });
      expect(apiClient.post).toHaveBeenCalledWith('/assets/12/notification-subscriptions', {
        recipient_id: 5,
        channel: 'sms',
      });
    });

    it('patches one subscription with an explicit JSON content type', async () => {
      await notificationsApi.updateSubscription(12, 34, {
        channel: 'email',
        is_active: true,
      });
      expect(apiClient.patch).toHaveBeenCalledWith(
        '/assets/12/notification-subscriptions/34',
        { channel: 'email', is_active: true },
        JSON_HEADERS
      );
    });
  });

  it('returns the axios response unchanged', async () => {
    const response = { data: { data: [{ id: 1, name: 'Ana' }] } };
    vi.mocked(apiClient.get).mockResolvedValue(response as never);
    await expect(notificationsApi.listRecipients()).resolves.toBe(response);
  });
});
