import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  useNotificationRecipients,
  useNotificationRecipientMutations,
} from './useNotificationRecipients';
import { notificationsApi } from '@/lib/api/notifications';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import type { NotificationRecipient } from '@/types/notifications';

vi.mock('@/lib/api/notifications');
vi.mock('@/lib/auth/orgContext');

const mockOrgState = { currentOrg: { id: 7, name: 'Test Org' } };
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: Object.assign(
    vi.fn((selector) => (selector ? selector(mockOrgState) : mockOrgState)),
    { getState: () => mockOrgState }
  ),
}));

const recipient: NotificationRecipient = {
  id: 3,
  org_id: 7,
  name: 'Dana',
  email: 'dana@example.com',
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const createClient = () =>
  new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

const wrapperFor = (queryClient: QueryClient) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };

describe('useNotificationRecipients', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('fetches recipients under the org-scoped key', async () => {
    vi.mocked(notificationsApi.listRecipients).mockResolvedValue({
      data: { data: [recipient] },
    } as any);
    const queryClient = createClient();

    const { result } = renderHook(() => useNotificationRecipients(), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.recipients).toEqual([recipient]);
    expect(queryClient.getQueryData(['notificationRecipients', 7])).toEqual([recipient]);
  });

  it('does not fetch when disabled', () => {
    const { result } = renderHook(() => useNotificationRecipients({ enabled: false }), {
      wrapper: wrapperFor(createClient()),
    });

    expect(notificationsApi.listRecipients).not.toHaveBeenCalled();
    expect(result.current.recipients).toEqual([]);
  });

  it('exposes the fetch error', async () => {
    vi.mocked(notificationsApi.listRecipients).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useNotificationRecipients(), {
      wrapper: wrapperFor(createClient()),
    });

    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.recipients).toEqual([]);
  });
});

describe('useNotificationRecipientMutations', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(ensureOrgContext).mockResolvedValue(7);
    queryClient = createClient();
    vi.spyOn(queryClient, 'invalidateQueries');
  });

  const render = () =>
    renderHook(() => useNotificationRecipientMutations(), { wrapper: wrapperFor(queryClient) });

  it('creates a recipient and invalidates the list only', async () => {
    vi.mocked(notificationsApi.createRecipient).mockResolvedValue({
      data: { data: recipient },
    } as any);
    const { result } = render();

    const created = await result.current.create({ name: 'Dana', email: 'dana@example.com' });

    expect(ensureOrgContext).toHaveBeenCalled();
    expect(notificationsApi.createRecipient).toHaveBeenCalledWith({
      name: 'Dana',
      email: 'dana@example.com',
    });
    expect(created).toEqual(recipient);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['notificationRecipients'],
    });
    expect(queryClient.invalidateQueries).not.toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions'],
    });
  });

  it('updates a recipient and invalidates recipients and subscriptions', async () => {
    vi.mocked(notificationsApi.updateRecipient).mockResolvedValue({
      data: { data: { ...recipient, is_active: false } },
    } as any);
    const { result } = render();

    await result.current.update({ id: 3, updates: { is_active: false } });

    expect(ensureOrgContext).toHaveBeenCalled();
    expect(notificationsApi.updateRecipient).toHaveBeenCalledWith(3, { is_active: false });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['notificationRecipients'],
    });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions'],
    });
  });

  it('deletes a recipient and invalidates recipients and subscriptions', async () => {
    vi.mocked(notificationsApi.deleteRecipient).mockResolvedValue({} as any);
    const { result } = render();

    const id = await result.current.delete(3);

    expect(ensureOrgContext).toHaveBeenCalled();
    expect(notificationsApi.deleteRecipient).toHaveBeenCalledWith(3);
    expect(id).toBe(3);
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['notificationRecipients'],
    });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ['assetSubscriptions'],
    });
  });

  it('does not call the API when org context fails', async () => {
    vi.mocked(ensureOrgContext).mockRejectedValue(new Error('no org'));
    const { result } = render();

    await expect(result.current.delete(3)).rejects.toThrow('no org');
    expect(notificationsApi.deleteRecipient).not.toHaveBeenCalled();
  });

  it('starts idle', () => {
    const { result } = render();

    expect(result.current.isCreating).toBe(false);
    expect(result.current.isUpdating).toBe(false);
    expect(result.current.isDeleting).toBe(false);
    expect(result.current.createError).toBeNull();
    expect(result.current.updateError).toBeNull();
    expect(result.current.deleteError).toBeNull();
  });
});
