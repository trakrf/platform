import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useNotificationAlerts, localDayBound, toAlertQuery } from './useNotificationAlerts';
import { notificationsApi } from '@/lib/api/notifications';
import type { NotificationAlert, NotificationAlertFilters } from '@/types/notifications';

vi.mock('@/lib/api/notifications');

const mockOrgState = { currentOrg: { id: 7, name: 'Test Org' } };
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: Object.assign(
    vi.fn((selector) => (selector ? selector(mockOrgState) : mockOrgState)),
    { getState: () => mockOrgState }
  ),
}));

const alert: NotificationAlert = {
  id: 11,
  asset_id: 4,
  asset_name: 'Pallet 4',
  asset_external_key: 'PAL-004',
  type: 'location_change',
  occurred_at: '2026-10-01T10:00:00Z',
  sent_at: '2026-10-01T10:00:05Z',
  recipient_id: 3,
  recipient_name: 'Dana',
  channel: 'email',
  subject: 'Pallet 4 moved',
  body: 'Pallet 4 moved to Gate A',
  details: { location: 'Gate A', direction: 'unknown' },
};

const createClient = () =>
  new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

const wrapperFor = (queryClient: QueryClient) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };

describe('localDayBound', () => {
  it('starts a bare date at local midnight with an offset', () => {
    const s = localDayBound('2026-03-10', 'start');
    expect(s).toMatch(/^2026-03-10T00:00:00\.000[+-]\d{2}:\d{2}$/);
    expect(Date.parse(s)).toBe(new Date(2026, 2, 10, 0, 0, 0, 0).getTime());
  });
  it('ends a bare date at local 23:59:59.999', () => {
    const s = localDayBound('2026-11-05', 'end');
    expect(Date.parse(s)).toBe(new Date(2026, 10, 5, 23, 59, 59, 999).getTime());
  });
  it("uses each day's own offset across a DST change", () => {
    for (const d of ['2026-01-15', '2026-07-15']) {
      const [y, m, day] = d.split('-').map(Number);
      expect(Date.parse(localDayBound(d, 'start'))).toBe(new Date(y, m - 1, day).getTime());
    }
  });
});

describe('toAlertQuery', () => {
  it('converts bare dates and defaults page and per_page', () => {
    const q = toAlertQuery({ from: '2026-10-01', to: '2026-10-02' });
    expect(q.from).toBe(localDayBound('2026-10-01', 'start'));
    expect(q.to).toBe(localDayBound('2026-10-02', 'end'));
    expect(q.page).toBe(1);
    expect(q.per_page).toBe(20);
  });
  it('passes full timestamps through unchanged', () => {
    expect(toAlertQuery({ from: '2026-10-01T05:00:00Z' }).from).toBe('2026-10-01T05:00:00Z');
  });
  it('drops filters that are not set', () => {
    expect(Object.keys(toAlertQuery({ asset_id: 3, type: undefined })).sort()).toEqual([
      'asset_id',
      'page',
      'per_page',
    ]);
  });
});

describe('useNotificationAlerts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns the server page and total', async () => {
    vi.mocked(notificationsApi.listAlerts).mockResolvedValue({
      data: { data: [alert], pagination: { page: 2, per_page: 10, total: 31 } },
    } as any);

    const { result } = renderHook(() => useNotificationAlerts({ page: 2, per_page: 10 }), {
      wrapper: wrapperFor(createClient()),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.alerts).toEqual([alert]);
    expect(result.current.total).toBe(31);
    expect(result.current.page).toBe(2);
    expect(result.current.perPage).toBe(10);
    expect(result.current.error).toBeNull();
  });

  it('sends the converted query to the API', async () => {
    vi.mocked(notificationsApi.listAlerts).mockResolvedValue({
      data: { data: [], pagination: { page: 1, per_page: 20, total: 0 } },
    } as any);
    const filters = { asset_id: 4, from: '2026-10-01' };

    const { result } = renderHook(() => useNotificationAlerts(filters), {
      wrapper: wrapperFor(createClient()),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(notificationsApi.listAlerts).toHaveBeenCalledWith(toAlertQuery(filters));
  });

  it('keys the query by org and query', async () => {
    vi.mocked(notificationsApi.listAlerts).mockResolvedValue({
      data: { data: [], pagination: { page: 1, per_page: 20, total: 0 } },
    } as any);
    const queryClient = createClient();

    const { result } = renderHook(() => useNotificationAlerts(), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(
      queryClient.getQueryCache().find({ queryKey: ['notificationAlerts', 7], exact: false })
    ).toBeDefined();
  });

  it('keeps the previous page visible while the next one loads', async () => {
    vi.mocked(notificationsApi.listAlerts)
      .mockResolvedValueOnce({
        data: {
          data: [alert],
          pagination: { page: 1, per_page: 20, total: 40 },
        },
      } as any)
      .mockReturnValueOnce(new Promise(() => {}) as any);

    const { result, rerender } = renderHook(
      ({ filters }: { filters: NotificationAlertFilters }) => useNotificationAlerts(filters),
      {
        wrapper: wrapperFor(createClient()),
        initialProps: { filters: { page: 1 } },
      }
    );

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.alerts).toEqual([alert]);

    rerender({ filters: { page: 2 } });

    await waitFor(() => expect(notificationsApi.listAlerts).toHaveBeenCalledTimes(2));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.alerts).toEqual([alert]);
  });

  it('exposes a request failure as error, with no alerts', async () => {
    vi.mocked(notificationsApi.listAlerts).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useNotificationAlerts(), {
      wrapper: wrapperFor(createClient()),
    });

    await waitFor(() => expect(result.current.error).toBeInstanceOf(Error));
    expect(result.current.alerts).toEqual([]);
    expect(result.current.total).toBe(0);
  });
});
