import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useAssetSubscriptions, useAssetSubscriptionMutations } from './useAssetSubscriptions';
import { notificationsApi } from '@/lib/api/notifications';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type { NotificationChannel, NotificationSubscription } from '@/types/notifications';

vi.mock('@/lib/api/notifications');
vi.mock('@/lib/auth/orgContext');

const subscription = (
  channel: NotificationChannel,
  overrides: Partial<NotificationSubscription> = {}
): NotificationSubscription => ({
  id: channel === 'email' ? 11 : 12,
  org_id: 7,
  asset_id: 5,
  recipient_id: 3,
  channel,
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
});

const createClient = () =>
  new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

const wrapperFor = (queryClient: QueryClient) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };

describe('useAssetSubscriptions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('fetches subscriptions under the asset key', async () => {
    const rows = [subscription('email'), subscription('sms', { is_active: false })];
    vi.mocked(notificationsApi.listAssetSubscriptions).mockResolvedValue({
      data: { data: rows },
    } as any);
    const queryClient = createClient();

    const { result } = renderHook(() => useAssetSubscriptions(5), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(notificationsApi.listAssetSubscriptions).toHaveBeenCalledWith(5);
    expect(result.current.subscriptions).toEqual(rows);
    expect(queryClient.getQueryData(['assetSubscriptions', 5])).toEqual(rows);
  });

  it.each([null, undefined])('does not fetch without an asset id (%s)', (assetId) => {
    const { result } = renderHook(() => useAssetSubscriptions(assetId), {
      wrapper: wrapperFor(createClient()),
    });

    expect(notificationsApi.listAssetSubscriptions).not.toHaveBeenCalled();
    expect(result.current.subscriptions).toEqual([]);
  });

  it('does not fetch when disabled', () => {
    renderHook(() => useAssetSubscriptions(5, { enabled: false }), {
      wrapper: wrapperFor(createClient()),
    });

    expect(notificationsApi.listAssetSubscriptions).not.toHaveBeenCalled();
  });
});

describe('useAssetSubscriptionMutations', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ensureOrgContext).mockResolvedValue(7);
    queryClient = createClient();
    vi.spyOn(queryClient, 'invalidateQueries');
  });

  const render = () =>
    renderHook(() => useAssetSubscriptionMutations(5), { wrapper: wrapperFor(queryClient) });

  it('posts one subscription per channel, in order', async () => {
    vi.mocked(notificationsApi.subscribe).mockImplementation(async (_assetId, body) => {
      return { data: { data: subscription(body.channel as NotificationChannel) } } as any;
    });
    const { result } = render();

    const outcome = await result.current.subscribe({ recipientId: 3, channels: ['email', 'sms'] });

    expect(ensureOrgContext).toHaveBeenCalled();
    expect(vi.mocked(notificationsApi.subscribe).mock.calls).toEqual([
      [5, { recipient_id: 3, channel: 'email' }],
      [5, { recipient_id: 3, channel: 'sms' }],
    ]);
    expect(outcome.succeeded.map((s) => s.channel)).toEqual(['email', 'sms']);
    expect(outcome.failed).toEqual([]);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions', 5],
    });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['notificationRecipients'],
    });
  });

  it('waits for each channel before posting the next', async () => {
    const order: string[] = [];
    vi.mocked(notificationsApi.subscribe).mockImplementation(async (_assetId, body) => {
      order.push(`start:${body.channel}`);
      await new Promise((resolve) => setTimeout(resolve, 5));
      order.push(`end:${body.channel}`);
      return { data: { data: subscription(body.channel as NotificationChannel) } } as any;
    });
    const { result } = render();

    await result.current.subscribe({ recipientId: 3, channels: ['email', 'sms'] });

    expect(order).toEqual(['start:email', 'end:email', 'start:sms', 'end:sms']);
  });

  it('reports a failed channel without throwing and still posts the rest', async () => {
    const error = new Error('sms needs a phone');
    vi.mocked(notificationsApi.subscribe)
      .mockRejectedValueOnce(error)
      .mockResolvedValueOnce({ data: { data: subscription('email') } } as any);
    const { result } = render();

    const outcome = await result.current.subscribe({ recipientId: 3, channels: ['sms', 'email'] });

    expect(notificationsApi.subscribe).toHaveBeenCalledTimes(2);
    expect(outcome.succeeded.map((s) => s.channel)).toEqual(['email']);
    expect(outcome.failed).toEqual([{ channel: 'sms', error }]);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions', 5],
    });
  });

  it('does not invalidate when every channel failed', async () => {
    vi.mocked(notificationsApi.subscribe).mockRejectedValue(new Error('nope'));
    const { result } = render();

    const outcome = await result.current.subscribe({ recipientId: 3, channels: ['email', 'sms'] });

    expect(outcome.succeeded).toEqual([]);
    expect(outcome.failed.map((f) => f.channel)).toEqual(['email', 'sms']);
    expect(queryClient.invalidateQueries).not.toHaveBeenCalled();
  });

  it.each([
    ['switch off', { is_active: false }],
    ['turn back on', { is_active: true }],
    ['change channel', { channel: 'sms' as const }],
  ])('update sends only the given fields (%s)', async (_label, body) => {
    vi.mocked(notificationsApi.updateSubscription).mockResolvedValue({
      data: { data: subscription('email', body) },
    } as any);
    const { result } = render();

    await result.current.update({ id: 11, ...body });

    expect(ensureOrgContext).toHaveBeenCalled();
    expect(notificationsApi.updateSubscription).toHaveBeenCalledWith(5, 11, body);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions', 5],
    });
  });

  it('starts idle', () => {
    const { result } = render();

    expect(result.current.isSubscribing).toBe(false);
    expect(result.current.isUpdating).toBe(false);
    expect(result.current.subscribeError).toBeNull();
    expect(result.current.updateError).toBeNull();
  });
});
