import { useState, useCallback } from 'react';
import { Bell } from 'lucide-react';
import { useAuthStore, useUIStore } from '@/stores';
import { useEntitlement } from '@/hooks/entitlement/useEntitlement';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';
import { formatRelativeTime } from '@/lib/reports/utils';
import type { NotificationAlert } from '@/types/notifications';

const RECENT_LIMIT = 5;

function TypeBadge({ type }: { type: NotificationAlert['type'] }) {
  return type === 'alarm' ? (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300 whitespace-nowrap">
      Alarm
    </span>
  ) : (
    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400 whitespace-nowrap">
      Location change
    </span>
  );
}

/** Header bell: the newest alerts at a glance, with a way into the full page. */
export function NotificationBell() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  // The data hooks live in the inner component so a signed-out header never
  // runs them.
  if (!isAuthenticated) return null;
  return <NotificationBellMenu />;
}

function NotificationBellMenu() {
  const [isOpen, setIsOpen] = useState(false);
  const { isLocked } = useEntitlement();
  const { alerts } = useNotificationAlerts({ page: 1, per_page: RECENT_LIMIT });
  const recent = isLocked ? [] : alerts.slice(0, RECENT_LIMIT);

  const close = useCallback(() => setIsOpen(false), []);
  useEscapeToClose(isOpen, close);

  const goToNotifications = () => {
    setIsOpen(false);
    useUIStore.getState().setActiveTab('notifications');
    window.history.pushState({ tab: 'notifications' }, '', '#notifications');
  };

  // A dot, not a count: there is no read tracking yet, so "unread" cannot be
  // told apart from "recent".
  const showDot = recent.length > 0;

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setIsOpen((open) => !open)}
        className="relative p-1.5 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
        aria-label="Notifications"
        aria-expanded={isOpen}
        data-testid="header-notifications-bell"
      >
        <Bell className="w-5 h-5 text-gray-600 dark:text-gray-400" />
        {showDot && (
          <span
            className="absolute top-1 right-1 w-2 h-2 rounded-full bg-red-500 ring-2 ring-white dark:ring-gray-800"
            data-testid="header-notifications-dot"
          />
        )}
      </button>

      {isOpen && (
        <>
          <div className="fixed inset-0 z-40" onClick={close} aria-hidden="true" />
          <div
            className="fixed left-2 right-2 top-14 sm:absolute sm:left-auto sm:right-0 sm:top-auto sm:mt-2 sm:w-80 sm:max-w-[calc(100vw-1rem)] z-50 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 shadow-lg"
            data-testid="header-notifications-panel"
          >
            <div className="px-3 py-2 border-b border-gray-100 dark:border-gray-700 text-sm font-semibold text-gray-900 dark:text-gray-100">
              Notifications
            </div>

            {isLocked ? (
              <p className="px-3 py-3 text-sm text-gray-600 dark:text-gray-400">
                Notifications aren&apos;t enabled for your organization.
              </p>
            ) : recent.length === 0 ? (
              <p className="px-3 py-3 text-sm text-gray-500 dark:text-gray-400">No alerts yet</p>
            ) : (
              <ul className="divide-y divide-gray-100 dark:divide-gray-700">
                {recent.map((alert) => (
                  <li key={alert.id}>
                    <button
                      type="button"
                      onClick={goToNotifications}
                      className="w-full flex items-center justify-between gap-2 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
                      data-testid={`header-notifications-row-${alert.id}`}
                    >
                      <div className="min-w-0">
                        <div className="text-sm text-gray-900 dark:text-gray-100 truncate">
                          {alert.asset_name}
                        </div>
                        <div className="text-xs text-gray-500 dark:text-gray-400">
                          {formatRelativeTime(alert.sent_at)}
                        </div>
                      </div>
                      <TypeBadge type={alert.type} />
                    </button>
                  </li>
                ))}
              </ul>
            )}

            <div className="border-t border-gray-100 dark:border-gray-700 px-3 py-2">
              <button
                type="button"
                onClick={goToNotifications}
                className="text-sm font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                data-testid="header-notifications-view-all"
              >
                View all
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
