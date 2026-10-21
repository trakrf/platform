import { X } from 'lucide-react';
import { useEscapeToClose } from '@/hooks/useEscapeToClose';
import { NotificationsGate } from './NotificationsGate';
import { SubscriberList } from './SubscriberList';

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
                    {/* Section actions (add subscriber) go here. */}
                  </div>
                  <SubscriberList assetId={assetId} assetName={assetName} />
                </section>
                {/* Further sections (alert history) follow as siblings. */}
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
