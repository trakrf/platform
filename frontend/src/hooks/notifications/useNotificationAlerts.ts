import { useQuery } from '@tanstack/react-query';
import { useOrgStore } from '@/stores/orgStore';
import { notificationsApi } from '@/lib/api/notifications';
import type { NotificationAlert, NotificationAlertFilters } from '@/types/notifications';

const DEFAULT_PER_PAGE = 20;

export interface UseNotificationAlertsResult {
  alerts: NotificationAlert[];
  total: number;
  page: number;
  perPage: number;
  isLoading: boolean;
  error: unknown;
}

const BARE_DATE = /^\d{4}-\d{2}-\d{2}$/;

const pad = (n: number, width = 2) => String(n).padStart(width, '0');

/**
 * The server takes only RFC 3339 instants, because it cannot know the
 * viewer's zone. A bare YYYY-MM-DD from the date filters becomes local
 * 00:00:00.000 or 23:59:59.999999 of that day, written with that day's own
 * offset so DST days come out right. The end keeps microseconds because the
 * server stores them and treats `to` as inclusive; toISOString() would stop
 * at milliseconds and miss the day's last 999 microseconds.
 */
export function toInstant(value: string, edge: 'start' | 'end'): string {
  if (!BARE_DATE.test(value)) return value;
  const [y, m, d] = value.split('-').map(Number);
  const dt =
    edge === 'start' ? new Date(y, m - 1, d, 0, 0, 0, 0) : new Date(y, m - 1, d, 23, 59, 59, 999);
  const offset = -dt.getTimezoneOffset();
  const sign = offset < 0 ? '-' : '+';
  const abs = Math.abs(offset);
  const time = edge === 'start' ? '00:00:00.000' : '23:59:59.999999';
  return (
    `${pad(dt.getFullYear(), 4)}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}T${time}` +
    `${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
  );
}

/**
 * Builds the endpoint's query. Unset filters are left out rather than sent
 * empty: the server rejects a present-but-empty parameter.
 */
export function alertQuery(
  filters: NotificationAlertFilters,
  page: number,
  perPage: number
): NotificationAlertFilters {
  const query: NotificationAlertFilters = { page, per_page: perPage };
  if (filters.asset_id !== undefined) query.asset_id = filters.asset_id;
  if (filters.type) query.type = filters.type;
  if (filters.recipient_id !== undefined) query.recipient_id = filters.recipient_id;
  if (filters.channel) query.channel = filters.channel;
  if (filters.from) query.from = toInstant(filters.from, 'start');
  if (filters.to) query.to = toInstant(filters.to, 'end');
  return query;
}

/**
 * Sent alerts from GET /notification-alerts: filtered, newest first and
 * paginated by the server.
 */
export function useNotificationAlerts(
  filters: NotificationAlertFilters = {}
): UseNotificationAlertsResult {
  const currentOrg = useOrgStore((state) => state.currentOrg);
  const page = filters.page ?? 1;
  const perPage = filters.per_page ?? DEFAULT_PER_PAGE;
  const query = alertQuery(filters, page, perPage);

  const result = useQuery({
    queryKey: ['notificationAlerts', currentOrg?.id, query],
    queryFn: async () => {
      const response = await notificationsApi.listAlerts(query);
      return response.data;
    },
    // Keep the previous page while the next loads, but never across an org
    // switch, or the last org's alerts would show until the new ones arrive.
    placeholderData: (prev, prevQuery) =>
      prevQuery?.queryKey[1] === currentOrg?.id ? prev : undefined,
  });

  return {
    alerts: result.data?.data ?? [],
    total: result.data?.pagination.total ?? 0,
    page,
    perPage,
    isLoading: result.isLoading,
    error: result.error,
  };
}
