import { useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import toast from 'react-hot-toast';
import {
  useAssetSubscriptionMutations,
  useAssetSubscriptions,
  useNotificationRecipients,
} from '@/hooks/notifications';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import { ConfirmModal } from '@/components/shared';
import { PaidGate } from '@/components/entitlement';
import type {
  NotificationChannel,
  NotificationRecipient,
  NotificationSubscription,
} from '@/types/notifications';

const CHANNEL_LABELS: Record<NotificationChannel, string> = { email: 'Email', sms: 'SMS' };

const BUTTON_BASE =
  'px-2.5 py-1 text-xs sm:text-sm font-medium rounded-lg border transition-colors disabled:opacity-50 disabled:cursor-not-allowed';

interface SubscriberListProps {
  assetId: number;
  assetName: string;
}

interface Row {
  subscription: NotificationSubscription;
  recipient: NotificationRecipient | undefined;
}

function detailFor(recipient: NotificationRecipient, channel: NotificationChannel) {
  return channel === 'email' ? recipient.email : recipient.phone;
}

function isConflict(error: unknown) {
  return (error as { response?: { status?: number } })?.response?.status === 409;
}

export function SubscriberList({ assetId, assetName }: SubscriberListProps) {
  const { subscriptions, isLoading, error } = useAssetSubscriptions(assetId);
  const {
    recipients,
    isLoading: recipientsLoading,
    error: recipientsError,
  } = useNotificationRecipients();
  const { update, isUpdating } = useAssetSubscriptionMutations(assetId);
  const [confirming, setConfirming] = useState<Row | null>(null);

  // Escape should dismiss only the confirm dialog, not the notifications modal
  // under it: catch it on window in the capture phase, before the modal's
  // document-level listener sees it.
  useEffect(() => {
    if (!confirming) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.stopPropagation();
      setConfirming(null);
    };
    window.addEventListener('keydown', handler, true);
    return () => window.removeEventListener('keydown', handler, true);
  }, [confirming]);

  const rows = useMemo<Row[]>(() => {
    const byId = new Map(recipients.map((r) => [r.id, r]));
    return subscriptions
      .map((subscription) => ({ subscription, recipient: byId.get(subscription.recipient_id) }))
      .sort((a, b) => {
        if (a.subscription.is_active !== b.subscription.is_active) {
          return a.subscription.is_active ? -1 : 1;
        }
        // Deleted contacts have no name; they sort last within their group.
        const an = a.recipient?.name ?? '￿';
        const bn = b.recipient?.name ?? '￿';
        return an.localeCompare(bn);
      });
  }, [subscriptions, recipients]);

  const changeChannel = async (subscription: NotificationSubscription, channel: NotificationChannel) => {
    if (channel === subscription.channel) return;
    try {
      await update({ id: subscription.id, channel });
      toast.success(`Now sending ${CHANNEL_LABELS[channel]} alerts`);
    } catch (err) {
      toast.error(
        isConflict(err)
          ? `This contact is already subscribed by ${CHANNEL_LABELS[channel]} on this asset`
          : getApiErrorMessage(err, 'Failed to change channel')
      );
    }
  };

  const setActive = async (subscription: NotificationSubscription, isActive: boolean) => {
    try {
      await update({ id: subscription.id, is_active: isActive });
      toast.success(isActive ? 'Alerts turned back on' : 'Alerts switched off');
    } catch (err) {
      toast.error(
        getApiErrorMessage(
          err,
          isActive ? 'Failed to turn alerts back on' : 'Failed to switch alerts off'
        )
      );
    }
  };

  const handleConfirmOff = async () => {
    const row = confirming;
    setConfirming(null);
    if (row) await setActive(row.subscription, false);
  };

  if (isLoading || recipientsLoading) {
    return <p className="py-2 text-sm text-gray-500 dark:text-gray-400">Loading subscribers…</p>;
  }
  const loadError = error ?? recipientsError;
  if (loadError) {
    return (
      <p role="alert" className="py-2 text-sm text-red-600 dark:text-red-400">
        {getApiErrorMessage(loadError, 'Failed to load subscribers')}
      </p>
    );
  }
  if (rows.length === 0) {
    return (
      <p data-testid="subscribers-empty" className="py-2 text-sm text-gray-600 dark:text-gray-400">
        No one is subscribed to this asset yet.
      </p>
    );
  }

  return (
    <>
      <ul
        data-testid="subscriber-list"
        className="border border-gray-200 dark:border-gray-700 rounded-lg divide-y divide-gray-200 dark:divide-gray-700"
      >
        {rows.map(({ subscription: sub, recipient }) => (
          <li
            key={sub.id}
            data-testid={`subscriber-row-${sub.id}`}
            className={`flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3 px-3 py-2.5 ${
              sub.is_active ? '' : 'opacity-60'
            }`}
          >
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                {recipient ? (
                  <span className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">
                    {recipient.name}
                  </span>
                ) : (
                  <span className="text-sm italic text-gray-500 dark:text-gray-400">
                    Deleted contact
                  </span>
                )}
                {!sub.is_active && (
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium border bg-gray-50 text-gray-700 border-gray-200 dark:bg-gray-900/20 dark:text-gray-400 dark:border-gray-800">
                    Off
                  </span>
                )}
              </div>
              <p className="text-xs text-gray-500 dark:text-gray-400 truncate">
                {CHANNEL_LABELS[sub.channel]}
                {recipient && detailFor(recipient, sub.channel)
                  ? ` · ${detailFor(recipient, sub.channel)}`
                  : ''}
              </p>
            </div>

            {recipient && (
              <PaidGate surface="notifications-subscriptions" silentImpression>
                <div className="flex items-center gap-2 flex-wrap">
                  {sub.is_active ? (
                    <>
                      <select
                        data-testid={`subscriber-channel-${sub.id}`}
                        aria-label={`Channel for ${recipient.name}`}
                        value={sub.channel}
                        disabled={isUpdating}
                        onChange={(e) =>
                          changeChannel(sub, e.target.value as NotificationChannel)
                        }
                        className="px-2 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 disabled:opacity-50"
                      >
                        {(['email', 'sms'] as const).map((channel) => (
                          <option
                            key={channel}
                            value={channel}
                            disabled={!detailFor(recipient, channel)}
                          >
                            {CHANNEL_LABELS[channel]}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        data-testid={`subscriber-off-${sub.id}`}
                        disabled={isUpdating}
                        onClick={() => setConfirming({ subscription: sub, recipient })}
                        className={`${BUTTON_BASE} text-red-700 bg-red-50 hover:bg-red-100 border-red-200 dark:text-red-400 dark:bg-red-900/20 dark:hover:bg-red-900/40 dark:border-red-800`}
                      >
                        Switch off
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      data-testid={`subscriber-on-${sub.id}`}
                      disabled={isUpdating}
                      onClick={() => setActive(sub, true)}
                      className={`${BUTTON_BASE} text-blue-700 bg-blue-50 hover:bg-blue-100 border-blue-200 dark:text-blue-400 dark:bg-blue-900/20 dark:hover:bg-blue-900/40 dark:border-blue-800`}
                    >
                      Turn back on
                    </button>
                  )}
                </div>
              </PaidGate>
            )}
          </li>
        ))}
      </ul>

      {/* ConfirmModal is z-50; lift it above the z-[60] notifications modal. */}
      {confirming &&
        createPortal(
          <div className="relative z-[70]">
            <ConfirmModal
              isOpen
              title="Switch off alerts"
              message={`${confirming.recipient?.name ?? 'This contact'} will stop receiving ${
                CHANNEL_LABELS[confirming.subscription.channel]
              } alerts for ${assetName}. You can turn this back on later.`}
              confirmText="Switch off"
              onConfirm={handleConfirmOff}
              onCancel={() => setConfirming(null)}
            />
          </div>,
          document.body
        )}
    </>
  );
}
