import { useEffect, useState } from 'react';
import { UserPlus, X } from 'lucide-react';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';
import { PaidGate } from '@/components/entitlement';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';
import { AddSubscriberForm } from './AddSubscriberForm';
import { AlertList } from './AlertList';
import { NotificationsGate } from './NotificationsGate';
import { SubscriberList } from './SubscriberList';

const ALERTS_PER_PAGE = 10;

const PAGE_BUTTON =
  'px-2.5 py-1 text-xs sm:text-sm font-medium rounded-lg border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed';

/**
 * One asset's alert history. Mounted only while the modal is open, so alerts
 * are read only then and paging starts again at 1 on reopen.
 */
function AssetAlertHistory({ assetId }: { assetId: number }) {
  const [page, setPage] = useState(1);
  const { alerts, total, isLoading } = useNotificationAlerts({
    asset_id: assetId,
    page,
    per_page: ALERTS_PER_PAGE,
  });
  const totalPages = Math.max(1, Math.ceil(total / ALERTS_PER_PAGE));

  let body;
  if (isLoading) {
    body = <p className="py-2 text-sm text-gray-500 dark:text-gray-400">Loading alerts…</p>;
  } else if (total === 0) {
    body = (
      <p className="py-2 text-sm text-gray-500 dark:text-gray-400">
        No alerts for this asset yet.
      </p>
    );
  } else {
    body = (
      <>
        <AlertList alerts={alerts} compact />
        {total > ALERTS_PER_PAGE && (
          <div className="flex items-center justify-between gap-2 mt-3">
            <button
              type="button"
              data-testid="asset-alerts-prev"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className={PAGE_BUTTON}
            >
              Previous
            </button>
            <span
              data-testid="asset-alerts-page"
              className="text-xs sm:text-sm text-gray-600 dark:text-gray-400"
            >
              Page {page} of {totalPages}
            </span>
            <button
              type="button"
              data-testid="asset-alerts-next"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className={PAGE_BUTTON}
            >
              Next
            </button>
          </div>
        )}
      </>
    );
  }

  return (
    <section
      aria-labelledby="asset-alerts-heading"
      className="border-t border-gray-200 dark:border-gray-700 pt-4"
    >
      <h3
        id="asset-alerts-heading"
        className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-3"
      >
        Alerts
      </h3>
      {body}
    </section>
  );
}

interface AssetNotificationsModalProps {
  isOpen: boolean;
  onClose: () => void;
  assetId: number;
  assetName: string;
  assetKey: string;
}

/**
 * One asset's notification settings. z-[60] so it can stack above
 * AssetDetailsModal (z-50). Full-screen below `sm`.
 */
export function AssetNotificationsModal({
  isOpen,
  onClose,
  assetId,
  assetName,
  assetKey,
}: AssetNotificationsModalProps) {
  useEscapeToClose(isOpen, onClose);
  const [adding, setAdding] = useState(false);
  // Reopening the modal starts with the add form closed.
  useEffect(() => {
    if (!isOpen) setAdding(false);
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <>
      <div
        className="fixed inset-0 bg-black/50 z-[60] animate-fadeIn"
        onClick={onClose}
        aria-hidden="true"
      />
      <div className="fixed inset-0 z-[60] flex items-center justify-center p-0 sm:p-4">
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="asset-notifications-title"
          data-testid="asset-notifications-modal"
          className="bg-white dark:bg-gray-800 shadow-2xl w-full h-full rounded-none sm:h-auto sm:max-w-2xl sm:max-h-[90vh] sm:rounded-lg overflow-hidden flex flex-col animate-slideUp"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between gap-3 px-4 sm:px-6 py-4 border-b border-gray-200 dark:border-gray-700">
            <div className="min-w-0">
              <h2
                id="asset-notifications-title"
                className="text-xl font-bold text-gray-900 dark:text-gray-100"
              >
                Notifications
              </h2>
              <p className="text-sm text-gray-600 dark:text-gray-400 truncate">
                {assetName} · {assetKey}
              </p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
              aria-label="Close modal"
            >
              <X className="w-5 h-5 text-gray-500 dark:text-gray-400" />
            </button>
          </div>

          <div className="flex-1 px-4 sm:px-6 py-4 overflow-y-auto">
            <NotificationsGate>
              <div className="space-y-6">
                <section aria-labelledby="asset-subscribers-heading">
                  <div className="flex items-center justify-between gap-2 mb-3">
                    <h3
                      id="asset-subscribers-heading"
                      className="text-base font-semibold text-gray-900 dark:text-gray-100"
                    >
                      Subscribers
                    </h3>
                    <PaidGate surface="notifications-subscriptions">
                      <button
                        type="button"
                        data-testid="add-subscriber-toggle"
                        aria-expanded={adding}
                        onClick={() => setAdding((v) => !v)}
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-blue-600 dark:bg-blue-500 rounded-lg hover:bg-blue-700 dark:hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 transition-colors"
                      >
                        <UserPlus className="w-4 h-4" aria-hidden="true" />
                        Add subscriber
                      </button>
                    </PaidGate>
                  </div>
                  {adding && (
                    <AddSubscriberForm
                      assetId={assetId}
                      assetName={assetName}
                      onDone={() => setAdding(false)}
                      onCancel={() => setAdding(false)}
                    />
                  )}
                  <SubscriberList assetId={assetId} assetName={assetName} />
                </section>
                <AssetAlertHistory assetId={assetId} />
              </div>
            </NotificationsGate>
          </div>

          <div className="flex justify-end gap-3 px-4 sm:px-6 py-4 border-t border-gray-200 dark:border-gray-700">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-gray-200 dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg hover:bg-gray-300 dark:hover:bg-gray-600 font-medium transition-colors"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
