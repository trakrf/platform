import { useMemo } from 'react';
import { useAssets } from '@/hooks/assets';
import { useNotificationRecipients } from './useNotificationRecipients';
import { getAppConfig, isNonProd } from '@/lib/appConfig';
import { buildMockAlerts } from '@/lib/notifications/mockAlerts';
import type { NotificationAlert, NotificationAlertFilters } from '@/types/notifications';

const DEFAULT_PER_PAGE = 20;

/**
 * Generated alerts show locally and in every deployed non-production
 * environment (preview, staging); production shows none. Checked at runtime,
 * not build time, because one bundle serves every environment.
 */
export function alertsMockEnabled(): boolean {
  return import.meta.env.DEV || isNonProd(getAppConfig().environmentLabel);
}

export interface UseNotificationAlertsResult {
  alerts: NotificationAlert[];
  total: number;
  page: number;
  perPage: number;
  isLoading: boolean;
  error: unknown;
}

/** A bare YYYY-MM-DD bound covers the whole local day. */
function boundTime(value: string, edge: 'start' | 'end'): number {
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return new Date(`${value}T${edge === 'start' ? '00:00:00.000' : '23:59:59.999'}`).getTime();
  }
  return new Date(value).getTime();
}

function applyFilters(
  alerts: NotificationAlert[],
  filters: NotificationAlertFilters
): NotificationAlert[] {
  const from = filters.from ? boundTime(filters.from, 'start') : null;
  const to = filters.to ? boundTime(filters.to, 'end') : null;
  return alerts.filter((a) => {
    if (filters.asset_id !== undefined && a.asset_id !== filters.asset_id) return false;
    if (filters.type && a.type !== filters.type) return false;
    if (filters.recipient_id !== undefined && a.recipient_id !== filters.recipient_id) return false;
    if (filters.channel && a.channel !== filters.channel) return false;
    const sent = new Date(a.sent_at).getTime();
    if (from !== null && sent < from) return false;
    if (to !== null && sent > to) return false;
    return true;
  });
}

/**
 * Alert history, filtered, newest first and paginated — the shape the list
 * endpoint will return. The endpoint does not exist yet; swapping to it only
 * changes this hook.
 */
export function useNotificationAlerts(
  filters: NotificationAlertFilters = {}
): UseNotificationAlertsResult {
  const mock = alertsMockEnabled();
  const page = filters.page ?? 1;
  const perPage = filters.per_page ?? DEFAULT_PER_PAGE;

  const { assets, isLoading: assetsLoading } = useAssets({ enabled: mock });
  const { recipients, isLoading: recipientsLoading } = useNotificationRecipients({
    enabled: mock,
  });

  const all = useMemo(
    () => (mock ? buildMockAlerts(assets, recipients) : []),
    [mock, assets, recipients]
  );

  const { asset_id, type, recipient_id, channel, from, to } = filters;
  const matching = useMemo(
    () =>
      applyFilters(all, { asset_id, type, recipient_id, channel, from, to }).sort(
        (a, b) => Date.parse(b.sent_at) - Date.parse(a.sent_at) || b.id - a.id
      ),
    [all, asset_id, type, recipient_id, channel, from, to]
  );

  const start = (page - 1) * perPage;
  return {
    alerts: matching.slice(start, start + perPage),
    total: matching.length,
    page,
    perPage,
    // Mock rows fall back to sample data, so a failed asset or contact read is
    // not an alert-list error.
    isLoading: mock && (assetsLoading || recipientsLoading),
    error: null,
  };
}
