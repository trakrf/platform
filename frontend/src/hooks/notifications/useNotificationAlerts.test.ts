import React, { type ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, it, expect, beforeEach, beforeAll, afterAll, vi } from 'vitest';
import { useNotificationAlerts, alertQuery, toInstant } from './useNotificationAlerts';
import { notificationsApi } from '@/lib/api/notifications';
import type { NotificationAlert } from '@/types/notifications';

vi.mock('@/lib/api/notifications');

const ORG_7 = { id: 7, name: 'Test Org' };
const mockOrgState = { currentOrg: ORG_7 };
vi.mock('@/stores/orgStore', () => ({
  useOrgStore: Object.assign(
    vi.fn((selector) => (selector ? selector(mockOrgState) : mockOrgState)),
    { getState: () => mockOrgState }
  ),
}));

const alert: NotificationAlert = {
  id: 'alert-20261020T110000Z-alarm-2814749767106561',
  asset_id: 5,
  asset_name: 'Forklift 3',
  asset_external_key: 'FL-003',
  type: 'alarm',
  occurred_at: '2026-10-20T10:59:55Z',
  sent_at: '2026-10-20T11:00:00Z',
  recipient_id: 1,
  recipient_name: 'Ana',
  channel: 'email',
  subject: 'Alarm',
  body: 'Forklift 3 left through Gate A',
  details: { location: 'Gate A' },
};

const wrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
};

describe('useNotificationAlerts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(notificationsApi.listAlerts).mockResolvedValue({
      data: { data: [alert], pagination: { total: 41, page: 3, per_page: 20 } },
    } as never);
  });

  it('returns the page and total the endpoint sends', async () => {
    const { result } = renderHook(() => useNotificationAlerts({ page: 3 }), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.alerts).toEqual([alert]);
    expect(result.current.total).toBe(41);
    expect(result.current.page).toBe(3);
    expect(result.current.perPage).toBe(20);
    expect(notificationsApi.listAlerts).toHaveBeenCalledWith({
      page: 3,
      per_page: 20,
    });
  });

  it('surfaces a failed request as an error with no alerts', async () => {
    const failure = new Error('boom');
    vi.mocked(notificationsApi.listAlerts).mockRejectedValue(failure);
    const { result } = renderHook(() => useNotificationAlerts(), {
      wrapper: wrapper(),
    });

    await waitFor(() => expect(result.current.error).toBe(failure));
    expect(result.current.alerts).toEqual([]);
    expect(result.current.total).toBe(0);
  });

  it('keeps the previous page visible while the next one loads', async () => {
    vi.mocked(notificationsApi.listAlerts)
      .mockResolvedValueOnce({
        data: { data: [alert], pagination: { total: 40, page: 1, per_page: 20 } },
      } as never)
      .mockReturnValueOnce(new Promise(() => {}) as never);
    const { result, rerender } = renderHook(({ page }) => useNotificationAlerts({ page }), {
      wrapper: wrapper(),
      initialProps: { page: 1 },
    });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    rerender({ page: 2 });

    await waitFor(() => expect(notificationsApi.listAlerts).toHaveBeenCalledTimes(2));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.alerts).toEqual([alert]);
  });

  it("does not show the previous org's alerts after an org switch", async () => {
    vi.mocked(notificationsApi.listAlerts)
      .mockResolvedValueOnce({
        data: { data: [alert], pagination: { total: 1, page: 1, per_page: 20 } },
      } as never)
      .mockReturnValueOnce(new Promise(() => {}) as never);
    const { result, rerender } = renderHook(() => useNotificationAlerts(), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.alerts).toEqual([alert]);

    mockOrgState.currentOrg = { id: 8, name: 'Other Org' };
    try {
      rerender();
      await waitFor(() => expect(notificationsApi.listAlerts).toHaveBeenCalledTimes(2));
      expect(result.current.alerts).toEqual([]);
      expect(result.current.isLoading).toBe(true);
    } finally {
      mockOrgState.currentOrg = ORG_7;
    }
  });
});

describe('alertQuery', () => {
  it('leaves unset filters out, because the server rejects empty values', () => {
    expect(alertQuery({}, 1, 20)).toEqual({ page: 1, per_page: 20 });
  });

  it('passes every set filter through', () => {
    expect(
      alertQuery(
        {
          asset_id: 5,
          type: 'location_change',
          recipient_id: 2,
          channel: 'sms',
        },
        2,
        10
      )
    ).toEqual({
      page: 2,
      per_page: 10,
      asset_id: 5,
      type: 'location_change',
      recipient_id: 2,
      channel: 'sms',
    });
  });

  it('widens bare dates to the whole local day', () => {
    const query = alertQuery({ from: '2026-10-20', to: '2026-10-20' }, 1, 20);
    expect(Date.parse(query.from!)).toBe(new Date(2026, 9, 20).getTime());
    expect(query.to).toMatch(/^2026-10-20T23:59:59\.999999[+-]\d{2}:\d{2}$/);
  });
});

describe('toInstant', () => {
  it('leaves an instant unchanged', () => {
    expect(toInstant('2026-10-20T08:00:00Z', 'start')).toBe('2026-10-20T08:00:00Z');
  });

  describe('in a zone with DST', () => {
    // Under the default UTC zone a DST check passes without proving anything.
    const originalTZ = process.env.TZ;
    beforeAll(() => {
      process.env.TZ = 'America/New_York';
    });
    afterAll(() => {
      if (originalTZ === undefined) delete process.env.TZ;
      else process.env.TZ = originalTZ;
    });

    it('ends the day at 23:59:59.999999, since the server keeps microseconds', () => {
      expect(toInstant('2026-11-05', 'end')).toBe('2026-11-05T23:59:59.999999-05:00');
    });

    it("writes each day's own offset across a DST change", () => {
      expect(toInstant('2026-01-15', 'start')).toBe('2026-01-15T00:00:00.000-05:00');
      expect(toInstant('2026-07-15', 'start')).toBe('2026-07-15T00:00:00.000-04:00');
      // Spring forward: midnight is still EST, the day's end is EDT.
      expect(toInstant('2026-03-08', 'start')).toBe('2026-03-08T00:00:00.000-05:00');
      expect(toInstant('2026-03-08', 'end')).toBe('2026-03-08T23:59:59.999999-04:00');
      // Fall back: midnight is still EDT, the day's end is EST.
      expect(toInstant('2026-11-01', 'start')).toBe('2026-11-01T00:00:00.000-04:00');
      expect(toInstant('2026-11-01', 'end')).toBe('2026-11-01T23:59:59.999999-05:00');
    });
  });
});
