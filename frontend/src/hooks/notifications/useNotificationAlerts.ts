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

const pad = (n: number, width = 2) => String(n).padStart(width, '0');

/**
 * A bare YYYY-MM-DD bound as a timestamp covering the whole local day: local
 * 00:00:00.000 or 23:59:59.999999 (the server stores microseconds), with the offset that applies on that day.
 */
export function localDayBound(date: string, edge: 'start' | 'end'): string {
  const [y, m, d] = date.split('-').map(Number);
  const dt =
    edge === 'start' ? new Date(y, m - 1, d, 0, 0, 0, 0) : new Date(y, m - 1, d, 23, 59, 59, 999);
  const offset = -dt.getTimezoneOffset();
  const sign = offset < 0 ? '-' : '+';
  const abs = Math.abs(offset);
  const frac = edge === 'start' ? '000' : '999999';
  return (
    `${pad(dt.getFullYear(), 4)}-${pad(dt.getMonth() + 1)}-${pad(dt.getDate())}` +
    `T${pad(dt.getHours())}:${pad(dt.getMinutes())}:${pad(dt.getSeconds())}.${frac}` +
    `${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
  );
}

const BARE_DATE = /^\d{4}-\d{2}-\d{2}$/;

/** Build the request query: set filters only, bare dates widened, paging defaulted. */
export function toAlertQuery(filters: NotificationAlertFilters): NotificationAlertFilters {
  const query: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) query[key] = value;
  }
  if (filters.from && BARE_DATE.test(filters.from))
    query.from = localDayBound(filters.from, 'start');
  if (filters.to && BARE_DATE.test(filters.to)) query.to = localDayBound(filters.to, 'end');
  query.page = filters.page ?? 1;
  query.per_page = filters.per_page ?? DEFAULT_PER_PAGE;
  return query as NotificationAlertFilters;
}

/**
 * Sent alert history for the current org. The server filters, sorts newest
 * first and pages; the previous page stays visible while the next loads.
 */
export function useNotificationAlerts(
  filters: NotificationAlertFilters = {}
): UseNotificationAlertsResult {
  const currentOrg = useOrgStore((state) => state.currentOrg);
  const query = toAlertQuery(filters);

  const q = useQuery({
    queryKey: ['notificationAlerts', currentOrg?.id, query],
    queryFn: () => notificationsApi.listAlerts(query).then((r) => r.data),
    // Keep the previous page across paging, but never across an org switch.
    placeholderData: (prev, prevQuery) =>
      prevQuery?.queryKey[1] === currentOrg?.id ? prev : undefined,
    staleTime: 30_000,
  });

  return {
    alerts: q.data?.data ?? [],
    total: q.data?.pagination.total ?? 0,
    page: query.page as number,
    perPage: query.per_page as number,
    isLoading: q.isLoading,
    error: q.error ?? null,
  };
}
